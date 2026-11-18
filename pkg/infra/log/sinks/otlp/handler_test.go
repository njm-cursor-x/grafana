package otlp

import (
	"compress/gzip"
	"context"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"testing/slogtest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/trace"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

var (
	testTraceID = trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	testSpanID  = trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
)

type grpcCollector struct {
	collogspb.UnimplementedLogsServiceServer
	mu       sync.Mutex
	requests []*collogspb.ExportLogsServiceRequest
	headers  []metadata.MD
}

func (c *grpcCollector) Export(ctx context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	c.headers = append(c.headers, md)
	return &collogspb.ExportLogsServiceResponse{}, nil
}

func startGRPCCollector(t *testing.T) (*grpcCollector, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	c := &grpcCollector{}
	collogspb.RegisterLogsServiceServer(srv, c)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return c, lis.Addr().String()
}

type httpRequest struct {
	path    string
	headers http.Header
	body    *collogspb.ExportLogsServiceRequest
}

func httpCollectorHandler(t *testing.T, out chan<- httpRequest) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if !assert.NoError(t, err) {
				return
			}
			body = gz
		}
		raw, err := io.ReadAll(body)
		if !assert.NoError(t, err) {
			return
		}
		req := &collogspb.ExportLogsServiceRequest{}
		if !assert.NoError(t, proto.Unmarshal(raw, req)) {
			return
		}
		out <- httpRequest{path: r.URL.Path, headers: r.Header.Clone(), body: req}
		w.Header().Set("Content-Type", "application/x-protobuf")
		resp, _ := proto.Marshal(&collogspb.ExportLogsServiceResponse{})
		_, _ = w.Write(resp)
	}
}

func testConfig(endpoint string) Config {
	cfg := DefaultConfig()
	cfg.Endpoint = endpoint
	cfg.ServiceVersion = "1.2.3"
	cfg.BatchExportInterval = time.Hour
	return cfg
}

func spanContext() context.Context {
	return trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    testTraceID,
		SpanID:     testSpanID,
		TraceFlags: trace.FlagsSampled,
	}))
}

func logSample(t *testing.T, h *Handler) {
	t.Helper()
	logger := slog.New(h).With("logger", "sqlstore")
	logger.DebugContext(spanContext(), "filtered out")
	logger.ErrorContext(spanContext(), "query failed",
		"error", errors.New("boom"),
		"trace_id", testTraceID.String(),
		"grafana.org.id", int64(7),
	)
	require.NoError(t, h.Close(context.Background()))
}

func attrMap(kvs []*commonpb.KeyValue) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		switch v := kv.Value.Value.(type) {
		case *commonpb.AnyValue_StringValue:
			m[kv.Key] = v.StringValue
		case *commonpb.AnyValue_IntValue:
			m[kv.Key] = strconv.FormatInt(v.IntValue, 10)
		default:
			m[kv.Key] = kv.Value.String()
		}
	}
	return m
}

func assertSample(t *testing.T, req *collogspb.ExportLogsServiceRequest) {
	t.Helper()
	require.Len(t, req.ResourceLogs, 1)
	rl := req.ResourceLogs[0]

	res := attrMap(rl.Resource.Attributes)
	assert.Equal(t, "grafana", res["service.name"])
	assert.Equal(t, "1.2.3", res["service.version"])
	assert.Equal(t, "eu-1", res["cluster"])

	require.Len(t, rl.ScopeLogs, 1)
	assert.Equal(t, instrumentationScope, rl.ScopeLogs[0].Scope.Name)
	require.Len(t, rl.ScopeLogs[0].LogRecords, 1, "debug record must be filtered by level")
	rec := rl.ScopeLogs[0].LogRecords[0]

	assert.Equal(t, "query failed", rec.Body.GetStringValue())
	assert.Equal(t, logspb.SeverityNumber_SEVERITY_NUMBER_ERROR, rec.SeverityNumber)
	assert.Equal(t, testTraceID[:], rec.TraceId)
	assert.Equal(t, testSpanID[:], rec.SpanId)
	assert.Equal(t, map[string]string{
		"logger":            "sqlstore",
		"exception.message": "boom",
		"exception.type":    "*errors.errorString",
		"grafana.org.id":    "7",
	}, attrMap(rec.Attributes))
}

