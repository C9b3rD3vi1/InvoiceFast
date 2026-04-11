package routes

import (
	"github.com/gofiber/fiber/v2"
)

// StaticRoutes serves the decoupled frontend pages from views/ directory
func StaticRoutes(app *fiber.App) fiber.Router {
	// Serve dashboard and SPA pages from views directory
	app.Get("/dashboard", func(c *fiber.Ctx) error {
		return c.SendFile("./views/dashboard/index.html")
	})
	app.Get("/invoices", func(c *fiber.Ctx) error {
		return c.SendFile("./views/invoices/index.html")
	})

	return app.Group("")
}
