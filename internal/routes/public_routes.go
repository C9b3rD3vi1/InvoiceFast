package routes

import (
	"time"

	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

// PublicRoutes configures all public-facing routes
// These routes do not require authentication
func PublicRoutes(app *fiber.App, h *handlers.PublicHandler) fiber.Router {
	// Create public route group
	public := app.Group("/")

	// Apply security headers to all public routes
	public.Use(func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		return c.Next()
	})

	// Landing page
	public.Get("/", h.ServeLanding)

	// Auth pages
	public.Get("/login.html", h.ServeLogin)
	public.Get("/register.html", h.ServeRegister)
	public.Get("/contact.html", h.ServeContact)

	// Contact form POST (HTMX)
	public.Post("/api/v1/contact", h.HandleContact)

	// Client Payment Portal - must be before /invoice to avoid route conflict
	public.Get("/pay/:token", h.ServePortal)
	public.Get("/invoice/:token", h.ServePortal)
	public.Get("/invoice/:token/success", h.ServeSuccess)

	return public
}

// PublicAPIRoutes configures public API endpoints with rate limiting
func PublicAPIRoutes(app *fiber.App, h *handlers.PublicHandler, rateLimiter *middleware.FiberRateLimiter) fiber.Router {
	api := app.Group("/api/v1")

	// Rate limit all public API endpoints - 30 requests per minute
	api.Use(rateLimiter.Limit(30, time.Minute))

	// Public invoice access (no auth required - token-based)
	api.Get("/invoice/:token", h.GetInvoiceByToken)
	api.Get("/invoice/:token/pdf", h.GetInvoicePDF)
	api.Get("/invoice/:token/receipt.pdf", h.GetInvoiceReceipt)

	// Payment endpoints - stricter rate limit (10/min)
	api.Post("/payment/stk-push", rateLimiter.Limit(10, time.Minute), h.InitiateSTKPush)
	api.Get("/payment/status/:token", h.CheckPaymentStatus)

	// Pricing endpoint (for HTMX currency toggle)
	api.Get("/pricing", h.GetPricing)

	return api
}

// AuthRoutes configures public authentication routes
func PublicAuthRoutes(app *fiber.App, h *handlers.PublicHandler, rateLimiter *middleware.FiberRateLimiter) fiber.Router {
	auth := app.Group("/api/v1/auth")

	// Apply rate limiting to auth endpoints - stricter (10/min)
	auth.Use(rateLimiter.Limit(10, time.Minute))

	// Auth endpoints (HTMX form submissions)
	auth.Post("/login", h.HandleLogin)
	auth.Post("/register", h.HandleRegister)

	return auth
}
