package routes

import (
	"time"

	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// AuthRoutes configures authentication routes
func AuthRoutes(app *fiber.App, h *handlers.FiberHandler, rateLimiter *middleware.FiberRateLimiter) fiber.Router {
	group := app.Group("/api/v1/auth")

	// Public auth routes (rate limited)
	group.Post("/register", rateLimiter.Limit(10, time.Minute), h.Register)
	group.Post("/login", rateLimiter.Limit(10, time.Minute), h.Login)
	group.Post("/refresh", h.RefreshToken)
	group.Post("/forgot-password", rateLimiter.Limit(5, time.Minute), h.ForgotPassword)
	group.Post("/reset-password", h.ResetPassword)
	group.Get("/validate-reset-token", h.ValidateResetToken)

	return group
}

// DashboardRoutes configures dashboard and tenant routes
func DashboardRoutes(app *fiber.App, h *handlers.FiberHandler, authService *services.AuthService, rateLimiter *middleware.FiberRateLimiter) fiber.Router {
	group := app.Group("/api/v1/tenant")
	group.Use(middleware.TenantMiddleware(authService))
	group.Use(rateLimiter.Limit(100, time.Minute))

	// User management
	group.Get("/me", h.GetMe)
	group.Put("/me", h.UpdateUser)
	group.Post("/change-password", h.ChangePassword)
	group.Post("/logout", h.Logout)
	group.Post("/api-keys", h.GenerateAPIKey)

	// Dashboard
	group.Get("/dashboard", h.GetDashboard)
	group.Get("/rates", h.GetExchangeRates)

	return group
}
