package routes

import (
	"invoicefast/internal/handlers"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

var layoutService = services.NewLayoutService()


// MetricsRoutes adds Prometheus metrics endpoint
func MetricsRoutes(app *fiber.App) {
	app.Get("/metrics", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).SendString("# Metrics endpoint - use /api/v1/metrics for JSON")
	})
	
	app.Get("/api/v1/metrics", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "ok",
			"message": "Use /metrics for Prometheus format",
		})
	})
}

func getLayoutData(c *fiber.Ctx) services.LayoutData {
	// Default values - will be overridden by Alpine.js client-side
	return services.LayoutData{
		Title:        "",
		TenantName:   "",
		UserName:     "",
		UserEmail:    "",
		UserInitials: "",
	}
}

// StaticRoutes serves frontend pages from views/
func StaticRoutes(app *fiber.App, authHandler *handlers.AuthHandler) fiber.Router {
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

	// Reports - shell
	app.Get("/reports", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/reports.html", getLayoutData(c))
	})

	// KRA Compliance - shell
	app.Get("/kra-compliance", func(c *fiber.Ctx) error {
		return layoutService.RenderWithShell(c, "./views/content/kra.html", getLayoutData(c))
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
	app.Get("/features", func(c *fiber.Ctx) error {
		return layoutService.RenderPublicWithShell(c, "./views/pages/features.html", "Features")
	})
	app.Get("/pricing", func(c *fiber.Ctx) error {
		return layoutService.RenderPublicWithShell(c, "./views/pages/pricing.html", "Pricing")
	})
	app.Get("/faq", func(c *fiber.Ctx) error {
		return layoutService.RenderPublicWithShell(c, "./views/pages/faq.html", "FAQ")
	})
	app.Get("/contact", func(c *fiber.Ctx) error {
		return layoutService.RenderPublicWithShell(c, "./views/pages/contact.html", "Contact Support")
	})
	app.Get("/success", func(c *fiber.Ctx) error {
		return c.SendFile("./views/pages/success.html")
	})

	// Email verification link handler (no auth required — token is in the link)
	app.Get("/verify-email", authHandler.HandleVerifyEmailLink)

	// Auth pages (no auth required)
	app.Get("/login", func(c *fiber.Ctx) error {
		return layoutService.RenderPublicWithShell(c, "./views/auth/login.html", "Sign In")
	})
	app.Get("/register", func(c *fiber.Ctx) error {
		return c.Redirect("/onboarding")
	})
	app.Get("/forgot-password", func(c *fiber.Ctx) error {
		return layoutService.RenderPublicWithShell(c, "./views/auth/forgot-password.html", "Forgot Password")
	})
	app.Get("/reset-password", func(c *fiber.Ctx) error {
		return layoutService.RenderPublicWithShell(c, "./views/auth/reset-password.html", "Reset Password")
	})
	app.Get("/logout", func(c *fiber.Ctx) error {
		c.ClearCookie("token")
		c.ClearCookie("refresh_token")
		return c.Redirect("/login")
	})

	return app.Group("")
}
