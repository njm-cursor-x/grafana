// Package console implements the console log sink.
package console

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/grafana/grafana/pkg/infra/log/slogfmt"
)

type Config struct {
	// Format is the `[log.console] format` value: console, text or json.
	Format string
	// Output is stdout (default) or stderr.
	Output string
	// Writer overrides Output, for tests.
	Writer io.Writer
}

// New returns a handler that writes formatted records to stdout or stderr. The standard
// streams are never closed, so the handler has no lifecycle methods.
func New(cfg Config) (slog.Handler, error) {
	w := cfg.Writer
	if w == nil {
		switch strings.ToLower(strings.TrimSpace(cfg.Output)) {
		case "", "stdout":
			w = os.Stdout
		case "stderr":
			w = os.Stderr
		default:
			return nil, fmt.Errorf("console log output %q is not supported, use stdout or stderr", cfg.Output)
		}
	}
	return slogfmt.New(cfg.Format, w), nil
}
