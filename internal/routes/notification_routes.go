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
	group.Use(middleware.RequireEmailVerified(db))

	group.Get("/", h.GetNotifications)
	group.Post("/read-all", h.MarkAllAsRead)
	group.Put("/:id/read", h.MarkAsRead)
	group.Delete("/:id", h.DeleteNotification)

	return group
}

// NotificationAdminRoutes configures notification admin endpoints (preferences, templates, logs)
func NotificationAdminRoutes(app *fiber.App, h *services.NotificationHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/notification-admin")
	group.Use(middleware.TenantMiddleware(authService, db))
	group.Use(middleware.RequireEmailVerified(db))

	// Preferences
	group.Get("/preferences", h.GetPreferences)
	group.Put("/preferences", h.UpdatePreferences)

	// Delivery logs
	group.Get("/logs", h.GetDeliveryLogs)

	// Templates
	group.Get("/templates", h.GetTemplates)
	group.Post("/templates", h.CreateTemplate)
	group.Put("/templates/:id", h.UpdateTemplate)
	group.Delete("/templates/:id", h.DeleteTemplate)

	// Queue management
	group.Post("/retry/:id", h.RetryNotification)

	return group
}
