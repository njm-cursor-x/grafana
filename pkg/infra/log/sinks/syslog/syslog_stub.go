//go:build windows || nacl || plan9

package syslog

import (
	"context"
	"log/slog"
)

// Config is the [log.syslog] section. Syslog is not available on this platform.
type Config struct {
	Format   string
	Network  string
	Address  string
	Facility string
	Tag      string
}

func New(Config) (slog.Handler, error) {
	return discard{}, nil
}

type discard struct{}

func (discard) Enabled(context.Context, slog.Level) bool  { return true }
func (discard) Handle(context.Context, slog.Record) error { return nil }
func (discard) WithAttrs([]slog.Attr) slog.Handler        { return discard{} }
func (discard) WithGroup(string) slog.Handler             { return discard{} }
