package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// NotificationRoutes configures /api/v1/tenant/notifications
func NotificationRoutes(app *fiber.App, h *handlers.NotificationHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/notifications")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/", h.GetNotifications)
	group.Post("/read-all", h.MarkAllAsRead)
	group.Put("/:id/read", h.MarkAsRead)
	group.Delete("/:id", h.DeleteNotification)

	return group
}
