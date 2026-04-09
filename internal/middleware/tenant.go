package middleware

import (
	"strings"

	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

const (
	TenantIDKey = "tenant_id"
	UserIDKey   = "user_id"
)

func TenantMiddleware(authService *services.AuthService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var tenantID, userID string
		var authErr error

		// Extract from JWT Bearer token
		authHeader := c.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := authService.ValidateToken(token)
			if err == nil && claims != nil {
				userID = claims.UserID
				tenantID = claims.TenantID
				// Fallback: if no tenant_id in JWT, use userID as tenant
				if tenantID == "" {
					tenantID = userID
				}
			} else {
				authErr = err
			}
		}

		// Allow X-Tenant-ID header for service-to-service calls
		if tenantID == "" {
			tenantID = c.Get("X-Tenant-ID")
		}

		// Store in Fiber context locals (request-scoped, thread-safe)
		if tenantID != "" {
			c.Locals(TenantIDKey, tenantID)
		}
		if userID != "" {
			c.Locals(UserIDKey, userID)
		}

		// If authentication failed and we need auth, return 401
		if authErr != nil && c.Path() != "/api/v1/auth/login" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or expired token",
			})
		}

		return c.Next()
	}
}

func GetTenantID(c *fiber.Ctx) string {
	if val := c.Locals(TenantIDKey); val != nil {
		if id, ok := val.(string); ok && id != "" {
			return id
		}
	}
	return ""
}

func GetUserID(c *fiber.Ctx) string {
	if val := c.Locals(UserIDKey); val != nil {
		if id, ok := val.(string); ok && id != "" {
			return id
		}
	}
	return ""
}

func RequireTenant() fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := GetTenantID(c)
		if tenantID == "" {
			return fiber.NewError(fiber.StatusForbidden, "tenant context required")
		}
		return c.Next()
	}
}

func GetTenantAndUser(c *fiber.Ctx) (tenantID, userID string, err error) {
	tenantID = GetTenantID(c)
	userID = GetUserID(c)

	if tenantID == "" {
		return "", "", fiber.NewError(fiber.StatusForbidden, "tenant_id not found in context")
	}
	if userID == "" {
		return "", "", fiber.NewError(fiber.StatusUnauthorized, "user_id not found in context")
	}

	return tenantID, userID, nil
}
