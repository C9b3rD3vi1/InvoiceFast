package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func PaymentMatchingRoutes(app *fiber.App, h *handlers.PaymentMatchingHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/payments")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Post("/request", h.RequestPayment)
	group.Post("/manual-match", h.ManualMatch)
	group.Get("/unallocated", h.GetUnallocated)
	group.Get("/stats", h.GetStats)
	group.Get("/", h.GetPayments)
	group.Get("/:id/receipt", h.GetReceipt)
	group.Post("/:id/match", h.MatchPayment)
	group.Post("/:id/reconcile", h.ReconcilePayment)

	return group
}
