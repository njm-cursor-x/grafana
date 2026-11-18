package log

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gokitlog "github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/ini.v1"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana/pkg/infra/log/sinks/console"
	"github.com/grafana/grafana/pkg/infra/log/slogcore"
	"github.com/grafana/grafana/pkg/util"
)

// SlogCoreFeatureToggle is the feature toggle that selects the slog pipeline. It is read
// from the [feature_toggles] config before feature management starts.
const SlogCoreFeatureToggle = "grafana.slogLoggingCore"

type ConfigOption func(*configOptions)

type configOptions struct {
	slogCore bool
}

// WithSlogCore makes ReadLoggingConfig build the slog pipeline instead of the go-kit one.
func WithSlogCore(enabled bool) ConfigOption {
	return func(o *configOptions) { o.slogCore = enabled }
}

// SinkFactory builds the handler for one `[log] mode` entry from its `[log.<mode>]`
// section. Level and filters are applied by the pipeline, not by the handler.
type SinkFactory func(section *ini.Section, logsPath string) (slog.Handler, error)

var (
	sinkFactoriesMu sync.RWMutex
	sinkFactories   = map[string]SinkFactory{
		"console": newConsoleSink,
	}
)

// RegisterSink makes mode usable in `[log] mode` when the slog core is enabled. It
// replaces any factory already registered for mode.
func RegisterSink(mode string, factory SinkFactory) {
	sinkFactoriesMu.Lock()
	defer sinkFactoriesMu.Unlock()
	sinkFactories[mode] = factory
}

func sinkFactory(mode string) (SinkFactory, bool) {
	sinkFactoriesMu.RLock()
	defer sinkFactoriesMu.RUnlock()
	f, ok := sinkFactories[mode]
	return f, ok
}

func newConsoleSink(section *ini.Section, _ string) (slog.Handler, error) {
	return console.New(console.Config{
		Format: section.Key("format").MustString(""),
		Output: section.Key("output").MustString("stdout"),
	})
}

var (
	slogManagerOnce sync.Once
	slogManager     *slogcore.Manager
)

// SlogManager returns the manager of the slog pipeline, creating it on first use. It has
// no sinks until ReadLoggingConfig runs with the slog core enabled.
func SlogManager() *slogcore.Manager {
	slogManagerOnce.Do(func() {
		slogManager = slogcore.NewManager(slogcore.Options{
			ContextExtractors: []slogcore.ContextExtractor{
				slogcore.SpanContextAttrs,
				contextProviderAttrs,
			},
			Registerer: prometheus.DefaultRegisterer,
		})
	})
	return slogManager
}

// slogCoreActive tells Close and Reload whether the last ReadLoggingConfig selected the
// slog pipeline.
var slogCoreActive atomic.Bool

func activeSlogManager() *slogcore.Manager {
	if !slogCoreActive.Load() {
		return nil
	}
	return slogManager
}

func contextProviderAttrs(ctx context.Context) []slog.Attr {
	return keyvalsToAttrs(FromContext(ctx))
}

func readSlogLoggingConfig(modes []string, logsPath string, cfg *ini.File, defaultLevelName string) error {
	defaultFilters, invalid := slogcore.ParseFilters(util.SplitString(cfg.Section("log").Key("filters").String()))
	for _, lvl := range invalid {
		root.Error("Unknown log level", "level", lvl)
	}
	defaultLevel, ok := slogcore.ParseLevel(defaultLevelName)
	if !ok {
		root.Error("Unknown log level", "level", defaultLevelName)
	}

	sinks := make([]slogcore.Sink, 0, len(modes))
	for _, mode := range modes {
		mode = strings.TrimSpace(mode)
		sec, err := cfg.GetSection("log." + mode)
		if err != nil {
			root.Error("Unknown log mode", "mode", mode)
			return fmt.Errorf("failed to get config section log. %s: %w", mode, err)
		}

		sinkLevel := defaultLevel
		if name := sec.Key("level").String(); name != "" {
			if sinkLevel, ok = slogcore.ParseLevel(name); !ok {
				root.Error("Unknown log level", "level", name)
			}
		}
		filters, invalid := slogcore.ParseFilters(util.SplitString(sec.Key("filters").String()))
		for _, lvl := range invalid {
			root.Error("Unknown log level", "level", lvl)
		}
		for name, lvl := range defaultFilters {
			if _, exists := filters[name]; !exists {
				filters[name] = lvl
			}
		}

		handler, err := newSlogSinkHandler(mode, sec, logsPath)
		if err != nil {
			return err
		}
		if handler == nil {
			continue
		}
		sinks = append(sinks, slogcore.Sink{Name: mode, Handler: handler, Level: sinkLevel, Filters: filters})
	}

	if len(sinks) == 0 {
		return nil
	}
	manager := SlogManager()
	if err := manager.Configure(sinks...); err != nil {
		return err
	}
	slogCoreActive.Store(true)
	root.initializeSlog(manager.Handler())
	return nil
}

