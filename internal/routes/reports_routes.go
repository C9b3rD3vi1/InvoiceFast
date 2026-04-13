package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// ReportRoutes configures /api/v1/tenant/reports
func ReportRoutes(app *fiber.App, h *handlers.ReportHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/reports")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/overview", h.GetOverview)
	group.Get("/revenue", h.GetRevenue)
	group.Get("/invoices", h.GetInvoices)
	group.Get("/payments", h.GetPayments)
	group.Get("/clients", h.GetClients)
	group.Get("/tax", h.GetTax)
	group.Get("/export", h.Export)

	return group
}
