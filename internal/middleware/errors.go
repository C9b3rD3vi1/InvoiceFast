package middleware

import (
	"context"
	"fmt"
	"runtime/debug"

	"invoicefast/internal/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ErrorCode definitions for consistent error responses
const (
	ErrCodeInternalError     = "INTERNAL_ERROR"
	ErrCodeNotFound          = "NOT_FOUND"
	ErrCodeValidationError  = "VALIDATION_ERROR"
	ErrCodeUnauthorized      = "UNAUTHORIZED"
	ErrCodeForbidden         = "FORBIDDEN"
	ErrCodeRateLimitExceeded = "RATE_LIMIT_EXCEEDED"
	ErrCodeInvalidToken      = "INVALID_TOKEN"
	ErrCodeConflict          = "CONFLICT"
)

// AppError represents a structured application error
type AppError struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"request_id"`
}

// ErrorHandler creates a secure error handling middleware
func ErrorHandler(log *logger.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		requestID := c.Locals("request_id")
		if requestID == nil {
			requestID = uuid.New().String()
		}

		// Log the error with stack trace for debugging
		log.Error(c.UserContext(), "request error",
			"error", err.Error(),
			"request_id", requestID,
			"path", c.Path(),
			"method", c.Method(),
		)

		// Check if it's a fiber error (HTTP error)
		if e, ok := err.(*fiber.Error); ok {
			return handleFiberError(c, e, requestID.(string))
		}

		// Check if it's an app error
		if appErr, ok := err.(*AppError); ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":     appErr.Message,
				"code":      appErr.Code,
				"request_id": appErr.RequestID,
			})
		}

		// Handle panic recovery
		debug.PrintStack()

		// Return generic error for unknown issues (don't leak internals)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":       "An unexpected error occurred",
			"code":        ErrCodeInternalError,
			"request_id":  requestID.(string),
			"help":        "Please contact support if this persists",
		})
	}
}

func handleFiberError(c *fiber.Ctx, e *fiber.Error, requestID string) error {
	code := e.Code

	// Map fiber error codes to our codes
	appErrCode := ErrCodeInternalError
	message := e.Message

	switch code {
	case fiber.StatusNotFound:
		appErrCode = ErrCodeNotFound
		message = "Resource not found"
	case fiber.StatusUnauthorized:
		appErrCode = ErrCodeUnauthorized
		message = "Authentication required"
	case fiber.StatusForbidden:
		appErrCode = ErrCodeForbidden
		message = "Access denied"
	case fiber.StatusBadRequest:
		appErrCode = ErrCodeValidationError
	case fiber.StatusTooManyRequests:
		appErrCode = ErrCodeRateLimitExceeded
		message = "Too many requests, please try again later"
	case fiber.StatusConflict:
		appErrCode = ErrCodeConflict
	}

	// Don't expose internal error details
	if code >= 500 {
		message = "An internal error occurred"
	}

	return c.Status(code).JSON(fiber.Map{
		"error":      message,
		"code":       appErrCode,
		"request_id": requestID,
	})
}

// NewAppError creates a new application error
func NewAppError(code, message string, details interface{}) *AppError {
	return &AppError{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: "",
	}
}

// WithRequestID adds request ID to error
func (e *AppError) WithRequestID(id string) *AppError {
	e.RequestID = id
	return e
}

// Error implements error interface
func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// WrapError wraps an error with additional context
func WrapError(err error, code, userMessage string) *AppError {
	return &AppError{
		Code:      code,
		Message:   userMessage,
		Details:   err.Error(), // Log actual error internally
		RequestID: "",
	}
}

// RequestIDMiddleware adds request ID to all requests
func RequestIDMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Locals("request_id", requestID)
		c.Set("X-Request-ID", requestID)

		// Create context with request ID for logging
		ctx := context.WithValue(c.UserContext(), "request_id", requestID)
		c.Locals("ctx", ctx)

		return c.Next()
	}
}

// GetRequestID retrieves request ID from context
func GetRequestID(c *fiber.Ctx) string {
	if id := c.Locals("request_id"); id != nil {
		return id.(string)
	}
	return ""
}

// RequireRequestID checks if request has a request ID
func RequireRequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := c.Get("X-Request-Id")
		if requestID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "X-Request-ID header required for idempotency",
				"code":  ErrCodeValidationError,
			})
		}

		// Validate UUID format
		if _, err := uuid.Parse(requestID); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "X-Request-ID must be a valid UUID",
				"code":  ErrCodeValidationError,
			})
		}

		return c.Next()
	}
}

// SecurityHeadersMiddleware adds security headers
func SecurityHeadersMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// CSP for API responses (more lenient for SPA)
		if c.Path() == "/api/v1/health" || c.Method() == "GET" {
			c.Set("Cache-Control", "no-store, private")
		}

		return c.Next()
	}
}

// SensitiveDataFilter filters sensitive data from logs
type SensitiveDataFilter struct {
	Fields []string
}

func NewSensitiveDataFilter() *SensitiveDataFilter {
	return &SensitiveDataFilter{
		Fields: []string{
			"password",
			"password_hash",
			"secret",
			"token",
			"api_key",
			"credit_card",
			"cvv",
			"pin",
		},
	}
}

// Filter removes sensitive fields from a map
func (f *SensitiveDataFilter) Filter(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range data {
		isSensitive := false
		for _, field := range f.Fields {
			if contains(k, field) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			result[k] = "[REDACTED]"
		} else {
			result[k] = v
		}
	}
	return result
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || 
		 len(s) > len(substr) && 
		  (s[:len(substr)] == substr || 
		   s[len(s)-len(substr):] == substr || 
		   containsAny(s, substr)))
}

func containsAny(s string, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}