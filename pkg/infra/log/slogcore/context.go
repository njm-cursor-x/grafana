package slogcore

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// ContextExtractor returns the log attributes carried by ctx.
type ContextExtractor func(ctx context.Context) []slog.Attr

// SpanContextAttrs returns trace_id and span_id for the span in ctx, if any.
func SpanContextAttrs(ctx context.Context) []slog.Attr {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return nil
	}
	return []slog.Attr{
		slog.String(KeyTraceID, sc.TraceID().String()),
		slog.String(KeySpanID, sc.SpanID().String()),
	}
}

// handlerOp is one WithAttrs or WithGroup call, kept so a handler chain can be rebuilt.
type handlerOp struct {
	attrs []slog.Attr
	group string
}

func applyOps(h slog.Handler, ops []handlerOp) slog.Handler {
	for _, op := range ops {
		if op.group != "" {
			h = h.WithGroup(op.group)
		} else {
			h = h.WithAttrs(op.attrs)
		}
	}
	return h
}

// contextHandler adds the attributes found on the record's context. They are always
// written at the top level, even when the logger has open groups, so trace and identity
// fields stay queryable under a fixed name.
type contextHandler struct {
	base       slog.Handler
	cur        slog.Handler
	ops        []handlerOp
	grouped    bool
	extractors []ContextExtractor
}

// NewContextHandler wraps next with context enrichment. Extractors run in order; when two
// produce the same canonical key, the first one wins.
func NewContextHandler(next slog.Handler, extractors ...ContextExtractor) slog.Handler {
	return &contextHandler{base: next, cur: next, extractors: extractors}
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.cur.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := h.contextAttrs(ctx)
	if len(attrs) == 0 {
		return h.cur.Handle(ctx, r)
	}
	if !h.grouped {
		r = r.Clone()
		r.AddAttrs(attrs...)
		return h.cur.Handle(ctx, r)
	}
	return applyOps(h.base.WithAttrs(attrs), h.ops).Handle(ctx, r)
}

func (h *contextHandler) contextAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	var out []slog.Attr
	var seen map[string]struct{}
	for _, extract := range h.extractors {
		for _, a := range extract(ctx) {
			key, keep := CanonicalKey(a.Key)
			if !keep {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			if seen == nil {
				seen = make(map[string]struct{})
			}
			seen[key] = struct{}{}
			a.Key = key
			out = append(out, a)
		}
	}
	return out
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.with(handlerOp{attrs: attrs}, h.cur.WithAttrs(attrs), h.grouped)
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.with(handlerOp{group: name}, h.cur.WithGroup(name), true)
}

func (h *contextHandler) with(op handlerOp, cur slog.Handler, grouped bool) *contextHandler {
	ops := make([]handlerOp, len(h.ops), len(h.ops)+1)
	copy(ops, h.ops)
	return &contextHandler{
		base:       h.base,
		cur:        cur,
		ops:        append(ops, op),
		grouped:    grouped,
		extractors: h.extractors,
	}
}
