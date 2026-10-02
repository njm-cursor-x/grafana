package log

import (
	"bytes"
	"errors"
	"testing"

	gokitlog "github.com/go-kit/log"
	"github.com/stretchr/testify/require"
)

func TestLogToStderr_structuredLogfmt(t *testing.T) {
	var buf bytes.Buffer
	orig := stderrLog
	stderrLog = gokitlog.NewLogfmtLogger(&buf)
	t.Cleanup(func() {
		stderrLog = orig
	})

	logToStderr("file log rotate failed", "filename", "grafana.log", "err", errors.New("disk full"))

	line := buf.String()
	require.Contains(t, line, "logger=infra.log")
	require.Contains(t, line, "msg=\"file log rotate failed\"")
	require.Contains(t, line, "filename=grafana.log")
	require.Contains(t, line, "err=\"disk full\"")
	require.NotContains(t, line, "FileLogWriter")
}

func TestLogToStderr_redactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	orig := stderrLog
	stderrLog = gokitlog.NewLogfmtLogger(&buf)
	t.Cleanup(func() {
		stderrLog = orig
	})

	logToStderr("syslog handler init failed", "token", "raw-token", "err", errors.New("Bearer test-secret"))

	line := buf.String()
	require.NotContains(t, line, "raw-token")
	require.NotContains(t, line, "test-secret")
	require.Contains(t, line, "token="+redactedValue)
	require.Contains(t, line, "msg=\"syslog handler init failed\"")
}
