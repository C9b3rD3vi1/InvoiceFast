package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// DashboardRoutes configures dashboard API routes
func DashboardRoutes(app *fiber.App, h *handlers.DashboardHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/dashboard")
	group.Use(middleware.TenantMiddleware(authService, db))

	// Add no-cache headers for all dashboard endpoints
	group.Use(func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
		c.Set("Surrogate-Control", "no-store")
		return c.Next()
	})

	// Main endpoints
	group.Get("/", h.GetDashboard)
	group.Get("/summary", h.GetDashboardSummary)

	// Stats and recent data
	group.Get("/stats", h.GetStats)
	group.Get("/invoices", h.GetRecentInvoices)
	group.Get("/clients", h.GetRecentClients)

	// HTMX endpoints for partial rendering
	group.Get("/htmx/invoices", h.GetHTMXInvoices)

	// Charts
	group.Get("/charts/revenue", h.GetRevenueChart)
	group.Get("/charts/status", h.GetStatusChart)
	group.Get("/charts/clients", h.GetClientRevenueChart)

	// Advanced analytics
	group.Get("/trend/revenue", h.GetRevenueTrend)
	group.Get("/trend/daily", h.GetDailyTrend)
	group.Get("/top-clients", h.GetTopClients)
	group.Get("/activity", h.GetActivityLog)

	return group
}
