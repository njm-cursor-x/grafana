package otlp

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/ini.v1"
)

const (
	ProtocolGRPC = "grpc"
	ProtocolHTTP = "http/protobuf"

	CompressionGzip = "gzip"
	CompressionNone = "none"

	defaultServiceName = "grafana"
)

// Config is the parsed `[log.otlp]` section.
type Config struct {
	Level slog.Level
	// Filters holds per-logger level overrides (`filters = sqlstore:debug`).
	// They are keyed by logger name, which only the engine's fan-out handler
	// knows, so the handler returned by New doesn't apply them.
	Filters map[string]slog.Level

	// Endpoint is `host:port`, or a URL with an `http`/`https` scheme. For
	// the HTTP protocol a URL is treated as a base URL and `/v1/logs` is
	// appended to its path, so Loki's `http://loki:3100/otlp` works as is.
	Endpoint    string
	Protocol    string
	Insecure    bool
	TLS         TLSConfig
	Headers     map[string]string
	Compression string
	Timeout     time.Duration

	BatchMaxQueueSize       int
	BatchMaxExportBatchSize int
	BatchExportInterval     time.Duration
	BatchExportTimeout      time.Duration

	ServiceName        string
	ServiceVersion     string
	ResourceAttributes []attribute.KeyValue
}

type TLSConfig struct {
	CACertPath     string
	ClientCertPath string
	ClientKeyPath  string
	SkipVerify     bool
}

func DefaultConfig() Config {
	return Config{
		Level:                   slog.LevelInfo,
		Filters:                 map[string]slog.Level{},
		Protocol:                ProtocolGRPC,
		Headers:                 map[string]string{},
		Compression:             CompressionGzip,
		Timeout:                 10 * time.Second,
		BatchMaxQueueSize:       2048,
		BatchMaxExportBatchSize: 512,
		BatchExportInterval:     time.Second,
		BatchExportTimeout:      30 * time.Second,
		ServiceName:             defaultServiceName,
	}
}

// ReadConfig parses `[log.otlp]`. The level falls back to `[log] level`.
// Service name and custom attributes are shared with
// `[tracing.opentelemetry]` so logs and traces carry the same resource;
// the tracing exporter endpoint is deliberately not inherited.
func ReadConfig(raw *ini.File, buildVersion string) (Config, error) {
	cfg := DefaultConfig()
	cfg.ServiceVersion = buildVersion

	tracing := raw.Section("tracing.opentelemetry")
	if name := tracing.Key("service_name").MustString(""); name != "" {
		cfg.ServiceName = name
	}
	tracingAttrs, err := parseKeyValues(tracing.Key("custom_attributes").MustString(""))
	if err != nil {
		return cfg, fmt.Errorf("[tracing.opentelemetry] custom_attributes: %w", err)
	}

	sec := raw.Section("log.otlp")

	levelName := sec.Key("level").MustString("")
	if levelName == "" {
		levelName = raw.Section("log").Key("level").MustString("info")
	}
	if cfg.Level, err = parseLevel(levelName); err != nil {
		return cfg, fmt.Errorf("[log.otlp] level: %w", err)
	}
	if cfg.Filters, err = parseFilters(sec.Key("filters").MustString("")); err != nil {
		return cfg, fmt.Errorf("[log.otlp] filters: %w", err)
	}

	cfg.Endpoint = strings.TrimSpace(sec.Key("endpoint").MustString(""))
	cfg.Protocol = sec.Key("protocol").MustString(cfg.Protocol)
	cfg.Insecure = sec.Key("insecure").MustBool(false)
	cfg.TLS = TLSConfig{
		CACertPath:     sec.Key("tls_ca_cert_path").MustString(""),
		ClientCertPath: sec.Key("tls_client_cert_path").MustString(""),
		ClientKeyPath:  sec.Key("tls_client_key_path").MustString(""),
		SkipVerify:     sec.Key("tls_skip_verify").MustBool(false),
	}
	headers, err := parseKeyValues(sec.Key("headers").MustString(""))
	if err != nil {
		return cfg, fmt.Errorf("[log.otlp] headers: %w", err)
	}
	for _, h := range headers {
		cfg.Headers[string(h.Key)] = h.Value.AsString()
	}
	cfg.Compression = sec.Key("compression").MustString(cfg.Compression)
	cfg.Timeout = sec.Key("timeout").MustDuration(cfg.Timeout)
	cfg.BatchMaxQueueSize = sec.Key("batch_max_queue_size").MustInt(cfg.BatchMaxQueueSize)
	cfg.BatchMaxExportBatchSize = sec.Key("batch_max_export_batch_size").MustInt(cfg.BatchMaxExportBatchSize)
	cfg.BatchExportInterval = sec.Key("batch_export_interval").MustDuration(cfg.BatchExportInterval)
	cfg.BatchExportTimeout = sec.Key("batch_export_timeout").MustDuration(cfg.BatchExportTimeout)

	logAttrs, err := parseKeyValues(sec.Key("resource_attributes").MustString(""))
	if err != nil {
		return cfg, fmt.Errorf("[log.otlp] resource_attributes: %w", err)
	}
	// Later attributes win in resource.New, so log-specific ones override tracing's.
	cfg.ResourceAttributes = append(tracingAttrs, logAttrs...)

	return cfg, cfg.Validate()
}

