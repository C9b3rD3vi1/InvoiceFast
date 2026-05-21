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
// Template management and queue operations require manager+ role
func NotificationAdminRoutes(app *fiber.App, h *services.NotificationHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/notification-admin")
	group.Use(middleware.TenantMiddleware(authService, db))
	group.Use(middleware.RequireEmailVerified(db))

	// Preferences - available to all authenticated users
	group.Get("/preferences", h.GetPreferences)
	group.Put("/preferences", h.UpdatePreferences)

	// Delivery logs - available to all authenticated users
	group.Get("/logs", h.GetDeliveryLogs)

	// Templates - require manager+ role (can modify notification content)
	tmplGroup := group.Group("/templates")
	tmplGroup.Get("/", h.GetTemplates)
	tmplGroup.Post("/", middleware.CanManageSettings(), h.CreateTemplate)
	tmplGroup.Put("/:id", middleware.CanManageSettings(), h.UpdateTemplate)
	tmplGroup.Delete("/:id", middleware.CanManageSettings(), h.DeleteTemplate)

	// Queue management - require manager+ role
	group.Post("/retry/:id", middleware.CanManageSettings(), h.RetryNotification)

	return group
}
