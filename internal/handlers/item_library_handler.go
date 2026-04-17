package handlers

import (
	"net/http"

	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// ItemLibraryHandler handles item library API endpoints
type ItemLibraryHandler struct {
	itemLibraryService *services.ItemLibraryService
}

// NewItemLibraryHandler creates ItemLibraryHandler
func NewItemLibraryHandler(itemLibrarySvc *services.ItemLibraryService) *ItemLibraryHandler {
	return &ItemLibraryHandler{itemLibraryService: itemLibrarySvc}
}

// CreateItem - create new item in library
func (h *ItemLibraryHandler) CreateItem(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "user ID required"})
	}

	var req services.CreateItemRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	item, err := h.itemLibraryService.CreateItem(tenantID, userID, &req)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(http.StatusCreated).JSON(item)
}

// GetItems - get all items for tenant
func (h *ItemLibraryHandler) GetItems(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	search := c.Query("q")
	items, err := h.itemLibraryService.GetItems(tenantID, search)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(items)
}

// GetItem - get specific item by ID
func (h *ItemLibraryHandler) GetItem(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	itemID := c.Params("id")
	if itemID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "item ID required"})
	}

	item, err := h.itemLibraryService.GetItemByID(tenantID, itemID)
	if err != nil {
		if err.Error() == "item not found" {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "item not found"})
		}
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(item)
}

// UpdateItem - update existing item
func (h *ItemLibraryHandler) UpdateItem(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	itemID := c.Params("id")
	if itemID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "item ID required"})
	}

	var req services.UpdateItemRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	item, err := h.itemLibraryService.UpdateItem(tenantID, itemID, &req)
	if err != nil {
		if err.Error() == "item not found" {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "item not found"})
		}
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(item)
}

// DeleteItem - delete item from library
func (h *ItemLibraryHandler) DeleteItem(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	itemID := c.Params("id")
	if itemID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "item ID required"})
	}

	err := h.itemLibraryService.DeleteItem(tenantID, itemID)
	if err != nil {
		if err.Error() == "item not found" {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "item not found"})
		}
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(http.StatusNoContent)
}
