package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func ActivityRoutes(app *fiber.App, h *handlers.ActivityHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/activity")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/", h.GetRecentActivity)

	return group
}
