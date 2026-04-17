package routes

import (
	"invoicefast/internal/handlers"

	"github.com/gofiber/fiber/v2"
)

// PublicRoutes configures all public-facing routes
func PublicRoutes(app *fiber.App, h *handlers.PublicHandler) fiber.Router {
	public := app.Group("/")

	public.Use(func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		return c.Next()
	})

	public.Get("/", h.ServeLanding)
	public.Get("/login.html", h.ServeLogin)
	public.Get("/register.html", h.ServeRegister)
	public.Get("/contact.html", h.ServeContact)
	public.Post("/api/v1/contact", h.HandleContact)
	public.Get("/pay/:token", h.ServePortal)
	public.Get("/invoice/:token", h.ServePortal)
	public.Get("/invoice/:token/success", h.ServeSuccess)

	// Email tracking endpoints (public - no auth required)
	public.Get("/api/track/open/:trackingId", h.TrackOpen)
	public.Get("/api/track/click/:linkId/:trackingId", h.TrackClick)

	return public
}

// PublicAPIRoutes configures public API endpoints
func PublicAPIRoutes(app *fiber.App, h *handlers.PublicHandler) fiber.Router {
	api := app.Group("/api/v1")

	api.Get("/invoice/:token", h.GetInvoiceByToken)
	api.Get("/invoice/:token/pdf", h.GetInvoicePDF)
	api.Get("/invoice/:token/receipt.pdf", h.GetInvoiceReceipt)
	api.Post("/payment/stk-push", h.InitiateSTKPush)
	api.Get("/payment/status/:token", h.CheckPaymentStatus)
	api.Get("/pricing", h.GetPricing)

	return api
}

// PublicAuthRoutes configures public authentication routes
// NOTE: /api/v1/auth/login, /api/v1/auth/register are handled by AuthRoutes via AuthHandler
func PublicAuthRoutes(app *fiber.App, h *handlers.PublicHandler) fiber.Router {
	auth := app.Group("/api/v1/public")

	auth.Post("/contact", h.HandleContact)

	return auth
}
