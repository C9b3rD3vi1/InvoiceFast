package routes

import (
	"github.com/gofiber/fiber/v2"
)

// StaticRoutes serves frontend pages from views/
func StaticRoutes(app *fiber.App) fiber.Router {
	// Public pages (in views/pages/)
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile("./views/pages/landing.html")
	})
	app.Get("/contact", func(c *fiber.Ctx) error {
		return c.SendFile("./views/pages/contact.html")
	})
	app.Get("/success", func(c *fiber.Ctx) error {
		return c.SendFile("./views/pages/success.html")
	})
	app.Get("/dashboard", func(c *fiber.Ctx) error {
		return c.SendFile("./views/pages/dashboard.html")
	})

	// SPA pages (Alpine.js)
	app.Get("/dashboard", func(c *fiber.Ctx) error {
		return c.SendFile("./views/dashboard/index.html")
	})
	app.Get("/invoices", func(c *fiber.Ctx) error {
		return c.SendFile("./views/invoices/index.html")
	})
	app.Get("/clients", func(c *fiber.Ctx) error {
		return c.SendFile("./views/clients/index.html")
	})

	// Auth pages (in views/auth/)
	app.Get("/login", func(c *fiber.Ctx) error {
		return c.SendFile("./views/auth/login.html")
	})
	app.Get("/register", func(c *fiber.Ctx) error {
		return c.SendFile("./views/auth/register.html")
	})

	return app.Group("")
}
