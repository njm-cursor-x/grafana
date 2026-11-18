package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/grafana/grafana/pkg/infra/log/logctx"
	"github.com/grafana/grafana/pkg/infra/log/logkeys"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/grafana/grafana/pkg/web"
)

func TestRequestID_GeneratesAndEchoes(t *testing.T) {
	m := web.New()
	m.UseMiddleware(RequestID(setting.NewCfg()))

	var got string
	var ok bool
	m.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok = logctx.RequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "from-client")
	m.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, ok)
	require.NotEmpty(t, got)
	assert.NotEqual(t, "from-client", got, "untrusted incoming ids must be ignored")
	assert.Equal(t, got, rec.Header().Get(RequestIDHeader))
}

func TestRequestID_TrustsIncomingWhenConfigured(t *testing.T) {
	cfg := setting.NewCfg()
	cfg.TrustIncomingRequestID = true

	m := web.New()
	m.UseMiddleware(RequestID(cfg))

	var got string
	m.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = logctx.RequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "abc-123_ok.1")
	m.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "abc-123_ok.1", got)
	assert.Equal(t, "abc-123_ok.1", rec.Header().Get(RequestIDHeader))
}

func TestRequestID_RejectsMalformedIncoming(t *testing.T) {
	cfg := setting.NewCfg()
	cfg.TrustIncomingRequestID = true

	m := web.New()
	m.UseMiddleware(RequestID(cfg))

	var got string
	m.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = logctx.RequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "bad id\nwith-newline")
	m.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, got)
	assert.NotEqual(t, "bad id\nwith-newline", got)
}

func TestRequestID_SetsSpanAttribute(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tracer := tracesdk.NewTracerProvider(tracesdk.WithSpanProcessor(recorder)).Tracer("test")

	m := web.New()
	m.UseMiddleware(RequestTracing(tracer, ShouldTraceAllPaths))
	m.UseMiddleware(RequestID(setting.NewCfg()))
	m.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	spans := recorder.Ended()
	require.Len(t, spans, 1)

	echoed := rec.Header().Get(RequestIDHeader)
	require.NotEmpty(t, echoed)

	var found string
	for _, a := range spans[0].Attributes() {
		if a.Key == attribute.Key(logkeys.KeyRequestID) {
			found = a.Value.AsString()
		}
	}
	assert.Equal(t, echoed, found)
}

func TestSanitizeRequestID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "ok-1_2.3", sanitizeRequestID("ok-1_2.3"))
	require.Empty(t, sanitizeRequestID(""))
	require.Empty(t, sanitizeRequestID("has space"))
	require.Empty(t, sanitizeRequestID("x/y"))
	tooLong := make([]byte, maxRequestIDLength+1)
	for i := range tooLong {
		tooLong[i] = 'a'
	}
	require.Empty(t, sanitizeRequestID(string(tooLong)))
}
