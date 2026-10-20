package log

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"
)

// Fake credentials are assembled at runtime so no token-shaped literal is
// committed; secret scanners would otherwise flag (and block) this file.
const (
	fakePassword      = "hunter2-Sup3rS3cret"
	fakeBasicUserPass = "admin:" + fakePassword
)

var (
	fakeServiceToken    = "glsa" + "_" + strings.Repeat("Fak3", 8) + "_" + "0a1b2c3d"
	fakeCloudToken      = "glc" + "_" + strings.Repeat("RmFrZQ", 6) + "=="
	fakeLegacyAPIKey    = "eyJrIjoi" + strings.Repeat("RmFrZUtleQ", 8) + "=="
	fakeBearerJWT       = strings.Join([]string{"eyJhbGciOiJub25lIn0", strings.Repeat("RmFrZTEy", 4), "c2ln"}, ".")
	fakeBasicCredential = base64.StdEncoding.EncodeToString([]byte(fakeBasicUserPass))
)

func TestIsSensitiveKey(t *testing.T) {
	for _, key := range []string{
		"password", "Password", "db_password", "passwd", "secret", "clientSecret",
		"token", "accessToken", "refresh_token", "id-token", "apiKey", "api_key", "X-Api-Key",
		"Authorization", "Proxy-Authorization", "cookie", "Set-Cookie", "credentials", "privateKey",
	} {
		assert.Truef(t, IsSensitiveKey(key), "expected %q to be sensitive", key)
	}
	for _, key := range []string{
		"msg", "error", "userId", "orgId", "tokenId", "apiKeyId", "secretName", "path", "status", "logger",
	} {
		assert.Falsef(t, IsSensitiveKey(key), "expected %q not to be sensitive", key)
	}
}

func TestRedactString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		secret string
		want   string
	}{
		{
			name:   "bearer authorization header",
			input:  "Authorization: Bearer " + fakeBearerJWT,
			secret: fakeBearerJWT,
			want:   "Authorization: Bearer " + RedactedValue,
		},
		{
			name:   "basic authorization header",
			input:  "authorization=Basic " + fakeBasicCredential,
			secret: fakeBasicCredential,
			want:   "authorization=Basic " + RedactedValue,
		},
		{
			name:   "service account token",
			input:  "request failed for token " + fakeServiceToken,
			secret: fakeServiceToken,
			want:   "request failed for token " + RedactedValue,
		},
		{
			name:   "cloud access policy token",
			input:  "using " + fakeCloudToken,
			secret: fakeCloudToken,
			want:   "using " + RedactedValue,
		},
		{
			name:   "legacy api key",
			input:  "key=" + fakeLegacyAPIKey,
			secret: fakeLegacyAPIKey,
			want:   "key=" + RedactedValue,
		},
		{
			name:   "url with password",
			input:  `Get "https://admin:` + fakePassword + `@example.com/api": dial tcp: timeout`,
			secret: fakePassword,
			want:   `Get "https://admin:` + RedactedValue + `@example.com/api": dial tcp: timeout`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactString(tc.input)
			assert.Equal(t, tc.want, got)
			assert.NotContains(t, got, tc.secret)
		})
	}

	t.Run("leaves prose about auth schemes untouched", func(t *testing.T) {
		for _, s := range []string{
			"bearer authentication is disabled",
			"failed to parse basic authentication header",
			"https://example.com/path@v1",
		} {
			assert.Equal(t, s, RedactString(s))
		}
	})
}

