package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func LateFeeRoutes(app *fiber.App, h *handlers.LateFeeHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/late-fees")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/config", h.GetConfig)
	group.Put("/config", h.UpdateConfig)
	group.Get("/invoice/:invoiceID/calculate", h.CalculateFee)
	group.Get("/invoice/:invoiceID", h.GetInvoiceLateFees)
	group.Post("/:lateFeeID/waive", h.WaiveLateFee)

	return group
}
