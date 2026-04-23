package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// PaymentRoutes configures payment routes with proper security
func PaymentRoutes(app *fiber.App, h *handlers.PaymentHandler, idempotencySvc *services.IdempotencyService, mpesaVerifier *middleware.WebhookVerifierMiddleware) fiber.Router {
	group := app.Group("/api/v1")

	group.Post("/webhook/mpesa",
		middleware.IdempotencyMiddleware(idempotencySvc),
		mpesaVerifier.MpesaVerification(),
		h.HandleMpesaCallback)

	group.Post("/webhook/intasend",
		middleware.IdempotencyMiddleware(idempotencySvc),
		mpesaVerifier.IntasendVerification(),
		h.HandleIntasendWebhook)

	return group
}

// PaymentAPIRoutes configures payment API endpoints
func PaymentAPIRoutes(app *fiber.App, h *handlers.PaymentHandler) fiber.Router {
	api := app.Group("/api/v1")

	api.Post("/payment/stk-push", h.InitiateSTKPush)
	api.Get("/payment/status/:token", h.CheckPaymentStatus)

	return api
}

// TenantPaymentRoutes - tenant-scoped payment routes
func TenantPaymentRoutes(app fiber.Router, h *handlers.PaymentHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/payments")
	group.Use(func(c *fiber.Ctx) error {
		tenantID := c.Locals("tenant_id")
		if tenantID == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
		}
		return c.Next()
	})

	// Main payment endpoints
	group.Get("/", h.GetPayments)
	group.Get("/summary", h.GetPaymentSummary)
	group.Get("/unmatched", h.GetUnmatchedPayments)
	group.Post("/manual-match", h.ManualMatchPayment)
	group.Post("/auto-match", h.AutoMatchPayments)
	group.Get("/audit/:id", h.GetPaymentAudit)

	return group
}
