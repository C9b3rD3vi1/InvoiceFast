package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// StaticRoutes serves frontend pages from views/
func StaticRoutes(app *fiber.App) fiber.Router {
	// Client pages - MUST come first, before any other /clients routes
	clientRouter := app.Group("/clients")

	clientRouter.Get("/", func(c *fiber.Ctx) error {
		println("[ROUTE] GET /clients -> index.html")
		return c.SendFile("./views/clients/index.html")
	})

	clientRouter.Get("/new", func(c *fiber.Ctx) error {
		println("[ROUTE] GET /clients/new -> index.html")
		return c.SendFile("./views/clients/index.html")
	})

	clientRouter.Get("/:id", func(c *fiber.Ctx) error {
		clientID := c.Params("id")
		println("[ROUTE] GET /clients/", clientID, " -> view.html")
		return c.SendFile("./views/clients/view.html")
	})

	// Public pages (in views/pages/) - / is handled by PublicRoutes
	app.Get("/contact", func(c *fiber.Ctx) error {
		return c.SendFile("./views/pages/contact.html")
	})
	app.Get("/success", func(c *fiber.Ctx) error {
		return c.SendFile("./views/pages/success.html")
	})

	// Dashboard pages (SPA with Alpine.js)
	app.Get("/dashboard", func(c *fiber.Ctx) error {
		return c.SendFile("./views/dashboard/index.html")
	})

	// Auth pages (in views/auth/)
	app.Get("/login", func(c *fiber.Ctx) error {
		return c.SendFile("./views/auth/login.html")
	})
	app.Get("/register", func(c *fiber.Ctx) error {
		return c.SendFile("./views/auth/register.html")
	})

	// Reports pages
	app.Get("/reports", func(c *fiber.Ctx) error {
		return c.SendFile("./views/reports/index.html")
	})

	// Invoice pages - MUST order specific routes before parameterized routes
	invoiceRouter := app.Group("/invoices")

	// Specific routes FIRST
	invoiceRouter.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile("./views/invoices/index.html")
	})

	invoiceRouter.Get("/new", func(c *fiber.Ctx) error {
		return c.SendFile("./views/invoices/new.html")
	})

	invoiceRouter.Get("/analytics", func(c *fiber.Ctx) error {
		return c.SendFile("./views/invoices/analytics.html")
	})

	invoiceRouter.Get("/public/:id", func(c *fiber.Ctx) error {
		return c.SendFile("./views/invoices/public.html")
	})

	// Parameterized routes LAST
	invoiceRouter.Get("/:id", func(c *fiber.Ctx) error {
		return c.SendFile("./views/invoices/view.html")
	})

	invoiceRouter.Get("/:id/edit", func(c *fiber.Ctx) error {
		return c.SendFile("./views/invoices/edit.html")
	})

	// Catch-all for other SPA routes
	app.Use(func(c *fiber.Ctx) error {
		path := c.Path()
		if strings.HasPrefix(path, "/invoices") {
			return c.SendFile("./views/invoices/index.html")
		}
		if strings.HasPrefix(path, "/profile") {
			return c.SendFile("./views/settings/profile.html")
		}
		if strings.HasPrefix(path, "/payments") {
			return c.SendFile("./views/payments/index.html")
		}
		if strings.HasPrefix(path, "/settings") {
			return c.SendFile("./views/settings/index.html")
		}
		if strings.HasPrefix(path, "/billing") {
			return c.SendFile("./views/settings/billing.html")
		}
		if strings.HasPrefix(path, "/automations") {
			if path == "/automations" || path == "/automations/" {
				return c.SendFile("./views/automations/index.html")
			}
			if strings.HasSuffix(path, "/new") || strings.HasSuffix(path, "/new/") {
				return c.SendFile("./views/automations/new.html")
			}
			if strings.Contains(path, "/edit") {
				return c.SendFile("./views/automations/new.html")
			}
			return c.SendFile("./views/automations/index.html")
		}
		if strings.HasPrefix(path, "/notifications") {
			return c.SendFile("./views/notifications/index.html")
		}
		return c.Next()
	})

	return app.Group("")
}
