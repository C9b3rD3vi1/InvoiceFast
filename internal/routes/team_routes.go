package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// TeamRoutes configures /api/v1/tenant/team
func TeamRoutes(app *fiber.App, h *handlers.TeamHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/team")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/members", h.GetTeamMembers)
	group.Post("/invite", h.InviteMember)
	group.Delete("/member/:id", h.RemoveMember)
	group.Put("/member/:id/role", h.UpdateMemberRole)
	group.Get("/invitations", h.GetInvitations)
	group.Delete("/invitation/:id", h.CancelInvitation)

	return group
}
