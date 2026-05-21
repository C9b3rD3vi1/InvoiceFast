package middleware

import (
	"strconv"
	"strings"
	"time"

	"invoicefast/internal/logger"

	"github.com/gofiber/fiber/v2"
)

// sensitiveFields are field names whose values should be redacted in logs.
// This is used by SanitizeLogArgs to prevent sensitive data leakage.
var sensitiveFields = []string{
	"password", "password_hash", "secret", "token", "api_key",
	"api_secret", "credit_card", "cvv", "pin", "ssn", "tin",
	"private_key", "access_token", "refresh_token", "authorization",
}

// isSensitiveField checks if a field name matches any sensitive pattern.
func isSensitiveField(field string) bool {
	lower := strings.ToLower(field)
	for _, sf := range sensitiveFields {
		if strings.Contains(lower, sf) {
			return true
		}
	}
	return false
}

// SanitizeLogArgs filters sensitive data from key-value log arguments.
// It accepts a slice of alternating key-value pairs (like slog args)
// and redacts values whose keys match sensitive field names.
// Usage: logger.Info(ctx, "msg", SanitizeLogArgs("key1", val1, "key2", val2)...)
func SanitizeLogArgs(args ...any) []any {
	result := make([]any, len(args))
	for i := 0; i < len(args)-1; i += 2 {
		result[i] = args[i]
		if key, ok := args[i].(string); ok && isSensitiveField(key) {
			result[i+1] = "[REDACTED]"
		} else if i+1 < len(args) {
			result[i+1] = args[i+1]
		}
	}
	// Handle odd trailing arg (shouldn't happen but be safe)
	if len(args)%2 != 0 && len(args) > 0 {
		result[len(args)-1] = args[len(args)-1]
	}
	return result
}

func LoggingMiddleware(log *logger.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		tenantID := GetTenantID(c)
		path := c.Path()
		method := c.Method()

		err := c.Next()

		latency := time.Since(start)
		status := c.Response().StatusCode()
		ip := c.IP()

		logArgs := []any{
			"method", method,
			"path", path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"ip_address", ip,
		}

		if tenantID != "" {
			logArgs = append(logArgs, "tenant_id", tenantID)
		}

		traceID := c.Get("X-Trace-ID")
		if traceID != "" {
			logArgs = append(logArgs, "trace_id", traceID)
		}

		if status >= 500 {
			log.Error(c.Context(), "Server Error", logArgs...)
		} else if status >= 400 {
			log.Warn(c.Context(), "Client Error", logArgs...)
		} else {
			log.Info(c.Context(), "Request", logArgs...)
		}

		return err
	}
}

type ResponseData struct {
	Status  int    `json:"status"`
	Message string `json:"message,omitempty"`
}

func SecurityLoggingMiddleware(log *logger.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		tenantID := GetTenantID(c)

		err := c.Next()

		if c.Response().StatusCode() == 401 || c.Response().StatusCode() == 403 {
			log.Warn(c.Context(), "Security: Access denied",
				"ip_address", ip,
				"tenant_id", tenantID,
				"path", c.Path(),
				"method", c.Method(),
			)
		}

		return err
	}
}

func TenantContextLogger(log *logger.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := GetTenantID(c)
		userID := GetUserID(c)

		if tenantID != "" {
			c.Locals("logger", log.WithTenant(tenantID))
		}

		_ = userID

		return c.Next()
	}
}

func GetLogger(c *fiber.Ctx) *logger.Logger {
	if l, ok := c.Locals("logger").(*logger.Logger); ok {
		return l
	}
	return logger.Get()
}

func LogError(c *fiber.Ctx, err error, msg string) {
	log := GetLogger(c)
	tenantID := GetTenantID(c)

	logArgs := []any{"error", err.Error()}
	if tenantID != "" {
		logArgs = append(logArgs, "tenant_id", tenantID)
	}

	log.Error(c.Context(), msg, logArgs...)
}

func LogPaymentEvent(c *fiber.Ctx, event, paymentID, merchantReqID string, args ...any) {
	log := GetLogger(c)
	tenantID := GetTenantID(c)

	logArgs := []any{
		"event", event,
		"payment_id", paymentID,
		"merchant_request_id", merchantReqID,
	}
	if tenantID != "" {
		logArgs = append(logArgs, "tenant_id", tenantID)
	}
	logArgs = append(logArgs, SanitizeLogArgs(args...)...)

	log.Info(c.Context(), "Payment: "+event, logArgs...)
}

func LogSecurityEvent(c *fiber.Ctx, event string, args ...any) {
	log := logger.Get()

	logArgs := []any{
		"ip_address", c.IP(),
		"path", c.Path(),
		"method", c.Method(),
	}
	logArgs = append(logArgs, SanitizeLogArgs(args...)...)

	log.Warn(c.Context(), "Security: "+event, logArgs...)
}

func ParseUint(s string) uint {
	i, _ := strconv.ParseUint(s, 10, 64)
	return uint(i)
}
