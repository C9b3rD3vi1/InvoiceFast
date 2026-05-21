package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// ExpenseRoutes configures /api/v1/tenant/expenses endpoints
func ExpenseRoutes(app *fiber.App, h *handlers.ExpenseHandler, authService *services.AuthService, db *database.DB, subMiddleware *middleware.SubscriptionMiddleware) fiber.Router {
	group := app.Group("/api/v1/tenant/expenses")
	group.Use(middleware.TenantMiddleware(authService, db))
	group.Use(middleware.RequireEmailVerified(db))

	group.Post("/", subMiddleware.EnforceLimits("expenses"), h.CreateExpense)
	group.Get("/", h.GetExpenses)
	group.Get("/:id", h.GetExpense)
	group.Put("/:id", h.UpdateExpense)
	group.Delete("/:id", h.DeleteExpense)
	
	// Expense attachment routes
	group.Post("/:id/attachments", h.UploadExpenseAttachment)
	group.Get("/:id/attachments", h.GetExpenseAttachments)
	group.Delete("/:id/attachments/:attachmentId", h.DeleteExpenseAttachment)

	// Attachment file serving - validates tenant via middleware
	app.Get("/api/v1/tenant/expenses/attachment-file/:attachmentId", middleware.TenantMiddleware(authService, db), h.GetExpenseAttachmentFile)

	return group
}
