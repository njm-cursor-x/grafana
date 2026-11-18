// Package slogcore is Grafana's log/slog pipeline: a context handler, key renaming,
// optional middleware and a fan-out to independently levelled sinks, owned by a Manager
// that can swap the whole pipeline at runtime.
package slogcore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

// Middleware wraps the fan-out handler, for example to redact sensitive values. It sees
// records after context enrichment and key renaming, before any sink.
type Middleware func(next slog.Handler) slog.Handler

type Options struct {
	// ContextExtractors add attributes from the record's context, in order.
	ContextExtractors []ContextExtractor
	// Middlewares run between key renaming and the fan-out; the first is outermost.
	Middlewares []Middleware
	// Registerer receives the self-metrics. Nil leaves them unregistered.
	Registerer prometheus.Registerer
	// ErrorOutput receives rate-limited sink failure reports. Defaults to os.Stderr.
	ErrorOutput io.Writer
}

// Manager owns the active pipeline. Handlers obtained from it stay valid across
// Configure calls and always write to the current sinks.
type Manager struct {
	opts    Options
	metrics *metrics
	mu      sync.Mutex
	current atomic.Pointer[pipeline]
}

type pipeline struct {
	root      slog.Handler
	sinks     []Sink
	closeOnce sync.Once
	closeErr  error
}

func NewManager(opts Options) *Manager {
	if opts.ErrorOutput == nil {
		opts.ErrorOutput = os.Stderr
	}
	m := &Manager{opts: opts, metrics: newMetrics(opts.Registerer)}
	m.current.Store(m.build(nil))
	return m
}

func (m *Manager) build(sinks []Sink) *pipeline {
	var h slog.Handler = newFanoutHandler(sinks, m.metrics, m.opts.ErrorOutput)
	for i := len(m.opts.Middlewares) - 1; i >= 0; i-- {
		h = m.opts.Middlewares[i](h)
	}
	h = NewRenameHandler(h)
	h = NewContextHandler(h, m.opts.ContextExtractors...)
	return &pipeline{root: h, sinks: sinks}
}

// Configure replaces the active sinks. The previous sinks are closed once the new
// pipeline is live, so no record is written to a closed sink by the swap itself.
func (m *Manager) Configure(sinks ...Sink) error {
	seen := make(map[string]struct{}, len(sinks))
	for _, s := range sinks {
		if s.Name == "" || s.Handler == nil {
			return fmt.Errorf("log sink %q: name and handler are required", s.Name)
		}
		if _, dup := seen[s.Name]; dup {
			return fmt.Errorf("log sink %q is configured twice", s.Name)
		}
		seen[s.Name] = struct{}{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	previous := m.current.Swap(m.build(sinks))
	return previous.close(context.Background())
}

// Handler returns the pipeline entry point. Use it as the handler of slog loggers or as
// the target of bridges from other logging libraries.
func (m *Manager) Handler() slog.Handler {
	return &dynamicHandler{m: m}
}

// Logger returns a logger whose records carry name as the logger attribute, which is what
// per-logger filters match on.
func (m *Manager) Logger(name string) *slog.Logger {
	return slog.New(m.Handler().WithAttrs([]slog.Attr{slog.String(KeyLogger, name)}))
}

// Flush flushes every sink that buffers records.
func (m *Manager) Flush(ctx context.Context) error {
	var errs []error
	for _, s := range m.current.Load().sinks {
		if f, ok := s.Handler.(Flusher); ok {
			if err := f.Flush(ctx); err != nil {
				errs = append(errs, fmt.Errorf("flush log sink %q: %w", s.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// Reload asks every sink to reopen its resources, for example after log rotation.
func (m *Manager) Reload() error {
	var errs []error
	for _, s := range m.current.Load().sinks {
		if r, ok := s.Handler.(Reloader); ok {
			if err := r.Reload(); err != nil {
				errs = append(errs, fmt.Errorf("reload log sink %q: %w", s.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// Close flushes and closes the active sinks. The pipeline stays installed, so late log
// calls still reach sinks that tolerate it, such as the console.
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current.Load().close(ctx)
}

func (p *pipeline) close(ctx context.Context) error {
	p.closeOnce.Do(func() {
		var errs []error
		for _, s := range p.sinks {
			if f, ok := s.Handler.(Flusher); ok {
				if err := f.Flush(ctx); err != nil {
					errs = append(errs, fmt.Errorf("flush log sink %q: %w", s.Name, err))
				}
			}
			var err error
			switch c := s.Handler.(type) {
			case Closer:
				err = c.Close(ctx)
			case io.Closer:
				err = c.Close()
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("close log sink %q: %w", s.Name, err))
			}
		}
		p.closeErr = errors.Join(errs...)
	})
	return p.closeErr
}

// dynamicHandler resolves the active pipeline on each call and caches the handler
// derived from it, so WithAttrs work is redone only after a Configure.
type dynamicHandler struct {
	m     *Manager
	ops   []handlerOp
	cache atomic.Pointer[derivedHandler]
}

type derivedHandler struct {
	p *pipeline
	h slog.Handler
}

func (d *dynamicHandler) resolve() slog.Handler {
	p := d.m.current.Load()
	if c := d.cache.Load(); c != nil && c.p == p {
		return c.h
	}
	h := applyOps(p.root, d.ops)
	d.cache.Store(&derivedHandler{p: p, h: h})
	return h
}

func (d *dynamicHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return d.resolve().Enabled(ctx, level)
}

func (d *dynamicHandler) Handle(ctx context.Context, r slog.Record) error {
	return d.resolve().Handle(ctx, r)
}

func (d *dynamicHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return d
	}
	return d.with(handlerOp{attrs: attrs})
}

func (d *dynamicHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return d
	}
	return d.with(handlerOp{group: name})
}

func (d *dynamicHandler) with(op handlerOp) *dynamicHandler {
	ops := make([]handlerOp, len(d.ops), len(d.ops)+1)
	copy(ops, d.ops)
	return &dynamicHandler{m: d.m, ops: append(ops, op)}
}