// newSlogSinkHandler returns nil, nil when the sink is skipped, matching the legacy
// behavior of logging and continuing when a file sink cannot be opened.
func newSlogSinkHandler(mode string, sec *ini.Section, logsPath string) (slog.Handler, error) {
	if factory, ok := sinkFactory(mode); ok {
		h, err := factory(sec, logsPath)
		if err != nil {
			return nil, fmt.Errorf("log mode %q: %w", mode, err)
		}
		return h, nil
	}
	switch mode {
	case "file", "syslog":
		// Native file and syslog sinks are not on the slog pipeline yet, so the legacy
		// writers are driven through an adapter to keep those modes working.
		logger, ok := newLegacyModeLogger(mode, sec, getLogFormat(sec.Key("format").MustString("")), logsPath)
		if !ok {
			return nil, nil
		}
		if logger == nil {
			return nil, fmt.Errorf("log mode %q is not supported on this platform", mode)
		}
		return &gokitSinkHandler{logger: logger}, nil
	}
	return nil, fmt.Errorf("log mode %q is not supported", mode)
}

func (lm *logManager) initializeSlog(h slog.Handler) {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	adapter := &slogGokitLogger{handler: h}
	lm.Swap(adapter)
	// Per-logger filters are applied by the fan-out handler using the logger attribute.
	lm.logFilters = nil
	for _, logger := range lm.loggersByName {
		logger.Swap(gokitlog.With(adapter, logger.ctx...))
	}

	logging.DefaultLogger = logging.NewSLogLogger(h)
}

// slogGokitLogger lets ConcreteLogger, which is built on go-kit, write to the slog
// pipeline. ConcreteLogger.Log passes the logger context, then "t", then level, msg and
// the call's key/value pairs.
type slogGokitLogger struct {
	handler slog.Handler
}

func (l *slogGokitLogger) Log(keyvals ...any) error {
	lvl := slog.LevelInfo
	msg := ""
	attrs := make([]slog.Attr, 0, len(keyvals)/2)
	for i := 0; i < len(keyvals); i += 2 {
		var v any = gokitlog.ErrMissingValue
		if i+1 < len(keyvals) {
			v = keyvals[i+1]
		}
		switch k := keyvals[i]; {
		case k == "t":
			continue
		case k == level.Key():
			lvl = gokitToSlogLevel(v)
			continue
		case k == "msg":
			if s, ok := v.(string); ok {
				msg = s
				continue
			}
		}
		attrs = append(attrs, slog.Any(keyString(keyvals[i]), v))
	}

	ctx := context.Background()
	if !l.handler.Enabled(ctx, lvl) {
		return nil
	}
	r := slog.NewRecord(now(), lvl, msg, 0)
	r.AddAttrs(attrs...)
	return l.handler.Handle(ctx, r)
}

func gokitToSlogLevel(v any) slog.Level {
	s, ok := v.(fmt.Stringer)
	if !ok {
		return slog.LevelInfo
	}
	switch s.String() {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func slogToGokitLevel(l slog.Level) level.Value {
	switch {
	case l < slog.LevelInfo:
		return level.DebugValue()
	case l < slog.LevelWarn:
		return level.InfoValue()
	case l < slog.LevelError:
		return level.WarnValue()
	default:
		return level.ErrorValue()
	}
}

func keyString(k any) string {
	if s, ok := k.(string); ok {
		return s
	}
	return fmt.Sprint(k)
}

func keyvalsToAttrs(keyvals []any) []slog.Attr {
	if len(keyvals) == 0 {
		return nil
	}
	attrs := make([]slog.Attr, 0, (len(keyvals)+1)/2)
	for i := 0; i < len(keyvals); i += 2 {
		var v any = gokitlog.ErrMissingValue
		if i+1 < len(keyvals) {
			v = keyvals[i+1]
		}
		attrs = append(attrs, slog.Any(keyString(keyvals[i]), v))
	}
	return attrs
}

// gokitSinkHandler writes slog records to a legacy go-kit writer (file or syslog) in the
// key order the legacy engine uses: t, level, msg, then attributes.
type gokitSinkHandler struct {
	logger gokitlog.Logger
	attrs  []any
	prefix string
}

func (h *gokitSinkHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *gokitSinkHandler) Handle(_ context.Context, r slog.Record) error {
	keyvals := make([]any, 0, 6+len(h.attrs)+2*r.NumAttrs())
	t := r.Time
	keyvals = append(keyvals,
		"t", gokitlog.TimestampFormat(func() time.Time { return t }, logTimeFormat)(),
		level.Key(), slogToGokitLevel(r.Level),
		"msg", r.Message,
	)
	keyvals = append(keyvals, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		keyvals = appendFlatAttr(keyvals, h.prefix, a)
		return true
	})
	return h.logger.Log(keyvals...)
}

func (h *gokitSinkHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = make([]any, len(h.attrs), len(h.attrs)+2*len(attrs))
	copy(c.attrs, h.attrs)
	for _, a := range attrs {
		c.attrs = appendFlatAttr(c.attrs, h.prefix, a)
	}
	return &c
}

func (h *gokitSinkHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.prefix = h.prefix + name + "."
	return &c
}

func appendFlatAttr(keyvals []any, prefix string, a slog.Attr) []any {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return keyvals
	}
	if a.Value.Kind() == slog.KindGroup {
		groupPrefix := prefix
		if a.Key != "" {
			groupPrefix += a.Key + "."
		}
		for _, ga := range a.Value.Group() {
			keyvals = appendFlatAttr(keyvals, groupPrefix, ga)
		}
		return keyvals
	}
	return append(keyvals, prefix+a.Key, a.Value.Any())
}
