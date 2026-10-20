package log

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	gokitlog "github.com/go-kit/log"
)

const RedactedValue = "[REDACTED]"

// Matched against the key after lowercasing and stripping non-alphanumerics,
// so "api_key", "X-Api-Key" and "apiKey" all normalize to a match.
var sensitiveKeySuffixes = []string{
	"password",
	"passwd",
	"secret",
	"token",
	"apikey",
	"authorization",
	"cookie",
	"credentials",
	"privatekey",
}

var (
	bearerPattern     = regexp.MustCompile(`(?i)\b(bearer)\s+([A-Za-z0-9\-._~+/]+=*)`)
	basicPattern      = regexp.MustCompile(`(?i)\b(basic)\s+([A-Za-z0-9+/]+={0,2})`)
	grafanaTokenRegex = regexp.MustCompile(`\b(glsa|glc|eyJrIjoi)[A-Za-z0-9_+/=\-]{8,}`)
	urlPasswordRegex  = regexp.MustCompile(`(://[^/\s:@]*:)[^/\s@]+@`)
)

// IsSensitiveKey reports whether a log key or header name names a credential.
func IsSensitiveKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	for _, suffix := range sensitiveKeySuffixes {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

// RedactString masks credentials embedded in free text: Authorization header
// values, Grafana API/service account tokens, and passwords in URLs.
func RedactString(s string) string {
	s = bearerPattern.ReplaceAllStringFunc(s, func(m string) string {
		parts := bearerPattern.FindStringSubmatch(m)
		// Require a digit and some length so prose like "bearer authentication" survives.
		if len(parts[2]) < 16 || !strings.ContainsAny(parts[2], "0123456789") {
			return m
		}
		return parts[1] + " " + RedactedValue
	})
	s = basicPattern.ReplaceAllStringFunc(s, func(m string) string {
		parts := basicPattern.FindStringSubmatch(m)
		decoded, err := base64.StdEncoding.DecodeString(parts[2])
		if err != nil || !strings.Contains(string(decoded), ":") {
			return m
		}
		return parts[1] + " " + RedactedValue
	})
	s = grafanaTokenRegex.ReplaceAllString(s, RedactedValue)
	s = urlPasswordRegex.ReplaceAllString(s, "${1}"+RedactedValue+"@")
	return s
}

type redactedError struct{ msg string }

func (e redactedError) Error() string { return e.msg }

func redactValue(v any) (any, bool) {
	switch val := v.(type) {
	case string:
		if r := RedactString(val); r != val {
			return r, true
		}
	case error:
		msg := val.Error()
		if r := RedactString(msg); r != msg {
			return redactedError{msg: r}, true
		}
	case http.Header:
		var out http.Header
		for name := range val {
			if IsSensitiveKey(name) {
				if out == nil {
					out = val.Clone()
				}
				out[name] = []string{RedactedValue}
			}
		}
		if out != nil {
			return out, true
		}
	case *url.URL:
		if val != nil {
			if _, hasPassword := val.User.Password(); hasPassword {
				return val.Redacted(), true
			}
		}
	case url.URL:
		if _, hasPassword := val.User.Password(); hasPassword {
			return val.Redacted(), true
		}
	}
	return v, false
}

// RedactKeyvals returns keyvals with credential values masked. The input slice
// is never mutated; a copy is made only when something needs redacting.
func RedactKeyvals(keyvals []any) []any {
	var out []any
	for i := 1; i < len(keyvals); i += 2 {
		var (
			newVal  any
			changed bool
		)
		if key, ok := keyvals[i-1].(string); ok && IsSensitiveKey(key) {
			newVal, changed = RedactedValue, true
		} else {
			newVal, changed = redactValue(keyvals[i])
		}
		if !changed {
			continue
		}
		if out == nil {
			out = make([]any, len(keyvals))
			copy(out, keyvals)
		}
		out[i] = newVal
	}
	if out == nil {
		return keyvals
	}
	return out
}

type redactingLogger struct {
	next gokitlog.Logger
}

func newRedactingLogger(next gokitlog.Logger) gokitlog.Logger {
	return &redactingLogger{next: next}
}

func (l *redactingLogger) Log(keyvals ...any) error {
	return l.next.Log(RedactKeyvals(keyvals)...)
}
