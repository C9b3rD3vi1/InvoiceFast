package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// ItemLibraryRoutes configures /api/v1/tenant/item-library
func ItemLibraryRoutes(app fiber.Router, h *handlers.ItemLibraryHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/item-library")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Post("/", h.CreateItem)
	group.Get("/", h.GetItems)
	group.Get("/:id", h.GetItem)
	group.Put("/:id", h.UpdateItem)
	group.Delete("/:id", h.DeleteItem)

	return group
}
