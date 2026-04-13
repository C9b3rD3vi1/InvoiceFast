package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// AutomationRoutes configures automation API routes
func AutomationRoutes(app *fiber.App, h *handlers.AutomationHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/automations")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/", h.GetAutomations)
	group.Get("/:id", h.GetAutomation)
	group.Post("/", h.CreateAutomation)
	group.Put("/:id", h.UpdateAutomation)
	group.Delete("/:id", h.DeleteAutomation)
	group.Post("/:id/run", h.RunAutomation)
	group.Get("/:id/logs", h.GetAutomationLogs)

	return group
}
