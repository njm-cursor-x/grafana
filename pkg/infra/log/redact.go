package log

import "github.com/grafana/grafana/pkg/infra/log/secretredact"

// Redacted is the placeholder written in place of secret material.
const Redacted = secretredact.Redacted

// RedactSecrets replaces credential-like substrings in s.
func RedactSecrets(s string) string {
	return secretredact.RedactSecrets(s)
}

// Redact is the Backend alias of RedactSecrets.
func Redact(s string) string {
	return RedactSecrets(s)
}

// RedactLogKeyvals copies keyvals and redacts sensitive keys and credential-like values.
func RedactLogKeyvals(keyvals []any) []any {
	return secretredact.RedactLogKeyvals(keyvals)
}

// RedactFields is the Backend alias of RedactLogKeyvals.
func RedactFields(keyvals []any) []any {
	return RedactLogKeyvals(keyvals)
}

// RedactValue redacts val when key is sensitive, and scrubs secret patterns from the value.
func RedactValue(key string, val any) any {
	return secretredact.RedactValue(key, val)
}
