package slogcore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

type captured struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

// recorder is a sink that stores records with attributes flattened to dotted keys.
type recorder struct {
	mu       *sync.Mutex
	records  *[]captured
	attrs    []slog.Attr
	prefix   string
	err      error
	panicMsg string

	flushed, reloaded, closed *int
}

func newRecorder() *recorder {
	return &recorder{mu: &sync.Mutex{}, records: &[]captured{}, flushed: new(int), reloaded: new(int), closed: new(int)}
}

func (r *recorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *recorder) Handle(_ context.Context, rec slog.Record) error {
	if r.panicMsg != "" {
		panic(r.panicMsg)
	}
	if r.err != nil {
		return r.err
	}
	c := captured{level: rec.Level, msg: rec.Message, attrs: map[string]any{}}
	for _, a := range r.attrs {
		c.attrs[a.Key] = a.Value.Any()
	}
	rec.Attrs(func(a slog.Attr) bool {
		flatten(c.attrs, r.prefix, a)
		return true
	})
	r.mu.Lock()
	*r.records = append(*r.records, c)
	r.mu.Unlock()
	return nil
}

func flatten(out map[string]any, prefix string, a slog.Attr) {
	if a.Value.Kind() == slog.KindGroup {
		for _, ga := range a.Value.Group() {
			flatten(out, prefix+a.Key+".", ga)
		}
		return
	}
	out[prefix+a.Key] = a.Value.Resolve().Any()
}

func (r *recorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *r
	c.attrs = append([]slog.Attr{}, r.attrs...)
	for _, a := range attrs {
		a.Key = r.prefix + a.Key
		c.attrs = append(c.attrs, a)
	}
	return &c
}

func (r *recorder) WithGroup(name string) slog.Handler {
	c := *r
	c.prefix = r.prefix + name + "."
	return &c
}

func (r *recorder) Flush(context.Context) error { *r.flushed++; return nil }
func (r *recorder) Reload() error               { *r.reloaded++; return nil }
func (r *recorder) Close(context.Context) error { *r.closed++; return nil }

func (r *recorder) all() []captured {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]captured{}, *r.records...)
}

func newTestManager(t *testing.T, opts Options) *Manager {
	t.Helper()
	if opts.ErrorOutput == nil {
		opts.ErrorOutput = io.Discard
	}
	return NewManager(opts)
}

func TestFanoutLevels(t *testing.T) {
	debugSink, warnSink := newRecorder(), newRecorder()
	m := newTestManager(t, Options{})
	require.NoError(t, m.Configure(
		Sink{Name: "debug", Handler: debugSink, Level: slog.LevelDebug},
		Sink{Name: "warn", Handler: warnSink, Level: slog.LevelWarn},
	))

	logger := slog.New(m.Handler())
	logger.Debug("d")
	logger.Info("i")
	logger.Warn("w")

	assert.Len(t, debugSink.all(), 3)
	require.Len(t, warnSink.all(), 1)
	assert.Equal(t, "w", warnSink.all()[0].msg)
}

func TestFanoutEnabled(t *testing.T) {
	m := newTestManager(t, Options{})
	assert.False(t, m.Handler().Enabled(context.Background(), slog.LevelError), "no sinks")

	require.NoError(t, m.Configure(Sink{
		Name:    "console",
		Handler: newRecorder(),
		Level:   slog.LevelWarn,
		Filters: map[string]slog.Level{"sqlstore": slog.LevelDebug},
	}))
	ctx := context.Background()
	assert.False(t, m.Logger("api").Enabled(ctx, slog.LevelInfo))
	assert.True(t, m.Logger("api").Enabled(ctx, slog.LevelWarn))
	assert.True(t, m.Logger("sqlstore").Enabled(ctx, slog.LevelDebug))
	assert.True(t, m.Handler().Enabled(ctx, slog.LevelDebug), "unnamed handlers defer to the record's logger attribute")
	assert.False(t, m.Handler().Enabled(ctx, slog.LevelDebug-1))
}

