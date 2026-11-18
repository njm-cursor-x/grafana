package log

import (
	"context"
	"io"
	"testing"
)

// Baselines for the slog swap (plan §7): disabled debug, info with 5 attrs,
// FromContext().Info, 3-sink fan-out, and a typical access-log line.
// Target after the swap: 0 allocs for disabled levels and no median
// regression for enabled logs. CI should run these as a non-blocking report.
func BenchmarkLog(b *testing.B) {
	for _, format := range []string{formatText, formatJSON} {
		b.Run(format, func(b *testing.B) {
			b.Run("disabled_debug", func(b *testing.B) {
				installCurrentEngine(b, io.Discard, engineOpts{format: format, level: "info"})
				l := New("bench")
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					l.Debug("disabled", "a", 1, "b", 2, "c", 3, "d", 4, "e", 5)
				}
			})

			b.Run("info_five_attrs", func(b *testing.B) {
				installCurrentEngine(b, io.Discard, engineOpts{format: format, level: "info"})
				l := New("bench")
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					l.Info("hello", "a", 1, "b", 2, "c", 3, "d", 4, "e", 5)
				}
			})

			b.Run("from_context_info", func(b *testing.B) {
				installCurrentEngine(b, io.Discard, engineOpts{format: format, level: "info"})
				l := New("bench")
				ctx := WithContextualAttributes(context.Background(), []any{
					"traceID", "trace-abc",
					"orgId", int64(1),
					"userId", int64(42),
				})
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					l.FromContext(ctx).Info("hello", "k", 1)
				}
			})

			b.Run("fanout_three_sinks", func(b *testing.B) {
				installCurrentEngine(b, io.Discard, engineOpts{format: format, level: "info", sinkCount: 3})
				l := New("bench")
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					l.Info("hello", "a", 1, "b", 2, "c", 3, "d", 4, "e", 5)
				}
			})

			b.Run("access_log_line", func(b *testing.B) {
				installCurrentEngine(b, io.Discard, engineOpts{format: format, level: "info"})
				l := New("http.server")
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					l.Info("Request Completed",
						"method", "GET",
						"path", "/api/dashboards/uid/abc",
						"status", 200,
						"remote_addr", "127.0.0.1",
						"time_ms", int64(12),
						"duration", "12ms",
						"size", 345,
						"referer", "",
						"handler", "grafana-apiserver",
						"status_source", "server",
					)
				}
			})
		})
	}
}
