package pluginslog

import (
	"context"
	"log/slog"

	"github.com/grafana/grafana/pkg/infra/log"
	pluginslog "github.com/grafana/grafana/pkg/plugins/log"
)

func init() {
	// Register Grafana's logger implementation for pkg/plugins
	pluginslog.SetLoggerFactory(func(name string) pluginslog.Logger {
		return newWrapper(log.New(name))
	})
}

var _ pluginslog.LevelEnabler = (*grafanaInfraLogWrapper)(nil)

type grafanaInfraLogWrapper struct {
	l *log.ConcreteLogger

	// s writes straight to the slog pipeline while the slog core is active. It is nil
	// when l does not write to the pipeline. ctx is the context given to FromContext,
	// which the pipeline reads trace and contextual attributes from.
	s   *slog.Logger
	ctx context.Context
}

func newWrapper(l *log.ConcreteLogger) *grafanaInfraLogWrapper {
	w := &grafanaInfraLogWrapper{l: l, ctx: context.Background()}
	if h := log.SlogHandler(l); h != nil {
		w.s = slog.New(h)
	}
	return w
}

func (d *grafanaInfraLogWrapper) useCore() bool {
	return d.s != nil && log.SlogCoreActive()
}

func (d *grafanaInfraLogWrapper) New(ctx ...any) pluginslog.Logger {
	s := d.s
	if s != nil && len(ctx) > 0 {
		s = s.With(ctx...)
	}
	return &grafanaInfraLogWrapper{l: d.l.New(ctx...), s: s, ctx: d.ctx}
}

func (d *grafanaInfraLogWrapper) Debug(msg string, ctx ...any) {
	if d.useCore() {
		d.s.Log(d.ctx, slog.LevelDebug, msg, ctx...)
		return
	}
	d.l.Debug(msg, ctx...)
}

func (d *grafanaInfraLogWrapper) Info(msg string, ctx ...any) {
	if d.useCore() {
		d.s.Log(d.ctx, slog.LevelInfo, msg, ctx...)
		return
	}
	d.l.Info(msg, ctx...)
}

func (d *grafanaInfraLogWrapper) Warn(msg string, ctx ...any) {
	if d.useCore() {
		d.s.Log(d.ctx, slog.LevelWarn, msg, ctx...)
		return
	}
	d.l.Warn(msg, ctx...)
}

func (d *grafanaInfraLogWrapper) Error(msg string, ctx ...any) {
	if d.useCore() {
		d.s.Log(d.ctx, slog.LevelError, msg, ctx...)
		return
	}
	d.l.Error(msg, ctx...)
}

func (d *grafanaInfraLogWrapper) FromContext(ctx context.Context) pluginslog.Logger {
	l := d.l
	if !d.useCore() {
		if concreteInfraLogger, ok := d.l.FromContext(ctx).(*log.ConcreteLogger); ok {
			l = concreteInfraLogger
		} else {
			l = d.l.New()
		}
	}
	return &grafanaInfraLogWrapper{l: l, s: d.s, ctx: ctx}
}

// Enabled reports the configured level only on the slog pipeline; the go-kit engine
// cannot answer it, so every level is reported as enabled there.
func (d *grafanaInfraLogWrapper) Enabled(ctx context.Context, level slog.Level) bool {
	if d.useCore() {
		return d.s.Enabled(ctx, level)
	}
	return true
}
