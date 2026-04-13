package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// InvoiceRoutes configures /api/v1/tenant/invoices
func InvoiceRoutes(app *fiber.App, h *handlers.InvoiceHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/invoices")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Post("/", h.CreateInvoice)
	group.Get("/", h.GetInvoices)
	group.Get("/:id", h.GetInvoice)
	group.Put("/:id", h.UpdateInvoice)
	group.Post("/:id/send", h.SendInvoice)
	group.Post("/:id/cancel", h.CancelInvoice)
	group.Get("/by-token/:token", h.GetInvoiceByToken)
	group.Get("/:token/pdf", h.GetInvoicePDF)

	return group
}

// ClientRoutes configures /api/v1/tenant/clients
func ClientRoutes(app *fiber.App, h *handlers.ClientHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/clients")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Post("/", h.CreateClient)
	group.Get("/", h.GetClients)
	group.Get("/:id", h.GetClient)
	group.Put("/:id", h.UpdateClient)
	group.Delete("/:id", h.DeleteClient)
	group.Get("/:id/stats", h.GetClientStats)

	return group
}
