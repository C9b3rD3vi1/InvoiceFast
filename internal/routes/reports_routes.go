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

	// Core Reports
	group.Get("/overview", h.GetOverview)
	group.Get("/dashboard", h.GetDashboard)

	// Financial Reports
	group.Get("/revenue", h.GetRevenue)
	group.Get("/profit", h.GetProfit)
	group.Get("/cashflow", h.GetCashFlow)

	// Entity Reports
	group.Get("/invoices", h.GetInvoices)
	group.Get("/payments", h.GetPayments)
	group.Get("/clients", h.GetClients)
	group.Get("/expenses", h.GetExpensesReport)

	// Aging & Tax
	group.Get("/tax", h.GetTax)
	group.Get("/vat", h.GetVATReport)
	group.Get("/aging", h.GetAging)
	group.Get("/aging-detailed", h.GetAgingDetailed)

	// Financial Statements
	group.Get("/income-statement", h.GetIncomeStatement)
	group.Get("/client/:clientID/statement", h.GetClientStatement)

	// Export
	group.Get("/export", h.Export)

	return group
}
