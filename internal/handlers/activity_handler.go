package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type ActivityHandler struct {
	service *services.ActivityService
}

func NewActivityHandler(svc *services.ActivityService) *ActivityHandler {
	return &ActivityHandler{service: svc}
}

func (h *ActivityHandler) GetRecentActivity(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	limit := c.QueryInt("limit", 20)
	if limit > 100 {
		limit = 100
	}

	activities, err := h.service.GetRecentActivity(tenantID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(activities)
}
