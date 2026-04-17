package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// RecurringInvoiceRoutes configures /api/v1/tenant/recurring endpoints
func RecurringInvoiceRoutes(app fiber.Router, h *handlers.RecurringInvoiceHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/recurring")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/", h.ListRecurring)
	group.Post("/:invoiceID/enable", h.EnableRecurring)
	group.Post("/:invoiceID/disable", h.DisableRecurring)
	group.Post("/process", h.ProcessRecurring)

	return group
}
