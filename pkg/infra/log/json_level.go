package log

import (
	"fmt"
	"io"
	"strings"

	gokitlog "github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

// jsonLevelLogger wraps go-kit's JSON logger so Grafana Logs Drilldown can
// group on a stable `level` field. Stock go-kit JSON emits `lvl` (and "eror"
// for errors). We keep `lvl` for existing queries and add `level`.
type jsonLevelLogger struct {
	next gokitlog.Logger
}

func newJSONLevelLogger(w io.Writer) gokitlog.Logger {
	return jsonLevelLogger{next: gokitlog.NewJSONLogger(gokitlog.NewSyncWriter(w))}
}

func (l jsonLevelLogger) Log(keyvals ...any) error {
	return l.next.Log(remapJSONLevelKeyvals(keyvals)...)
}

func remapJSONLevelKeyvals(keyvals []any) []any {
	if len(keyvals) == 0 {
		return keyvals
	}
	hasLevel := false
	var levelVal any
	for i := 0; i < len(keyvals); i += 2 {
		k := keyvals[i]
		if k == "level" {
			hasLevel = true
		}
		if i+1 >= len(keyvals) {
			continue
		}
		if isLvlKey(k) {
			levelVal = normalizeJSONLogLevel(keyvals[i+1])
		}
	}
	if hasLevel || levelVal == nil {
		return keyvals
	}
	out := make([]any, len(keyvals)+2)
	copy(out, keyvals)
	out[len(keyvals)] = "level"
	out[len(keyvals)+1] = levelVal
	return out
}

func isLvlKey(k any) bool {
	if k == level.Key() {
		return true
	}
	s, ok := k.(string)
	return ok && s == "lvl"
}

func normalizeJSONLogLevel(v any) string {
	s := strings.ToLower(fmt.Sprint(v))
	if s == "eror" {
		return "error"
	}
	return s
}
