package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type IntegrationHandler struct {
	integrationService *services.IntegrationService
}

func NewIntegrationHandler(integrationSvc *services.IntegrationService) *IntegrationHandler {
	return &IntegrationHandler{
		integrationService: integrationSvc,
	}
}

func (h *IntegrationHandler) GetIntegrations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	integrations, err := h.integrationService.GetIntegrations(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(integrations)
}

func (h *IntegrationHandler) GetIntegration(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	provider := c.Params("provider")
	integration, err := h.integrationService.GetIntegrationByProvider(tenantID, provider)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if integration == nil {
		return c.JSON(nil)
	}

	return c.JSON(integration)
}

type SaveIntegrationRequest struct {
	Name        string                        `json:"name"`
	Description string                        `json:"description"`
	Config      services.IntegrationConfig     `json:"config"`
}

func (h *IntegrationHandler) SaveIntegration(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	provider := c.Params("provider")
	if provider == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "provider required"})
	}

	var req SaveIntegrationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	integration, err := h.integrationService.SaveIntegration(tenantID, provider, req.Name, req.Description, &req.Config)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(integration)
}

func (h *IntegrationHandler) DeleteIntegration(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	integrationID := c.Params("id")
	if integrationID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "integration ID required"})
	}

	err := h.integrationService.DeleteIntegration(tenantID, integrationID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *IntegrationHandler) ToggleIntegration(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	integrationID := c.Params("id")
	if integrationID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "integration ID required"})
	}

	var req struct {
		Active bool `json:"active"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	err := h.integrationService.ToggleIntegration(tenantID, integrationID, req.Active)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

func (h *IntegrationHandler) GetIntegrationConfig(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	provider := c.Params("provider")
	config, err := h.integrationService.GetIntegrationConfig(tenantID, provider)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(config)
}