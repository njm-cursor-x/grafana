package slogcore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const sinkErrorReportInterval = time.Minute

// Sink is one output of the pipeline. Handler does the formatting and writing; it may
// also implement Flusher, Reloader, Closer or io.Closer to take part in the lifecycle.
type Sink struct {
	// Name identifies the sink in metrics and error reports, for example "console".
	Name string
	// Handler receives every record that passes Level and Filters.
	Handler slog.Handler
	// Level is the minimum level written by this sink.
	Level slog.Level
	// Filters override Level per logger name, for example {"sqlstore": slog.LevelDebug}.
	Filters map[string]slog.Level
}

// Flusher is implemented by sinks that buffer records.
type Flusher interface {
	Flush(ctx context.Context) error
}

// Reloader is implemented by sinks that reopen resources on SIGHUP.
type Reloader interface {
	Reload() error
}

// Closer is implemented by sinks that hold resources.
type Closer interface {
	Close(ctx context.Context) error
}

type sinkState struct {
	Sink
	// floor is the lowest level any logger can write to this sink.
	floor      slog.Level
	records    [4]prometheus.Counter
	errors     prometheus.Counter
	lastReport atomic.Int64
	errOut     io.Writer
}

func (s *sinkState) level(logger string) slog.Level {
	if lvl, ok := s.Filters[logger]; ok {
		return lvl
	}
	return s.Level
}

func (s *sinkState) recordCounter(l slog.Level) prometheus.Counter {
	switch levelLabel(l) {
	case "debug":
		return s.records[0]
	case "info":
		return s.records[1]
	case "warn":
		return s.records[2]
	default:
		return s.records[3]
	}
}

func (s *sinkState) fail(err error) error {
	s.errors.Inc()
	now := time.Now().UnixNano()
	last := s.lastReport.Load()
	if now-last >= int64(sinkErrorReportInterval) && s.lastReport.CompareAndSwap(last, now) {
		_, _ = fmt.Fprintf(s.errOut, "logging: sink %q failed to write a record: %v\n", s.Name, err)
	}
	return fmt.Errorf("sink %q: %w", s.Name, err)
}

// fanoutHandler sends each record to every sink whose level admits it. A sink that
// returns an error or panics is counted and reported, and never stops the other sinks.
type fanoutHandler struct {
	sinks      []*sinkState
	handlers   []slog.Handler
	minLevel   slog.Level
	hasFilters bool
	logger     string
	grouped    bool
}

func newFanoutHandler(sinks []Sink, m *metrics, errOut io.Writer) *fanoutHandler {
	h := &fanoutHandler{
		sinks:    make([]*sinkState, len(sinks)),
		handlers: make([]slog.Handler, len(sinks)),
		minLevel: slog.Level(1 << 30),
	}
	for i, s := range sinks {
		st := &sinkState{Sink: s, floor: s.Level, errOut: errOut}
		for _, lvl := range s.Filters {
			h.hasFilters = true
			st.floor = min(st.floor, lvl)
		}
		for j, label := range []string{"debug", "info", "warn", "error"} {
			st.records[j] = m.records.WithLabelValues(s.Name, label)
		}
		st.errors = m.sinkErrors.WithLabelValues(s.Name)
		h.sinks[i] = st
		h.handlers[i] = s.Handler
		h.minLevel = min(h.minLevel, st.floor)
	}
	return h
}

func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if level < h.minLevel {
		return false
	}
	for i, s := range h.sinks {
		threshold := s.floor
		if h.logger != "" {
			threshold = s.level(h.logger)
		}
		if level >= threshold && h.handlers[i].Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	logger := h.logger
	if logger == "" && h.hasFilters && !h.grouped {
		logger = recordLogger(r)
	}
	var errs []error
	for i, s := range h.sinks {
		if r.Level < s.level(logger) || !h.handlers[i].Enabled(ctx, r.Level) {
			continue
		}
		s.recordCounter(r.Level).Inc()
		if err := safeHandle(ctx, h.handlers[i], r.Clone()); err != nil {
			errs = append(errs, s.fail(err))
		}
	}
	return errors.Join(errs...)
}

func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	c := h.clone()
	if !h.grouped {
		for _, a := range attrs {
			if a.Key == KeyLogger && a.Value.Kind() == slog.KindString {
				c.logger = a.Value.String()
			}
		}
	}
	for i := range c.handlers {
		c.handlers[i] = c.handlers[i].WithAttrs(attrs)
	}
	return c
}

func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := h.clone()
	c.grouped = true
	for i := range c.handlers {
		c.handlers[i] = c.handlers[i].WithGroup(name)
	}
	return c
}

func (h *fanoutHandler) clone() *fanoutHandler {
	c := *h
	c.handlers = make([]slog.Handler, len(h.handlers))
	copy(c.handlers, h.handlers)
	return &c
}

// recordLogger finds the logger name on records from callers that pass it per record
// instead of through WithAttrs, such as the go-kit compatibility layer.
func recordLogger(r slog.Record) string {
	name := ""
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == KeyLogger && a.Value.Kind() == slog.KindString {
			name = a.Value.String()
			return false
		}
		return true
	})
	return name
}

func safeHandle(ctx context.Context, h slog.Handler, r slog.Record) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return h.Handle(ctx, r)
}
