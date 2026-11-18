// Package logctx stores typed logging metadata on context.Context.
// It is engine-agnostic: callers set values here, and a slog handler (or
// tests) read them. Identity is IDs only; login/username is never stored.
package logctx

import (
	"context"
	"log/slog"

	"github.com/grafana/grafana/pkg/infra/log/logkeys"
)

type ctxKey int

const (
	orgKey ctxKey = iota
	userKey
	requestIDKey
	attrsKey
)

// WithOrg records the Grafana organization id on ctx.
func WithOrg(ctx context.Context, orgID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, orgKey, orgID)
}

// Org returns the organization id stored by WithOrg.
func Org(ctx context.Context) (int64, bool) {
	if ctx == nil {
		return 0, false
	}
	v, ok := ctx.Value(orgKey).(int64)
	return v, ok
}

// WithUser records the user id on ctx. Login is intentionally not accepted.
func WithUser(ctx context.Context, userID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, userKey, userID)
}

// User returns the user id stored by WithUser.
func User(ctx context.Context) (int64, bool) {
	if ctx == nil {
		return 0, false
	}
	v, ok := ctx.Value(userKey).(int64)
	return v, ok
}

// WithRequestID records the HTTP request id on ctx.
func WithRequestID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the request id stored by WithRequestID.
func RequestID(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(requestIDKey).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// WithAttrs appends slog attributes onto ctx. Existing attributes are kept.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(attrs) == 0 {
		return ctx
	}
	prev, _ := ctx.Value(attrsKey).([]slog.Attr)
	next := make([]slog.Attr, 0, len(prev)+len(attrs))
	next = append(next, prev...)
	next = append(next, attrs...)
	return context.WithValue(ctx, attrsKey, next)
}

// Attrs returns identity fields plus any extra attributes stored on ctx.
// The returned slice is a copy.
func Attrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}

	extra, _ := ctx.Value(attrsKey).([]slog.Attr)
	n := len(extra)
	if _, ok := Org(ctx); ok {
		n++
	}
	if _, ok := User(ctx); ok {
		n++
	}
	if _, ok := RequestID(ctx); ok {
		n++
	}
	if n == 0 {
		return nil
	}

	out := make([]slog.Attr, 0, n)
	if orgID, ok := Org(ctx); ok {
		out = append(out, slog.Int64(logkeys.KeyOrgID, orgID))
	}
	if userID, ok := User(ctx); ok {
		out = append(out, slog.Int64(logkeys.KeyUserID, userID))
	}
	if requestID, ok := RequestID(ctx); ok {
		out = append(out, slog.String(logkeys.KeyRequestID, requestID))
	}
	out = append(out, extra...)
	return out
}

// Extract is a context extractor: func(context.Context) []slog.Attr.
// Pass it to slogcore.NewManager as Options.ContextExtractors when the slog pipeline is wired.
func Extract(ctx context.Context) []slog.Attr {
	return Attrs(ctx)
}
