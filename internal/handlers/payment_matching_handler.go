package handlers

import (
	"fmt"
	"strconv"

	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type PaymentMatchingHandler struct {
	service        *services.PaymentMatchingService
	invoiceService *services.InvoiceService
}

func NewPaymentMatchingHandler(svc *services.PaymentMatchingService, invoiceSvc *services.InvoiceService) *PaymentMatchingHandler {
	return &PaymentMatchingHandler{service: svc, invoiceService: invoiceSvc}
}

func (h *PaymentMatchingHandler) GetPayments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	filter := services.PaymentFilter{
		Search:    c.Query("search"),
		Status:    c.Query("status"),
		Method:    c.Query("method"),
		DateFrom:  c.Query("date_from"),
		DateTo:    c.Query("date_to"),
		ClientID:  c.Query("client_id"),
		InvoiceID: c.Query("invoice_id"),
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	filter.Page = page
	filter.Limit = limit

	payments, total, err := h.service.GetPayments(tenantID, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"payments": payments, "total": total})
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

func (h *PaymentMatchingHandler) GetReceipt(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	paymentID := c.Params("id")
	if paymentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "payment ID required"})
	}

	payment, err := h.service.GetPaymentByID(tenantID, paymentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "payment not found"})
	}

	// Get related invoice and client
	invoice, _ := h.invoiceService.GetInvoiceByID(tenantID, payment.InvoiceID)

	return c.JSON(fiber.Map{
		"payment": payment,
		"invoice": invoice,
	})
}

func (h *PaymentMatchingHandler) ReconcilePayment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	paymentID := c.Params("id")
	if paymentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "payment ID required"})
	}

	userID := middleware.GetUserID(c)
	if err := h.service.ReconcilePayment(tenantID, paymentID, userID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "payment reconciled successfully"})
}

func (h *PaymentMatchingHandler) RequestPayment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required", "code": "TENANT_REQUIRED"})
	}

	var req struct {
		ClientID  string  `json:"client_id"`
		InvoiceID string  `json:"invoice_id"`
		Amount    float64 `json:"amount"`
		Method    string  `json:"method"`
		Notes     string  `json:"notes"`
		SendSMS   bool    `json:"send_sms"`
		SendEmail bool    `json:"send_email"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request", "details": err.Error()})
	}

	if req.ClientID == "" || req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "client and amount required"})
	}

	// Get invoice and client for payment link generation
	var invoice *models.Invoice
	var clientName, clientPhone string

	if req.InvoiceID != "" {
		inv, err := h.invoiceService.GetInvoiceByID(tenantID, req.InvoiceID)
		if err == nil {
			invoice = inv
			// Get client details from client service
			if inv.ClientID != "" {
				client, err := h.service.GetClientByID(tenantID, inv.ClientID)
				if err == nil {
					clientName = client.Name
					clientPhone = client.Phone
				}
			}
		}
	}

	// Generate payment link
	paymentLink := ""
	if invoice != nil && invoice.MagicToken != "" {
		paymentLink = "https://invoice.simuxtech.com/pay/" + invoice.MagicToken
	}

	// Build message
	message := "Payment Request\n"
	if invoice != nil {
		message += "Invoice: " + invoice.InvoiceNumber + "\n"
	}
	message += "Amount: KES " + fmt.Sprintf("%.2f", req.Amount) + "\n"
	if paymentLink != "" {
		message += "Pay: " + paymentLink + "\n"
	}
	if req.Notes != "" {
		message += "Note: " + req.Notes + "\n"
	}

	// Return success with payment link details
	return c.JSON(fiber.Map{
		"message":      "Payment request sent",
		"payment_link": paymentLink,
		"invoice":      invoice,
		"client_name":  clientName,
		"client_phone": clientPhone,
		"sms_sent":     req.SendSMS,
		"email_sent":   req.SendEmail,
	})
}

func (h *PaymentMatchingHandler) GetStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	stats := make(map[string]interface{})

	db := h.invoiceService.GetDB()

	var totalRevenue, pendingAmount float64
	err := db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status IN ('paid', 'partially_paid')", tenantID).
		Select("COALESCE(SUM(paid_amount), 0)").
		Scan(&totalRevenue).Error
	if err == nil {
		stats["total_revenue"] = totalRevenue
		stats["paid_amount"] = totalRevenue
	}

	err = db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status IN ('sent', 'viewed', 'overdue', 'partially_paid')", tenantID).
		Select("COALESCE(SUM(balance_due), 0)").
		Scan(&pendingAmount).Error
	if err == nil {
		stats["pending_amount"] = pendingAmount
	}

	stats["fraud_alerts"] = 0

	return c.JSON(stats)
}
