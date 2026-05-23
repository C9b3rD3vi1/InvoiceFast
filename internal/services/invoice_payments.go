package services

import (
	"context"
	"errors"
	"fmt"
	"time"


	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/metrics"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SendInvoice marks invoice as sent, triggers notifications, and submits to KRA e-TIMS (tenant-scoped)
func (s *InvoiceService) SendInvoice(tenantID, invoiceID, userID string) (*models.Invoice, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	invoice, err := s.GetInvoiceByID(tenantID, invoiceID)
	if err != nil {
		return nil, err
	}

	// Edge case: Cannot send if already sent or paid
	if invoice.Status == models.InvoiceStatusSent || invoice.Status == models.InvoiceStatusPaid {
		return nil, ErrAlreadySent
	}

	// Edge case: Cannot send if cancelled
	if invoice.Status == models.InvoiceStatusCancelled {
		return nil, errors.New("cannot send cancelled invoice")
	}

	// Load client and user for notifications
	// SECURITY: Added TenantFilter to prevent IDOR
	var client models.Client
	var user models.User
	s.db.Scopes(database.TenantFilter(tenantID)).First(&client, "id = ?", invoice.ClientID)
	s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", userID)

	// Submit to KRA e-TIMS if configured (non-blocking, async)
	// KRA ICN will be updated after successful submission
	if s.kraService != nil && invoice.KRAICN == "" {
		tenantID := invoice.TenantID
		invoiceID := invoice.ID
		invoiceNum := invoice.InvoiceNumber
		createdAt := invoice.CreatedAt
		subtotal := invoice.Subtotal
		discount := invoice.Discount
		taxRate := invoice.TaxRate
		taxAmount := invoice.TaxAmount
		total := invoice.Total
		currency := invoice.Currency

		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Get().Error(context.Background(), "panic recovered", "category", "panic", "recover", r)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			// Load fresh data for KRA submission
			// SECURITY: Added TenantFilter to prevent IDOR
			var cli models.Client
			var usr models.User
			s.db.WithContext(ctx).Scopes(database.TenantFilter(tenantID)).First(&cli, "id = ?", invoice.ClientID)
			s.db.WithContext(ctx).Scopes(database.TenantFilter(tenantID)).First(&usr, "id = ?", userID)

			// Build KRA data using the service's format
			dbItems := make([]models.InvoiceItem, 0)
			s.db.WithContext(ctx).Model(&models.InvoiceItem{}).Where("invoice_id = ?", invoiceID).Find(&dbItems)

			// Convert to KRAItem format
			kraPayloadItems := make([]KRAItem, len(dbItems))
			for i, item := range dbItems {
				kraPayloadItems[i] = KRAItem{
					ItemCode:        item.ItemCode,
					ItemDescription: item.Description,
					Quantity:      item.Quantity,
					UnitOfMeasure:  item.Unit,
					UnitPrice:    item.UnitPrice.Float64(),
					Discount:     item.DiscountAmt.Float64(),
					DiscountRate: item.DiscountRate,
					VATRate:      item.TaxRate,
					VATAmount:    item.TaxAmount.Float64(),
					Total:       item.Total.Float64(),
				}
			}

			kraData := &KRAInvoiceData{
				InvoiceNumber: invoiceNum,
				InvoiceDate:   createdAt.Format("2006-01-02"),
				InvoiceTime:   createdAt.Format("15:04:05"),
				Seller: KRASeller{
					RegistrationNumber: usr.KRAPIN,
					BusinessName:       usr.CompanyName,
					ContactMobile:      usr.Phone,
					ContactEmail:       usr.Email,
				},
				Buyer: KRABuyer{
					CustomerName:       cli.Name,
					ContactMobile:      cli.Phone,
					ContactEmail:       cli.Email,
					RegistrationNumber: cli.KRAPIN,
				},
				Items:             kraPayloadItems,
				SubTotal:          subtotal.Float64(),
				TotalExcludingVAT: subtotal.Subtract(discount).Float64(),
				VATRate:           taxRate,
				VATAmount:         taxAmount.Float64(),
				TotalIncludingVAT: total.Float64(),
				Currency:          currency,
			}

			kraResp, err := s.kraService.SubmitInvoice(kraData, invoice.TenantID, invoice.ID)
			if err != nil {
				logger.Get().Error(context.Background(), "KRA submission failed", "category", "kra", "invoice_number", invoiceNum, "error", err)
				s.db.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
					"kra_status": models.KRAInvoiceStatusFailed,
					"kra_error":  err.Error(),
				})
				return
			}

			if kraResp == nil {
				logger.Get().Error(context.Background(), "KRA submission returned nil response", "category", "kra", "invoice_number", invoiceNum)
				s.db.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
					"kra_status": models.KRAInvoiceStatusFailed,
					"kra_error":  "nil response from KRA service",
				})
				return
			}

			s.db.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
				"kra_icn":          kraResp.ICN,
				"kra_qr_code":      kraResp.QRCode,
				"kra_status":       models.KRAInvoiceStatusSubmitted,
				"kra_submitted_at": time.Now(),
				"kra_error":        "",
			})
			logger.Get().Info(context.Background(), "KRA invoice submitted", "category", "kra", "invoice_number", invoiceNum, "icn", kraResp.ICN)
		}()
	}

	invoice.Status = models.InvoiceStatusSent
	now := time.Now()
	invoice.SentAt = &now

	if err := s.db.Save(invoice).Error; err != nil {
		return nil, fmt.Errorf("failed to send invoice: %w", err)
	}

	// Log the action
	s.db.Create(&models.AuditLog{
		ID:         uuid.New().String(),
		UserID:     userID,
		Action:     "invoice.sent",
		EntityType: "invoice",
		EntityID:   invoiceID,
		Details:    fmt.Sprintf(`{"invoice_number": "%s"}`, invoice.InvoiceNumber),
	})

	// Send email notification (async, don't fail if email fails)
	go s.sendInvoiceNotifications(invoice, userID)

	return invoice, nil
}

