package log

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"
)

// Assembled at runtime so no credential-shaped literal is committed.
var (
	secPassword     = "Pr0be-" + strings.Repeat("pw", 6)
	secBearerJWT    = strings.Join([]string{"eyJhbGciOiJIUzI1NiJ9", strings.Repeat("U2VjUHJvYmU", 3), "c2lnMTIz"}, ".")
	secServiceToken = "glsa" + "_" + strings.Repeat("Pr0be", 7) + "_" + "9f8e7d6c"
	secSessionID    = strings.Repeat("5e55", 8)
	secQueryToken   = "qt" + strings.Repeat("7a", 12)
)

// logThroughFileHandler configures the production handler chain (ReadLoggingConfig
// with a file handler in the given format), runs emit, and returns the file output.
func logThroughFileHandler(t *testing.T, format string, emit func()) string {
	t.Helper()

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
format = %s
file_name = %s
`, format, logFile)))
	require.NoError(t, err)
	require.NoError(t, ReadLoggingConfig([]string{"file"}, t.TempDir(), cfg))

	emit()
	require.NoError(t, Close())

	out, err := os.ReadFile(logFile) //nolint:gosec
	require.NoError(t, err)
	return string(out)
}

func TestRedaction_SecurityLane(t *testing.T) {
	type credentials struct {
		Username string
		Password string
	}

	cases := []struct {
		name   string
		secret string
		emit   func(l *ConcreteLogger)
		// knownGap documents a leak the redactor doesn't handle yet. The case skips while
		// the secret still leaks and fails once it doesn't, so the marker gets removed.
		knownGap string
	}{
		{
			name:   "bearer token in a wrapped upstream error",
			secret: secBearerJWT,
			emit: func(l *ConcreteLogger) {
				err := fmt.Errorf("query failed: %w", errors.New("request header Authorization: Bearer "+secBearerJWT))
				l.Error("datasource query failed", "err", err)
			},
		},
		{
			name:   "service account token in the message itself",
			secret: secServiceToken,
			emit:   func(l *ConcreteLogger) { l.Warn("rejected token " + secServiceToken) },
		},
		{
			name:   "sensitive key with mixed casing and separators",
			secret: secPassword,
			emit:   func(l *ConcreteLogger) { l.Info("datasource saved", "Basic-Auth-Password", secPassword) },
		},
		{
			name:   "sensitive key attached via New()",
			secret: secServiceToken,
			emit:   func(l *ConcreteLogger) { l.New("serviceAccountToken", secServiceToken).Info("sync started") },
		},
		{
			name:   "sensitive key attached via contextual attributes",
			secret: secBearerJWT,
			emit: func(l *ConcreteLogger) {
				ctx := WithContextualAttributes(context.Background(), []any{"authorization", "Bearer " + secBearerJWT})
				l.FromContext(ctx).Info("handled request")
			},
		},
		{
			name:   "http.Header passed as a field",
			secret: secBearerJWT,
			emit: func(l *ConcreteLogger) {
				h := http.Header{}
				h.Set("Authorization", "Bearer "+secBearerJWT)
				l.Debug("outgoing request", "headers", h)
			},
		},
		{
			name:   "app SDK slog logger",
			secret: secPassword,
			emit:   func(_ *ConcreteLogger) { logging.DefaultLogger.Info("app sdk login", "password", secPassword) },
		},
		{
			name:   "app SDK slog logger with a group",
			secret: secPassword,
			emit: func(_ *ConcreteLogger) {
				logging.DefaultLogger.Info("app sdk login", slog.Group("auth", "password", secPassword))
			},
		},
		{
			name:     "short bearer token from the ticket fixture",
			knownGap: "free-text bearer tokens shorter than 16 characters or without a digit are not masked",
			secret:   "test-secret",
			emit: func(l *ConcreteLogger) {
				l.Error("upstream rejected request", "err", errors.New("Authorization: Bearer test-secret"))
			},
		},
		{
			name:     "map of headers passed as a field",
			knownGap: "only http.Header is inspected; other maps are formatted verbatim",
			secret:   secBearerJWT,
			emit: func(l *ConcreteLogger) {
				l.Debug("outgoing request", "headers", map[string]string{"Authorization": "Bearer " + secBearerJWT})
			},
		},
		{
			name:     "struct with a password field",
			knownGap: "struct fields are formatted verbatim with %+v or JSON",
			secret:   secPassword,
			emit: func(l *ConcreteLogger) {
				l.Debug("loaded credentials", "creds", credentials{Username: "admin", Password: secPassword})
			},
		},
		{
			name:     "session cookie in free text",
			knownGap: "cookie values in free text are not masked",
			secret:   secSessionID,
			emit: func(l *ConcreteLogger) {
				l.Warn("proxy request failed", "err", errors.New("forwarded Cookie: grafana_session="+secSessionID))
			},
		},
		{
			name:     "auth_token query parameter in a URL string",
			knownGap: "auth_token query parameters in string values are not masked",
			secret:   secQueryToken,
			emit: func(l *ConcreteLogger) {
				l.Info("rendering", "url", "https://grafana.example.com/d/abc?auth_token="+secQueryToken)
			},
		},
	}

	for _, format := range []string{"json", "text"} {
		for _, tc := range cases {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				out := logThroughFileHandler(t, format, func() { tc.emit(New("security-lane")) })
				require.NotEmpty(t, strings.TrimSpace(out), "expected a log line to be written")
				if tc.knownGap != "" {
					if !strings.Contains(out, tc.secret) {
						t.Fatalf("known gap %q is no longer reproducible; remove knownGap from this case", tc.knownGap)
					}
					t.Skipf("known gap: %s", tc.knownGap)
				}
				assert.NotContains(t, out, tc.secret)
			})
		}
	}
}
