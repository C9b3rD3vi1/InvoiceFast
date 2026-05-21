package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// PaymentRoutes configures payment routes with proper security
func PaymentRoutes(app *fiber.App, h *handlers.PaymentHandler, idempotencySvc *services.IdempotencyService, mpesaVerifier *middleware.WebhookVerifierMiddleware, rateLimiter *middleware.FiberRateLimiter) fiber.Router {
	group := app.Group("/api/v1/webhook")
	group.Use(rateLimiter.WebhookRateLimiter())

	group.Post("/mpesa",
		middleware.IdempotencyMiddleware(idempotencySvc),
		mpesaVerifier.MpesaVerification(),
		h.HandleMpesaCallback)

	group.Post("/intasend",
		middleware.IdempotencyMiddleware(idempotencySvc),
		mpesaVerifier.IntasendVerification(),
		h.HandleIntasendWebhook)

	return group
}

// TenantPaymentRoutes - tenant-scoped payment routes
func TenantPaymentRoutes(app fiber.Router, h *handlers.PaymentHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/payments")
	group.Use(middleware.TenantMiddleware(authService, db))
	group.Use(middleware.RequireEmailVerified(db))

	// Main payment endpoints
	group.Get("/", h.GetPayments)
	group.Get("/summary", h.GetPaymentSummary)
	group.Get("/unmatched", h.GetUnmatchedPayments)
	group.Post("/manual-match", h.ManualMatchPayment)
	group.Post("/auto-match", h.AutoMatchPayments)
	group.Get("/audit/:id", h.GetPaymentAudit)

	return group
}
