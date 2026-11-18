package log

import (
	"context"
	"log/slog"
	"testing"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlogHandler(t *testing.T) {
	buf := useSlogCore(t, `
[log]
mode = capture
level = info
filters = slogtest.bridge.verbose:debug

[log.capture]
`)
	ctx := context.Background()

	h := SlogHandler(New("slogtest.bridge").New("orgId", 7))
	require.NotNil(t, h)
	assert.False(t, h.Enabled(ctx, slog.Level(-6)), "klog verbosity maps to debug")
	assert.True(t, h.Enabled(ctx, slog.LevelInfo))

	logger := slog.New(h)
	logger.Log(ctx, slog.Level(-1), "dropped")
	logger.Log(ctx, slog.LevelInfo+2, "between levels")
	logger.WithGroup("req").Info("grouped", "id", "r1")

	verbose := slog.New(SlogHandler(New("slogtest.bridge.verbose")))
	verbose.Log(ctx, slog.Level(-8), "kept by filter")

	lines := buf.lines(t)
	require.Len(t, lines, 3)

	assert.Equal(t, "between levels", lines[0]["msg"])
	assert.Equal(t, "info", lines[0]["level"])
	assert.Equal(t, "slogtest.bridge", lines[0]["logger"])
	assert.Equal(t, 7.0, lines[0]["grafana.org.id"])

	assert.Equal(t, map[string]any{"id": "r1"}, lines[1]["req"])
	assert.Equal(t, "slogtest.bridge", lines[1]["logger"])

	assert.Equal(t, "kept by filter", lines[2]["msg"])
	assert.Equal(t, "debug", lines[2]["level"])
}

func TestSlogHandlerOffPipeline(t *testing.T) {
	assert.Nil(t, SlogHandler(NewNopLogger()))
}

func TestSlogCoreActive(t *testing.T) {
	useSlogCore(t, "[log]\nmode = capture\n[log.capture]\n")
	assert.True(t, SlogCoreActive())
}

func TestSlogCoreAppSDKLogger(t *testing.T) {
	buf := useSlogCore(t, "[log]\nmode = capture\nlevel = info\n[log.capture]\n")

	logging.DefaultLogger.Debug("dropped")
	logging.DefaultLogger.With("logger", "app.test").Info("from app sdk", "orgID", 2)

	lines := buf.lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "from app sdk", lines[0]["msg"])
	assert.Equal(t, "app.test", lines[0]["logger"])
	assert.Equal(t, 2.0, lines[0]["grafana.org.id"])
}
