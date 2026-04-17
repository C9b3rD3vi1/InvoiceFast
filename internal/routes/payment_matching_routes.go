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

	group.Get("/unallocated", h.GetUnallocated)
	group.Post("/:id/match", h.MatchPayment)
	group.Post("/manual-match", h.ManualMatch)

	return group
}
