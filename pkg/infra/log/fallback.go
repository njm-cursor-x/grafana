package log

import (
	"os"

	gokitlog "github.com/go-kit/log"
)

// stderrLog is used when a sink cannot log through itself: file rotation and
// flush run inside the file writer, and syslog init can fail before root is
// configured (root discards until ReadLoggingConfig).
var stderrLog gokitlog.Logger = gokitlog.NewLogfmtLogger(gokitlog.NewSyncWriter(os.Stderr))

func logToStderr(msg string, keyvals ...any) {
	kv := make([]any, 0, 4+len(keyvals))
	kv = append(kv, "logger", "infra.log", "msg", msg)
	kv = append(kv, keyvals...)
	_ = stderrLog.Log(redactKeyvals(kv)...)
}