// sendInvoiceNotifications sends email and WhatsApp notifications for an invoice
func (s *InvoiceService) sendInvoiceNotifications(invoice *models.Invoice, userID string) {
	// Load client and user for notifications (tenant-scoped)
	var client models.Client
	var user models.User
	tenantID := invoice.TenantID
	s.db.Scopes(database.TenantFilter(tenantID)).First(&client, "id = ?", invoice.ClientID)
	s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", userID)

	// Build notification data
	invoiceLink := fmt.Sprintf("%s/invoice/%s", s.BaseURL(), invoice.MagicToken)
	amount := fmt.Sprintf("%s %.2f", invoice.Currency, invoice.Total.Float64())
	
	// Use NotificationService if available
	if s.notificationSvc != nil {
		// Trigger invoice.created event
		vars := map[string]string{
			"client_name":    client.Name,
			"invoice_number": invoice.InvoiceNumber,
			"amount":        amount,
			"due_date":     invoice.DueDate.Format("02 Jan 2006"),
			"link":         invoiceLink,
		}
		
		// Queue notification for invoice created
		s.notificationSvc.Send(context.Background(), &NotificationRequest{
			TenantID:   tenantID,
			UserID:     userID,
			EventType:  EventInvoiceCreated,
			Channels:  []string{ChannelEmail, ChannelWA},
			Recipient: client.Email,
			Subject:   "New Invoice " + invoice.InvoiceNumber,
			Body:     fmt.Sprintf("Invoice %s for %s has been created. Amount: %s. Due: %s", invoice.InvoiceNumber, client.Name, amount, invoice.DueDate.Format("02 Jan 2006")),
			Variables: vars,
			Reference: invoice.InvoiceNumber,
		})
		return
	}
	
	// Legacy direct calls (deprecated) - only runs if no NotificationService
	if s.emailService != nil {
		emailData := &InvoiceEmailData{
			CompanyName:   user.CompanyName,
			CompanyEmail:  user.Email,
			ClientName:    client.Name,
			ClientEmail:   client.Email,
			InvoiceNumber: invoice.InvoiceNumber,
			InvoiceLink:   invoiceLink,
			Amount:        invoice.Total.Float64(),
			Currency:      invoice.Currency,
			DueDate:       invoice.DueDate.Format("02 Jan 2006"),
		}

		if err := s.emailService.SendInvoiceEmail(emailData); err != nil {
			logger.Get().Error(context.Background(), "Failed to send invoice email", "invoice_number", invoice.InvoiceNumber, "error", err)
		}
	}

	// Send WhatsApp notification if configured
	if s.whatsappService != nil && client.Phone != "" {
		waData := map[string]string{
			"company": user.CompanyName,
			"invoice": invoice.InvoiceNumber,
			"amount":  amount,
			"link":    invoiceLink,
		}
		if err := s.whatsappService.SendInvoiceNotification(client.Phone, waData); err != nil {
			logger.Get().Error(context.Background(), "Failed to send WhatsApp notification", "invoice_number", invoice.InvoiceNumber, "error", err)
		}
	}
}

