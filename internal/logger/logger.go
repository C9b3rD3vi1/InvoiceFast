package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"

	"invoicefast/internal/config"
)

type Level = slog.Level

const (
	DebugLevel = slog.LevelDebug
	InfoLevel  = slog.LevelInfo
	WarnLevel  = slog.LevelWarn
	ErrorLevel = slog.LevelError
)

var (
	globalLogger *Logger
	once         sync.Once
)

type Logger struct {
	logger *slog.Logger
	cfg    *Config
}

type Config struct {
	Level     Level
	Format    string
	Output    io.Writer
	Service   string
	Version   string
	AddSource bool
}

func DefaultConfig() *Config {
	return &Config{
		Level:     InfoLevel,
		Format:    "text",
		Output:    os.Stdout,
		Service:   "invoicefast",
		Version:   "1.0.0",
		AddSource: true,
	}
}

func Init(cfg *Config) error {
	var initErr error
	once.Do(func() {
		globalLogger = &Logger{cfg: cfg}
		globalLogger.logger = buildLogger(cfg)
	})
	return initErr
}

func buildLogger(cfg *Config) *slog.Logger {
	var handler slog.Handler
	handlerOpts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.AddSource,
	}

	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(cfg.Output, handlerOpts)
	} else {
		handler = slog.NewTextHandler(cfg.Output, handlerOpts)
	}

	return slog.New(handler)
}

func Get() *Logger {
	if globalLogger == nil {
		Init(DefaultConfig())
	}
	return globalLogger
}

func (l *Logger) Debug(ctx context.Context, msg string, args ...any) {
	l.logger.DebugContext(ctx, msg, args...)
}

func (l *Logger) Info(ctx context.Context, msg string, args ...any) {
	l.logger.InfoContext(ctx, msg, args...)
}

func (l *Logger) Warn(ctx context.Context, msg string, args ...any) {
	l.logger.WarnContext(ctx, msg, args...)
}

func (l *Logger) Error(ctx context.Context, msg string, args ...any) {
	l.logger.ErrorContext(ctx, msg, args...)
}

func (l *Logger) Fatal(ctx context.Context, msg string, args ...any) {
	l.logger.ErrorContext(ctx, msg, args...)
	os.Exit(1)
}

func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		logger: l.logger.With(args...),
		cfg:    l.cfg,
	}
}

func (l *Logger) WithTenant(tenantID string) *Logger {
	if tenantID == "" {
		return l
	}
	return l.With("tenant_id", tenantID)
}

func (l *Logger) WithPayment(paymentID, merchantReqID string) *Logger {
	return l.With("payment_id", paymentID, "merchant_request_id", merchantReqID)
}

func (l *Logger) Log(level Level, ctx context.Context, msg string, args ...any) {
	l.logger.Log(ctx, level, msg, args...)
}

func (l *Logger) SetLevel(level Level) {
	if globalLogger != nil {
		globalLogger.cfg.Level = level
	}
}

func FromContext(ctx context.Context) *Logger {
	return Get()
}

func NewWithConfig(cfg *Config) *Logger {
	return &Logger{
		logger: buildLogger(cfg),
		cfg:    cfg,
	}
}

func LoadFromConfig(cfg *config.Config) *Logger {
	level := InfoLevel
	if cfg.Server.Mode == "development" {
		level = DebugLevel
	}

	logCfg := &Config{
		Level:     level,
		Format:    "json",
		Output:    os.Stdout,
		Service:   "invoicefast",
		Version:   "1.0.0",
		AddSource: cfg.Server.Mode == "development",
	}

	if cfg.Server.Mode == "development" {
		logCfg.Format = "text"
	}

	logger := NewWithConfig(logCfg)
	globalLogger = logger
	return logger
}

const TraceIDKey = "trace_id"

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

func GetTraceID(ctx context.Context) string {
	if v := ctx.Value(TraceIDKey); v != nil {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, "tenant_id_ctx", tenantID)
}

func GetTenantIDFromCtx(ctx context.Context) string {
	if v := ctx.Value("tenant_id_ctx"); v != nil {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

func MaskKRA_PIN(pin string) string {
	if len(pin) <= 4 {
		return "****"
	}
	return "****" + pin[len(pin)-4:]
}

func MaskPhone(phone string) string {
	if len(phone) <= 4 {
		return "****"
	}
	return "****" + phone[len(phone)-4:]
}

func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "****"
	}
	username := parts[0]
	if len(username) <= 2 {
		return "**@" + parts[1]
	}
	return string(username[0]) + strings.Repeat("*", len(username)-2) + string(username[len(username)-1]) + "@" + parts[1]
}

func GetStackTrace() string {
	pc := make([]uintptr, 20)
	n := runtime.Callers(2, pc)
	frames := runtime.CallersFrames(pc[:n])

	var builder strings.Builder
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.File, "invoicefast") {
			builder.WriteString(fmt.Sprintf("%s:%d\n", frame.File, frame.Line))
		}
		if !more {
			break
		}
	}
	return builder.String()
}

type ErrorWithStack struct {
	Err error
	Msg string
}

func (e *ErrorWithStack) Error() string {
	return fmt.Sprintf("%s: %v", e.Msg, e.Err)
}

func (e *ErrorWithStack) Unwrap() error {
	return e.Err
}

func (e *ErrorWithStack) StackTrace() string {
	return GetStackTrace()
}

func WrapError(err error, msg string) *ErrorWithStack {
	return &ErrorWithStack{Err: err, Msg: msg}
}
