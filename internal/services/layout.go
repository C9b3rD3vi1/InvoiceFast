package services

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type LayoutService struct{}

func NewLayoutService() *LayoutService {
	return &LayoutService{}
}

type LayoutData struct {
	Title        string
	TenantName   string
	UserName     string
	UserEmail    string
	UserInitials string
}

// RenderWithShell reads content file and wraps with shell template
func (s *LayoutService) RenderWithShell(c *fiber.Ctx, contentFile string, data LayoutData) error {
	content, err := os.ReadFile(contentFile)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Cannot read content: " + err.Error())
	}

	shell, err := os.ReadFile("./views/layouts/dashboard-shell.html")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Cannot read shell: " + err.Error())
	}

	result := strings.Replace(string(shell), "{{embed}}", string(content), 1)

	title := data.Title
	if title == "" {
		title = "Dashboard"
	}

	// Try to get actual user data from database if we have db
	userName := data.UserName
	userEmail := data.UserEmail
	tenantName := data.TenantName

	// These will be replaced at runtime with actual data from database
	// The Alpine.js will also fetch fresh user data client-side
	result = strings.ReplaceAll(result, "{{.Title}}", title)
	result = strings.ReplaceAll(result, "{{.TenantName}}", tenantName)
	result = strings.ReplaceAll(result, "{{.UserName}}", userName)
	result = strings.ReplaceAll(result, "{{.UserEmail}}", userEmail)
	result = strings.ReplaceAll(result, "{{.UserInitials}}", data.UserInitials)

	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.SendString(result)
}

// Helper to get user initials from name
func GetInitials(name string) string {
	if name == "" {
		return "U"
	}
	parts := strings.Fields(name)
	if len(parts) >= 2 {
		return strings.ToUpper(string(parts[0][0]) + string(parts[1][0]))
	}
	return strings.ToUpper(string(name[0]))
}
