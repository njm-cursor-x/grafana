package text

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	gokitlog "github.com/go-kit/log"
	"github.com/go-logfmt/logfmt"
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

		assert.NotContains(t, strings.TrimSuffix(buf.String(), "\n"), "\n", "value must not split the record across lines")

		dec := logfmt.NewDecoder(&buf)
		require.True(t, dec.ScanRecord())
		fields := map[string]string{}
		for dec.ScanKeyval() {
			key := string(dec.Key())
			_, dup := fields[key]
			assert.Falsef(t, dup, "value must not inject a duplicate %q key", key)
			fields[key] = string(dec.Value())
		}
		require.NoError(t, dec.Err())
		assert.False(t, dec.ScanRecord(), "value must not produce a second record")

		assert.Equal(t, map[string]string{"level": "info", "msg": "saved", "dashboardTitle": hostile}, fields)
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