func TestHandler_GRPC(t *testing.T) {
	collector, addr := startGRPCCollector(t)

	cfg := testConfig(addr)
	cfg.Insecure = true
	cfg.Headers = map[string]string{"x-scope-orgid": "tenant-1"}
	cfg.ResourceAttributes = append(cfg.ResourceAttributes, attribute.String("cluster", "eu-1"))

	h, err := New(context.Background(), cfg, nil)
	require.NoError(t, err)
	logSample(t, h)

	collector.mu.Lock()
	defer collector.mu.Unlock()
	require.Len(t, collector.requests, 1)
	assertSample(t, collector.requests[0])
	assert.Equal(t, []string{"tenant-1"}, collector.headers[0].Get("x-scope-orgid"))
}

func TestHandler_HTTP(t *testing.T) {
	reqs := make(chan httpRequest, 4)
	srv := httptest.NewServer(httpCollectorHandler(t, reqs))
	t.Cleanup(srv.Close)

	cfg := testConfig(srv.URL + "/otlp")
	cfg.Protocol = ProtocolHTTP
	cfg.Headers = map[string]string{"X-Scope-OrgID": "tenant-1"}
	cfg.ResourceAttributes = append(cfg.ResourceAttributes, attribute.String("cluster", "eu-1"))

	h, err := New(context.Background(), cfg, nil)
	require.NoError(t, err)
	logSample(t, h)

	req := <-reqs
	assert.Equal(t, "/otlp/v1/logs", req.path)
	assert.Equal(t, "gzip", req.headers.Get("Content-Encoding"))
	assert.Equal(t, "tenant-1", req.headers.Get("X-Scope-OrgID"))
	assertSample(t, req.body)
}

func TestHandler_HTTPWithCustomCA(t *testing.T) {
	reqs := make(chan httpRequest, 4)
	srv := httptest.NewTLSServer(httpCollectorHandler(t, reqs))
	t.Cleanup(srv.Close)

	caPath := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600))

	cfg := testConfig(srv.URL)
	cfg.Protocol = ProtocolHTTP
	cfg.TLS.CACertPath = caPath

	h, err := New(context.Background(), cfg, nil)
	require.NoError(t, err)
	slog.New(h).Info("over tls")
	require.NoError(t, h.Close(context.Background()))

	req := <-reqs
	assert.Equal(t, "/v1/logs", req.path)
	assert.Equal(t, "over tls", req.body.ResourceLogs[0].ScopeLogs[0].LogRecords[0].Body.GetStringValue())
}

func TestNew_InvalidConfig(t *testing.T) {
	cfg := testConfig("collector:4317")
	cfg.Protocol = "thrift"
	_, err := New(context.Background(), cfg, nil)
	require.Error(t, err)

	cfg = testConfig("collector:4317")
	cfg.TLS.CACertPath = filepath.Join(t.TempDir(), "missing.pem")
	_, err = New(context.Background(), cfg, nil)
	require.ErrorContains(t, err, "tls_ca_cert_path")
}

type memExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
	err     error
}

func (e *memExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range records {
		e.records = append(e.records, r.Clone())
	}
	return e.err
}

func (e *memExporter) Shutdown(context.Context) error   { return nil }
func (e *memExporter) ForceFlush(context.Context) error { return nil }

func newMemHandler(t *testing.T, cfg Config, reg prometheus.Registerer) (*Handler, *memExporter) {
	t.Helper()
	exp := &memExporter{}
	h, err := newWithExporter(context.Background(), cfg, exp, reg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close(context.Background()) })
	return h, exp
}

