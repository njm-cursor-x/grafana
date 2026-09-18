package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	gokitlog "github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		in    string
		want  string
		leaks []string
	}{
		{
			name:  "bearer token",
			in:    "Authorization: Bearer test-secret",
			want:  "Authorization: " + Redacted,
			leaks: []string{"test-secret"},
		},
		{
			name:  "bearer in message",
			in:    "upstream said Bearer abc.def-ghi",
			want:  "upstream said Bearer " + Redacted,
			leaks: []string{"abc.def-ghi"},
		},
		{
			name:  "basic auth",
			in:    "Authorization: Basic dXNlcjpwYXNz",
			want:  "Authorization: " + Redacted,
			leaks: []string{"dXNlcjpwYXNz"},
		},
		{
			name:  "password assignment",
			in:    "login failed password=supersecret user=admin",
			want:  "login failed password=" + Redacted + " user=admin",
			leaks: []string{"supersecret"},
		},
		{
			name:  "api_key assignment",
			in:    "query api_key=abcd1234 failed",
			want:  "query api_key=" + Redacted + " failed",
			leaks: []string{"abcd1234"},
		},
		{
			name: "plain error stays",
			in:   "datasource timeout",
			want: "datasource timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Redact(tt.in)
			assert.Equal(t, tt.want, got)
			for _, leak := range tt.leaks {
				assert.NotContains(t, got, leak)
			}
		})
	}
}

func TestRedactFields_SensitiveKeys(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	fields := []any{
		"msg", "query failed",
		"Authorization", "Bearer " + secret,
		"password", "hunter2",
		"api_key", secret,
		"err", errors.New("Authorization: Bearer " + secret),
		"dashboardId", 42,
	}

	got := RedactFields(fields)
	require.Len(t, got, len(fields))

	kv := map[any]any{}
	for i := 0; i < len(got); i += 2 {
		kv[got[i]] = got[i+1]
	}

	assert.Equal(t, "query failed", kv["msg"])
	assert.Equal(t, Redacted, kv["Authorization"])
	assert.Equal(t, Redacted, kv["password"])
	assert.Equal(t, Redacted, kv["api_key"])
	assert.Equal(t, 42, kv["dashboardId"])
	require.Error(t, kv["err"].(error))
	assert.NotContains(t, kv["err"].(error).Error(), secret)
	assert.Contains(t, kv["err"].(error).Error(), Redacted)

	encoded := fmt.Sprintf("%v", got)
	assert.NotContains(t, encoded, secret)
	assert.NotContains(t, encoded, "hunter2")
}

func TestRedaction_AppliedOnLogAndJSON(t *testing.T) {
	scenario := newLoggerScenario(t)
	secret := "test-secret"

	New("http").Error("request failed",
		"Authorization", "Bearer "+secret,
		"password", "hunter2",
		"err", errors.New("Authorization: Bearer "+secret),
		"orgId", 1,
	)

	require.Len(t, scenario.loggedArgs, 1)
	line := scenario.loggedArgs[0]
	joined := fmt.Sprintf("%v", line)
	assert.NotContains(t, joined, secret)
	assert.NotContains(t, joined, "hunter2")
	assert.Contains(t, joined, Redacted)
	assert.Contains(t, joined, "request failed")

	kv := map[any]any{}
	for i := 0; i < len(line); i += 2 {
		kv[line[i]] = line[i+1]
	}
	assert.Equal(t, "http", kv["logger"])
	assert.Equal(t, "request failed", kv["msg"])
	assert.Equal(t, Redacted, kv["Authorization"])
	assert.Equal(t, Redacted, kv["password"])
	assert.Equal(t, 1, kv["orgId"])
	require.Error(t, kv["err"].(error))
	assert.NotContains(t, kv["err"].(error).Error(), secret)

	var buf bytes.Buffer
	jsonLogger := gokitlog.NewJSONLogger(&buf)
	require.NoError(t, jsonLogger.Log(RedactFields([]any{
		"msg", "query failed",
		"logger", "query_data",
		"Authorization", "Bearer " + secret,
		"err", errors.New("Authorization: Bearer " + secret),
	})...))

	encoded := buf.String()
	assert.NotContains(t, encoded, secret)
	assert.Contains(t, encoded, Redacted)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "query failed", parsed["msg"])
	assert.Equal(t, "query_data", parsed["logger"])
	assert.Equal(t, Redacted, parsed["Authorization"])
	assert.True(t, strings.Contains(fmt.Sprint(parsed["err"]), Redacted))
}

func TestRedactFields_OddTrailingValue(t *testing.T) {
	t.Parallel()
	got := RedactFields([]any{"orphan Bearer test-secret"})
	require.Len(t, got, 1)
	assert.Equal(t, "orphan Bearer "+Redacted, got[0])
}
