package log

import (
	"context"
	"log/slog"
)

// SlogCoreActive reports whether the last ReadLoggingConfig selected the slog pipeline.
// Bridges from other logging libraries check it on every call, because they are often
// built during package initialization, before the logging config is read.
func SlogCoreActive() bool {
	return slogCoreActive.Load()
}

// SlogHandler returns a handler that writes to the slog pipeline with logger's name and
// context attached. It returns nil when logger does not write to the global pipeline, for
// example NewNopLogger or a test fake; bridges should keep using logger itself then.
//
// The handler maps arbitrary slog levels, such as klog verbosity or hclog trace, onto
// Grafana's four levels, as the go-kit engine did.
func SlogHandler(logger Logger) slog.Handler {
	cl, ok := logger.(*ConcreteLogger)
	if !ok || !cl.slogPipeline {
		return nil
	}
	h := SlogManager().Handler()
	if attrs := keyvalsToAttrs(cl.slogCtx); len(attrs) > 0 {
		h = h.WithAttrs(attrs)
	}
	return bridgeHandler{next: h}
}

type bridgeHandler struct {
	next slog.Handler
}

func (h bridgeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, grafanaLevel(level))
}

func (h bridgeHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Level = grafanaLevel(r.Level)
	return h.next.Handle(ctx, r)
}

func (h bridgeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return bridgeHandler{next: h.next.WithAttrs(attrs)}
}

func (h bridgeHandler) WithGroup(name string) slog.Handler {
	return bridgeHandler{next: h.next.WithGroup(name)}
}

func grafanaLevel(l slog.Level) slog.Level {
	switch {
	case l < slog.LevelInfo:
		return slog.LevelDebug
	case l < slog.LevelWarn:
		return slog.LevelInfo
	case l < slog.LevelError:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}
