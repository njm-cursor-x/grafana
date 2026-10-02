package log

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	gokitlog "github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/stretchr/testify/require"
)

func TestLogger_redactsSensitiveFields(t *testing.T) {
	scenario := newLoggerScenario(t)
	logger := New("auth", "password", "hunter2", "service", "grafana")
	logger.Error(
		"login failed",
		"err", errors.New("Authorization: Bearer test-secret"),
		"authorization", "Bearer test-secret",
		"api-key", "sk-live-secret",
		"access_token", "tok-123",
		"user", "ada",
	)

	require.Len(t, scenario.loggedArgs, 1)
	flat := fmt.Sprint(scenario.loggedArgs[0])
	require.NotContains(t, flat, "hunter2")
	require.NotContains(t, flat, "test-secret")
	require.NotContains(t, flat, "sk-live-secret")
	require.NotContains(t, flat, "tok-123")
	require.Contains(t, flat, redactedValue)
	require.Contains(t, flat, "ada")
	require.Contains(t, flat, "grafana")

	scenario.ValidateLineEquality(t, 0, []any{
		"logger", "auth",
		"password", redactedValue,
		"service", "grafana",
		"t", scenario.mockedTime,
		level.Key(), level.ErrorValue(),
		"msg", "login failed",
		"err", "Authorization: " + redactedValue,
		"authorization", redactedValue,
		"api-key", redactedValue,
		"access_token", redactedValue,
		"user", "ada",
	})
}

func TestLogger_redactsBearerInsideMessage(t *testing.T) {
	scenario := newLoggerScenario(t)
	New("http").Info("saw Authorization: Bearer test-secret in header", "requestId", "r1")

	require.Len(t, scenario.loggedArgs, 1)
	flat := fmt.Sprint(scenario.loggedArgs[0])
	require.NotContains(t, flat, "test-secret")
	require.Contains(t, flat, "requestId")
	require.Contains(t, flat, "r1")
}

func TestWithPrefix_redactsContext(t *testing.T) {
	scenario := newLoggerScenario(t)
	ls := WithPrefix(New("test"), "db_password", "hunter2", "k1", "v1")
	ls.Info("hello")

	flat := fmt.Sprint(scenario.loggedArgs)
	require.NotContains(t, flat, "hunter2")
	require.Contains(t, flat, redactedValue)
	require.Contains(t, flat, "v1")
}

func TestJSONOutput_redactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	logger := newConcreteLogger(gokitlog.NewJSONLogger(&buf))
	logger.Info(
		"auth header received",
		"authorization", "Bearer test-secret",
		"note", "Authorization: Bearer test-secret",
		"detail", slog.StringValue("Bearer test-secret"),
		"dashboardId", "abc",
	)

	out := buf.String()
	require.NotContains(t, out, "test-secret")
	require.Contains(t, out, redactedValue)
	require.Contains(t, out, "auth header received")
	require.Contains(t, out, "abc")
	require.True(t, strings.HasPrefix(strings.TrimSpace(out), "{"), "expected a JSON log line, got %s", out)
}

func TestRedactKeyvals_oddLengthDoesNotPanic(t *testing.T) {
	out := redactKeyvals([]any{"password", "hunter2", "dangling"})
	require.Equal(t, []any{"password", redactedValue, "dangling"}, out)
}

func TestIsSensitiveKey(t *testing.T) {
	require.True(t, isSensitiveKey("Authorization"))
	require.True(t, isSensitiveKey("api-key"))
	require.True(t, isSensitiveKey("db_password"))
	require.True(t, isSensitiveKey("set-cookie"))
	require.False(t, isSensitiveKey("user"))
	require.False(t, isSensitiveKey("dashboardId"))
	require.False(t, isSensitiveKey("tokenizer"))
}
