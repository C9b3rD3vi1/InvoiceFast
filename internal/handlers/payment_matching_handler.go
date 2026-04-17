package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type PaymentMatchingHandler struct {
	service *services.PaymentMatchingService
}

func NewPaymentMatchingHandler(svc *services.PaymentMatchingService) *PaymentMatchingHandler {
	return &PaymentMatchingHandler{service: svc}
}

func (h *PaymentMatchingHandler) GetUnallocated(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	payments, err := h.service.GetUnallocated(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(payments)
}

func (h *PaymentMatchingHandler) MatchPayment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	paymentID := c.Params("id")
	if paymentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "payment ID required"})
	}

	var req struct {
		InvoiceID string `json:"invoice_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.InvoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	userID := middleware.GetUserID(c)
	if err := h.service.MatchPayment(paymentID, req.InvoiceID, userID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "payment matched successfully"})
}

func (h *PaymentMatchingHandler) ManualMatch(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req struct {
		InvoiceID string  `json:"invoice_id"`
		Reference string  `json:"reference"`
		Phone     string  `json:"phone"`
		Amount    float64 `json:"amount"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.InvoiceID == "" || req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID and amount required"})
	}

	userID := middleware.GetUserID(c)
	if err := h.service.ManualMatch(tenantID, req.InvoiceID, req.Reference, req.Phone, req.Amount, userID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "payment matched successfully"})
}
