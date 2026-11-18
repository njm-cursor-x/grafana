package log

import (
	"context"
	"io"
	"testing"
	"time"

	gokitlog "github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/grafana-app-sdk/logging"

	"github.com/grafana/grafana/pkg/infra/log/text"
)

// goldenNow is the timestamp baked into every golden line. RFC3339Nano so the
// "t" field is stable across engines.
var goldenNow = time.Date(2024, 1, 15, 10, 11, 12, 123456789, time.UTC)

const (
	formatText    = "text"
	formatConsole = "console"
	formatJSON    = "json"
)

// goldenFormats is the set of sink formats these snapshots pin. console is the
// non-TTY path (text.NewTextLogger), matching getLogFormat when stdout is not
// a terminal. TTY coloring is out of scope.
var goldenFormats = []string{formatText, formatConsole, formatJSON}

// engineOpts configures the pipeline the golden/benchmark tests drive.
// Engine-swap PRs keep this shape and replace installCurrentEngine.
type engineOpts struct {
	format    string
	level     string
	filters   map[string]string
	sinkCount int
}

// installCurrentEngine wires package-level log.New / FromContext so records
// land on w in the requested format. Swap this function (not the cases) when
// the slog core lands. These snapshots are the *legacy* go-kit byte contract;
// the slog engine with OTel-style keys is a separate compatibility surface.
var installCurrentEngine = installGoKitEngine

func installGoKitEngine(tb testing.TB, w io.Writer, opts engineOpts) {
	tb.Helper()

	if opts.format == "" {
		opts.format = formatText
	}
	if opts.level == "" {
		opts.level = "info"
	}
	if opts.sinkCount <= 0 {
		opts.sinkCount = 1
	}

	origNow := now
	origRoot := root
	origProviders := ctxLogProviders
	origAppSDK := logging.DefaultLogger
	now = func() time.Time { return goldenNow }
	ctxLogProviders = []ContextualLogProviderFunc{contextualAttributesFromContext}
	tb.Cleanup(func() {
		now = origNow
		root = origRoot
		ctxLogProviders = origProviders
		logging.DefaultLogger = origAppSDK
	})

	filters := make(map[string]level.Option, len(opts.filters))
	for name, lvl := range opts.filters {
		filters[name] = getLogLevelFromString(lvl)
	}

	loggers := make([]logWithFilters, opts.sinkCount)
	for i := range loggers {
		loggers[i] = logWithFilters{
			val:      loggerForFormat(opts.format, w),
			filters:  filters,
			maxLevel: getLogLevelFromString(opts.level),
		}
	}

	root = newManager(gokitlog.NewNopLogger())
	root.initialize(loggers, opts.level)
}

func loggerForFormat(format string, w io.Writer) gokitlog.Logger {
	switch format {
	case formatJSON:
		return gokitlog.NewJSONLogger(gokitlog.NewSyncWriter(w))
	case formatText, formatConsole:
		// getLogFormat("console") uses NewTextLogger when stdout is not a TTY.
		return text.NewTextLogger(w)
	default:
		panic("unknown log format " + format)
	}
}

func contextualAttributesFromContext(ctx context.Context) ([]any, bool) {
	pFromCtx := ctx.Value(logParamsContextKey{})
	if pFromCtx != nil {
		return pFromCtx.([]any), true
	}
	return nil, false
}
