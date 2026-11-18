// Package otlp is a log sink that exports slog records over OTLP (gRPC or
// HTTP), through the OpenTelemetry slog bridge and the batching log SDK.
package otlp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	SinkName = "otlp"

	instrumentationScope = "github.com/grafana/grafana/pkg/infra/log"

	keyError            = "error"
	keyTraceID          = "trace_id"
	keySpanID           = "span_id"
	keyExceptionMessage = "exception.message"
	keyExceptionType    = "exception.type"
)

// Handler is an slog.Handler that batches records and exports them over
// OTLP. Trace and span IDs come from the span context on the ctx passed to
// Handle. Handlers derived through WithAttrs and WithGroup share one export
// pipeline, so Flush and Close on any of them act on all of them.
type Handler struct {
	next    slog.Handler
	level   slog.Leveler
	grouped bool
	p       *pipeline
}

type pipeline struct {
	provider *sdklog.LoggerProvider
	metrics  *metrics
}

var _ slog.Handler = (*Handler)(nil)

// New builds the OTLP sink from cfg. Metrics are registered on reg when it's
// not nil. Call Close on shutdown to flush buffered records.
func New(ctx context.Context, cfg Config, reg prometheus.Registerer) (*Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	exp, err := newExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create OTLP log exporter: %w", err)
	}
	h, err := newWithExporter(ctx, cfg, exp, reg)
	if err != nil {
		_ = exp.Shutdown(ctx)
		return nil, err
	}
	return h, nil
}

func newWithExporter(ctx context.Context, cfg Config, exp sdklog.Exporter, reg prometheus.Registerer) (*Handler, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		),
		resource.WithAttributes(cfg.ResourceAttributes...),
		resource.WithFromEnv(),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP log resource: %w", err)
	}

	m := newMetrics(reg)
	processor := sdklog.NewBatchProcessor(instrumentedExporter{Exporter: exp, m: m},
		sdklog.WithMaxQueueSize(cfg.BatchMaxQueueSize),
		sdklog.WithExportMaxBatchSize(cfg.BatchMaxExportBatchSize),
		sdklog.WithExportInterval(cfg.BatchExportInterval),
		sdklog.WithExportTimeout(cfg.BatchExportTimeout),
	)
	provider := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(processor))

	return &Handler{
		next: otelslog.NewHandler(instrumentationScope,
			otelslog.WithLoggerProvider(provider),
			otelslog.WithVersion(cfg.ServiceVersion),
		),
		level: cfg.Level,
		p:     &pipeline{provider: provider, metrics: m},
	}, nil
}

func (h *Handler) Name() string { return SinkName }

func (h *Handler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= h.level.Level() && h.next.Enabled(ctx, l)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	h.p.metrics.records.WithLabelValues(levelLabel(r.Level)).Inc()

	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	if h.grouped {
		r.Attrs(func(a slog.Attr) bool {
			out.AddAttrs(a)
			return true
		})
	} else {
		hasSpan := trace.SpanContextFromContext(ctx).IsValid()
		attrs := make([]slog.Attr, 0, r.NumAttrs()+1)
		r.Attrs(func(a slog.Attr) bool {
			attrs = appendMapped(attrs, a, hasSpan)
			return true
		})
		out.AddAttrs(attrs...)
	}
	return h.next.Handle(ctx, out)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	if !h.grouped {
		mapped := make([]slog.Attr, 0, len(attrs)+1)
		for _, a := range attrs {
			mapped = appendMapped(mapped, a, false)
		}
		attrs = mapped
	}
	h2 := *h
	h2.next = h.next.WithAttrs(attrs)
	return &h2
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.next = h.next.WithGroup(name)
	h2.grouped = true
	return &h2
}

// Flush exports all buffered records.
func (h *Handler) Flush(ctx context.Context) error {
	return h.p.provider.ForceFlush(ctx)
}

// Close flushes buffered records and stops the exporter. Records handled
// afterwards are dropped.
func (h *Handler) Close(ctx context.Context) error {
	return h.p.provider.Shutdown(ctx)
}

// Reload is a no-op: the exporter reconnects on its own.
func (h *Handler) Reload() error { return nil }

// appendMapped translates canonical log keys to their OTLP form. Top-level
// trace_id and span_id are dropped when ctx carries a span, because the SDK
// already sets them as record fields from that span.
func appendMapped(dst []slog.Attr, a slog.Attr, hasSpan bool) []slog.Attr {
	switch a.Key {
	case keyError:
		v := a.Value.Resolve()
		if err, ok := v.Any().(error); ok && v.Kind() == slog.KindAny && err != nil {
			return append(dst,
				slog.String(keyExceptionMessage, err.Error()),
				slog.String(keyExceptionType, fmt.Sprintf("%T", err)),
			)
		}
		return append(dst, slog.String(keyExceptionMessage, v.String()))
	case keyTraceID, keySpanID:
		if hasSpan {
			return dst
		}
	}
	return append(dst, a)
}
