package log

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"

	"github.com/grafana/grafana/pkg/infra/log/slogfmt"
	"github.com/grafana/grafana/pkg/util"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) lines(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	raw := bytes.Split(bytes.TrimSpace(b.buf.Bytes()), []byte("\n"))
	out := make([]map[string]any, 0, len(raw))
	for _, line := range raw {
		if len(line) == 0 {
			continue
		}
		m := map[string]any{}
		require.NoError(t, json.Unmarshal(line, &m), string(line))
		out = append(out, m)
	}
	return out
}

// useSlogCore configures logging with the slog core and a "capture" mode that writes JSON
// to the returned buffer, and restores the legacy engine afterwards.
func useSlogCore(t *testing.T, conf string) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	RegisterSink("capture", func(*ini.Section, string) (slog.Handler, error) {
		return slogfmt.NewJSON(buf), nil
	})

	cfg, err := ini.Load([]byte(conf))
	require.NoError(t, err)
	modes := util.SplitString(cfg.Section("log").Key("mode").String())
	require.NoError(t, ReadLoggingConfig(modes, t.TempDir(), cfg, WithSlogCore(true)))

	t.Cleanup(func() {
		sinkFactoriesMu.Lock()
		delete(sinkFactories, "capture")
		sinkFactoriesMu.Unlock()
		legacy, err := ini.Load([]byte("[log]\n[log.console]\nlevel = info\n"))
		require.NoError(t, err)
		require.NoError(t, ReadLoggingConfig([]string{"console"}, "", legacy))
		assert.False(t, slogCoreActive.Load())
	})
	return buf
}

func TestSlogCoreConcreteLogger(t *testing.T) {
	buf := useSlogCore(t, `
[log]
mode = capture
level = info
filters = slogtest.filtered:debug

[log.capture]
`)
	named := New("slogtest.named")
	named.Debug("dropped by level")
	named.Info("hello", "orgId", 1, "userId", 2, "uname", "admin", "err", "boom")
	named.New("traceID", "abc").Warn("child")
	New("slogtest.filtered").Debug("kept by filter")

	lines := buf.lines(t)
	require.Len(t, lines, 3)

	assert.Equal(t, "info", lines[0]["level"])
	assert.Equal(t, "hello", lines[0]["msg"])
	assert.Equal(t, "slogtest.named", lines[0]["logger"])
	assert.Equal(t, 1.0, lines[0]["grafana.org.id"])
	assert.Equal(t, 2.0, lines[0]["enduser.id"])
	assert.Equal(t, "boom", lines[0]["error"])
	assert.NotContains(t, lines[0], "uname")
	assert.NotContains(t, lines[0], "orgId")
	_, err := time.Parse(time.RFC3339Nano, lines[0]["t"].(string))
	require.NoError(t, err)

	assert.Equal(t, "warn", lines[1]["level"])
	assert.Equal(t, "abc", lines[1]["trace_id"])

	assert.Equal(t, "kept by filter", lines[2]["msg"])
}

func TestSlogCoreContextProviders(t *testing.T) {
	buf := useSlogCore(t, "[log]\nmode = capture\n[log.capture]\n")

	ctx := WithContextualAttributes(context.Background(), []any{"orgID", 3, "requestID", "r1"})
	New("slogtest.ctx").FromContext(ctx).Info("via FromContext")
	slog.New(SlogManager().Handler()).InfoContext(ctx, "via slog context")

	lines := buf.lines(t)
	require.Len(t, lines, 2)
	for _, line := range lines {
		assert.Equal(t, 3.0, line["grafana.org.id"], line["msg"])
		assert.Equal(t, "r1", line["http.request.id"], line["msg"])
	}
}

func TestSlogCoreNativeFileMode(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "grafana.log")
	cfg, err := ini.Load([]byte(`
[log]
mode = file
level = debug

[log.file]
format = json
log_rotate = false
file_name = ` + logFile + `
`))
	require.NoError(t, err)
	require.NoError(t, ReadLoggingConfig([]string{"file"}, dir, cfg, WithSlogCore(true)))
	t.Cleanup(func() {
		legacy, err := ini.Load([]byte("[log]\n[log.console]\n"))
		require.NoError(t, err)
		require.NoError(t, ReadLoggingConfig([]string{"console"}, "", legacy))
	})

	New("slogtest.file").Error("written", "orgId", 4)
	require.NoError(t, Reload())
	New("slogtest.file").Info("after reload")
	require.NoError(t, Close())

	body, err := os.ReadFile(logFile)
	require.NoError(t, err)
	var lines []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSpace(body), []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		m := map[string]any{}
		require.NoError(t, json.Unmarshal(raw, &m), string(raw))
		lines = append(lines, m)
	}
	require.Len(t, lines, 2)
	assert.Equal(t, "error", lines[0]["level"])
	assert.Equal(t, "written", lines[0]["msg"])
	assert.Equal(t, "slogtest.file", lines[0]["logger"])
	assert.Equal(t, 4.0, lines[0]["grafana.org.id"])
	assert.Contains(t, lines[0], "t")
	assert.Equal(t, "after reload", lines[1]["msg"])
}

func TestSlogCoreConfigErrors(t *testing.T) {
	t.Cleanup(func() {
		legacy, _ := ini.Load([]byte("[log]\n[log.console]\n"))
		_ = ReadLoggingConfig([]string{"console"}, "", legacy)
	})

	cfg, err := ini.Load([]byte("[log]\n[log.bogus]\n"))
	require.NoError(t, err)
	require.ErrorContains(t, ReadLoggingConfig([]string{"bogus"}, "", cfg, WithSlogCore(true)), `log mode "bogus" is not supported`)
	require.Error(t, ReadLoggingConfig([]string{"missing"}, "", cfg, WithSlogCore(true)))

	cfg, err = ini.Load([]byte("[log]\n[log.console]\noutput = somewhere\n"))
	require.NoError(t, err)
	require.ErrorContains(t, ReadLoggingConfig([]string{"console"}, "", cfg, WithSlogCore(true)), "somewhere")
}

func TestSlogCoreOffRestoresLegacyEngine(t *testing.T) {
	buf := useSlogCore(t, "[log]\nmode = capture\n[log.capture]\n")
	logger := New("slogtest.switch")
	logger.Info("slog")

	cfg, err := ini.Load([]byte("[log]\n[log.console]\nlevel = error\n"))
	require.NoError(t, err)
	require.NoError(t, ReadLoggingConfig([]string{"console"}, "", cfg))
	assert.False(t, slogCoreActive.Load())
	logger.Info("legacy")

	lines := buf.lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "slog", lines[0]["msg"])
}
