package otlp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"google.golang.org/grpc/credentials"
)

const httpLogsPath = "/v1/logs"

func newExporter(ctx context.Context, cfg Config) (sdklog.Exporter, error) {
	var ep endpoint
	if cfg.Endpoint != "" {
		var err error
		if ep, err = parseEndpoint(cfg.Endpoint); err != nil {
			return nil, err
		}
	}
	insecure := cfg.Insecure || ep.scheme == "http"

	var tlsCfg *tls.Config
	if !insecure {
		var err error
		if tlsCfg, err = buildTLSConfig(cfg.TLS); err != nil {
			return nil, err
		}
	}

	switch cfg.Protocol {
	case ProtocolGRPC:
		opts := []otlploggrpc.Option{otlploggrpc.WithTimeout(cfg.Timeout)}
		if ep.hostPort != "" {
			opts = append(opts, otlploggrpc.WithEndpoint(ep.hostPort))
		}
		if insecure {
			opts = append(opts, otlploggrpc.WithInsecure())
		} else {
			opts = append(opts, otlploggrpc.WithTLSCredentials(credentials.NewTLS(tlsCfg)))
		}
		if len(cfg.Headers) > 0 {
			opts = append(opts, otlploggrpc.WithHeaders(cfg.Headers))
		}
		if cfg.Compression == CompressionGzip {
			opts = append(opts, otlploggrpc.WithCompressor(CompressionGzip))
		}
		return otlploggrpc.New(ctx, opts...)
	case ProtocolHTTP:
		opts := []otlploghttp.Option{otlploghttp.WithTimeout(cfg.Timeout)}
		if ep.hostPort != "" {
			opts = append(opts, otlploghttp.WithEndpoint(ep.hostPort), otlploghttp.WithURLPath(httpPath(ep.path)))
		}
		if insecure {
			opts = append(opts, otlploghttp.WithInsecure())
		} else {
			opts = append(opts, otlploghttp.WithTLSClientConfig(tlsCfg))
		}
		if len(cfg.Headers) > 0 {
			opts = append(opts, otlploghttp.WithHeaders(cfg.Headers))
		}
		if cfg.Compression == CompressionGzip {
			opts = append(opts, otlploghttp.WithCompression(otlploghttp.GzipCompression))
		} else {
			opts = append(opts, otlploghttp.WithCompression(otlploghttp.NoCompression))
		}
		return otlploghttp.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("unsupported protocol %q", cfg.Protocol)
	}
}

func httpPath(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, httpLogsPath) {
		return base
	}
	return base + httpLogsPath
}

func buildTLSConfig(c TLSConfig) (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// #nosec G402 -- opt-in via tls_skip_verify, for collectors with self-signed certificates.
		InsecureSkipVerify: c.SkipVerify,
	}
	if c.CACertPath != "" {
		pem, err := os.ReadFile(c.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("read tls_ca_cert_path: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("tls_ca_cert_path %q contains no PEM certificates", c.CACertPath)
		}
		cfg.RootCAs = pool
	}
	if c.ClientCertPath != "" {
		cert, err := tls.LoadX509KeyPair(c.ClientCertPath, c.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
