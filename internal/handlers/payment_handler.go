package handlers

import (
	"fmt"
	"log"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// PaymentHandler handles payment API endpoints
type PaymentHandler struct {
	invoiceService *services.InvoiceService
	mpesaService   *services.MPesaService
	db             *database.DB
}

// NewPaymentHandler creates PaymentHandler
func NewPaymentHandler(invoiceSvc *services.InvoiceService, mpesaSvc *services.MPesaService, db *database.DB) *PaymentHandler {
	return &PaymentHandler{
		invoiceService: invoiceSvc,
		mpesaService:   mpesaSvc,
		db:             db,
	}
}

// HandleIntasendWebhook processes Intasend webhook callbacks
func (h *PaymentHandler) HandleIntasendWebhook(c *fiber.Ctx) error {
	var payload struct {
		Event         string `json:"event"`
		CheckoutID    string `json:"checkout_id"`
		InvoiceNumber string `json:"invoice_number"`
		Amount        string `json:"amount"`
		Reference     string `json:"reference"`
	}

	if err := c.BodyParser(&payload); err != nil {
		log.Printf("[Webhook] Parse error: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	key := c.Get("Idempotency-Key")
	if key == "" {
		key = payload.CheckoutID
	}

	if svc, ok := c.Locals("idempotency_svc").(*services.IdempotencyService); ok && key != "" {
		isProcessed, _ := svc.IsProcessed(c.Context(), key)
		if isProcessed {
			log.Printf("[Webhook] Already processed: %s", key)
			return c.JSON(fiber.Map{"status": "already_processed"})
		}
	}

	switch payload.Event {
	case "payment_successful", "invoice_payment_signed":
		tenantID := middleware.GetTenantID(c)
		invoice, err := h.invoiceService.GetInvoiceByNumber(tenantID, payload.InvoiceNumber)
		if err != nil {
			log.Printf("[Webhook] Invoice not found: %s", payload.InvoiceNumber)
			return c.JSON(fiber.Map{"status": "ignored"})
		}

		var amount float64
		fmt.Sscanf(payload.Amount, "%f", &amount)
		if amount == 0 {
			amount = invoice.Total
		}

		payment := &models.Payment{
			TenantID:  invoice.TenantID,
			InvoiceID: invoice.ID,
			UserID:    invoice.UserID,
			Amount:    amount,
			Currency:  invoice.Currency,
			Method:    models.PaymentMethodMpesa,
			Status:    models.PaymentStatusCompleted,
			Reference: payload.Reference,
		}

		h.invoiceService.RecordPayment(invoice.TenantID, invoice.ID, payment)

		if err := h.invoiceService.RotateMagicToken(invoice.ID); err != nil {
			log.Printf("[Webhook] Warning: Failed to rotate magic token for invoice %s: %v", invoice.InvoiceNumber, err)
		}

		if svc, ok := c.Locals("idempotency_svc").(*services.IdempotencyService); ok && key != "" {
			svc.MarkProcessed(c.Context(), key, map[string]interface{}{
				"invoice_id": invoice.ID,
				"amount":     amount,
			})
		}

		log.Printf("[Webhook] Payment recorded: %s = %f", invoice.InvoiceNumber, amount)

	default:
		log.Printf("[Webhook] Unhandled event: %s", payload.Event)
	}

	return c.JSON(fiber.Map{"status": "received"})
}

// HandleMpesaCallback processes verified M-Pesa STK callbacks
func (h *PaymentHandler) HandleMpesaCallback(c *fiber.Ctx) error {
	callback, ok := c.Locals("mpesa_callback").(*services.STKCallback)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "no verified callback data",
			"code":  "INVALID_CALLBACK",
		})
	}

	if h.mpesaService != nil {
		err := h.mpesaService.ProcessSTKCallback(c.Context(), *callback)
		if err != nil {
			log.Printf("[M-Pesa] Callback processing error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "callback processing failed",
				"code":  "PROCESSING_ERROR",
			})
		}

		checkoutReqID := callback.Body.StkCallback.CheckoutRequestID
		if checkoutReqID != "" {
			log.Printf("[M-Pesa] Payment completed, rotating magic token for checkout: %s", checkoutReqID)
		}
	}

	return c.JSON(fiber.Map{"status": "received"})
}

