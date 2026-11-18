package logger

import (
	"context"
	"log/slog"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/openfga/openfga/pkg/logger"
)

var _ logger.Logger = (*ZanzanaLogger)(nil)

// ZanzanaLogger is a grafana logger wrapper compatible with OpenFGA logger interface
type ZanzanaLogger struct {
	logger log.Logger
	// handler writes straight to the slog pipeline while the slog core is active. It is
	// nil when logger does not write to the pipeline.
	handler slog.Handler
}

func New(logger log.Logger) *ZanzanaLogger {
	return &ZanzanaLogger{
		logger:  logger,
		handler: log.SlogHandler(logger),
	}
}

// Simple converter for zap logger fields
func zapFieldsToArgs(fields []zap.Field) []any {
	// We need to pre-allocated space for key and value
	args := make([]any, 0, len(fields)*2)
	for _, f := range fields {
		args = append(args, f.Key)
		if f.Interface != nil {
			args = append(args, f.Interface)
		} else if f.String != "" {
			args = append(args, f.String)
		} else {
			args = append(args, f.Integer)
		}
	}
	return args
}

// zapFieldsToAttrs lets zap encode each field, so typed fields such as durations, bools
// and errors keep their value instead of zap's internal representation.
func zapFieldsToAttrs(fields []zap.Field) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(fields))
	for _, f := range fields {
		enc := zapcore.NewMapObjectEncoder()
		f.AddTo(enc)
		for k, v := range enc.Fields {
			attrs = append(attrs, slog.Any(k, v))
		}
	}
	return attrs
}

// write sends the record to the slog pipeline and reports whether it did so. When it
// returns false the caller logs through the go-kit engine instead.
func (l *ZanzanaLogger) write(ctx context.Context, level slog.Level, msg string, fields []zap.Field) bool {
	if l.handler == nil || !log.SlogCoreActive() {
		return false
	}
	if !l.handler.Enabled(ctx, level) {
		return true
	}
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.AddAttrs(zapFieldsToAttrs(fields)...)
	_ = l.handler.Handle(ctx, r)
	return true
}

// With implements logger.Logger.
func (l *ZanzanaLogger) With(fields ...zapcore.Field) logger.Logger {
	if l.handler != nil && log.SlogCoreActive() {
		return &ZanzanaLogger{
			logger:  l.logger,
			handler: l.handler.WithAttrs(zapFieldsToAttrs(fields)),
		}
	}
	return &ZanzanaLogger{
		logger: l.logger.New(zapFieldsToArgs(fields)...),
	}
}

func (l *ZanzanaLogger) Debug(msg string, fields ...zap.Field) {
	l.DebugWithContext(context.Background(), msg, fields...)
}

func (l *ZanzanaLogger) Info(msg string, fields ...zap.Field) {
	l.InfoWithContext(context.Background(), msg, fields...)
}

func (l *ZanzanaLogger) Warn(msg string, fields ...zap.Field) {
	l.WarnWithContext(context.Background(), msg, fields...)
}

func (l *ZanzanaLogger) Error(msg string, fields ...zap.Field) {
	l.ErrorWithContext(context.Background(), msg, fields...)
}

func (l *ZanzanaLogger) Panic(msg string, fields ...zap.Field) {
	l.ErrorWithContext(context.Background(), msg, fields...)
}

func (l *ZanzanaLogger) Fatal(msg string, fields ...zap.Field) {
	l.ErrorWithContext(context.Background(), msg, fields...)
}

func (l *ZanzanaLogger) DebugWithContext(ctx context.Context, msg string, fields ...zap.Field) {
	if !l.write(ctx, slog.LevelDebug, msg, fields) {
		l.logger.Debug(msg, zapFieldsToArgs(fields)...)
	}
}

func (l *ZanzanaLogger) InfoWithContext(ctx context.Context, msg string, fields ...zap.Field) {
	if !l.write(ctx, slog.LevelInfo, msg, fields) {
		l.logger.Info(msg, zapFieldsToArgs(fields)...)
	}
}

func (l *ZanzanaLogger) WarnWithContext(ctx context.Context, msg string, fields ...zap.Field) {
	if !l.write(ctx, slog.LevelWarn, msg, fields) {
		l.logger.Warn(msg, zapFieldsToArgs(fields)...)
	}
}

func (l *ZanzanaLogger) ErrorWithContext(ctx context.Context, msg string, fields ...zap.Field) {
	if !l.write(ctx, slog.LevelError, msg, fields) {
		l.logger.Error(msg, zapFieldsToArgs(fields)...)
	}
}

func (l *ZanzanaLogger) PanicWithContext(ctx context.Context, msg string, fields ...zap.Field) {
	l.ErrorWithContext(ctx, msg, fields...)
}

func (l *ZanzanaLogger) FatalWithContext(ctx context.Context, msg string, fields ...zap.Field) {
	l.ErrorWithContext(ctx, msg, fields...)
}
