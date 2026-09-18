package log

import (
	"fmt"
	"regexp"
	"strings"
)

// Redacted is the stable replacement for secret material in log output.
const Redacted = "[REDACTED]"

var sensitiveKeyFragments = []string{
	"password",
	"passwd",
	"secret",
	"token",
	"authorization",
	"api_key",
	"apikey",
	"api-key",
	"credential",
	"cookie",
	"set-cookie",
}

var sensitiveKeyExact = map[string]struct{}{
	"auth":          {},
	"authorization": {},
	"password":      {},
	"passwd":        {},
	"secret":        {},
	"token":         {},
	"api_key":       {},
	"apikey":        {},
	"api-key":       {},
	"access_token":  {},
	"refresh_token": {},
	"client_secret": {},
	"private_key":   {},
	"credential":    {},
	"credentials":   {},
	"cookie":        {},
	"set-cookie":    {},
	"x-api-key":     {},
	"x-auth-token":  {},
}

var (
	bearerRe     = regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9\-_~+/.=]+`)
	basicRe      = regexp.MustCompile(`(?i)\b(basic)\s+[A-Za-z0-9+/=]+`)
	authHeaderRe = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(\S.+)`)
	secretKVRe   = regexp.MustCompile(`(?i)((?:password|passwd|secret|api[_-]?key|access_token|refresh_token|client_secret)\s*[:=]\s*)([^\s&,;]+)`)
)

// Redact replaces tokens, passwords, and Authorization credentials in s.
func Redact(s string) string {
	if s == "" {
		return s
	}
	s = authHeaderRe.ReplaceAllString(s, "$1"+Redacted)
	s = bearerRe.ReplaceAllString(s, "$1 "+Redacted)
	s = basicRe.ReplaceAllString(s, "$1 "+Redacted)
	s = secretKVRe.ReplaceAllString(s, "$1"+Redacted)
	return s
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	if _, ok := sensitiveKeyExact[k]; ok {
		return true
	}
	for _, frag := range sensitiveKeyFragments {
		if strings.Contains(k, frag) {
			return true
		}
	}
	return false
}

// RedactValue redacts val when the field key is sensitive, and always
// scrubs secret patterns out of strings and errors.
func RedactValue(key string, val any) any {
	if isSensitiveKey(key) {
		return Redacted
	}
	switch v := val.(type) {
	case nil:
		return nil
	case string:
		return Redact(v)
	case []byte:
		return Redact(string(v))
	case error:
		redacted := Redact(v.Error())
		if redacted == v.Error() {
			return v
		}
		return fmt.Errorf("%s", redacted)
	default:
		return val
	}
}

// RedactFields copies a key/value log argument list with secrets removed.
func RedactFields(keyvals []any) []any {
	if len(keyvals) == 0 {
		return keyvals
	}
	out := make([]any, len(keyvals))
	copy(out, keyvals)
	for i := 0; i < len(out); i += 2 {
		var key string
		if k, ok := out[i].(string); ok {
			key = k
		}
		if i+1 < len(out) {
			out[i+1] = RedactValue(key, out[i+1])
			continue
		}
		if s, ok := out[i].(string); ok {
			out[i] = Redact(s)
		}
	}
	return out
}
