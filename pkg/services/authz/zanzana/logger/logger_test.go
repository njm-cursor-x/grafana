package logger

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/log/logtest"
)

func TestSlogCore(t *testing.T) {
	capture := logtest.UseSlogCore(t, "level = info")
	ctx := log.WithContextualAttributes(context.Background(), []any{"orgID", 9})

	l := New(log.New("zanzana.test")).With(zap.String("store", "s1"))
	l.Debug("dropped", zap.String("k", "v"))
	l.InfoWithContext(ctx, "check", zap.Bool("allowed", true), zap.Int("count", 3), zap.Error(errors.New("boom")))

	lines := capture.Lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "check", lines[0]["msg"])
	assert.Equal(t, "zanzana.test", lines[0]["logger"])
	assert.Equal(t, "s1", lines[0]["store"])
	assert.Equal(t, true, lines[0]["allowed"])
	assert.Equal(t, 3.0, lines[0]["count"])
	assert.Equal(t, "boom", lines[0]["error"])
	assert.Equal(t, 9.0, lines[0]["grafana.org.id"])
}

func TestLegacyEngine(t *testing.T) {
	fake := &logtest.Fake{}
	New(fake).Warn("legacy", zap.String("k", "v"))

	require.Equal(t, 1, fake.WarnLogs.Calls)
	assert.Equal(t, []any{"k", "v"}, fake.WarnLogs.Ctx)
}
