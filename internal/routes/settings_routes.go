package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// SettingsRoutes configures settings API routes
// Only admin/manager roles can modify settings; all authenticated users can read
func SettingsRoutes(app *fiber.App, h *handlers.SettingsHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/settings")
	group.Use(middleware.TenantMiddleware(authService, db))
	group.Use(middleware.RequireEmailVerified(db))

	// Read operations - available to all authenticated users
	group.Get("/", h.GetSettings)
	group.Get("/mpesa", h.GetMpesaSettings)
	group.Get("/kra", h.GetKRASettings)
	group.Get("/notifications", h.GetNotificationSettings)

	// Write operations - require manager+ role
	group.Put("/", middleware.CanManageSettings(), h.SaveSettings)
	group.Put("/mpesa", middleware.CanManageSettings(), h.SaveSettingsMpesa)
	group.Put("/kra", middleware.CanManageSettings(), h.SaveSettingsKRA)
	group.Put("/branding", middleware.CanManageSettings(), h.SaveBranding)
	group.Put("/notifications", middleware.CanManageSettings(), h.SaveNotificationSettings)

	return group
}
