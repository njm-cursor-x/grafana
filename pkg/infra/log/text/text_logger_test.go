package text

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	gokitlog "github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// User-controlled strings (dashboard titles, query text) must stay inside a single
// field value; a newline or logfmt pair in them must not forge a second record or key.
func TestLog_UserControlledValuesCannotForgeRecords(t *testing.T) {
	hostile := "My dashboard %s %d %!v\nlevel=error msg=\"forged\" logger=auth"

	t.Run("text", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, NewTextLogger(&buf).Log("level", "info", "msg", "saved", "dashboardTitle", hostile))

		out := strings.TrimSuffix(buf.String(), "\n")
		assert.NotContains(t, out, "\n", "value must not split the record across lines")
		assert.Equal(t, 1, strings.Count(out, "level="), "value must not inject a second level key")
		assert.Contains(t, out, `%s %d %!v`, "format verbs must be logged literally, not interpreted")
	})

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, gokitlog.NewJSONLogger(&buf).Log("level", "info", "msg", "saved", "dashboardTitle", hostile))

		var record map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
		assert.Equal(t, "info", record["level"])
		assert.Equal(t, hostile, record["dashboardTitle"])
	})
}
