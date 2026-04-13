package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// SettingsRoutes configures settings API routes
func SettingsRoutes(app *fiber.App, h *handlers.SettingsHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/settings")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/", h.GetSettings)
	group.Put("/", h.SaveSettings)
	group.Get("/mpesa", h.GetMpesaSettings)
	group.Put("/mpesa", h.SaveSettingsMpesa)
	group.Get("/kra", h.GetKRASettings)
	group.Put("/kra", h.SaveSettingsKRA)
	group.Put("/branding", h.SaveBranding)
	group.Get("/notifications", h.GetNotificationSettings)
	group.Put("/notifications", h.SaveNotificationSettings)

	return group
}
