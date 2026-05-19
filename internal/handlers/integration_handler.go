package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type IntegrationHandler struct {
	integrationService *services.IntegrationService
	quickBooksService *services.QuickBooksService
}

func NewIntegrationHandler(integrationSvc *services.IntegrationService, qbSvc *services.QuickBooksService) *IntegrationHandler {
	return &IntegrationHandler{
		integrationService: integrationSvc,
		quickBooksService: qbSvc,
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

func (h *IntegrationHandler) QuickBooksConnect(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	if h.quickBooksService == nil || !h.quickBooksService.IsEnabled() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "QuickBooks integration not configured"})
	}

	authURL, err := h.quickBooksService.GetAuthorizationURL(tenantID, "")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"authorization_url": authURL})
}

func (h *IntegrationHandler) QuickBooksCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing code or state"})
	}

	tokens, tenantID, err := h.quickBooksService.HandleOAuthCallback(code, state)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.quickBooksService.SaveIntegration(tenantID, tokens); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save integration"})
	}

	return c.JSON(fiber.Map{"message": "QuickBooks connected successfully"})
}

func (h *IntegrationHandler) QuickBooksDisconnect(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	if err := h.quickBooksService.Disconnect(tenantID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "QuickBooks disconnected"})
}

func (h *IntegrationHandler) QuickBooksTest(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	connected, err := h.quickBooksService.TestConnection(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error(), "connected": false})
	}

	return c.JSON(fiber.Map{"connected": connected})
}

func (h *IntegrationHandler) QuickBooksSync(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	count, err := h.quickBooksService.SyncInvoices(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error(), "synced": 0})
	}

	return c.JSON(fiber.Map{"synced_invoices": count, "message": "Sync completed"})
}