package utils

import (
	"github.com/gofiber/fiber/v2"
)

// IsHTMXRequest checks if the request is from HTMX
func IsHTMXRequest(c *fiber.Ctx) bool {
	return c.Get("HX-Request") == "true"
}

// IsHTMXBoosted checks if HTMX boosted the request
func IsHTMXBoosted(c *fiber.Ctx) bool {
	return c.Get("HX-Boosted") == "true"
}

// GetHTMXTrigger returns the HTMX trigger element ID
func GetHTMXTrigger(c *fiber.Ctx) string {
	return c.Get("HX-Trigger")
}

// GetHTMXTarget returns the HTMX target element ID
func GetHTMXTarget(c *fiber.Ctx) string {
	return c.Get("HX-Target")
}

// GetHTMMPrompt returns the HTMX prompt response
func GetHTMMPrompt(c *fiber.Ctx) string {
	return c.Get("HX-Prompt")
}

// RenderHTMXOrFullPage renders different templates based on HTMX request
func RenderHTMXOrFullPage(c *fiber.Ctx, htmxTemplate, fullTemplate string, data fiber.Map) error {
	if IsHTMXRequest(c) {
		return c.Render(htmxTemplate, data)
	}
	return c.Render(fullTemplate, data)
}

// RenderHTMXFragment renders only the HTMX fragment
func RenderHTMXFragment(c *fiber.Ctx, template string, data fiber.Map) error {
	return c.Render(template, data)
}

// SetHXRedirect sets the HTMX redirect header
func SetHXRedirect(c *fiber.Ctx, location string) {
	c.Set("HX-Redirect", location)
}

// SetHXRetarget sets the HTMX retarget header
func SetHXRetarget(c *fiber.Ctx, target string) {
	c.Set("HX-Retarget", target)
}

// SetHXReswap sets the HTMX reswap header
func SetHXReswap(c *fiber.Ctx, method string) {
	c.Set("HX-Reswap", method)
}

// RefreshHTMX triggers a full page refresh via HTMX
func RefreshHTMX(c *fiber.Ctx) {
	c.Set("HX-Refresh", "true")
}
