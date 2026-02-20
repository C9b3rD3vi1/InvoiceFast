package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// Error codes - business logic errors (4xx)
	ErrCodeBadRequest       = "BAD_REQUEST"
	ErrCodeUnauthorized     = "UNAUTHORIZED"
	ErrCodeForbidden        = "FORBIDDEN"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeConflict         = "CONFLICT"
	ErrCodeRateLimited      = "RATE_LIMITED"
	ErrCodeValidationFailed = "VALIDATION_FAILED"

	// Server errors (5xx)
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	ErrCodeDatabaseError      = "DATABASE_ERROR"
	ErrCodeExternalAPIError   = "EXTERNAL_API_ERROR"

	// Success codes
	ErrCodeCreated = "CREATED"
	ErrCodeUpdated = "UPDATED"
	ErrCodeDeleted = "DELETED"
)

// ErrorResponse is the standard error response format
type ErrorResponse struct {
	Success bool        `json:"success"`
	Error   ErrorDetail `json:"error"`
}

// ErrorDetail contains error information
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Details   any    `json:"details,omitempty"`
}

// SuccessResponse is the standard success response format
type SuccessResponse struct {
	Success bool  `json:"success"`
	Data    any   `json:"data,omitempty"`
	Meta    *Meta `json:"meta,omitempty"`
}

// Meta contains pagination and other metadata
type Meta struct {
	Total      int64  `json:"total,omitempty"`
	Page       int    `json:"page,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// RequestIDKey is the key for request ID in context
const RequestIDKey = "request_id"

// NewErrorResponse creates a new error response
func NewErrorResponse(code, message string) ErrorResponse {
	return ErrorResponse{
		Success: false,
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
}

// WithRequestID adds request ID to error response
func (e ErrorResponse) WithRequestID(reqID string) ErrorResponse {
	e.Error.RequestID = reqID
	return e
}

// WithDetails adds details to error response
func (e ErrorResponse) WithDetails(details any) ErrorResponse {
	e.Error.Details = details
	return e
}

// NewSuccessResponse creates a new success response
func NewSuccessResponse(data any) SuccessResponse {
	return SuccessResponse{
		Success: true,
		Data:    data,
	}
}

// WithMeta adds metadata to success response
func (s SuccessResponse) WithMeta(meta *Meta) SuccessResponse {
	s.Meta = meta
	return s
}

// RespondWithError sends error response
func RespondWithError(c *gin.Context, status int, code, message string) {
	reqID := c.GetString(RequestIDKey)
	if reqID == "" {
		reqID = uuid.New().String()[:8]
	}

	response := NewErrorResponse(code, message).WithRequestID(reqID)
	c.JSON(status, response)
}

// RespondWithValidationError sends validation error (400)
func RespondWithValidationError(c *gin.Context, message string, details any) {
	reqID := c.GetString(RequestIDKey)
	response := NewErrorResponse(ErrCodeValidationFailed, message).
		WithRequestID(reqID)
	if details != nil {
		response = response.WithDetails(details)
	}
	c.JSON(http.StatusBadRequest, response)
}

// RespondWithNotFound sends 404
func RespondWithNotFound(c *gin.Context, resource string) {
	RespondWithError(c, http.StatusNotFound, ErrCodeNotFound,
		fmt.Sprintf("%s not found", resource))
}

// RespondWithUnauthorized sends 401
func RespondWithUnauthorized(c *gin.Context, message string) {
	if message == "" {
		message = "Authentication required"
	}
	RespondWithError(c, http.StatusUnauthorized, ErrCodeUnauthorized, message)
}

// RespondWithForbidden sends 403
func RespondWithForbidden(c *gin.Context, message string) {
	if message == "" {
		message = "Access denied"
	}
	RespondWithError(c, http.StatusForbidden, ErrCodeForbidden, message)
}

// RespondWithConflict sends 409
func RespondWithConflict(c *gin.Context, message string) {
	RespondWithError(c, http.StatusConflict, ErrCodeConflict, message)
}

// RespondWithRateLimited sends 429
func RespondWithRateLimited(c *gin.Context, retryAfter time.Duration) {
	reqID := c.GetString(RequestIDKey)
	c.Header("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
	response := NewErrorResponse(ErrCodeRateLimited, "Too many requests").
		WithRequestID(reqID)
	c.JSON(http.StatusTooManyRequests, response)
}

// RespondWithSuccess sends success response
func RespondWithSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, NewSuccessResponse(data))
}

// RespondWithCreated sends 201 with success response
func RespondWithCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, NewSuccessResponse(data))
}

// RespondWithNoContent sends 204
func RespondWithNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Middleware for request ID
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()[:8]
		}
		c.Set(RequestIDKey, reqID)
		c.Header("X-Request-ID", reqID)
		c.Next()
	}
}

// Recovery middleware - recovers from panics and logs stack trace
func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				reqID := c.GetString(RequestIDKey)

				// Log the stack trace internally (never expose to client)
				stackTrace := string(debug.Stack())
				fmt.Printf("[PANIC] RequestID: %s | Error: %v\n%s\n",
					reqID, err, stackTrace)

				// Return generic error to client
				response := NewErrorResponse(ErrCodeInternalError,
					"An unexpected error occurred").
					WithRequestID(reqID)
				c.JSON(http.StatusInternalServerError, response)
				c.Abort()
			}
		}()
		c.Next()
	}
}

// Logger middleware - structured logging
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		reqID := c.GetString(RequestIDKey)
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// Structured log format (JSON-like)
		logMsg := fmt.Sprintf("[%s] %s | %d | %s | %s | %s | %v",
			time.Now().Format("2006-01-02 15:04:05"),
			reqID,
			status,
			c.Request.Method,
			path,
			query,
			latency,
		)

		// Log errors with more detail
		if status >= 400 {
			fmt.Println(logMsg + " | ERROR")
		}
	}
}

// ValidateInput validates required fields
func ValidateInput(c *gin.Context, fields map[string]string) error {
	errors := make(map[string]string)

	for field, name := range fields {
		value := c.GetHeader(field)
		if value == "" {
			value = c.Query(field)
		}
		if value == "" {
			errors[field] = fmt.Sprintf("%s is required", name)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed")
	}
	return nil
}

// SanitizeInput removes potentially dangerous characters
func SanitizeInput(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")
	// Trim whitespace
	input = strings.TrimSpace(input)
	return input
}

// JSONMiddleware sets proper JSON headers
func JSONMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.Next()
	}
}

// CORSHeaders sets CORS headers
func CORSHeaders(allowedOrigins string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// In production, validate origin against allowlist
		if allowedOrigins == "*" {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if strings.Contains(allowedOrigins, origin) {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-API-Key")
		c.Header("Access-Control-Max-Age", "3600")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// PaginationParams extracts pagination from query params
func PaginationParams(c *gin.Context) (page, limit int, offset int) {
	page = 1
	limit = 20
	offset = 0

	if p := c.Query("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
		if page < 1 {
			page = 1
		}
	}

	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
		if limit < 1 {
			limit = 20
		}
		if limit > 100 {
			limit = 100 // Max limit
		}
	}

	offset = (page - 1) * limit
	return
}

// Response helpers for list endpoints
func PaginatedResponse(c *gin.Context, data any, total int64, page, limit int) {
	meta := &Meta{
		Total: total,
		Page:  page,
		Limit: limit,
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// MarshalJSON for ErrorResponse to ensure consistent formatting
func (e ErrorResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Success bool        `json:"success"`
		Error   ErrorDetail `json:"error"`
	}{
		Success: false,
		Error:   e.Error,
	})
}
