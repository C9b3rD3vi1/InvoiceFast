package handlers

import(
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"
	
	"github.com/gofiber/fiber/v2"
	
)



func (h *FiberHandler) CreateClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Phone   string `json:"phone"`
		Address string `json:"address"`
		KRAPIN  string `json:"kra_pin"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	userID := middleware.GetUserID(c)
	client, err := h.clientService.CreateClient(tenantID, userID, &services.CreateClientRequest{
		Name:    req.Name,
		Email:   req.Email,
		Phone:   req.Phone,
		Address: req.Address,
		KRAPIN:  req.KRAPIN,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(client)
}

func (h *FiberHandler) GetClients(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	clients, _, err := h.clientService.GetUserClients(tenantID, services.ClientFilter{})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(clients)
}

func (h *FiberHandler) GetClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	clientID := c.Params("id")

	client, err := h.clientService.GetClient(clientID, tenantID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "client not found"})
	}

	return c.JSON(client)
}

func (h *FiberHandler) UpdateClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	clientID := c.Params("id")

	var req struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Phone   string `json:"phone"`
		Address string `json:"address"`
		KRAPIN  string `json:"kra_pin"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	client, err := h.clientService.UpdateClient(clientID, tenantID, &services.UpdateClientRequest{
		Name:    &req.Name,
		Email:   &req.Email,
		Phone:   &req.Phone,
		Address: &req.Address,
		KRAPIN:  &req.KRAPIN,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(client)
}

func (h *FiberHandler) DeleteClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	clientID := c.Params("id")

	if err := h.clientService.DeleteClient(clientID, tenantID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "client deleted"})
}

func (h *FiberHandler) GetClientStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	clientID := c.Params("id")

	stats, err := h.clientService.GetClientStats(tenantID, clientID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}
