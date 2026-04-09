package routes

import (
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// PaymentRoutes configures payment routes
func PaymentRoutes(app *fiber.App, h *handlers.FiberHandler, idempotencySvc *services.IdempotencyService) fiber.Router {
	group := app.Group("/api/v1")

	// Public webhook routes
	group.Post("/webhook/intasend",
		middleware.IdempotencyMiddleware(idempotencySvc),
		h.HandleIntasendWebhook)

	return group
}

// PublicInvoiceRoutes configures public invoice access routes
func PublicInvoiceRoutes(app *fiber.App, h *handlers.FiberHandler) fiber.Router {
	group := app.Group("/api/v1")

	// Public invoice view (via magic token)
	group.Get("/invoice/:token", h.GetInvoiceByToken)

	return group
}
