package routes

import (
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// AuthRoutes configures /api/v1/auth endpoints
func AuthRoutes(app *fiber.App, h *handlers.AuthHandler, rateLimiter *middleware.FiberRateLimiter) fiber.Router {
	group := app.Group("/api/v1/auth")

	// Public auth routes (rate limited)
	group.Post("/register", rateLimiter.Limit(10, time.Minute), h.Register)
	group.Post("/login", rateLimiter.Limit(10, time.Minute), h.Login)
	group.Post("/refresh", h.RefreshToken)

	return group
}

// TenantRoutes configures /api/v1/tenant endpoints
func TenantRoutes(app *fiber.App, h *handlers.AuthHandler, authService *services.AuthService, rateLimiter *middleware.FiberRateLimiter, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant")
	group.Use(middleware.TenantMiddleware(authService, db))
	group.Use(rateLimiter.Limit(100, time.Minute))

	// User management
	group.Get("/me", h.GetMe)
	group.Put("/me", h.UpdateUser)
	group.Post("/change-password", h.ChangePassword)
	group.Post("/logout", h.Logout)

	return group
}
