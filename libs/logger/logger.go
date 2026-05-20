package logger

import (
	"context"
	"os"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.SugaredLogger
	base *zap.Logger

	// exiter is the function called by FatalSync to terminate the process.
	// Defaults to defaultExiter (os.Exit) in New(); same-package tests
	// override via setExiter to assert exit behavior without killing the
	// test process. Unexported so production callers cannot disable
	// process termination by accident.
	exiter func(int)

	// syncObserver, when non-nil, replaces the Sync() call inside
	// FatalSync. Same-package tests install one to assert "Sync was
	// called before exiter" with deterministic ordering; production
	// code path uses l.Sync() directly when this is nil.
	syncObserver func() error
}

type Config struct {
	Level       string `json:"level" yaml:"level"`
	Development bool   `json:"development" yaml:"development"`
	Encoding    string `json:"encoding" yaml:"encoding"` // json or console
}

func New(cfg Config) (*Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	var zapCfg zap.Config
	if cfg.Development {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		zapCfg = zap.NewProductionConfig()
	}

	zapCfg.Level = zap.NewAtomicLevelAt(level)
	if cfg.Encoding != "" {
		zapCfg.Encoding = cfg.Encoding
	}

	base, err := zapCfg.Build(
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	return &Logger{
		SugaredLogger: base.Sugar(),
		base:          base,
		exiter:        defaultExiter,
	}, nil
}

func Default() *Logger {
	cfg := Config{
		Level:       os.Getenv("LOG_LEVEL"),
		Development: os.Getenv("ENV") != "production",
		Encoding:    "console",
	}
	if cfg.Level == "" {
		cfg.Level = "info"
	}

	l, _ := New(cfg)
	return l
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return l
	}

	return &Logger{
		SugaredLogger: l.base.Sugar().With(
			zap.String("trace_id", span.SpanContext().TraceID().String()),
			zap.String("span_id", span.SpanContext().SpanID().String()),
		),
		base:         l.base,
		exiter:       l.exiter,
		syncObserver: l.syncObserver,
	}
}

func (l *Logger) With(args ...interface{}) *Logger {
	return &Logger{
		SugaredLogger: l.SugaredLogger.With(args...),
		base:          l.base,
		exiter:        l.exiter,
		syncObserver:  l.syncObserver,
	}
}

func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	args := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return l.With(args...)
}

func (l *Logger) Sync() error {
	return l.base.Sync()
}

type contextKey struct{}

func ToContext(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

func FromContext(ctx context.Context) *Logger {
	if l, ok := ctx.Value(contextKey{}).(*Logger); ok {
		return l
	}
	return Default()
}
