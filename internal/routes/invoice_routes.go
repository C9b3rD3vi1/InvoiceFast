package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// InvoiceRoutes configures invoice routes
func InvoiceRoutes(app *fiber.App, h *handlers.FiberHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/invoices")
	group.Use(middleware.TenantMiddleware(authService, db))

	// Invoice CRUD
	group.Post("/", h.CreateInvoice)
	group.Get("/", h.GetInvoices)
	group.Get("/:id", h.GetInvoice)
	group.Put("/:id", h.UpdateInvoice)
	group.Put("/:id/items", h.UpdateInvoiceItems)

	// Invoice actions
	group.Post("/:id/send", h.SendInvoice)
	group.Post("/:id/cancel", h.CancelInvoice)
	group.Post("/:id/pay", h.RequestPayment)

	// Invoice exports
	group.Get("/:id/pdf", h.GetInvoicePDF)
	group.Get("/:id/status", h.GetInvoiceStatus)

	return group
}

// ClientRoutes configures client routes
func ClientRoutes(app *fiber.App, h *handlers.FiberHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/clients")
	group.Use(middleware.TenantMiddleware(authService, db))

	// Client CRUD
	group.Post("/", h.CreateClient)
	group.Get("/", h.GetClients)
	group.Get("/:id", h.GetClient)
	group.Put("/:id", h.UpdateClient)
	group.Delete("/:id", h.DeleteClient)

	// Client analytics
	group.Get("/:id/stats", h.GetClientStats)

	return group
}
