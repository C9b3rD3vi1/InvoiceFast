package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func BulkActionRoutes(app *fiber.App, h *handlers.BulkActionHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/bulk")
	group.Use(middleware.TenantMiddleware(authService, db))
	group.Use(middleware.RequireEmailVerified(db))

	group.Post("/overdue-reminders", h.SendOverdueReminders)

	return group
}
