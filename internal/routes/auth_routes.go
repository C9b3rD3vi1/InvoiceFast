package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// AuthRoutes configures /api/v1/auth endpoints
func AuthRoutes(app fiber.Router, h *handlers.AuthHandler) fiber.Router {
	group := app.Group("/api/v1/auth")

	group.Post("/register", h.Register)
	group.Post("/login", h.Login)
	group.Post("/refresh", h.RefreshToken)

	return group
}

// TenantRoutes configures /api/v1/tenant endpoints
func TenantRoutes(app fiber.Router, h *handlers.AuthHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/me", h.GetMe)
	group.Put("/me", h.UpdateUser)
	group.Post("/change-password", h.ChangePassword)
	group.Post("/logout", h.Logout)

	group.Post("/2fa/setup", h.SetupTwoFactor)
	group.Post("/2fa/verify", h.VerifyTwoFactor)
	group.Post("/2fa/disable", h.DisableTwoFactor)

	group.Get("/sessions", h.GetSessions)
	group.Delete("/session/:id", h.RevokeSession)
	group.Post("/sessions/revoke-all", h.RevokeAllSessions)

	group.Get("/login-history", h.GetLoginHistory)
	group.Put("/login-alerts", h.UpdateLoginAlerts)
	group.Get("/security-status", h.GetSecurityStatus)

	group.Get("/search", h.Search)

	return group
}
