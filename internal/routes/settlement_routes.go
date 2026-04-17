package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func SettlementRoutes(app *fiber.App, h *handlers.SettlementHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/settlement")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/daily", h.GetDailySettlement)
	group.Get("/export", h.ExportSettlement)

	return group
}
