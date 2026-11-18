package logtest

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"gopkg.in/ini.v1"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/log/slogfmt"
)

// SlogCapture collects the JSON records written to the "logtest" sink.
type SlogCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *SlogCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

// Lines returns every record written so far, decoded.
func (c *SlogCapture) Lines(t testing.TB) []map[string]any {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(c.buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

// UseSlogCore configures global logging with the slog core and a single "logtest" sink
// that writes JSON to the returned capture. [log] settings such as level and filters can
// be passed as `key = value` lines. The legacy engine is restored when the test ends, so
// tests using it must not run in parallel.
func UseSlogCore(t testing.TB, logSection ...string) *SlogCapture {
	t.Helper()
	capture := &SlogCapture{}
	log.RegisterSink("logtest", func(*ini.Section, string) (slog.Handler, error) {
		return slogfmt.NewJSON(capture), nil
	})
	cfg, err := ini.Load([]byte("[log]\n" + strings.Join(logSection, "\n") + "\n[log.logtest]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := log.ReadLoggingConfig([]string{"logtest"}, t.TempDir(), cfg, log.WithSlogCore(true)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		legacy, err := ini.Load([]byte("[log]\n[log.console]\n"))
		if err == nil {
			err = log.ReadLoggingConfig([]string{"console"}, "", legacy)
		}
		if err != nil {
			t.Error(err)
		}
	})
	return capture
}
