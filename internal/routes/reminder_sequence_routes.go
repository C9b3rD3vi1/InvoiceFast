package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func ReminderSequenceRoutes(app *fiber.App, h *handlers.ReminderSequenceHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/reminder-sequences")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/", h.GetSequences)
	group.Post("/", h.CreateSequence)
	group.Put("/:id", h.UpdateSequence)
	group.Delete("/:id", h.DeleteSequence)

	return group
}
