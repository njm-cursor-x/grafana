package console

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("writes the configured format", func(t *testing.T) {
		buf := &bytes.Buffer{}
		h, err := New(Config{Format: "json", Writer: buf})
		require.NoError(t, err)
		require.NoError(t, h.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)))
		assert.Contains(t, buf.String(), `"msg":"hello"`)
	})

	t.Run("selects the output stream", func(t *testing.T) {
		for _, output := range []string{"", "stdout", "STDERR"} {
			_, err := New(Config{Output: output})
			require.NoError(t, err, output)
		}
		_, err := New(Config{Output: "/var/log/grafana.log"})
		require.Error(t, err)
	})

	t.Run("never closes the standard streams", func(t *testing.T) {
		h, err := New(Config{})
		require.NoError(t, err)
		_, closable := h.(interface{ Close() error })
		assert.False(t, closable)
	})
}