// BaseURL returns the base URL for the application
func (s *InvoiceService) BaseURL() string {
	if s.cfg != nil && s.cfg.Server.BaseURL != "" {
		return s.cfg.Server.BaseURL
	}
	return "https://invoice.simuxtech.com"
}

// RecordPayment records a payment for an invoice with proper state machine enforcement
func (s *InvoiceService) RecordPayment(tenantID, invoiceID string, payment *models.Payment) error {
	if tenantID == "" {
		return ErrTenantRequired
	}

	// Use transaction for payment processing
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var invoice models.Invoice
		if err := tx.Scopes(database.TenantFilter(tenantID)).
			Preload("Items").
			First(&invoice, "id = ?", invoiceID).Error; err != nil {
			return ErrInvoiceNotFound
		}

		// State machine: Only allow payments on SENT, VIEWED, OVERDUE, or PARTIALLY_PAID
		allowedStatuses := []models.InvoiceStatus{
			models.InvoiceStatusSent,
			models.InvoiceStatusViewed,
			models.InvoiceStatusOverdue,
			models.InvoiceStatusPartiallyPaid,
		}
		allowed := false
		for _, status := range allowedStatuses {
			if invoice.Status == status {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("cannot record payment on invoice with status %s", invoice.Status)
		}

		// Validate payment amount
		if payment.Amount <= 0 {
			return errors.New("payment amount must be positive")
		}

		// Calculate new balance using exact Money arithmetic
		newPaidAmount := invoice.PaidAmount.Add(payment.Amount)
		total := invoice.Total

		// Handle overpayment gracefully
		if newPaidAmount.GreaterThan(total) {
			overpayment := newPaidAmount.Subtract(total)
			newPaidAmount = total
			payment.Amount = overpayment
		}

		// Set payment details
		payment.InvoiceID = invoiceID
		payment.Status = models.PaymentStatusCompleted
		if payment.ID == "" {
			payment.ID = uuid.New().String()
		}

		// Check for duplicate idempotency key at DB level
		if payment.IdempotencyKey != "" {
			var existing int64
			tx.Model(&models.Payment{}).Where("idempotency_key = ?", payment.IdempotencyKey).Count(&existing)
			if existing > 0 {
				return nil // Already processed, silently succeed
			}
		}

		// Save payment
		if err := tx.Create(payment).Error; err != nil {
			return fmt.Errorf("failed to record payment: %w", err)
		}

		// Update invoice
		invoice.PaidAmount = newPaidAmount
		balanceDue := total.Subtract(newPaidAmount)
		invoice.BalanceDue = balanceDue

		// Determine new status based on paid amount
		newStatus := invoice.Status
		if newPaidAmount.Equals(total) || invoice.BalanceDue <= 0 {
			// Full payment
			newStatus = models.InvoiceStatusPaid
			now := time.Now()
			invoice.PaidAt = &now
		} else if newPaidAmount.GreaterThan(0) {
			// Partial payment
			newStatus = models.InvoiceStatusPartiallyPaid
		}

		// Validate state transition
		if err := models.ValidateTransition(invoice.Status, newStatus); err != nil {
			return err
		}
		invoice.Status = newStatus
		invoice.Version++

		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		// Log the action
		tx.Create(&models.AuditLog{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			UserID:     payment.UserID,
			Action:     "payment.received",
			EntityType: "payment",
			EntityID:   payment.ID,
			Details:    fmt.Sprintf(`{"invoice_id": "%s", "amount": %f, "method": "%s", "status": "%s"}`, invoiceID, payment.Amount.Float64(), payment.Method, newStatus),
		})

		// Send payment notification
		go s.sendPaymentNotification(tenantID, &invoice, payment)

		return nil
	})
	if err != nil {
		return err
	}

	metrics.RecordInvoicePaid()
	return nil
}