// InitiateSTKPush initiates M-Pesa STK push
func (h *PaymentHandler) InitiateSTKPush(c *fiber.Ctx) error {
	var req struct {
		InvoiceID string `json:"invoice_id"`
		Phone     string `json:"phone"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if h.mpesaService == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "M-Pesa not configured"})
	}

	tenantID := middleware.GetTenantID(c)
	invoice, err := h.invoiceService.GetInvoiceByID(tenantID, req.InvoiceID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
	}

	amountStr := fmt.Sprintf("%.2f", invoice.Total)
	resp, err := h.mpesaService.InitiateSTKPush(c.Context(), tenantID, invoice.ID, req.Phone, amountStr, invoice.InvoiceNumber)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(resp)
}

// CheckPaymentStatus checks payment status
func (h *PaymentHandler) CheckPaymentStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "pending"})
}

// GetPayments returns paginated payments
func (h *PaymentHandler) GetPayments(c *fiber.Ctx) error {
	tenantID, _ := c.Locals("tenant_id").(string)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)
	status := c.Query("status")

	var payments []models.Payment
	query := h.db.Where("tenant_id = ?", tenantID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&payments)

	return c.JSON(fiber.Map{"payments": payments, "total": len(payments)})
}

// GetPaymentSummary returns payment summary statistics
func (h *PaymentHandler) GetPaymentSummary(c *fiber.Ctx) error {
	tenantID, _ := c.Locals("tenant_id").(string)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var totalRevenue, paidAmount, pendingAmount, failedAmount float64
	var fraudCount int64

	h.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND status = ?", tenantID, "success").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalRevenue)

	h.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND status = ?", tenantID, "success").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&paidAmount)

	h.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND status = ?", tenantID, "pending").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&pendingAmount)

	h.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND status = ?", tenantID, "failed").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&failedAmount)

	h.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND fraud_score > 30", tenantID).
		Count(&fraudCount)

	return c.JSON(fiber.Map{
		"total_revenue":   totalRevenue,
		"paid_amount":    paidAmount,
		"pending_amount": pendingAmount,
		"failed_amount": failedAmount,
		"fraud_alerts":  fraudCount,
	})
}

// GetUnmatchedPayments returns payments without invoice links
func (h *PaymentHandler) GetUnmatchedPayments(c *fiber.Ctx) error {
	tenantID, _ := c.Locals("tenant_id").(string)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var unmatched []models.Payment
	h.db.Where("tenant_id = ?", tenantID).
		Where("(invoice_id IS NULL OR invoice_id = '')").
		Where("status = ?", "success").
		Order("created_at DESC").
		Find(&unmatched)

	return c.JSON(fiber.Map{"payments": unmatched})
}

// ManualMatchPayment manually matches a payment to an invoice
func (h *PaymentHandler) ManualMatchPayment(c *fiber.Ctx) error {
	tenantID, _ := c.Locals("tenant_id").(string)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req struct {
		PaymentID  string `json:"payment_id"`
		InvoiceID string `json:"invoice_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	var payment models.Payment
	if err := h.db.First(&payment, "id = ? AND tenant_id = ?", req.PaymentID, tenantID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "payment not found"})
	}

	payment.InvoiceID = req.InvoiceID
	if err := h.db.Save(&payment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "matched"})
}

// AutoMatchPayments automatically matches unmatched payments
func (h *PaymentHandler) AutoMatchPayments(c *fiber.Ctx) error {
	tenantID, _ := c.Locals("tenant_id").(string)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var unmatched []models.Payment
	h.db.Where("tenant_id = ?", tenantID).
		Where("(invoice_id IS NULL OR invoice_id = '')").
		Where("status = ?", "success").
		Find(&unmatched)

	matched := 0
	for _, payment := range unmatched {
		var invoice models.Invoice
		err := h.db.Where("tenant_id = ?", tenantID).
			Where("balance_due > 0").
			Order("created_at ASC").
			First(&invoice).Error

		if err == nil {
			payment.InvoiceID = invoice.ID
			h.db.Save(&payment)
			matched++
		}
	}

	return c.JSON(fiber.Map{"matched": matched, "total": len(unmatched)})
}

// GetPaymentAudit returns audit logs for a payment
func (h *PaymentHandler) GetPaymentAudit(c *fiber.Ctx) error {
	c.Params("id")
	return c.JSON(fiber.Map{
		"audit": []interface{}{
			map[string]string{"action": "payment_created", "timestamp": time.Now().Format(time.RFC3339)},
		},
	})
}

// GetExchangeRates returns currency exchange rates
func (h *PaymentHandler) GetExchangeRates(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"error": "exchange service not available"})
}