func TestFanoutPerLoggerFilters(t *testing.T) {
	sink := newRecorder()
	m := newTestManager(t, Options{})
	require.NoError(t, m.Configure(Sink{
		Name:    "console",
		Handler: sink,
		Level:   slog.LevelInfo,
		Filters: map[string]slog.Level{"sqlstore": slog.LevelDebug, "noisy": slog.LevelError},
	}))

	m.Logger("sqlstore").Debug("kept")
	m.Logger("noisy").Warn("dropped")
	m.Logger("other").Debug("dropped")
	// Loggers that pass their name per record, like the go-kit compatibility layer.
	slog.New(m.Handler()).Debug("kept via record", "logger", "sqlstore")
	slog.New(m.Handler()).Warn("dropped via record", "logger", "noisy")

	records := sink.all()
	msgs := make([]string, 0, len(records))
	for _, r := range records {
		msgs = append(msgs, r.msg)
	}
	assert.Equal(t, []string{"kept", "kept via record"}, msgs)
}

func TestFanoutFailureIsolation(t *testing.T) {
	reg := prometheus.NewRegistry()
	errOut := &bytes.Buffer{}
	failing, panicking, healthy := newRecorder(), newRecorder(), newRecorder()
	failing.err = errors.New("disk full")
	panicking.panicMsg = "boom"

	m := NewManager(Options{Registerer: reg, ErrorOutput: errOut})
	require.NoError(t, m.Configure(
		Sink{Name: "failing", Handler: failing, Level: slog.LevelInfo},
		Sink{Name: "panicking", Handler: panicking, Level: slog.LevelInfo},
		Sink{Name: "healthy", Handler: healthy, Level: slog.LevelInfo},
	))

	h := m.Handler()
	for range 3 {
		r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
		err := h.Handle(context.Background(), r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disk full")
		assert.Contains(t, err.Error(), "panic: boom")
	}

	assert.Len(t, healthy.all(), 3)
	assert.Equal(t, 3.0, testutil.ToFloat64(m.metrics.sinkErrors.WithLabelValues("failing")))
	assert.Equal(t, 3.0, testutil.ToFloat64(m.metrics.sinkErrors.WithLabelValues("panicking")))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.metrics.sinkErrors.WithLabelValues("healthy")))
	assert.Equal(t, 3.0, testutil.ToFloat64(m.metrics.records.WithLabelValues("healthy", "info")))
	assert.Equal(t, 1, strings.Count(errOut.String(), `sink "failing"`), "errors are reported at most once per interval")
}

func TestManagerMetricsShareRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	a := NewManager(Options{Registerer: reg})
	b := NewManager(Options{Registerer: reg})
	assert.Same(t, a.metrics.records, b.metrics.records)
}

func TestManagerHotSwap(t *testing.T) {
	first, second := newRecorder(), newRecorder()
	m := newTestManager(t, Options{})
	logger := m.Logger("api").With("k", "v")

	logger.Info("before configure")
	require.NoError(t, m.Configure(Sink{Name: "a", Handler: first, Level: slog.LevelInfo}))
	logger.Info("one")
	require.NoError(t, m.Configure(Sink{Name: "b", Handler: second, Level: slog.LevelInfo}))
	logger.Info("two")

	require.Len(t, first.all(), 1)
	require.Len(t, second.all(), 1)
	assert.Equal(t, map[string]any{"logger": "api", "k": "v"}, second.all()[0].attrs)
	assert.Equal(t, 1, *first.closed, "replaced sinks are closed")
	assert.Equal(t, 0, *second.closed)
}

func TestManagerConfigureValidation(t *testing.T) {
	m := newTestManager(t, Options{})
	require.Error(t, m.Configure(Sink{Name: "", Handler: newRecorder()}))
	require.Error(t, m.Configure(Sink{Name: "x"}))
	require.Error(t, m.Configure(
		Sink{Name: "x", Handler: newRecorder()},
		Sink{Name: "x", Handler: newRecorder()},
	))
}

func TestManagerLifecycle(t *testing.T) {
	sink := newRecorder()
	m := newTestManager(t, Options{})
	require.NoError(t, m.Configure(Sink{Name: "a", Handler: sink, Level: slog.LevelInfo}))

	require.NoError(t, m.Flush(context.Background()))
	require.NoError(t, m.Reload())
	require.NoError(t, m.Close(context.Background()))
	require.NoError(t, m.Close(context.Background()))
	require.NoError(t, m.Configure())

	assert.Equal(t, 1, *sink.reloaded)
	assert.Equal(t, 2, *sink.flushed, "explicit flush plus flush on close")
	assert.Equal(t, 1, *sink.closed, "close is idempotent, including on reconfigure")
}

