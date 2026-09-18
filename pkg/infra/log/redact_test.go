package log

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	gokitlog "github.com/go-kit/log"
	"github.com/stretchr/testify/require"
)

const leakProbe = "test-secret"

func TestRedactSecrets_BearerAuthorization(t *testing.T) {
	got := RedactSecrets("Authorization: Bearer " + leakProbe)
	require.NotContains(t, got, leakProbe)
	require.Contains(t, got, Redacted)
}

func TestBearerTokenDoesNotAppearInBackendLogOutput(t *testing.T) {
	scenario := newLoggerScenario(t)
	logger := New("http")

	logger.Info("request failed", "Authorization", "Bearer "+leakProbe)
	logger.Error("Authorization: Bearer " + leakProbe)
	logger.Warn("proxy hop", "headers", http.Header{
		"Authorization": []string{"Bearer " + leakProbe},
		"Cookie":        []string{"grafana_session=" + leakProbe},
	})
	logger.Debug("login", "password", leakProbe, "api_key", leakProbe)

	require.NotEmpty(t, scenario.loggedArgs)
	for i, args := range scenario.loggedArgs {
		for _, arg := range args {
			require.NotContainsf(t, fmt.Sprint(arg), leakProbe, "line %d leaked probe secret: %#v", i, args)
		}
	}
}

func TestBearerTokenDoesNotAppearInJSONLogBytes(t *testing.T) {
	var buf bytes.Buffer
	origRoot := root
	origNow := now
	now = func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) }
	root = newManager(gokitlog.NewJSONLogger(&buf))
	t.Cleanup(func() {
		root = origRoot
		now = origNow
	})

	New("http").Info(
		"request failed",
		"Authorization", "Bearer "+leakProbe,
		"cookie", "grafana_session="+leakProbe,
		"err", fmt.Errorf("upstream rejected Authorization: Bearer %s", leakProbe),
	)

	out := buf.String()
	require.NotEmpty(t, out)
	require.NotContains(t, out, leakProbe)
	require.NotContains(t, out, "Bearer "+leakProbe)
	require.Contains(t, out, "request failed")
	require.Contains(t, out, Redacted)
}

func TestUserControlledStringsAreFieldsNotFormatSinks(t *testing.T) {
	var buf bytes.Buffer
	origRoot := root
	origNow := now
	now = func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) }
	root = newManager(gokitlog.NewJSONLogger(&buf))
	t.Cleanup(func() {
		root = origRoot
		now = origNow
	})

	// Verbatim user input that would be dangerous as a fmt format string.
	title := `Ops %s %!(EXTRA string=boom) %d`
	query := `up{job='%s'} OR rate(http_requests[5m])`

	New("dashboard").Info("query failed", "dashboardTitle", title, "query", query)

	out := buf.String()
	require.Contains(t, out, title)
	require.Contains(t, out, query)
}

func TestRedactLogKeyvalsLeavesSafeFields(t *testing.T) {
	in := []any{"msg", "hello", "dashboardTitle", "Sales %s", "orgId", 1}
	out := RedactLogKeyvals(in)
	require.Equal(t, in, out)
	// copy, not alias — mutating the result must not change the caller slice
	out[1] = "mutated"
	require.Equal(t, "hello", in[1])
}

func TestBackendAliasesMatchSecurityNames(t *testing.T) {
	in := "Authorization: Bearer " + leakProbe
	require.Equal(t, RedactSecrets(in), Redact(in))
	require.NotContains(t, Redact(in), leakProbe)

	fields := []any{"msg", "query failed", "Authorization", "Bearer " + leakProbe, "sessionToken", leakProbe}
	viaSecurity := RedactLogKeyvals(fields)
	viaBackend := RedactFields(fields)
	require.Equal(t, viaSecurity, viaBackend)
	require.Equal(t, Redacted, RedactValue("Authorization", "Bearer "+leakProbe))
	require.Equal(t, Redacted, viaBackend[3])
	require.Equal(t, Redacted, viaBackend[5])
	require.NotContains(t, fmt.Sprint(viaBackend), leakProbe)
}
