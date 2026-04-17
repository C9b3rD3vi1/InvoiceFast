package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type LateFeeHandler struct {
	service *services.LateFeeService
}

func NewLateFeeHandler(svc *services.LateFeeService) *LateFeeHandler {
	return &LateFeeHandler{service: svc}
}

func (h *LateFeeHandler) GetConfig(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	config, err := h.service.GetConfig(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(config)
}

func (h *LateFeeHandler) UpdateConfig(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req services.UpdateLateFeeConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	config, err := h.service.UpdateConfig(tenantID, &req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(config)
}

func (h *LateFeeHandler) CalculateFee(c *fiber.Ctx) error {
	invoiceID := c.Params("invoiceID")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	fee, err := h.service.CalculateLateFee(invoiceID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"late_fee": fee})
}

func (h *LateFeeHandler) GetInvoiceLateFees(c *fiber.Ctx) error {
	invoiceID := c.Params("invoiceID")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	fees, err := h.service.GetLateFeesForInvoice(invoiceID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fees)
}

func (h *LateFeeHandler) WaiveLateFee(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	lateFeeID := c.Params("lateFeeID")

	if err := h.service.WaiveLateFee(lateFeeID, userID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "late fee waived"})
}
