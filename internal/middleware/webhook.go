package middleware

import (
	"strings"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// WebhookVerifierMiddleware provides Fiber middleware for webhook verification
type WebhookVerifierMiddleware struct {
	verifier *services.WebhookVerifier
}

// NewWebhookVerifierMiddleware creates a new webhook verifier middleware
func NewWebhookVerifierMiddleware(verifier *services.WebhookVerifier) *WebhookVerifierMiddleware {
	return &WebhookVerifierMiddleware{verifier: verifier}
}

// MpesaWebhookHandler handles verified M-Pesa callbacks
// This middleware verifies the callback BEFORE any business logic runs
func (m *WebhookVerifierMiddleware) MpesaVerification() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get client IP for allowlist verification
		ip := c.IP()
		if xff := c.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			ip = strings.TrimSpace(parts[0])
		}

		// Get signature from header
		signature := c.Get("X-Mpesa-Signature")

		// Read body for verification
		body := c.Body()
		if len(body) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "empty request body",
				"code":  "INVALID_BODY",
			})
		}

		// Verify callback using the webhook verifier
		callback, err := m.verifier.VerifyMpesaCallback(body, signature, ip)
		if err != nil {
			// Log security failure - but don't expose internal details
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "callback verification failed",
				"code":  "WEBHOOK_VERIFICATION_FAILED",
			})
		}

		// Store verified callback data in context for handler to use
		// Use the same STKCallback type that MPesaService expects
		c.Locals("mpesa_callback", callback)
		c.Locals("webhook_verified", true)

		return c.Next()
	}
}

// IntasendVerification verifies Intasend webhook signatures
func (m *WebhookVerifierMiddleware) IntasendVerification() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get client IP
		ip := c.IP()
		if xff := c.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			ip = strings.TrimSpace(parts[0])
		}

		// Get signature and timestamp headers
		signature := c.Get("X-Intasend-Signature")
		if signature == "" {
			signature = c.Get("X-Signature")
		}
		timestamp := c.Get("X-Timestamp")

		// Read body
		body := c.Body()
		if len(body) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "empty request body",
				"code":  "INVALID_BODY",
			})
		}

		// Verify webhook
		event, err := m.verifier.VerifyIntasendWebhook(body, signature, timestamp, ip)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "webhook verification failed",
				"code":  "WEBHOOK_VERIFICATION_FAILED",
			})
		}

		// Store verified event in context
		c.Locals("intasend_event", event)
		c.Locals("webhook_verified", true)

		return c.Next()
	}
}

// VerifyTenantMiddleware ensures tenant context is present
func VerifyTenantMiddleware(db *database.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := c.Get("X-Tenant-ID")

		// Try to get from context locals if set by previous middleware
		if tenantID == "" {
			if localTenant, ok := c.Locals("tenant_id").(string); ok {
				tenantID = localTenant
			}
		}

		// For webhook endpoints, tenant ID may be required
		path := c.Path()
		if strings.HasPrefix(path, "/api/v1/webhook") && tenantID == "" {
			// Webhooks might need tenant resolution from payload
			// Let handler resolve it
		}

		if tenantID != "" {
			c.Locals("tenant_id", tenantID)
		}

		return c.Next()
	}
}

// RequireTenantForPrivateRoutes rejects requests without tenant context on protected paths
func RequireTenantForPrivateRoutes() fiber.Handler {
	protectedPaths := []string{"/api/v1/tenant", "/dashboard", "/invoices", "/clients", "/payments"}

	return func(c *fiber.Ctx) error {
		path := c.Path()
		isProtected := false

		for _, p := range protectedPaths {
			if strings.HasPrefix(path, p) {
				isProtected = true
				break
			}
		}

		if isProtected {
			tenantID := c.Get("X-Tenant-ID")
			if tenantID == "" {
				if localTenant, ok := c.Locals("tenant_id").(string); ok {
					tenantID = localTenant
				}
			}

			if tenantID == "" {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "tenant context required",
					"code":  "TENANT_REQUIRED",
				})
			}
		}

		return c.Next()
	}
}

// SanitizedErrorResponse returns standardized error without internal details
func SanitizedErrorResponse(c *fiber.Ctx, status int, code string, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"error": message,
		"code":  code,
	})
}

// SecurityHeaders adds security headers
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		return c.Next()
	}
}

// RequestID adds request ID for tracing
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		reqID := c.Get("X-Request-ID")
		if reqID == "" {
			// Generate simple ID - in production use UUID
			reqID = "req-" + time.Now().Format("20060102150405") + "-" + c.IP()
		}
		c.Locals("request_id", reqID)
		c.Set("X-Request-ID", reqID)
		return c.Next()
	}
}
