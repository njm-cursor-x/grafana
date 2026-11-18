package slogadapter

import (
	"context"
	"log/slog"

	"github.com/grafana/grafana/pkg/infra/log"
)

var _ slog.Handler = &slogHandler{}

type slogHandler struct {
	log.Logger
	// group is the WithGroup prefix applied to keys on the go-kit path.
	group string
	// core is the slog pipeline handler for Logger, used while the slog core is active.
	// It is nil when Logger does not write to the pipeline.
	core slog.Handler
}

func init() {
	// Lots of New's here: Default = slog.Logger <- slog.Handler <- infra/log.Logger
	slog.SetDefault(slog.New(New(log.New())))
}

// Provide is a helper method to be used with Wire, however most services should use slog.Default()
func Provide() slog.Handler { return New(log.New()) }

// NewSLogHandler returns a new slog.Handler that logs to the given log.Logger.
func New(logger log.Logger) *slogHandler {
	return &slogHandler{Logger: logger, core: log.SlogHandler(logger)}
}

func (h *slogHandler) useCore() bool {
	return h.core != nil && log.SlogCoreActive()
}

// Enabled implements slog.Handler.Enabled.
func (h *slogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.useCore() {
		return h.core.Enabled(ctx, level)
	}
	return true
}

// Handle implements slog.Handler.Handle.
func (h *slogHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.useCore() {
		return h.core.Handle(ctx, r)
	}

	attrs := make([]any, 0, 2*r.NumAttrs())
	r.Attrs(func(attr slog.Attr) bool {
		attrs = appendAttr(attrs, h.group, attr)
		return true
	})
	attrs = append(attrs, log.FromContext(ctx)...)

	switch level := r.Level; {
	case level < slog.LevelInfo:
		h.Debug(r.Message, attrs...)
	case level < slog.LevelWarn:
		h.Info(r.Message, attrs...)
	case level < slog.LevelError:
		h.Warn(r.Message, attrs...)
	default:
		h.Error(r.Message, attrs...)
	}
	return nil
}

// WithAttrs implements slog.Handler.WithAttrs.
func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]any, 0, 2*len(attrs))
	for _, attr := range attrs {
		out = appendAttr(out, h.group, attr)
	}
	c := &slogHandler{Logger: h.New(out...), group: h.group, core: h.core}
	if h.core != nil {
		c.core = h.core.WithAttrs(attrs)
	}
	return c
}

// WithGroup implements slog.Handler.WithGroup.
func (h *slogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := &slogHandler{Logger: h.Logger, group: h.group + name + ".", core: h.core}
	if h.core != nil {
		c.core = h.core.WithGroup(name)
	}
	return c
}

// appendAttr flattens groups into dotted keys, since the go-kit engine has no nesting.
func appendAttr(kv []any, prefix string, attr slog.Attr) []any {
	if attr.Value.Kind() != slog.KindGroup {
		return append(kv, prefix+attr.Key, attr.Value)
	}
	if attr.Key != "" {
		prefix += attr.Key + "."
	}
	for _, a := range attr.Value.Group() {
		kv = appendAttr(kv, prefix, a)
	}
	return kv
}