func TestRenameHandler(t *testing.T) {
	sink := newRecorder()
	m := newTestManager(t, Options{})
	require.NoError(t, m.Configure(Sink{Name: "a", Handler: sink, Level: slog.LevelInfo}))

	logger := slog.New(m.Handler()).With("orgId", 1, "uname", "admin")
	logger.Info("hello", "userId", 2, "err", "bad", "traceID", "abc", "namespace", "default", "custom", true)
	logger.WithGroup("g").Info("grouped", "orgId", 3)

	records := sink.all()
	require.Len(t, records, 2)
	assert.Equal(t, map[string]any{
		KeyOrgID:     int64(1),
		KeyUserID:    int64(2),
		KeyError:     "bad",
		KeyTraceID:   "abc",
		KeyNamespace: "default",
		"custom":     true,
	}, records[0].attrs)
	assert.Equal(t, map[string]any{KeyOrgID: int64(1), "g.orgId": int64(3)}, records[1].attrs,
		"keys inside groups keep their names")
}

func TestContextHandler(t *testing.T) {
	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	}))
	legacy := func(context.Context) []slog.Attr {
		return []slog.Attr{slog.String("traceID", "ignored"), slog.Int("orgId", 7), slog.String("uname", "admin")}
	}

	sink := newRecorder()
	m := newTestManager(t, Options{ContextExtractors: []ContextExtractor{SpanContextAttrs, legacy}})
	require.NoError(t, m.Configure(Sink{Name: "a", Handler: sink, Level: slog.LevelInfo}))

	logger := slog.New(m.Handler())
	logger.InfoContext(ctx, "plain")
	logger.WithGroup("req").With("id", 1).InfoContext(ctx, "grouped", "path", "/")
	logger.InfoContext(context.Background(), "no span")

	records := sink.all()
	require.Len(t, records, 3)
	want := map[string]any{KeyTraceID: traceID.String(), KeySpanID: spanID.String(), KeyOrgID: int64(7)}
	assert.Equal(t, want, records[0].attrs)
	assert.Equal(t, map[string]any{
		KeyTraceID: traceID.String(), KeySpanID: spanID.String(), KeyOrgID: int64(7),
		"req.id": int64(1), "req.path": "/",
	}, records[1].attrs, "context attributes stay top-level when groups are open")
	assert.Equal(t, map[string]any{KeyTraceID: "ignored", KeyOrgID: int64(7)}, records[2].attrs)
}

func TestMiddlewareOrder(t *testing.T) {
	var seen []string
	mw := func(next slog.Handler) slog.Handler {
		return &keyObserver{Handler: next, seen: &seen}
	}
	sink := newRecorder()
	m := newTestManager(t, Options{Middlewares: []Middleware{mw}})
	require.NoError(t, m.Configure(Sink{Name: "a", Handler: sink, Level: slog.LevelInfo}))

	slog.New(m.Handler()).Info("hello", "orgId", 1)
	assert.Equal(t, []string{KeyOrgID}, seen, "middlewares see canonical keys")
}

type keyObserver struct {
	slog.Handler
	seen *[]string
}

func (k *keyObserver) Handle(ctx context.Context, r slog.Record) error {
	r.Attrs(func(a slog.Attr) bool {
		*k.seen = append(*k.seen, a.Key)
		return true
	})
	return k.Handler.Handle(ctx, r)
}

func TestParseFilters(t *testing.T) {
	filters, invalid := ParseFilters([]string{"sqlstore:debug", ";", "skipped:debug", "#comment:info", "rendering:trace", "alerting:critical", "x:bogus", "nocolon"})
	assert.Equal(t, map[string]slog.Level{
		"sqlstore":  slog.LevelDebug,
		"rendering": slog.LevelDebug,
		"alerting":  slog.LevelError,
		"x":         slog.LevelError,
	}, filters)
	assert.Equal(t, []string{"bogus"}, invalid)
}

func BenchmarkDisabledDebug(b *testing.B) {
	m := NewManager(Options{ErrorOutput: io.Discard})
	_ = m.Configure(Sink{Name: "a", Handler: newRecorder(), Level: slog.LevelInfo})
	logger := m.Logger("bench")
	b.ReportAllocs()
	for b.Loop() {
		logger.Debug("disabled", "k", 1)
	}
}