func TestHandler_SlogContract(t *testing.T) {
	var exp *memExporter
	var h *Handler
	slogtest.Run(t,
		func(t *testing.T) slog.Handler {
			cfg := testConfig("collector:4317")
			h, exp = newMemHandler(t, cfg, nil)
			return h
		},
		func(t *testing.T) map[string]any {
			require.NoError(t, h.Flush(context.Background()))
			exp.mu.Lock()
			defer exp.mu.Unlock()
			require.Len(t, exp.records, 1)
			return recordToMap(exp.records[0])
		},
	)
}

func recordToMap(r sdklog.Record) map[string]any {
	m := map[string]any{
		slog.MessageKey: r.Body().AsString(),
		slog.LevelKey:   r.SeverityText(),
	}
	if !r.Timestamp().IsZero() {
		m[slog.TimeKey] = r.Timestamp()
	}
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		m[string(kv.Key)] = valueToAny(kv.Value)
		return true
	})
	return m
}

func valueToAny(v attribute.Value) any {
	switch v.Type() {
	case attribute.MAP:
		m := map[string]any{}
		for _, kv := range v.AsMap() {
			m[string(kv.Key)] = valueToAny(kv.Value)
		}
		return m
	case attribute.STRING:
		return v.AsString()
	default:
		return v.String()
	}
}

func TestHandler_GroupedErrorKeyIsNotMapped(t *testing.T) {
	h, exp := newMemHandler(t, testConfig("collector:4317"), nil)
	slog.New(h).WithGroup("req").Info("msg", "error", "inside group")
	require.NoError(t, h.Flush(context.Background()))

	require.Len(t, exp.records, 1)
	got := recordToMap(exp.records[0])
	assert.Equal(t, map[string]any{"error": "inside group"}, got["req"])
}

func TestHandler_TraceKeysKeptWithoutSpan(t *testing.T) {
	h, exp := newMemHandler(t, testConfig("collector:4317"), nil)
	slog.New(h).Info("msg", "trace_id", "abc", "span_id", "def")
	require.NoError(t, h.Flush(context.Background()))

	require.Len(t, exp.records, 1)
	got := recordToMap(exp.records[0])
	assert.Equal(t, "abc", got["trace_id"])
	assert.Equal(t, "def", got["span_id"])
}

func TestHandler_Level(t *testing.T) {
	cfg := testConfig("collector:4317")
	cfg.Level = slog.LevelWarn
	h, _ := newMemHandler(t, cfg, nil)

	assert.False(t, h.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, h.Enabled(context.Background(), slog.LevelWarn))
	assert.False(t, h.WithAttrs([]slog.Attr{slog.String("a", "b")}).Enabled(context.Background(), slog.LevelInfo))
}

func TestHandler_Metrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	h, exp := newMemHandler(t, testConfig("collector:4317"), reg)
	logger := slog.New(h)

	logger.Info("one")
	logger.Warn("two")
	logger.Warn("three")
	require.NoError(t, h.Flush(context.Background()))

	m := h.p.metrics
	assert.Equal(t, 1.0, testutil.ToFloat64(m.records.WithLabelValues("info")))
	assert.Equal(t, 2.0, testutil.ToFloat64(m.records.WithLabelValues("warn")))
	assert.Equal(t, 3.0, testutil.ToFloat64(m.exported))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.exportFailed))

	exp.mu.Lock()
	exp.err = errors.New("collector down")
	exp.mu.Unlock()
	logger.Error("four")
	_ = h.Flush(context.Background())
	assert.Equal(t, 1.0, testutil.ToFloat64(m.exportFailed))

	// A rebuilt sink on config reload reuses the registered collectors.
	h2, _ := newMemHandler(t, testConfig("collector:4317"), reg)
	assert.Same(t, m.exported, h2.p.metrics.exported)
}
