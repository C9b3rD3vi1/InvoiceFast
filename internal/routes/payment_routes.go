package routes

import (
	"time"

	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// PaymentRoutes configures payment routes with proper security
func PaymentRoutes(app *fiber.App, h *handlers.FiberHandler, idempotencySvc *services.IdempotencyService, rateLimiter *middleware.FiberRateLimiter, mpesaVerifier *middleware.WebhookVerifierMiddleware) fiber.Router {
	group := app.Group("/api/v1")

	// M-Pesa webhook - with signature verification and idempotency
	group.Post("/webhook/mpesa",
		middleware.IdempotencyMiddleware(idempotencySvc),
		mpesaVerifier.MpesaVerification(),
		h.HandleMpesaCallback)

	// Intasend webhook - with signature verification and idempotency
	group.Post("/webhook/intasend",
		middleware.IdempotencyMiddleware(idempotencySvc),
		mpesaVerifier.IntasendVerification(),
		h.HandleIntasendWebhook)

	return group
}

// PaymentAPIRoutes configures payment API endpoints
func PaymentAPIRoutes(app *fiber.App, h *handlers.PublicHandler, rateLimiter *middleware.FiberRateLimiter) fiber.Router {
	api := app.Group("/api/v1")

	// Rate limit payment initiation - 10 requests per minute per IP
	api.Post("/payment/stk-push",
		rateLimiter.Limit(10, time.Minute),
		h.InitiateSTKPush)

	api.Get("/payment/status/:token", h.CheckPaymentStatus)

	return api
}

// PublicInvoiceRoutes configures public invoice access routes
func PublicInvoiceRoutes(app *fiber.App, h *handlers.FiberHandler, rateLimiter *middleware.FiberRateLimiter) fiber.Router {
	group := app.Group("/api/v1")

	// Rate limit invoice token lookups to prevent enumeration
	group.Get("/invoice/:token",
		rateLimiter.Limit(30, time.Minute),
		h.GetInvoiceByToken)

	return group
}
