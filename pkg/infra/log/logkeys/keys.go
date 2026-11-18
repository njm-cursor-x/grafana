// Package logkeys is the source of truth for canonical structured log field names
// and the mapping from Grafana-legacy keys. Other log packages should import this
// table instead of copying it.
package logkeys

const (
	KeyLogger    = "logger"
	KeyError     = "error"
	KeyOrgID     = "grafana.org.id"
	KeyUserID    = "enduser.id"
	KeyTraceID   = "trace_id"
	KeySpanID    = "span_id"
	KeyRequestID = "http.request.id"
	KeyNamespace = "k8s.namespace.name"
	KeyLevel     = "level"
	KeyMessage   = "msg"
	KeyTime      = "t"
)

var legacyKeys = map[string]string{
	"err":       KeyError,
	"orgId":     KeyOrgID,
	"orgID":     KeyOrgID,
	"userId":    KeyUserID,
	"userID":    KeyUserID,
	"traceID":   KeyTraceID,
	"traceId":   KeyTraceID,
	"spanID":    KeySpanID,
	"spanId":    KeySpanID,
	"requestID": KeyRequestID,
	"requestId": KeyRequestID,
	"namespace": KeyNamespace,
}

// Login and username are personal data; only user IDs are logged.
var droppedKeys = map[string]struct{}{
	"uname":     {},
	"username":  {},
	"login":     {},
	"user.name": {},
}

// CanonicalKey returns the canonical name for a legacy (or already canonical) key.
// ok is false when the key must not be logged (login/username).
func CanonicalKey(key string) (string, bool) {
	if _, drop := droppedKeys[key]; drop {
		return "", false
	}
	if canonical, ok := legacyKeys[key]; ok {
		return canonical, true
	}
	return key, true
}