// Validate checks values that ReadConfig can't express through defaults. An
// empty endpoint is accepted when the standard OpenTelemetry environment
// variables provide one, because the exporters read them as a fallback.
func (c Config) Validate() error {
	var errs []error
	if c.Protocol != ProtocolGRPC && c.Protocol != ProtocolHTTP {
		errs = append(errs, fmt.Errorf("protocol must be %q or %q, got %q", ProtocolGRPC, ProtocolHTTP, c.Protocol))
	}
	if c.Compression != CompressionGzip && c.Compression != CompressionNone {
		errs = append(errs, fmt.Errorf("compression must be %q or %q, got %q", CompressionGzip, CompressionNone, c.Compression))
	}
	if c.Endpoint == "" && os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		errs = append(errs, errors.New("endpoint is required"))
	}
	if c.Endpoint != "" {
		if _, err := parseEndpoint(c.Endpoint); err != nil {
			errs = append(errs, err)
		}
	}
	if (c.TLS.ClientCertPath == "") != (c.TLS.ClientKeyPath == "") {
		errs = append(errs, errors.New("tls_client_cert_path and tls_client_key_path must be set together"))
	}
	if c.BatchMaxQueueSize <= 0 || c.BatchMaxExportBatchSize <= 0 {
		errs = append(errs, errors.New("batch_max_queue_size and batch_max_export_batch_size must be positive"))
	} else if c.BatchMaxExportBatchSize > c.BatchMaxQueueSize {
		errs = append(errs, errors.New("batch_max_export_batch_size must not exceed batch_max_queue_size"))
	}
	if c.Timeout <= 0 || c.BatchExportInterval <= 0 || c.BatchExportTimeout <= 0 {
		errs = append(errs, errors.New("timeout, batch_export_interval and batch_export_timeout must be positive"))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("invalid [log.otlp] config: %w", err)
	}
	return nil
}

type endpoint struct {
	hostPort string
	// path is only set when the endpoint was given as a URL.
	path   string
	scheme string
}

func parseEndpoint(raw string) (endpoint, error) {
	if !strings.Contains(raw, "://") {
		if strings.Contains(raw, "/") {
			return endpoint{}, fmt.Errorf("endpoint %q: a path requires an http:// or https:// scheme", raw)
		}
		return endpoint{hostPort: raw}, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return endpoint{}, fmt.Errorf("endpoint %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return endpoint{}, fmt.Errorf("endpoint %q: scheme must be http or https", raw)
	}
	if u.Host == "" {
		return endpoint{}, fmt.Errorf("endpoint %q: missing host", raw)
	}
	return endpoint{hostPort: u.Host, path: u.Path, scheme: u.Scheme}, nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace", "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error", "critical", "crit":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown level %q", s)
	}
}

func parseFilters(s string) (map[string]slog.Level, error) {
	filters := map[string]slog.Level{}
	for _, f := range strings.Fields(s) {
		name, lvl, ok := strings.Cut(f, ":")
		if !ok || name == "" {
			return nil, fmt.Errorf("filter %q must be in 'logger:level' form", f)
		}
		l, err := parseLevel(lvl)
		if err != nil {
			return nil, fmt.Errorf("filter %q: %w", f, err)
		}
		filters[name] = l
	}
	return filters, nil
}

// parseKeyValues parses `key:value,key2:value2`, the format used by
// `[tracing.opentelemetry] custom_attributes`. Values may contain colons.
func parseKeyValues(s string) ([]attribute.KeyValue, error) {
	var res []attribute.KeyValue
	for _, kv := range strings.Split(s, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		k, v, ok := strings.Cut(kv, ":")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("%q must be in 'key:value' form", kv)
		}
		res = append(res, attribute.String(k, strings.TrimSpace(v)))
	}
	return res, nil
}
