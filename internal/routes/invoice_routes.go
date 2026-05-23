package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func InvoiceRoutes(app fiber.Router, h *handlers.InvoiceHandler, authService *services.AuthService, db *database.DB, subMiddleware *middleware.SubscriptionMiddleware) fiber.Router {
	group := app.Group("/api/v1/tenant/invoices")
	group.Use(middleware.TenantMiddleware(authService, db))
	group.Use(middleware.RequireEmailVerified(db))

	group.Post("/", middleware.CanEditInvoice(), subMiddleware.EnforceLimits("invoices"), h.CreateInvoice)
	group.Get("/", h.GetInvoices)

	// STATIC ROUTES FIRST
	group.Get("/stats", h.GetDashboardStats)
	group.Get("/kra-stats", h.GetKRADashboardStats)
	group.Get("/by-token/:token", h.GetInvoiceByToken)

	// DYNAMIC ROUTES AFTER
	group.Get("/:id", h.GetInvoice)
	group.Put("/:id", middleware.CanEditInvoice(), h.UpdateInvoice)
	group.Delete("/:id", middleware.CanDeleteInvoice(), h.DeleteInvoice)

	group.Post("/:id/send", middleware.CanEditInvoice(), h.SendInvoice)
	group.Post("/:id/whatsapp", middleware.CanEditInvoice(), h.SendWhatsApp)
	group.Post("/:id/reminder", middleware.CanEditInvoice(), h.SendReminder)

	group.Post("/:id/attachments", middleware.CanEditInvoice(), h.CreateInvoiceAttachment)
	group.Get("/:id/attachments", h.GetInvoiceAttachments)
	group.Delete("/:id/attachments/:attachmentId", middleware.CanEditInvoice(), h.DeleteInvoiceAttachment)

	group.Get("/:id/pdf", h.GetInvoicePDF)

	group.Post("/:id/kra/submit", middleware.CanEditInvoice(), h.SubmitToKRA)
	group.Get("/:id/kra/status", h.GetKRAStatus)
	group.Post("/:id/kra/retry", middleware.CanEditInvoice(), h.RetryKRA)

	// KRA bulk operations
	group.Get("/kra/activity", h.GetKRAActivityFeed)
	group.Post("/kra/submit-all", middleware.CanEditInvoice(), h.SubmitAllPendingToKRA)

	group.Post("/:id/payments", middleware.CanEditInvoice(), h.RecordPayment)
	group.Post("/:id/cancel", middleware.CanEditInvoice(), h.CancelInvoice)

	// Duplicate and payment request
	group.Post("/:id/duplicate", middleware.CanEditInvoice(), h.DuplicateInvoice)
	group.Post("/:id/payment-request", middleware.CanEditInvoice(), h.RequestPayment)

	// Credit/Debit Note routes
	group.Post("/:id/credit-note", middleware.CanEditInvoice(), h.CreateCreditNote)
	group.Post("/:id/debit-note", middleware.CanEditInvoice(), h.CreateDebitNote)

	return group
}

// ClientRoutes configures /api/v1/tenant/clients endpoints
func ClientRoutes(app fiber.Router, h *handlers.ClientHandler, authService *services.AuthService, db *database.DB, subMiddleware *middleware.SubscriptionMiddleware) fiber.Router {
	group := app.Group("/api/v1/tenant/clients")
	group.Use(middleware.TenantMiddleware(authService, db))
	group.Use(middleware.RequireEmailVerified(db))

	group.Post("/", subMiddleware.EnforceLimits("clients"), h.CreateClient)
	group.Get("/", h.GetClients)
	group.Get("/stats", h.GetDashboardStats)

	// Buyer type endpoint MUST come before :id route
	group.Get("/:id/buyer-type", h.GetBuyerType)
	group.Post("/:id/buyer-type", h.SetBuyerType)

	group.Get("/:id", h.GetClient)
	group.Put("/:id", h.UpdateClient)
	group.Delete("/:id", h.DeleteClient)
	group.Post("/:id/stats", h.GetClientStats)
	group.Get("/:id/invoices", h.GetClientInvoices)
	group.Get("/:id/payments", h.GetClientPayments)
	group.Get("/:id/activity", h.GetClientActivity)

	return group
}
