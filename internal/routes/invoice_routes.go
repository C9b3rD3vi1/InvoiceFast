package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func InvoiceRoutes(app fiber.Router, h *handlers.InvoiceHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/invoices")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Post("/", h.CreateInvoice)
	group.Get("/", h.GetInvoices)

	// STATIC ROUTES FIRST
	group.Get("/stats", h.GetDashboardStats)
	group.Get("/kra-stats", h.GetKRADashboardStats)
	group.Get("/by-token/:token", h.GetInvoiceByToken)

	// DYNAMIC ROUTES AFTER
	group.Get("/:id", h.GetInvoice)
	group.Put("/:id", h.UpdateInvoice)
	group.Delete("/:id", h.DeleteInvoice)

	group.Post("/:id/send", h.SendInvoice)
	group.Post("/:id/whatsapp", h.SendWhatsApp)
	group.Post("/:id/reminder", h.SendReminder)

	group.Post("/:id/attachments", h.CreateInvoiceAttachment)
	group.Get("/:id/attachments", h.GetInvoiceAttachments)
	group.Delete("/:id/attachments/:attachmentId", h.DeleteInvoiceAttachment)

	group.Get("/:id/pdf", h.GetInvoicePDF)

	group.Post("/:id/kra/submit", h.SubmitToKRA)
	group.Get("/:id/kra/status", h.GetKRAStatus)
	group.Post("/:id/kra/retry", h.RetryKRA)

	// KRA bulk operations
	group.Get("/kra/activity", h.GetKRAActivityFeed)
	group.Post("/kra/submit-all", h.SubmitAllPendingToKRA)

	group.Post("/:id/payments", h.RecordPayment)

	return group
}

// ClientRoutes configures /api/v1/tenant/clients endpoints
func ClientRoutes(app fiber.Router, h *handlers.ClientHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/clients")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Post("/", h.CreateClient)
	group.Get("/", h.GetClients)
	group.Get("/stats", h.GetDashboardStats)
	group.Get("/:id", h.GetClient)
	group.Put("/:id", h.UpdateClient)
	group.Delete("/:id", h.DeleteClient)
	group.Post("/:id/stats", h.GetClientStats)
	group.Get("/:id/invoices", h.GetClientInvoices)
	group.Get("/:id/payments", h.GetClientPayments)
	group.Get("/:id/activity", h.GetClientActivity)

	return group
}
