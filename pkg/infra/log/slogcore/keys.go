package slogcore

import (
	"context"
	"log/slog"
)

// Canonical field names written by every sink when the slog core is enabled.
const (
	KeyLogger    = "logger"
	KeyError     = "error"
	KeyOrgID     = "grafana.org.id"
	KeyUserID    = "enduser.id"
	KeyTraceID   = "trace_id"
	KeySpanID    = "span_id"
	KeyRequestID = "http.request.id"
	KeyNamespace = "k8s.namespace.name"
)

var legacyKeys = map[string]string{
	"err":       KeyError,
	"orgId":     KeyOrgID,
	"orgID":     KeyOrgID,
	"userId":    KeyUserID,
	"userID":    KeyUserID,
	"traceID":   KeyTraceID,
	"traceId":   KeyTraceID,
	"spanID":    KeySpanID,
	"spanId":    KeySpanID,
	"requestID": KeyRequestID,
	"requestId": KeyRequestID,
	"namespace": KeyNamespace,
}

// Logins are personal data; only user IDs are logged.
var droppedKeys = map[string]struct{}{
	"uname": {},
}

// CanonicalKey returns the canonical name for a legacy key, and false when the key must
// not be logged at all.
func CanonicalKey(key string) (string, bool) {
	if _, drop := droppedKeys[key]; drop {
		return "", false
	}
	if canonical, ok := legacyKeys[key]; ok {
		return canonical, true
	}
	return key, true
}

func needsRename(key string) bool {
	if _, ok := legacyKeys[key]; ok {
		return true
	}
	_, ok := droppedKeys[key]
	return ok
}

// renameHandler rewrites top-level legacy keys to their canonical names. Keys inside
// groups belong to the group's own schema and are left alone.
type renameHandler struct {
	next    slog.Handler
	grouped bool
}

// NewRenameHandler wraps next so that legacy keys (orgId, traceID, err, ...) reach it
// under their canonical names and login names are dropped.
func NewRenameHandler(next slog.Handler) slog.Handler {
	return &renameHandler{next: next}
}

func (h *renameHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *renameHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.grouped {
		return h.next.Handle(ctx, r)
	}
	rename := false
	r.Attrs(func(a slog.Attr) bool {
		rename = needsRename(a.Key)
		return !rename
	})
	if !rename {
		return h.next.Handle(ctx, r)
	}
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if key, keep := CanonicalKey(a.Key); keep {
			a.Key = key
			out.AddAttrs(a)
		}
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *renameHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if !h.grouped {
		attrs = renameAttrs(attrs)
	}
	return &renameHandler{next: h.next.WithAttrs(attrs), grouped: h.grouped}
}

func (h *renameHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &renameHandler{next: h.next.WithGroup(name), grouped: true}
}

func renameAttrs(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		if key, keep := CanonicalKey(a.Key); keep {
			a.Key = key
			out = append(out, a)
		}
	}
	return out
}
