package routes

import (
	"github.com/gofiber/fiber/v2"
)

// StaticRoutes serves the decoupled frontend pages
func StaticRoutes(app *fiber.App) fiber.Router {
	// Pages should already be handled by the Static middleware
	// but we need at least one route to make this function work
	return app.Group("")
}
