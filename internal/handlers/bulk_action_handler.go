package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type BulkActionHandler struct {
	reminderService *services.ReminderService
}

func NewBulkActionHandler(reminderSvc *services.ReminderService) *BulkActionHandler {
	return &BulkActionHandler{
		reminderService: reminderSvc,
	}
}

func (h *BulkActionHandler) SendOverdueReminders(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	count, err := h.reminderService.BulkSendOverdueReminders(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":        "reminders sent",
		"reminders_sent": count,
	})
}
