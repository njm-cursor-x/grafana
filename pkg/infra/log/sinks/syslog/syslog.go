//go:build !windows && !nacl && !plan9

// Package syslog implements the syslog log sink over unix, udp or tcp.
package syslog

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"log/syslog"
	"strings"
	"sync"

	"github.com/grafana/grafana/pkg/infra/log/slogfmt"
)

// Config is the [log.syslog] section. RFC 5424 and TLS are later work.
type Config struct {
	Format   string
	Network  string
	Address  string
	Facility string
	Tag      string
}

// New dials syslog and returns a handler that writes formatted records at a priority
// taken from the record level.
func New(cfg Config) (slog.Handler, error) {
	c := &conn{cfg: cfg}
	if err := c.dial(); err != nil {
		return nil, err
	}
	return &Handler{conn: c}, nil
}

// Handler writes slog records to syslog.
type Handler struct {
	ops  []handlerOp
	conn *conn
}

type handlerOp struct {
	attrs []slog.Attr
	group string
}

type conn struct {
	mu  sync.Mutex
	cfg Config
	w   *syslog.Writer
}

func (h *Handler) Enabled(context.Context, slog.Level) bool { return true }

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	var buf bytes.Buffer
	var next slog.Handler = slogfmt.New(h.conn.cfg.Format, &buf)
	for _, op := range h.ops {
		if op.group != "" {
			next = next.WithGroup(op.group)
			continue
		}
		next = next.WithAttrs(op.attrs)
	}
	if err := next.Handle(ctx, r); err != nil {
		return err
	}
	return h.conn.write(r.Level, buf.Bytes())
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.with(handlerOp{attrs: attrs})
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.with(handlerOp{group: name})
}

func (h *Handler) with(op handlerOp) *Handler {
	ops := make([]handlerOp, len(h.ops), len(h.ops)+1)
	copy(ops, h.ops)
	return &Handler{ops: append(ops, op), conn: h.conn}
}

func (h *Handler) Reload() error {
	return h.conn.reload()
}

func (h *Handler) Close(context.Context) error {
	return h.conn.close()
}

func (c *conn) dial() error {
	w, err := syslog.Dial(c.cfg.Network, c.cfg.Address, parseFacility(c.cfg.Facility), c.cfg.Tag)
	if err != nil {
		return err
	}
	c.w = w
	return nil
}

func (c *conn) write(level slog.Level, p []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.w == nil {
		return fmt.Errorf("syslog is closed")
	}
	msg := strings.TrimSuffix(string(p), "\n")
	switch {
	case level >= slog.LevelError:
		return c.w.Err(msg)
	case level >= slog.LevelWarn:
		return c.w.Warning(msg)
	case level >= slog.LevelInfo:
		return c.w.Info(msg)
	default:
		return c.w.Debug(msg)
	}
}

func (c *conn) reload() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.w != nil {
		_ = c.w.Close()
		c.w = nil
	}
	return c.dial()
}

func (c *conn) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.w == nil {
		return nil
	}
	err := c.w.Close()
	c.w = nil
	return err
}

var facilities = map[string]syslog.Priority{
	"user":   syslog.LOG_USER,
	"daemon": syslog.LOG_DAEMON,
	"local0": syslog.LOG_LOCAL0,
	"local1": syslog.LOG_LOCAL1,
	"local2": syslog.LOG_LOCAL2,
	"local3": syslog.LOG_LOCAL3,
	"local4": syslog.LOG_LOCAL4,
	"local5": syslog.LOG_LOCAL5,
	"local6": syslog.LOG_LOCAL6,
	"local7": syslog.LOG_LOCAL7,
}

func parseFacility(facility string) syslog.Priority {
	if v, found := facilities[facility]; found {
		return v
	}
	return syslog.LOG_LOCAL7
}