// sendPaymentNotification sends payment received notifications
func (s *InvoiceService) sendPaymentNotification(tenantID string, invoice *models.Invoice, payment *models.Payment) {
	if s.notificationSvc == nil {
		return
	}

	var client models.Client
	s.db.Scopes(database.TenantFilter(tenantID)).First(&client, "id = ?", invoice.ClientID)

	amount := fmt.Sprintf("%s %.2f", invoice.Currency, payment.Amount.Float64())
	
		s.notificationSvc.Send(context.Background(), &NotificationRequest{
			TenantID:   tenantID,
			UserID:    invoice.UserID,
			EventType: EventPaymentReceived,
		Channels: []string{ChannelEmail, ChannelWA},
		Recipient: client.Email,
		Subject:  "Payment Received - " + invoice.InvoiceNumber,
		Body:    fmt.Sprintf("Payment of %s received for Invoice %s. Thank you!", amount, invoice.InvoiceNumber),
		Variables: map[string]string{
			"invoice_number": invoice.InvoiceNumber,
			"amount":      amount,
			"reference":   payment.Reference,
			"client_name": client.Name,
		},
		Reference: invoice.InvoiceNumber,
	})
}

// CancelInvoice cancels an invoice with proper state machine validation
func (s *InvoiceService) CancelInvoice(tenantID, invoiceID, userID string) error {
	if tenantID == "" {
		return ErrTenantRequired
	}

	// Use transaction for cancellation
	return s.db.Transaction(func(tx *gorm.DB) error {
		var invoice models.Invoice
		if err := tx.Scopes(database.TenantFilter(tenantID)).First(&invoice, "id = ?", invoiceID).Error; err != nil {
			return ErrInvoiceNotFound
		}

		// Validate state transition using state machine
		newStatus := models.InvoiceStatusCancelled
		if err := models.ValidateTransition(invoice.Status, newStatus); err != nil {
			return err
		}

		// Additional checks: cannot cancel if KRA accepted
		if invoice.KRAStatus == models.KRAInvoiceStatusAccepted {
			return errors.New("cannot cancel invoice: KRA accepted")
		}

		now := time.Now()
		invoice.Status = newStatus
		invoice.CancelledAt = &now
		invoice.Version++

		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to cancel invoice: %w", err)
		}

		// Log cancellation
		tx.Create(&models.AuditLog{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			UserID:     userID,
			Action:     "invoice_cancelled",
			EntityType: "invoice",
			EntityID:   invoiceID,
			Details:    fmt.Sprintf(`{"invoice_number": "%s", "previous_status": "%s"}`, invoice.InvoiceNumber, invoice.Status),
		})

		return nil
	})
}

// DeleteInvoice permanently deletes an invoice (tenant-scoped)
func (s *InvoiceService) DeleteInvoice(tenantID, invoiceID string) error {
	if tenantID == "" {
		return ErrTenantRequired
	}

	invoice, err := s.GetInvoiceByID(tenantID, invoiceID)
	if err != nil {
		return err
	}

	// Delete related records first
	s.db.Where("invoice_id = ?", invoiceID).Delete(&models.Payment{})
	s.db.Where("invoice_id = ?", invoiceID).Delete(&models.InvoiceItem{})

	// Delete invoice
	if err := s.db.Delete(invoice).Error; err != nil {
		return fmt.Errorf("failed to delete invoice: %w", err)
	}

	return nil
}
