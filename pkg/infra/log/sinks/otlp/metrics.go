package otlp

import (
	"context"
	"errors"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

type metrics struct {
	records        *prometheus.CounterVec
	exported       prometheus.Counter
	exportFailed   prometheus.Counter
	exportDuration prometheus.Histogram
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		records: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "grafana",
			Subsystem: "log_otlp",
			Name:      "records_total",
			Help:      "Log records accepted by the OTLP sink, before batching.",
		}, []string{"level"}),
		exported: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "grafana",
			Subsystem: "log_otlp",
			Name:      "exported_records_total",
			Help:      "Log records successfully exported by the OTLP sink.",
		}),
		exportFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "grafana",
			Subsystem: "log_otlp",
			Name:      "export_failed_records_total",
			Help:      "Log records the OTLP sink failed to export after retries.",
		}),
		exportDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "grafana",
			Subsystem: "log_otlp",
			Name:      "export_duration_seconds",
			Help:      "Duration of OTLP log export requests, including retries.",
			Buckets:   prometheus.ExponentialBuckets(0.005, 4, 7),
		}),
	}
	if reg == nil {
		return m
	}
	m.records = register(reg, m.records)
	m.exported = register(reg, m.exported)
	m.exportFailed = register(reg, m.exportFailed)
	m.exportDuration = register(reg, m.exportDuration)
	return m
}

// register reuses an already-registered collector so the sink can be rebuilt
// on config reload without a duplicate-registration error.
func register[C prometheus.Collector](reg prometheus.Registerer, c C) C {
	if err := reg.Register(c); err != nil {
		var are prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			if existing, ok := are.ExistingCollector.(C); ok {
				return existing
			}
		}
	}
	return c
}

func levelLabel(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}

type instrumentedExporter struct {
	sdklog.Exporter
	m *metrics
}

func (e instrumentedExporter) Export(ctx context.Context, records []sdklog.Record) error {
	timer := prometheus.NewTimer(e.m.exportDuration)
	err := e.Exporter.Export(ctx, records)
	timer.ObserveDuration()
	if err != nil {
		e.m.exportFailed.Add(float64(len(records)))
	} else {
		e.m.exported.Add(float64(len(records)))
	}
	return err
}
