package main

import (
	"fmt"
	"log"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type PaymentWebhookRequest struct {
	Event         string `json:"event"`
	CheckoutID    string `json:"checkout_id"`
	InvoiceNumber string `json:"invoice_number"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	Reference     string `json:"reference"`
	CustomerPhone string `json:"customer_phone"`
}

func HandleIntasendWebhook(c *fiber.Ctx, db *database.DB, idempotencySvc *services.IdempotencyService) error {
	var req PaymentWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		log.Printf("[Webhook] Parse error: %v", err)
		return fiber.NewError(fiber.StatusBadRequest, "invalid payload")
	}

	log.Printf("[Webhook] Received: event=%s checkout=%s invoice=%s amount=%s",
		req.Event, req.CheckoutID, req.InvoiceNumber, req.Amount)

	// Validate required fields
	if req.InvoiceNumber == "" && req.CheckoutID == "" {
		log.Printf("[Webhook] Missing invoice_number and checkout_id")
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ignored", "reason": "no_identifier"})
	}

	// IDEMPOTENCY: Check if already processed via Redis AND acquire distributed lock
	idemKey := req.CheckoutID
	if idemKey == "" {
		idemKey = req.Reference
	}

	if idemKey != "" && idempotencySvc != nil {
		// Acquire distributed lock to prevent race conditions
		isNew, err := idempotencySvc.HandlePaymentCallback(c.Context(), idemKey, nil)
		if err != nil {
			log.Printf("[Webhook] Idempotency error: %v", err)
			// Continue without lock - best effort
		} else if !isNew {
			// Already processed or being processed by another worker
			log.Printf("[Webhook] Already processing or processed: %s", idemKey)
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"status":          "already_processed",
				"idempotency_key": idemKey,
			})
		}
		// Lock acquired - proceed with processing
		defer func() {
			if idempotencySvc != nil && idemKey != "" {
				idempotencySvc.Unlock(c.Context(), idemKey)
			}
		}()
	}

	// SECURITY HARD-STOP: Enforce tenant isolation
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		// Log security event for potential attack
		log.Printf("[SECURITY] Payment webhook attempted without tenant_id from IP: %s, Reference: %s",
			c.IP(), req.Reference)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "tenant context required",
			"code":  "TENANT_REQUIRED",
		})
	}

	// Find invoice with strict tenant scoping
	var invoice models.Invoice

	err := db.DB.Scopes(database.TenantFilter(tenantID)).
		Preload("Client").
		Preload("Items").
		Preload("Payments").
		First(&invoice, "invoice_number = ?", req.InvoiceNumber).Error

	if err != nil {
		log.Printf("[Webhook] Invoice not found: %s (tenant: %s)", req.InvoiceNumber, tenantID)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ignored", "reason": "not_found"})
	}

	// Handle event types
	switch req.Event {
	case "payment_successful", "invoice_payment_signed":
		return processSuccessfulPayment(c, db, &invoice, &req, idempotencySvc)

	case "payment_reversed", "chargeback":
		return processReversedPayment(c, db, &invoice)

	default:
		log.Printf("[Webhook] Unhandled event: %s", req.Event)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "received"})
}

func processSuccessfulPayment(c *fiber.Ctx, db *database.DB, invoice *models.Invoice, req *PaymentWebhookRequest, idempotencySvc *services.IdempotencyService) error {
	// Parse amount
	amount := 0.0
	if req.Amount != "" {
		fmt.Sscanf(req.Amount, "%f", &amount)
	}
	if amount == 0 {
		amount = invoice.Total
	}

	// IDEMPOTENCY: Check for duplicate M-Pesa receipt number (Reference) WITHIN TRANSACTION
	// This is enforced at DB level with unique constraint, but we also check here for early detection
	// Include tenant_id in the check to prevent cross-tenant conflicts
	if req.Reference != "" {
		var existingPayment models.Payment
		err := db.DB.Where("reference = ? AND tenant_id = ?", req.Reference, invoice.TenantID).First(&existingPayment).Error
		if err == nil {
			log.Printf("[Webhook] Payment with reference %s already exists for tenant %s - skipping", maskRef(req.Reference), invoice.TenantID)
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"status":    "duplicate_reference",
				"reference": req.Reference,
			})
		}
	}

	// BEGIN TRANSACTION - All DB operations must succeed or rollback
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Create payment record
		payment := models.Payment{
			ID:          fmt.Sprintf("pay-%d", time.Now().UnixNano()),
			TenantID:    invoice.TenantID,
			InvoiceID:   invoice.ID,
			UserID:      invoice.UserID,
			Amount:      amount,
			Currency:    invoice.Currency,
			Method:      models.PaymentMethodMpesa,
			Status:      models.PaymentStatusCompleted,
			Reference:   req.Reference,
			PhoneNumber: req.CustomerPhone,
		}
	now := time.Now()
	payment.CompletedAt = &now

		if err := tx.Create(&payment).Error; err != nil {
			log.Printf("[Webhook] Failed to create payment: %v", err)
			return fmt.Errorf("failed to create payment: %w", err)
		}
		log.Printf("[Webhook] Payment created: %s for invoice %s", payment.ID, invoice.InvoiceNumber)

		// 2. Update invoice status
		// Check if partial payment or full
		newStatus := models.InvoiceStatusPaid
		if invoice.PaidAmount+amount < invoice.Total {
			newStatus = models.InvoiceStatusPartiallyPaid
		}

		if err := tx.Model(invoice).Updates(map[string]interface{}{
			"status":      newStatus,
			"paid_amount": gorm.Expr("paid_amount + ?", amount),
			"paid_at":     time.Now(),
		}).Error; err != nil {
			log.Printf("[Webhook] Failed to update invoice: %v", err)
			return fmt.Errorf("failed to update invoice: %w", err)
		}
		log.Printf("[Webhook] Invoice %s updated to %s", invoice.InvoiceNumber, newStatus)

		// 3. Update client total paid (with tenant filter for extra safety)
		if err := tx.Model(&models.Client{}).
			Scopes(database.TenantFilter(invoice.TenantID)).
			Where("id = ?", invoice.ClientID).
			Update("total_paid", gorm.Expr("total_paid + ?", amount)).Error; err != nil {
			log.Printf("[Webhook] Failed to update client: %v", err)
			return fmt.Errorf("failed to update client: %w", err)
		}

		return nil
	})

	// Check transaction result
	if err != nil {
		log.Printf("[Webhook] Transaction failed: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "payment processing failed")
	}

	// Mark as processed in Redis for idempotency
	if idempotencySvc != nil && req.CheckoutID != "" {
		idemKey := req.CheckoutID
		if idemKey == "" {
			idemKey = req.Reference
		}
		if idemKey != "" {
			_ = idempotencySvc.MarkProcessed(c.Context(), idemKey, map[string]interface{}{
				"invoice_id": invoice.ID,
				"amount":     amount,
			})
		}
	}

	log.Printf("[Webhook] Payment completed for invoice %s: amount=%f", invoice.InvoiceNumber, amount)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":         "processed",
		"invoice_id":     invoice.ID,
		"invoice_number": invoice.InvoiceNumber,
		"amount":         amount,
		"currency":       invoice.Currency,
	})
}

func processReversedPayment(c *fiber.Ctx, db *database.DB, invoice *models.Invoice) error {
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		// Reverse invoice status
		if err := tx.Model(invoice).Update("status", models.InvoiceStatusSent).Error; err != nil {
			return fmt.Errorf("failed to reverse invoice: %w", err)
		}

		// Reverse client paid amount
		if err := tx.Model(&models.Client{}).Where("id = ?", invoice.ClientID).
			Update("total_paid", gorm.Expr("total_paid - ?", invoice.PaidAmount)).Error; err != nil {
			return fmt.Errorf("failed to reverse client: %w", err)
		}

		return nil
	})

	if err != nil {
		log.Printf("[Webhook] Reverse failed: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "reversal failed")
	}

	log.Printf("[Webhook] Payment reversed for invoice %s", invoice.InvoiceNumber)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":     "reversed",
		"invoice_id": invoice.ID,
	})
}

func maskRef(ref string) string {
	if len(ref) <= 4 {
		return "****"
	}
	return "****" + ref[len(ref)-4:]
}
