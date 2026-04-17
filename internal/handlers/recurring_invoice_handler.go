package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type RecurringInvoiceHandler struct {
	service *services.RecurringInvoiceService
}

func NewRecurringInvoiceHandler(svc *services.RecurringInvoiceService) *RecurringInvoiceHandler {
	return &RecurringInvoiceHandler{service: svc}
}

func (h *RecurringInvoiceHandler) ListRecurring(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoices, err := h.service.GetRecurringInvoices(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(invoices)
}

func (h *RecurringInvoiceHandler) EnableRecurring(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("invoiceID")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	var req struct {
		Frequency string `json:"frequency"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Frequency == "" {
		req.Frequency = "monthly"
	}

	if err := h.service.EnableRecurring(invoiceID, req.Frequency); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "recurring enabled"})
}

func (h *RecurringInvoiceHandler) DisableRecurring(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("invoiceID")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	if err := h.service.DisableRecurring(invoiceID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "recurring disabled"})
}

func (h *RecurringInvoiceHandler) ProcessRecurring(c *fiber.Ctx) error {
	if err := h.service.ProcessRecurringInvoices(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "recurring invoices processed"})
}
