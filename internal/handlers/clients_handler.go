package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// ClientHandler handles client API endpoints
type ClientHandler struct {
	clientService *services.ClientService
	subService    *services.SubscriptionService
}

// NewClientHandler creates ClientHandler
func NewClientHandler(clientSvc *services.ClientService, subSvc *services.SubscriptionService) *ClientHandler {
	return &ClientHandler{clientService: clientSvc, subService: subSvc}
}

// CreateClient - create new client
func (h *ClientHandler) CreateClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	if h.subService != nil {
		allowed, reason, _ := h.subService.CheckLimits(tenantID, "clients", 1)
		if !allowed {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error":   "Client limit exceeded",
				"reason":  reason,
				"upgrade": "/billing/upgrade",
			})
		}
	}

	var req services.CreateClientRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	userID := middleware.GetUserID(c)
	client, err := h.clientService.CreateClient(tenantID, userID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if h.subService != nil {
		h.subService.IncrementUsage(tenantID, "clients", 1)
	}

	return c.Status(fiber.StatusCreated).JSON(client)
}

// GetClients - list clients
func (h *ClientHandler) GetClients(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	filter := services.ClientFilter{
		Search: c.Query("search"),
		Limit:  20,
	}

	clients, total, err := h.clientService.GetUserClients(tenantID, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"clients": clients, "total": total})
}

// GetClient - get single client
func (h *ClientHandler) GetClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	clientID := c.Params("id")
	client, err := h.clientService.GetClient(tenantID, clientID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "client not found"})
	}

	return c.JSON(client)
}

// UpdateClient - update client
func (h *ClientHandler) UpdateClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	clientID := c.Params("id")
	var req services.UpdateClientRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	client, err := h.clientService.UpdateClient(clientID, tenantID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(client)
}

// DeleteClient - delete client
func (h *ClientHandler) DeleteClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	clientID := c.Params("id")
	if err := h.clientService.DeleteClient(clientID, tenantID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Client deleted"})
}

// GetClientStats - get client stats
func (h *ClientHandler) GetClientStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	clientID := c.Params("id")
	stats, err := h.clientService.GetClientStats(clientID, tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}
