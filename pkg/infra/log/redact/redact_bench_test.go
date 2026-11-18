package redact

import (
	"io"
	"log/slog"
	"testing"
)

func BenchmarkHandler(b *testing.B) {
	passthrough := slog.NewJSONHandler(io.Discard, nil)
	redacting := New(slog.NewJSONHandler(io.Discard, nil))

	b.Run("passthrough", func(b *testing.B) {
		logger := slog.New(passthrough)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			logger.Info("request", "user", "n", "org", 1, "path", "/api", "status", 200)
		}
	})

	b.Run("no_secrets", func(b *testing.B) {
		logger := slog.New(redacting)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			logger.Info("request", "user", "n", "org", 1, "path", "/api", "status", 200)
		}
	})

	b.Run("with_secret", func(b *testing.B) {
		logger := slog.New(redacting)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			logger.Info("login", "user", "n", "password", "hunter2", "token", "abc")
		}
	})

	b.Run("disabled_debug", func(b *testing.B) {
		h := New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger := slog.New(h)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			logger.Debug("skip", "password", "hunter2", "user", "n")
		}
	})
}
