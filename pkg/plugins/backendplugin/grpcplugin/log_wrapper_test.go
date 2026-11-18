package grpcplugin

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"

	plog "github.com/grafana/grafana/pkg/plugins/log"
)

func TestLogWrapper(t *testing.T) {
	tcs := []struct {
		args           []any
		expectedResult []any
	}{
		{args: []any{}, expectedResult: []any{}},
		{args: []any{"1", "2", "3"}, expectedResult: []any{"1", "2", "3"}},
		{args: []any{"1", "2"}, expectedResult: []any{"1", "2"}},
		{args: []any{"1", "2", "timestamp", time.Now()}, expectedResult: []any{"1", "2"}},
		{args: []any{"1", "2", "timestamp", time.Now(), "3", "4"}, expectedResult: []any{"1", "2", "3", "4"}},
	}

	for i, tc := range tcs {
		t.Run(fmt.Sprintf("formatArgs testcase %d", i), func(t *testing.T) {
			res := formatArgs(tc.args...)
			assert.Exactly(t, tc.expectedResult, res)
		})
	}
}

type levelLogger struct {
	*plog.TestLogger
	min slog.Level
}

func (l levelLogger) Enabled(_ context.Context, level slog.Level) bool { return level >= l.min }

func TestLogWrapperLevels(t *testing.T) {
	t.Run("respects the wrapped logger level", func(t *testing.T) {
		logger := levelLogger{TestLogger: plog.NewTestLogger(), min: slog.LevelInfo}
		lw := logWrapper{Logger: logger}

		assert.False(t, lw.IsTrace())
		assert.False(t, lw.IsDebug())
		assert.True(t, lw.IsInfo())
		assert.True(t, lw.IsError())
		assert.Equal(t, hclog.Info, lw.GetLevel())

		lw.Debug("dropped")
		lw.Log(hclog.Trace, "dropped")
		lw.Info("kept")
		assert.Equal(t, 0, logger.DebugLogs.Calls)
		assert.Equal(t, 1, logger.InfoLogs.Calls)
	})

	t.Run("loggers without levels write everything", func(t *testing.T) {
		logger := plog.NewTestLogger()
		lw := logWrapper{Logger: logger}

		assert.True(t, lw.IsTrace())
		assert.Equal(t, hclog.Trace, lw.GetLevel())
		lw.Debug("kept")
		assert.Equal(t, 1, logger.DebugLogs.Calls)
	})
}
