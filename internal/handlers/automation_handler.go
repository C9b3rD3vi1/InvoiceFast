package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type AutomationHandler struct {
	automationService *services.AutomationService
}

func NewAutomationHandler(automationSvc *services.AutomationService) *AutomationHandler {
	return &AutomationHandler{automationService: automationSvc}
}

func (h *AutomationHandler) GetAutomations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	automations, err := h.automationService.GetAutomations(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"automations": automations})
}

func (h *AutomationHandler) GetAutomation(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	automation, err := h.automationService.GetAutomation(tenantID, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "automation not found"})
	}

	return c.JSON(automation)
}

func (h *AutomationHandler) CreateAutomation(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	userID := middleware.GetUserID(c)

	var req services.CreateAutomationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	automation, err := h.automationService.CreateAutomation(tenantID, userID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(automation)
}

func (h *AutomationHandler) UpdateAutomation(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")

	var req services.UpdateAutomationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	automation, err := h.automationService.UpdateAutomation(tenantID, id, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(automation)
}

func (h *AutomationHandler) DeleteAutomation(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")

	if err := h.automationService.DeleteAutomation(tenantID, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *AutomationHandler) RunAutomation(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")

	result, err := h.automationService.RunAutomation(tenantID, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *AutomationHandler) GetAutomationLogs(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	
	// Get limit from query param, default to 100
	limit := c.QueryInt("limit", 100)

	logs, err := h.automationService.GetLogs(tenantID, id, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"logs": logs})
}
