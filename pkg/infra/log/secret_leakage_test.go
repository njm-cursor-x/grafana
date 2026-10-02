package log

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Canaries are fake fixtures. They are not credentials.
const (
	bearerCanary   = "nds10-bearer-canary"
	sessionCanary  = "nds10-session-canary"
	userinfoCanary = "nds10-userinfo-canary"
)

func TestJSONAndTextLogsDoNotInventRequestSecrets(t *testing.T) {
	t.Parallel()

	// Held only by the test. The logger must not copy ambient material it was not given.
	authorization := "Bearer " + bearerCanary
	cookie := "grafana_session=" + sessionCanary
	_ = authorization
	_ = cookie

	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			logger := newConcreteLogger(getLogFormat(format)(&buf))
			logger.Info("request completed", "method", "GET", "path", "/api/search", "status", 200)

			out := buf.String()
			require.Contains(t, out, "request completed")
			require.Contains(t, out, "/api/search")
			require.NotContains(t, out, bearerCanary)
			require.NotContains(t, out, sessionCanary)
			require.NotContains(t, out, userinfoCanary)
			require.NotContains(t, out, "Bearer")
			require.NotContains(t, out, "grafana_session")
		})
	}
}

func TestUserControlledLogFieldsAreNotFormatStringSinks(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := newConcreteLogger(getLogFormat("json")(&buf))
	title := "ops %s overview"
	logger.Info("dashboard loaded", "title", title, "dashboardUid", "abc")

	out := buf.String()
	require.Contains(t, out, "dashboard loaded")
	require.Contains(t, out, "ops %s overview")
	require.Contains(t, out, "abc")
	require.NotContains(t, out, bearerCanary)
	require.False(t, strings.Contains(out, "ops nds10"), "title must stay a field, not a formatted string")
}
