package middleware

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"

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

		code := fiber.StatusInternalServerError
		message := "An unexpected error occurred"

		// Check if it's a fiber error (HTTP error)
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			message = mapFiberError(e)
		}

		// Check if it's an app error
		if appErr, ok := err.(*AppError); ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":      appErr.Message,
				"code":       appErr.Code,
				"request_id": appErr.RequestID,
			})
		}

		// Handle panic recovery
		debug.PrintStack()

		// Don't leak internal details for 5xx
		if code >= 500 {
			message = "An unexpected error occurred"
		}

		// JSON error for API routes
		if strings.HasPrefix(c.Path(), "/api/") {
			return c.Status(code).JSON(fiber.Map{
				"error":      message,
				"code":       errorCodeForStatus(code),
				"request_id": requestID,
			})
		}

		// HTML error for web routes
		return c.Status(code).Render("error", fiber.Map{
			"Status": code,
			"Error":  message,
		})
	}
}

func errorCodeForStatus(code int) string {
	switch code {
	case fiber.StatusNotFound:
		return ErrCodeNotFound
	case fiber.StatusUnauthorized:
		return ErrCodeUnauthorized
	case fiber.StatusForbidden:
		return ErrCodeForbidden
	case fiber.StatusBadRequest:
		return ErrCodeValidationError
	case fiber.StatusTooManyRequests:
		return ErrCodeRateLimitExceeded
	case fiber.StatusConflict:
		return ErrCodeConflict
	default:
		return ErrCodeInternalError
	}
}

func mapFiberError(e *fiber.Error) string {
	switch e.Code {
	case fiber.StatusNotFound:
		return "Resource not found"
	case fiber.StatusUnauthorized:
		return "Authentication required"
	case fiber.StatusForbidden:
		return "Access denied"
	case fiber.StatusBadRequest:
		return e.Message
	case fiber.StatusTooManyRequests:
		return "Too many requests, please try again later"
	case fiber.StatusConflict:
		return e.Message
	default:
		return "An unexpected error occurred"
	}
}

// Error implements error interface
func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
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
func SecurityHeadersMiddleware(mode string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// CSP for API responses (more lenient for SPA)
		c.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://js.stripe.com; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; img-src 'self' data: blob:; font-src 'self' https://fonts.gstatic.com; frame-src https://js.stripe.com; connect-src 'self'")
		if mode == "production" {
			c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if c.Path() == "/api/v1/health" || c.Method() == "GET" {
			c.Set("Cache-Control", "no-store, private")
		}

		return c.Next()
	}
}