func TestRedactKeyvals(t *testing.T) {
	t.Run("does not copy or mutate when nothing is sensitive", func(t *testing.T) {
		in := []any{"msg", "hello", "userId", 42}
		out := RedactKeyvals(in)
		assert.Equal(t, []any{"msg", "hello", "userId", 42}, out)
	})

	t.Run("masks sensitive keys without mutating the input", func(t *testing.T) {
		in := []any{"msg", "login", "password", fakePassword, "userId", 1}
		out := RedactKeyvals(in)
		assert.Equal(t, []any{"msg", "login", "password", RedactedValue, "userId", 1}, out)
		assert.Equal(t, fakePassword, in[3])
	})

	t.Run("masks credentials inside error values", func(t *testing.T) {
		err := fmt.Errorf("upstream rejected request: %w", errors.New("Authorization: Bearer "+fakeBearerJWT))
		out := RedactKeyvals([]any{"error", err})
		redacted, ok := out[1].(error)
		require.True(t, ok, "redacted value should still be an error")
		assert.NotContains(t, redacted.Error(), fakeBearerJWT)
		assert.Contains(t, redacted.Error(), "upstream rejected request")
	})

	t.Run("masks sensitive http headers without mutating the original", func(t *testing.T) {
		h := http.Header{}
		h.Set("Authorization", "Bearer "+fakeBearerJWT)
		h.Set("X-Api-Key", fakeServiceToken)
		h.Set("Cookie", "grafana_session=abc123")
		h.Set("Content-Type", "application/json")

		out := RedactKeyvals([]any{"headers", h})
		redacted := out[1].(http.Header)
		assert.Equal(t, RedactedValue, redacted.Get("Authorization"))
		assert.Equal(t, RedactedValue, redacted.Get("X-Api-Key"))
		assert.Equal(t, RedactedValue, redacted.Get("Cookie"))
		assert.Equal(t, "application/json", redacted.Get("Content-Type"))
		assert.Equal(t, "Bearer "+fakeBearerJWT, h.Get("Authorization"))
	})

	t.Run("masks url passwords", func(t *testing.T) {
		u, err := url.Parse("postgres://grafana:" + fakePassword + "@db:5432/grafana")
		require.NoError(t, err)
		out := RedactKeyvals([]any{"dsn", u})
		assert.NotContains(t, fmt.Sprint(out[1]), fakePassword)
	})
}

// Exercises the same handler chain production uses: ReadLoggingConfig with a
// JSON file handler, named and contextual loggers, and FromContext attributes.
func TestReadLoggingConfig_RedactsSecretsFromOutput(t *testing.T) {
	origRoot := root
	t.Cleanup(func() {
		_ = Close()
		root = origRoot
	})
	root = newManager(origRoot.ConcreteLogger)

	logFile := filepath.Join(t.TempDir(), "grafana.log")
	cfg, err := ini.Load([]byte(fmt.Sprintf(`
[log]
mode = file
level = debug

[log.file]
format = json
file_name = %s
`, logFile)))
	require.NoError(t, err)
	require.NoError(t, ReadLoggingConfig([]string{"file"}, t.TempDir(), cfg))

	header := http.Header{}
	header.Set("Authorization", "Basic "+fakeBasicCredential)

	logger := New("redaction-test")
	logger.Info("user login", "userId", 7, "password", fakePassword)
	logger.Debug("calling upstream", "apiKey", fakeServiceToken, "headers", header)
	logger.Error("failed to call upstream", "error", errors.New("401 for Authorization: Bearer "+fakeBearerJWT))
	logger.New("token", fakeCloudToken).Warn("token about to expire")
	logger.Info("request with legacy key " + fakeLegacyAPIKey)

	ctx := WithContextualAttributes(context.Background(), []any{"accessToken", fakeBearerJWT})
	logger.FromContext(ctx).Info("contextual attributes")

	require.NoError(t, Close())

	out, err := os.ReadFile(logFile) //nolint:gosec
	require.NoError(t, err)
	output := string(out)

	require.Contains(t, output, `"msg":"user login"`)
	require.Contains(t, output, `"userId":7`)
	require.Contains(t, output, `"msg":"contextual attributes"`)
	require.Contains(t, output, RedactedValue)

	for name, secret := range map[string]string{
		"password":                      fakePassword,
		"basic credential":              fakeBasicCredential,
		"service account token":         fakeServiceToken,
		"cloud token":                   fakeCloudToken,
		"legacy api key":                fakeLegacyAPIKey,
		"bearer JWT":                    fakeBearerJWT,
		"basic user:password plaintext": fakeBasicUserPass,
	} {
		assert.NotContainsf(t, output, secret, "%s leaked into log output", name)
	}
}
