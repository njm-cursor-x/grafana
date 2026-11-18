package setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrustIncomingRequestID(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		cfg, err := NewCfgFromBytes([]byte(``))
		require.NoError(t, err)
		require.False(t, cfg.TrustIncomingRequestID)
	})

	t.Run("parses [log] trust_incoming_request_id", func(t *testing.T) {
		cfg, err := NewCfgFromBytes([]byte(`
[log]
trust_incoming_request_id = true
`))
		require.NoError(t, err)
		require.True(t, cfg.TrustIncomingRequestID)
	})
}
