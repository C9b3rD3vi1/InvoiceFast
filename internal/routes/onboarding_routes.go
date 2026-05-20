package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func OnboardingRoutes(app *fiber.App, h *handlers.OnboardingHandler, authService *services.AuthService, db *database.DB, sensitiveRateLimit fiber.Handler) {
	// Serve the onboarding single-page wizard
	app.Get("/onboarding", h.ServeOnboardingPage)

	// Onboarding API endpoints (some require auth, some don't)
	api := app.Group("/api/v1/onboarding")
	api.Use(sensitiveRateLimit)

	// Registration endpoint (no auth)
	api.Post("/register", h.HandleOnboardingRegister)

	// Login endpoint (no auth)
	api.Post("/login", h.HandleOnboardingLogin)

	// Endpoints that require authentication
	authed := api.Group("")
	authed.Use(middleware.TenantMiddleware(authService, db))
	authed.Use(middleware.RequireTenant())

	authed.Get("/progress", h.HandleOnboardingProgress)
	authed.Post("/dismiss", h.HandleDismissOnboarding)
	authed.Post("/verify-email", h.HandleVerifyEmail)
	authed.Post("/resend-code", h.HandleResendCode)
	authed.Post("/business-profile", h.HandleBusinessProfile)
	authed.Post("/create-invoice", h.HandleCreateInvoice)
	authed.Post("/save-payment", h.HandleSavePayment)
	authed.Get("/email-status", h.HandleCheckEmailVerified)
}
