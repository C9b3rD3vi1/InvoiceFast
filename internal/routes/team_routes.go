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
	group.Use(middleware.RequireEmailVerified(db))

	// Read operations - available to all authenticated users
	group.Get("/members", h.GetTeamMembers)
	group.Get("/invitations", h.GetInvitations)

	// Write operations - require manager+ role
	group.Post("/invite", middleware.CanManageUsers(), h.InviteMember)
	group.Delete("/member/:id", middleware.CanManageUsers(), h.RemoveMember)
	group.Put("/member/:id/role", middleware.CanManageUsers(), h.UpdateMemberRole)
	group.Delete("/invitation/:id", middleware.CanManageUsers(), h.CancelInvitation)

	return group
}
