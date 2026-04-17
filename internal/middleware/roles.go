package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	RoleAdmin   = "admin"
	RoleOwner   = "owner"
	RoleManager = "manager"
	RoleStaff   = "staff"
	RoleUser    = "user"
	RoleViewer  = "viewer"

	RoleKey      = "user_role"
	UserEmailKey = "user_email"
)

var roleHierarchy = map[string]int{
	RoleAdmin:   5,
	RoleOwner:   5,
	RoleManager: 4,
	RoleStaff:   3,
	RoleUser:    2,
	RoleViewer:  1,
}

func HasRole(required string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRole := GetUserRole(c)
		if userRole == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "role required"})
		}

		if !hasPermission(userRole, required) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":    "insufficient permissions",
				"required": required,
				"current":  userRole,
			})
		}

		return c.Next()
	}
}

func HasAnyRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRole := GetUserRole(c)
		if userRole == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "role required"})
		}

		for _, role := range roles {
			if hasPermission(userRole, role) {
				return c.Next()
			}
		}

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":    "insufficient permissions",
			"required": strings.Join(roles, " or "),
			"current":  userRole,
		})
	}
}

func hasPermission(userRole, required string) bool {
	userLevel := roleHierarchy[userRole]
	requiredLevel := roleHierarchy[required]

	if userLevel == 0 || requiredLevel == 0 {
		return false
	}

	return userLevel >= requiredLevel
}

func GetUserRole(c *fiber.Ctx) string {
	role, ok := c.Locals(RoleKey).(string)
	if !ok {
		return ""
	}
	return role
}

func GetUserEmail(c *fiber.Ctx) string {
	email, ok := c.Locals(UserEmailKey).(string)
	if !ok {
		return ""
	}
	return email
}

func RequireAdmin() fiber.Handler {
	return HasRole(RoleAdmin)
}

func RequireManager() fiber.Handler {
	return HasRole(RoleManager)
}

func RequireOwnerOrAdmin() fiber.Handler {
	return HasAnyRole(RoleOwner, RoleAdmin)
}

func CanEditInvoice() fiber.Handler {
	return HasAnyRole(RoleAdmin, RoleOwner, RoleManager, RoleStaff)
}

func CanDeleteInvoice() fiber.Handler {
	return HasAnyRole(RoleAdmin, RoleOwner, RoleManager)
}

func CanManageUsers() fiber.Handler {
	return HasAnyRole(RoleAdmin, RoleOwner, RoleManager)
}

func CanViewReports() fiber.Handler {
	return HasAnyRole(RoleAdmin, RoleOwner, RoleManager, RoleStaff)
}

func CanManageSettings() fiber.Handler {
	return HasAnyRole(RoleAdmin, RoleOwner, RoleManager)
}
