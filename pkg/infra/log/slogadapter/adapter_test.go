package slogadapter

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/log/logtest"
)

func TestLegacyWithGroup(t *testing.T) {
	fake := &logtest.Fake{}
	logger := slog.New(New(fake)).WithGroup("req")

	logger.Info("hello", "id", 1, slog.Group("user", "id", 2))

	require.Equal(t, 1, fake.InfoLogs.Calls)
	assert.Equal(t, []any{"req.id", slog.IntValue(1), "req.user.id", slog.IntValue(2)}, fake.InfoLogs.Ctx)
}

func TestSlogCore(t *testing.T) {
	capture := logtest.UseSlogCore(t, "level = info")
	ctx := log.WithContextualAttributes(context.Background(), []any{"orgID", 3})

	h := New(log.New("slogadapter.test"))
	assert.False(t, h.Enabled(ctx, slog.LevelDebug))

	logger := slog.New(h)
	logger.Debug("dropped")
	logger.With("component", "x").WithGroup("req").InfoContext(ctx, "hello", "id", "r1")

	lines := capture.Lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "hello", lines[0]["msg"])
	assert.Equal(t, "slogadapter.test", lines[0]["logger"])
	assert.Equal(t, "x", lines[0]["component"])
	assert.Equal(t, 3.0, lines[0]["grafana.org.id"])
	assert.Equal(t, map[string]any{"id": "r1"}, lines[0]["req"])
	assert.NotContains(t, lines[0], "group")
}

func TestSlogCoreKlogVerbosity(t *testing.T) {
	capture := logtest.UseSlogCore(t, "level = debug")

	// klog writes through logr, which maps V(n) to slog level -n.
	slog.New(New(log.New("grafana-apiserver"))).Log(context.Background(), slog.Level(-7), "verbose")

	lines := capture.Lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "verbose", lines[0]["msg"])
	assert.Equal(t, "debug", lines[0]["level"])
}

func TestSlogCoreFakeLoggerStaysLegacy(t *testing.T) {
	capture := logtest.UseSlogCore(t)
	fake := &logtest.Fake{}

	slog.New(New(fake)).Info("to the fake")

	assert.Equal(t, 1, fake.InfoLogs.Calls)
	assert.Empty(t, capture.Lines(t))
}
