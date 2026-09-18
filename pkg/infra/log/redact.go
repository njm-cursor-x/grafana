package log

import "github.com/grafana/grafana/pkg/infra/log/secretredact"

// Redacted is the placeholder written in place of secret material.
const Redacted = secretredact.Redacted

// RedactSecrets replaces credential-like substrings in s.
func RedactSecrets(s string) string {
	return secretredact.RedactSecrets(s)
}

// RedactLogKeyvals copies keyvals and redacts sensitive keys and credential-like values.
func RedactLogKeyvals(keyvals []any) []any {
	return secretredact.RedactLogKeyvals(keyvals)
}
