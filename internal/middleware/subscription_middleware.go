package middleware

import (
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type SubscriptionMiddleware struct {
	subService *services.SubscriptionService
}

func NewSubscriptionMiddleware(subSvc *services.SubscriptionService) *SubscriptionMiddleware {
	return &SubscriptionMiddleware{subService: subSvc}
}

func (m *SubscriptionMiddleware) EnforceLimits(resource string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := GetTenantID(c)
		if tenantID == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
		}

		allowed, reason, err := m.subService.CheckLimits(tenantID, resource, 1)
		if err != nil {
			return c.Next()
		}

		if !allowed {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error":   "Plan limit exceeded",
				"reason":  reason,
				"upgrade": "/billing/upgrade",
			})
		}

		return c.Next()
	}
}

func (m *SubscriptionMiddleware) EnforceFeature(feature string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := GetTenantID(c)
		if tenantID == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
		}

		if !m.subService.HasFeature(tenantID, feature) {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error":   "Premium feature not available on your plan",
				"upgrade": "/billing/plans",
			})
		}

		return c.Next()
	}
}

func (m *SubscriptionMiddleware) EnforceActiveSubscription() fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := GetTenantID(c)
		if tenantID == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
		}

		sub, err := m.subService.GetActiveSubscription(tenantID)
		if err != nil {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error":   "Active subscription required",
				"upgrade": "/billing/plans",
			})
		}

		if sub.Status == "suspended" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":      "Subscription suspended",
				"message":    sub.LastPaymentError,
				"reactivate": "/billing/reactivate",
			})
		}

		if sub.HasTrial() && sub.TrialEndsAt != nil {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error":   "Trial expired",
				"upgrade": "/billing/plans",
			})
		}

		c.Locals("subscription", sub)
		return c.Next()
	}
}
