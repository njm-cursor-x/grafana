package log

import (
	"context"
	"log/slog"
)

// Logger is the default logger
type Logger interface {
	// New returns a new contextual Logger that has this logger's context plus the given context.
	New(ctx ...any) Logger

	// Debug logs a message with debug level and key/value pairs, if any.
	Debug(msg string, ctx ...any)

	// Info logs a message with info level and key/value pairs, if any.
	Info(msg string, ctx ...any)

	// Warn logs a message with warning level and key/value pairs, if any.
	Warn(msg string, ctx ...any)

	// Error logs a message with error level and key/value pairs, if any.
	Error(msg string, ctx ...any)

	// FromContext returns a new contextual Logger that has this logger's context plus the given context.
	FromContext(ctx context.Context) Logger
}

// LevelEnabler is implemented by loggers that can tell whether a record at level would be
// written, so bridges such as hclog can skip building records nobody reads. Loggers that
// do not implement it are treated as writing every level.
type LevelEnabler interface {
	Enabled(ctx context.Context, level slog.Level) bool
}

// PrettyLogger is used primarily to facilitate logging/user feedback for both
// the grafana-cli and the grafana backend when managing plugin installs
type PrettyLogger interface {
	Successf(format string, args ...any)
	Failuref(format string, args ...any)

	Info(args ...any)
	Infof(format string, args ...any)
	Debug(args ...any)
	Debugf(format string, args ...any)
	Warn(args ...any)
	Warnf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
}
