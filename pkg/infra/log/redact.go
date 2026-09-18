package log

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

// Redacted is the placeholder written in place of secret material.
const Redacted = "[REDACTED]"

var sensitiveKeys = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
	"password":            {},
	"passwd":              {},
	"secret":              {},
	"api_key":             {},
	"api-key":             {},
	"apikey":              {},
	"access_token":        {},
	"access-token":        {},
	"accesstoken":         {},
	"refresh_token":       {},
	"id_token":            {},
	"client_secret":       {},
	"grafana_session":     {},
	"x-access-token":      {},
	"auth_token":          {},
	"bearer":              {},
}

var (
	reAuthScheme = regexp.MustCompile(`(?i)(\b(?:authorization\s*[:=]\s*)?(?:bearer|basic|token)\s+)\S+`)
	reAssign     = regexp.MustCompile(`(?i)(\b(?:password|passwd|secret|api[_-]?key|access[_-]?token|client_secret|refresh_token|auth_token|id_token)\s*[:=]\s*)\S+`)
	reCookie     = regexp.MustCompile(`(?i)(\b(?:grafana_session|grafana_session_expiry)=)[^;\s]+`)
)

func isSensitiveKey(key string) bool {
	_, ok := sensitiveKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

func maybeContainsSecret(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "bearer") ||
		strings.Contains(lower, "basic ") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "api-key") ||
		strings.Contains(lower, "apikey") ||
		strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "access_token") ||
		strings.Contains(lower, "auth_token") ||
		strings.Contains(lower, "grafana_session") ||
		strings.Contains(lower, "client_secret") ||
		strings.Contains(lower, "refresh_token")
}

// RedactSecrets replaces credential-like substrings in s.
func RedactSecrets(s string) string {
	if s == "" || !maybeContainsSecret(s) {
		return s
	}
	s = reAuthScheme.ReplaceAllString(s, "${1}"+Redacted)
	s = reAssign.ReplaceAllString(s, "${1}"+Redacted)
	s = reCookie.ReplaceAllString(s, "${1}"+Redacted)
	return s
}

// RedactLogKeyvals copies keyvals and redacts sensitive keys and credential-like values.
// User-controlled strings that are not secrets (dashboard titles, query text) are left intact
// so they stay fields, not format-string sinks.
func RedactLogKeyvals(keyvals []any) []any {
	if len(keyvals) == 0 {
		return keyvals
	}
	out := make([]any, len(keyvals))
	copy(out, keyvals)
	for i := 0; i < len(out); i++ {
		if key, ok := out[i].(string); ok && i+1 < len(out) && isSensitiveKey(key) {
			out[i+1] = Redacted
			i++
			continue
		}
		out[i] = redactValue(out[i])
	}
	return out
}

func redactValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return RedactSecrets(x)
	case []byte:
		return []byte(RedactSecrets(string(x)))
	case error:
		if x == nil {
			return x
		}
		redactedErr := RedactSecrets(x.Error())
		if redactedErr == x.Error() {
			return x
		}
		return fmt.Errorf("%s", redactedErr)
	case slog.Value:
		if x.Kind() == slog.KindString {
			s := RedactSecrets(x.String())
			if s != x.String() {
				return slog.StringValue(s)
			}
		}
		return x
	case map[string]string:
		return redactStringMap(x)
	case map[string][]string:
		return redactStringSliceMap(x)
	case http.Header:
		return http.Header(redactStringSliceMap(x))
	case map[string]any:
		return redactAnyMap(x)
	default:
		return v
	}
}

func redactStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	changed := false
	for k, v := range in {
		if isSensitiveKey(k) {
			out[k] = Redacted
			if v != Redacted {
				changed = true
			}
			continue
		}
		next := RedactSecrets(v)
		out[k] = next
		if next != v {
			changed = true
		}
	}
	if !changed {
		return in
	}
	return out
}

func redactStringSliceMap(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	changed := false
	for k, vals := range in {
		if isSensitiveKey(k) {
			out[k] = []string{Redacted}
			changed = true
			continue
		}
		next := make([]string, len(vals))
		for i, v := range vals {
			next[i] = RedactSecrets(v)
			if next[i] != v {
				changed = true
			}
		}
		out[k] = next
	}
	if !changed {
		return in
	}
	return out
}

func redactAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	changed := false
	for k, v := range in {
		if isSensitiveKey(k) {
			out[k] = Redacted
			if v != Redacted {
				changed = true
			}
			continue
		}
		next := redactValue(v)
		out[k] = next
		if next != v {
			changed = true
		}
	}
	if !changed {
		return in
	}
	return out
}
