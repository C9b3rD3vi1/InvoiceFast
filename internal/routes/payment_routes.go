package routes

import (
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
