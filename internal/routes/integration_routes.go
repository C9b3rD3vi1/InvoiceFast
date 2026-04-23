package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func IntegrationRoutes(app *fiber.App, h *handlers.IntegrationHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/integrations")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/", h.GetIntegrations)
	group.Get("/:provider", h.GetIntegration)
	group.Get("/:provider/config", h.GetIntegrationConfig)
	group.Put("/:provider", h.SaveIntegration)
	group.Delete("/:id", h.DeleteIntegration)
	group.Post("/:id/toggle", h.ToggleIntegration)

	return group
}
