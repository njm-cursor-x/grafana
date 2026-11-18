// Package slogfmt provides the text (logfmt) and JSON record formats used by Grafana's
// local log sinks. Level filtering is left to the pipeline, so these handlers accept
// every record they are given.
package slogfmt

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"time"
)

// TimeKey is the record time key, kept from the legacy format.
const TimeKey = "t"

// New returns a handler for a `[log.<mode>] format` value: "json" selects JSON and
// anything else, including "console" and "text", selects logfmt.
func New(format string, w io.Writer) slog.Handler {
	if strings.EqualFold(strings.TrimSpace(format), "json") {
		return NewJSON(w)
	}
	return NewText(w)
}

// NewText returns a logfmt handler: t=... level=info msg=... key=value.
func NewText(w io.Writer) slog.Handler {
	return slog.NewTextHandler(w, handlerOptions())
}

// NewJSON returns a handler that writes one JSON object per record.
func NewJSON(w io.Writer) slog.Handler {
	return slog.NewJSONHandler(w, handlerOptions())
}

func handlerOptions() *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level:       slog.Level(math.MinInt),
		ReplaceAttr: replaceAttr,
	}
}

func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 {
		switch a.Key {
		case slog.TimeKey:
			if t, ok := a.Value.Any().(time.Time); ok {
				return slog.String(TimeKey, t.Format(time.RFC3339Nano))
			}
		case slog.LevelKey:
			if l, ok := a.Value.Any().(slog.Level); ok {
				return slog.String(slog.LevelKey, strings.ToLower(l.String()))
			}
		}
	}
	if a.Value.Kind() == slog.KindAny {
		if err, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, errorString(err))
		}
	}
	return a
}

// errorString uses Error() rather than %+v, which would print stack traces for some error
// types, and survives typed nil errors.
func errorString(err error) (s string) {
	defer func() {
		if p := recover(); p != nil {
			s = fmt.Sprintf("<error: %v>", p)
		}
	}()
	return err.Error()
}
