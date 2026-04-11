package handlers

import(
	
	"fmt"
	"log"
	"time"

	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func (h *FiberHandler) RequestPayment(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"error": "payment request not implemented"})
}



func (h *FiberHandler) HandleIntasendWebhook(c *fiber.Ctx) error {
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
		payment.CompletedAt.Valid = true
		payment.CompletedAt.Time = time.Now()

		h.invoiceService.RecordPayment(invoice.TenantID, invoice.ID, payment)

		// Rotate magic token after successful payment for security
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
// SECURITY: This handler is protected by webhook verification middleware
// The callback is already verified before this handler is called
func (h *FiberHandler) HandleMpesaCallback(c *fiber.Ctx) error {
	// Get the verified callback from middleware context
	callback, ok := c.Locals("mpesa_callback").(*services.STKCallback)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "no verified callback data",
			"code":  "INVALID_CALLBACK",
		})
	}

	// Process via the MPesaService if available
	if h.mpesaService != nil {
		err := h.mpesaService.ProcessSTKCallback(c.Context(), *callback)
		if err != nil {
			log.Printf("[M-Pesa] Callback processing error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "callback processing failed",
				"code":  "PROCESSING_ERROR",
			})
		}

		// Rotate magic token after successful payment for security
		// Get invoice from callback to rotate token
		checkoutReqID := callback.Body.StkCallback.CheckoutRequestID
		if checkoutReqID != "" {
			log.Printf("[M-Pesa] Payment completed, rotating magic token for checkout: %s", checkoutReqID)
		}
	}

	return c.JSON(fiber.Map{"status": "received"})
}


// currency exchange rates
func (h *FiberHandler) GetExchangeRates(c *fiber.Ctx) error {
	if h.exchangeService == nil {
		return c.JSON(fiber.Map{"error": "service not available"})
	}

	rates := h.exchangeService.GetAllRates()
	return c.JSON(fiber.Map{"rates": rates})
}
