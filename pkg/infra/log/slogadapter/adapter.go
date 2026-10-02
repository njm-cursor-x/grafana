package slogadapter

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/grafana/grafana/pkg/infra/log"
)

var _ slog.Handler = &slogHandler{}

type slogHandler struct {
	log.Logger
	// groups is the slog group stack. go-kit has no nested records, so keys
	// are prefixed (req.method) instead of dropped into a single "group" field.
	groups []string
}

func init() {
	// Lots of New's here: Default = slog.Logger <- slog.Handler <- infra/log.Logger
	slog.SetDefault(slog.New(New(log.New())))
}

// Provide is a helper method to be used with Wire, however most services should use slog.Default()
func Provide() slog.Handler { return New(log.New()) }

// NewSLogHandler returns a new slog.Handler that logs to the given log.Logger.
func New(logger log.Logger) *slogHandler {
	return &slogHandler{Logger: logger}
}

// Enabled implements slog.Handler.Enabled.
// Filtering stays on the go-kit logger so per-logger config levels still apply.
func (h *slogHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

// Handle implements slog.Handler.Handle.
func (h *slogHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make([]any, 0, 2*r.NumAttrs())
	r.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, h.attrKey(attr.Key), attrValue(attr.Value))
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
	if len(attrs) == 0 {
		return h
	}
	out := make([]any, 0, 2*len(attrs))
	for _, attr := range attrs {
		out = append(out, h.attrKey(attr.Key), attrValue(attr.Value))
	}
	return &slogHandler{Logger: h.New(out...), groups: h.groups}
}

// WithGroup implements slog.Handler.WithGroup.
func (h *slogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := make([]string, len(h.groups)+1)
	copy(next, h.groups)
	next[len(h.groups)] = name
	return &slogHandler{Logger: h.Logger, groups: next}
}

func (h *slogHandler) attrKey(key string) string {
	if len(h.groups) == 0 {
		return key
	}
	return strings.Join(h.groups, ".") + "." + key
}

// attrValue unwraps slog.Value so sinks see strings and numbers, not slog internals.
func attrValue(v slog.Value) any {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindBool:
		return v.Bool()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindGroup:
		attrs := v.Group()
		m := make(map[string]any, len(attrs))
		for _, a := range attrs {
			m[a.Key] = attrValue(a.Value)
		}
		return m
	default:
		return v.Any()
	}
}
