package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type ReminderSequenceHandler struct {
	service *services.ReminderSequenceService
}

func NewReminderSequenceHandler(svc *services.ReminderSequenceService) *ReminderSequenceHandler {
	return &ReminderSequenceHandler{service: svc}
}

func (h *ReminderSequenceHandler) CreateSequence(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req services.CreateSequenceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	seq, err := h.service.CreateSequence(tenantID, &req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(seq)
}

func (h *ReminderSequenceHandler) GetSequences(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	sequences, err := h.service.GetSequences(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(sequences)
}

func (h *ReminderSequenceHandler) UpdateSequence(c *fiber.Ctx) error {
	sequenceID := c.Params("id")
	if sequenceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sequence ID required"})
	}

	var req services.UpdateSequenceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	seq, err := h.service.UpdateSequence(sequenceID, &req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(seq)
}

func (h *ReminderSequenceHandler) DeleteSequence(c *fiber.Ctx) error {
	sequenceID := c.Params("id")
	if sequenceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sequence ID required"})
	}

	if err := h.service.DeleteSequence(sequenceID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
