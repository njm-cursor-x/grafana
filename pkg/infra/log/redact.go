package log

import (
	"log/slog"
	"regexp"
	"strings"
)

const redactedValue = "[REDACTED]"

// sensitiveKeyFragments match log field names that must not keep their values.
// Matching is on the normalized key (lower-case, '-' → '_'), including compounds
// such as db_password or access_token.
var sensitiveKeyFragments = []string{
	"password",
	"passwd",
	"secret",
	"token",
	"authorization",
	"api_key",
	"apikey",
	"bearer",
	"cookie",
	"credential",
	"private_key",
}

// bearerSecret and inlineSecret strip credentials that show up inside a
// message or error string rather than under a sensitive key.
var (
	bearerSecret = regexp.MustCompile(`(?i)(bearer\s+)\S+`)
	// Optional "Bearer" so "Authorization: Bearer <token>" is one replacement,
	// not "Authorization: [REDACTED] <token>".
	inlineSecret = regexp.MustCompile(`(?i)((?:password|passwd|secret|api[_-]?key|token|authorization)\s*[:=]\s*)(?:bearer\s+)?\S+`)
)

func redactKeyvals(keyvals []any) []any {
	if len(keyvals) < 2 {
		return keyvals
	}
	out := make([]any, len(keyvals))
	copy(out, keyvals)
	for i := 0; i+1 < len(out); i += 2 {
		if key, ok := keyString(out[i]); ok && isSensitiveKey(key) {
			out[i+1] = redactedValue
			continue
		}
		out[i+1] = redactValue(out[i+1])
	}
	return out
}

func keyString(key any) (string, bool) {
	switch v := key.(type) {
	case string:
		return v, true
	case slog.Value:
		if v.Kind() == slog.KindString {
			return v.String(), true
		}
		return "", false
	default:
		return "", false
	}
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	k = strings.ReplaceAll(k, "-", "_")
	k = strings.ReplaceAll(k, " ", "")
	for _, frag := range sensitiveKeyFragments {
		if k == frag || strings.HasSuffix(k, "_"+frag) || strings.HasPrefix(k, frag+"_") || strings.Contains(k, "_"+frag+"_") {
			return true
		}
	}
	return false
}

func redactValue(v any) any {
	switch val := v.(type) {
	case string:
		red := redactString(val)
		if red == val {
			return v
		}
		return red
	case error:
		s := val.Error()
		red := redactString(s)
		if red == s {
			return v
		}
		return red
	case slog.Value:
		if val.Kind() != slog.KindString {
			return v
		}
		red := redactString(val.String())
		if red == val.String() {
			return v
		}
		return red
	default:
		return v
	}
}

func redactString(s string) string {
	if s == "" || !mightContainSecret(s) {
		return s
	}
	out := inlineSecret.ReplaceAllString(s, "${1}"+redactedValue)
	return bearerSecret.ReplaceAllString(out, "${1}"+redactedValue)
}

func mightContainSecret(s string) bool {
	return strings.Contains(s, "earer") ||
		strings.Contains(s, "assword") ||
		strings.Contains(s, "ecret") ||
		strings.Contains(s, "oken") ||
		strings.Contains(s, "uthorization") ||
		strings.Contains(s, "api_key") ||
		strings.Contains(s, "api-key") ||
		strings.Contains(s, "apikey") ||
		strings.Contains(s, "ookie")
}
