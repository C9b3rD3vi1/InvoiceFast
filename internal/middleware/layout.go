package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

func LayoutMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()

		if strings.HasPrefix(path, "/dashboard") ||
			strings.HasPrefix(path, "/invoices") ||
			strings.HasPrefix(path, "/clients") ||
			strings.HasPrefix(path, "/settings") ||
			strings.HasPrefix(path, "/payments") ||
			strings.HasPrefix(path, "/htmx") {
			c.Locals("layout", "layouts/dashboard")
		} else {
			c.Locals("layout", "layouts/main")
		}

		return c.Next()
	}
}
