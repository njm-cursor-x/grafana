package pluginslog

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/log/logtest"
	pluginslog "github.com/grafana/grafana/pkg/plugins/log"
)

func TestSlogCore(t *testing.T) {
	capture := logtest.UseSlogCore(t, "level = info")
	ctx := log.WithContextualAttributes(context.Background(), []any{"orgID", 5})

	logger := pluginslog.New("plugin.test")
	enabler, ok := logger.(pluginslog.LevelEnabler)
	require.True(t, ok)
	assert.False(t, enabler.Enabled(ctx, slog.LevelDebug))
	assert.True(t, enabler.Enabled(ctx, slog.LevelInfo))

	logger.Debug("dropped")
	logger.New("pluginId", "test-app").FromContext(ctx).Warn("careful", "err", "boom")

	lines := capture.Lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "careful", lines[0]["msg"])
	assert.Equal(t, "warn", lines[0]["level"])
	assert.Equal(t, "plugin.test", lines[0]["logger"])
	assert.Equal(t, "test-app", lines[0]["pluginId"])
	assert.Equal(t, "boom", lines[0]["error"])
	assert.Equal(t, 5.0, lines[0]["grafana.org.id"])
}

func TestLegacyEngineReportsAllLevels(t *testing.T) {
	logger := pluginslog.New("plugin.legacy")
	enabler, ok := logger.(pluginslog.LevelEnabler)
	require.True(t, ok)
	assert.True(t, enabler.Enabled(context.Background(), slog.LevelDebug))
}
