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

	// Search endpoint
	group.Get("/search", h.Search)

	return group
}
