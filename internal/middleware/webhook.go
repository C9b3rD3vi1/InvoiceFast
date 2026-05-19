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
		result := m.verifier.VerifyMpesaCallback(body, signature)
		if !result.Valid {
			// Log security failure - but don't expose internal details
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "callback verification failed",
				"code":  "WEBHOOK_VERIFICATION_FAILED",
			})
		}

		// Store verified callback data in context
		c.Locals("webhook_verified", true)
		c.Locals("webhook_provider", "mpesa")
		c.Locals("webhook_timestamp", result.Timestamp)

		return c.Next()
	}
}

// IntasendVerification verifies Intasend webhook signatures
func (m *WebhookVerifierMiddleware) IntasendVerification() fiber.Handler {
	return func(c *fiber.Ctx) error {
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
		result := m.verifier.VerifyIntasendWebhook(body, signature, timestamp)
		if !result.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "webhook verification failed",
				"code":  "WEBHOOK_VERIFICATION_FAILED",
			})
		}

		// Store verified event in context
		c.Locals("webhook_verified", true)
		c.Locals("webhook_provider", "intasend")
		c.Locals("webhook_timestamp", result.Timestamp)

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

		// Allow public paths (login, register, public APIs)
		if !isProtected {
			return c.Next()
		}

		// Check for tenant context
		tenantID := c.Locals("tenant_id")
		if tenantID == nil {
			// Try to get from header
			tenantID = c.Get("X-Tenant-ID")
		}

		if tenantID == nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "tenant context required",
				"code":  "TENANT_REQUIRED",
			})
		}

		return c.Next()
	}
}

// WebhookTimeoutMiddleware adds timeout to webhook handlers
func WebhookTimeoutMiddleware(timeout time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Only apply to webhook routes
		if !strings.HasPrefix(c.Path(), "/api/v1/webhook") {
			return c.Next()
		}

		// Fiber handles timeouts via the server config
		// This middleware is for logging/metrics purposes
		start := time.Now()

		err := c.Next()

		duration := time.Since(start)
		if duration > timeout {
			// Log slow webhook
			// In production, emit metric
		}

		return err
	}
}