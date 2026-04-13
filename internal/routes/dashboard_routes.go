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

	// Main endpoints
	group.Get("/", h.GetDashboard)
	group.Get("/summary", h.GetDashboardSummary)

	// Stats and recent data
	group.Get("/stats", h.GetStats)
	group.Get("/invoices", h.GetRecentInvoices)
	group.Get("/clients", h.GetRecentClients)

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
