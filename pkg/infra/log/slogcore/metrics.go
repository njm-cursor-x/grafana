package slogcore

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	records    *prometheus.CounterVec
	sinkErrors *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		records: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "grafana",
			Subsystem: "log",
			Name:      "records_total",
			Help:      "Log records handed to each sink, by level.",
		}, []string{"sink", "level"}),
		sinkErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "grafana",
			Subsystem: "log",
			Name:      "sink_errors_total",
			Help:      "Log records a sink failed to write.",
		}, []string{"sink"}),
	}
	if reg != nil {
		m.records = registerOrReuse(reg, m.records)
		m.sinkErrors = registerOrReuse(reg, m.sinkErrors)
	}
	return m
}

// registerOrReuse lets several managers (for example in tests) share one registry.
func registerOrReuse(reg prometheus.Registerer, c *prometheus.CounterVec) *prometheus.CounterVec {
	if err := reg.Register(c); err != nil {
		var are prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing
			}
		}
	}
	return c
}
