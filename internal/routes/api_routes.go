package routes

import (
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

var layoutService = services.NewLayoutService()

func getLayoutData(c *fiber.Ctx) services.LayoutData {
	// Default values - will be overridden by Alpine.js client-side
	return services.LayoutData{
		Title:        "",
		TenantName:   "Loading...",
		UserName:     "Loading...",
		UserEmail:    "Loading...",
		UserInitials: "L",
	}
}

// StaticRoutes serves frontend pages from views/
func StaticRoutes(app *fiber.App) fiber.Router {
	// Root redirects to dashboard
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/dashboard")
	})

	// Dashboard
	app.Get("/dashboard", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/dashboard.html", getLayoutData(c))
	})

	// Clients - all using shell
	clientRouter := app.Group("/clients")
	clientRouter.Get("/", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/clients.html", getLayoutData(c))
	})
	// Specific routes BEFORE /:id
	clientRouter.Get("/new", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/clients.html", getLayoutData(c))
	})
	clientRouter.Get("/:id", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/clients.html", getLayoutData(c))
	})

	// Invoices - index using shell
	invoiceRouter := app.Group("/invoices")
	invoiceRouter.Get("/", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/invoices.html", getLayoutData(c))
	})
	// IMPORTANT: specific routes must come BEFORE /:id
	invoiceRouter.Get("/new", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/invoice-create.html", getLayoutData(c))
	})
	invoiceRouter.Get("/analytics", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/invoices.html", getLayoutData(c))
	})
	invoiceRouter.Get("/public/:id", func(c *fiber.Ctx) error {
		return c.SendFile("./views/invoices/public.html")
	})
	// Invoice detail - using shell
	invoiceRouter.Get("/:id", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/invoice-view.html", getLayoutData(c))
	})
	invoiceRouter.Get("/:id/edit", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/invoice-create.html", getLayoutData(c))
	})

	// Payments - shell
	app.Get("/payments", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/payments.html", getLayoutData(c))
	})

	// Expenses - shell
	app.Get("/expenses", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/expenses.html", getLayoutData(c))
	})
	// IMPORTANT: specific routes must come BEFORE /:id
	app.Get("/expenses/new", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/expenses.html", getLayoutData(c))
	})
	app.Get("/expenses/:id", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/expenses.html", getLayoutData(c))
	})

	// Settings - shell with subsections as standalone
	settingsRouter := app.Group("/settings")
	settingsRouter.Get("/", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/settings.html", getLayoutData(c))
	})
	settingsRouter.Get("/profile", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/settings.html", getLayoutData(c))
	})

	// Billing - dedicated page
	settingsRouter.Get("/billing", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/billing.html", getLayoutData(c))
	})

	// Pricing - public page
	app.Get("/pricing", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/pricing.html", getLayoutData(c))
	})

	// Reports - shell
	app.Get("/reports", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/reports.html", getLayoutData(c))
	})

	// Automations - shell with new/edit using shell
	automationRouter := app.Group("/automations")
	automationRouter.Get("/", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/automations.html", getLayoutData(c))
	})
	// IMPORTANT: /new must come BEFORE /:id to avoid :id matching "new"
	automationRouter.Get("/new", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/automation-edit.html", getLayoutData(c))
	})
	automationRouter.Get("/:id/edit", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/automation-edit.html", getLayoutData(c))
	})

	// Notifications - shell
	app.Get("/notifications", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/notifications.html", getLayoutData(c))
	})

	// Public pages (no auth required)
	app.Get("/contact", func(c *fiber.Ctx) error {
		return c.SendFile("./views/pages/contact.html")
	})
	app.Get("/success", func(c *fiber.Ctx) error {
		return c.SendFile("./views/pages/success.html")
	})

	// Auth pages (no auth required)
	app.Get("/login", func(c *fiber.Ctx) error {
		return c.SendFile("./views/auth/login.html")
	})
	app.Get("/register", func(c *fiber.Ctx) error {
		return c.SendFile("./views/auth/register.html")
	})
	app.Get("/logout", func(c *fiber.Ctx) error {
		c.ClearCookie("token")
		c.ClearCookie("refresh_token")
		return c.Redirect("/login")
	})

	return app.Group("")
}
