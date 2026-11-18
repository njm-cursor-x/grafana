package otlp

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/ini.v1"
)

func loadINI(t *testing.T, s string) *ini.File {
	t.Helper()
	f, err := ini.Load([]byte(s))
	require.NoError(t, err)
	return f
}

func TestReadConfig_Defaults(t *testing.T) {
	cfg, err := ReadConfig(loadINI(t, "[log.otlp]\nendpoint = collector:4317\n"), "12.0.0")
	require.NoError(t, err)

	want := DefaultConfig()
	want.Endpoint = "collector:4317"
	want.ServiceVersion = "12.0.0"
	assert.Equal(t, want, cfg)
}

func TestReadConfig_ShippedINIMatchesDefaults(t *testing.T) {
	for _, file := range []string{"defaults.ini", "sample.ini"} {
		t.Run(file, func(t *testing.T) {
			raw, err := ini.Load(filepath.Join("..", "..", "..", "..", "..", "conf", file))
			require.NoError(t, err)
			raw.Section("log").Key("level").SetValue("info")
			raw.Section("tracing.opentelemetry").Key("custom_attributes").SetValue("")
			raw.Section("log.otlp").Key("endpoint").SetValue("collector:4317")

			cfg, err := ReadConfig(raw, "")
			require.NoError(t, err)

			want := DefaultConfig()
			want.Endpoint = "collector:4317"
			assert.Equal(t, want, cfg)
		})
	}
}

func TestReadConfig_AllKeys(t *testing.T) {
	cfg, err := ReadConfig(loadINI(t, `
[log]
level = warn

[tracing.opentelemetry]
service_name = grafana-prod
custom_attributes = cluster:eu-1,team:obs

[tracing.opentelemetry.otlp]
address = tracing-collector:4317

[log.otlp]
level = debug
filters = sqlstore:error plugins:warn
endpoint = https://loki.example.com/otlp
protocol = http/protobuf
insecure = true
tls_ca_cert_path = /ca.pem
tls_client_cert_path = /cert.pem
tls_client_key_path = /key.pem
tls_skip_verify = true
headers = Authorization:Basic dXNlcjpwYXNz, X-Scope-OrgID:tenant-1
compression = none
timeout = 3s
batch_max_queue_size = 100
batch_max_export_batch_size = 10
batch_export_interval = 250ms
batch_export_timeout = 5s
resource_attributes = team:logs,deployment.environment:prod
`), "12.0.0")
	require.NoError(t, err)

	assert.Equal(t, slog.LevelDebug, cfg.Level)
	assert.Equal(t, map[string]slog.Level{"sqlstore": slog.LevelError, "plugins": slog.LevelWarn}, cfg.Filters)
	assert.Equal(t, "https://loki.example.com/otlp", cfg.Endpoint)
	assert.Equal(t, ProtocolHTTP, cfg.Protocol)
	assert.True(t, cfg.Insecure)
	assert.Equal(t, TLSConfig{CACertPath: "/ca.pem", ClientCertPath: "/cert.pem", ClientKeyPath: "/key.pem", SkipVerify: true}, cfg.TLS)
	assert.Equal(t, map[string]string{"Authorization": "Basic dXNlcjpwYXNz", "X-Scope-OrgID": "tenant-1"}, cfg.Headers)
	assert.Equal(t, CompressionNone, cfg.Compression)
	assert.Equal(t, 3*time.Second, cfg.Timeout)
	assert.Equal(t, 100, cfg.BatchMaxQueueSize)
	assert.Equal(t, 10, cfg.BatchMaxExportBatchSize)
	assert.Equal(t, 250*time.Millisecond, cfg.BatchExportInterval)
	assert.Equal(t, 5*time.Second, cfg.BatchExportTimeout)
	assert.Equal(t, "grafana-prod", cfg.ServiceName)
	assert.Equal(t, []attribute.KeyValue{
		attribute.String("cluster", "eu-1"),
		attribute.String("team", "obs"),
		attribute.String("team", "logs"),
		attribute.String("deployment.environment", "prod"),
	}, cfg.ResourceAttributes)
}

func TestReadConfig_LevelFallsBackToLogSection(t *testing.T) {
	cfg, err := ReadConfig(loadINI(t, "[log]\nlevel = error\n[log.otlp]\nendpoint = c:4317\n"), "")
	require.NoError(t, err)
	assert.Equal(t, slog.LevelError, cfg.Level)
}

func TestReadConfig_DoesNotInheritTracingEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	_, err := ReadConfig(loadINI(t, "[tracing.opentelemetry.otlp]\naddress = tracing:4317\n"), "")
	require.ErrorContains(t, err, "endpoint is required")
}

func TestReadConfig_EndpointFromEnv(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "http://collector:4318/v1/logs")
	cfg, err := ReadConfig(loadINI(t, ""), "")
	require.NoError(t, err)
	assert.Empty(t, cfg.Endpoint)
}

func TestReadConfig_Invalid(t *testing.T) {
	tests := map[string]string{
		"level":              "level = loud",
		"filter":             "filters = sqlstore",
		"filter level":       "filters = sqlstore:loud",
		"protocol":           "protocol = thrift",
		"compression":        "compression = zstd",
		"headers":            "headers = novalue",
		"resource attrs":     "resource_attributes = :x",
		"path without url":   "endpoint = loki:3100/otlp",
		"scheme":             "endpoint = ftp://loki:3100",
		"client cert alone":  "tls_client_cert_path = /cert.pem",
		"batch over queue":   "batch_max_queue_size = 10\nbatch_max_export_batch_size = 20",
		"zero queue":         "batch_max_queue_size = 0",
		"zero export period": "batch_export_interval = 0s",
	}
	for name, lines := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ReadConfig(loadINI(t, "[log.otlp]\nendpoint = c:4317\n"+lines+"\n"), "")
			require.Error(t, err)
		})
	}
}

func TestParseEndpoint(t *testing.T) {
	ep, err := parseEndpoint("collector:4317")
	require.NoError(t, err)
	assert.Equal(t, endpoint{hostPort: "collector:4317"}, ep)

	ep, err = parseEndpoint("http://loki:3100/otlp")
	require.NoError(t, err)
	assert.Equal(t, endpoint{hostPort: "loki:3100", path: "/otlp", scheme: "http"}, ep)
}

func TestHTTPPath(t *testing.T) {
	assert.Equal(t, "/v1/logs", httpPath(""))
	assert.Equal(t, "/v1/logs", httpPath("/"))
	assert.Equal(t, "/otlp/v1/logs", httpPath("/otlp"))
	assert.Equal(t, "/otlp/v1/logs", httpPath("/otlp/"))
	assert.Equal(t, "/otlp/v1/logs", httpPath("/otlp/v1/logs"))
}
