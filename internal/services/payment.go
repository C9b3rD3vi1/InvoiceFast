package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PaymentService handles payment processing
type PaymentService struct {
	db  *database.DB
	cfg *config.Config
}

// NewPaymentService creates a new PaymentService
func NewPaymentService(db *database.DB, cfg *config.Config) *PaymentService {
	return &PaymentService{
		db:  db,
		cfg: cfg,
	}
}

// InitiateSTKPush initiates an M-Pesa STK push payment request
func (s *PaymentService) InitiateSTKPush(ctx context.Context, invoice *models.Invoice, phoneNumber string, amount float64) (*models.Payment, error) {
	// Calculate remaining amount
	remainingAmount := invoice.Total - invoice.PaidAmount
	if remainingAmount <= 0 {
		return nil, fmt.Errorf("invoice already paid")
	}

	// Use provided amount or remaining amount, whichever is smaller
	paymentAmount := amount
	if paymentAmount > remainingAmount || amount == 0 {
		paymentAmount = remainingAmount
	}

	// Generate payment reference
	reference := fmt.Sprintf("MPESA-%s", uuid.New().String()[:8])

	// Create payment record
	payment := &models.Payment{
		ID:          uuid.New().String(),
		TenantID:    invoice.TenantID,
		InvoiceID:   invoice.ID,
		UserID:      invoice.UserID,
		Amount:      paymentAmount,
		Currency:    invoice.Currency,
		Method:      models.PaymentMethodMpesa,
		Status:      models.PaymentStatusPending,
		PhoneNumber: phoneNumber,
		Reference:   reference,
	}

	// Save payment
	if err := s.db.Create(payment).Error; err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	// TODO: Integrate with actual M-Pesa Daraja API or Intasend
	// For now, simulate STK push being sent
	log.Printf("[Payment] STK push initiated: %s for %s %f", reference, phoneNumber, paymentAmount)

	return payment, nil
}

// CheckPaymentStatus checks the status of a payment
func (s *PaymentService) CheckPaymentStatus(ctx context.Context, paymentID string) (*models.Payment, error) {
	var payment models.Payment
	err := s.db.First(&payment, "id = ?", paymentID).Error
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}
	return &payment, nil
}

// ProcessWebhook processes an incoming payment webhook (M-Pesa/Intasend)
func (s *PaymentService) ProcessWebhook(ctx context.Context, payload *WebhookPayload) error {
	log.Printf("[Payment] Processing webhook: event=%s checkout=%s", payload.Event, payload.CheckoutID)

	// Find payment by reference or checkout ID
	var payment models.Payment
	err := s.db.Where("reference = ? OR intasend_id = ?", payload.Reference, payload.CheckoutID).First(&payment).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("payment not found")
		}
		return fmt.Errorf("failed to find payment: %w", err)
	}

	// Handle different events
	switch payload.Event {
	case "payment_successful", "invoice_payment_signed":
		return s.markPaymentCompleted(&payment, payload)
	case "payment_failed", "payment_cancelled":
		return s.markPaymentFailed(&payment, payload.Reason)
	case "payment_reversed", "chargeback":
		return s.markPaymentReversed(&payment)
	default:
		log.Printf("[Payment] Unknown event type: %s", payload.Event)
	}

	return nil
}

// markPaymentCompleted marks a payment as completed and updates invoice
func (s *PaymentService) markPaymentCompleted(payment *models.Payment, payload *WebhookPayload) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Update payment status
		payment.Status = models.PaymentStatusCompleted
		payment.CompletedAt = sql.NullTime{Time: time.Now(), Valid: true}

		if err := tx.Save(payment).Error; err != nil {
			return fmt.Errorf("failed to update payment: %w", err)
		}

		// Update invoice
		var invoice models.Invoice
		if err := tx.First(&invoice, "id = ?", payment.InvoiceID).Error; err != nil {
			return fmt.Errorf("invoice not found: %w", err)
		}

		invoice.PaidAmount += payment.Amount
		if invoice.PaidAmount >= invoice.Total {
			invoice.PaidAmount = invoice.Total
			invoice.Status = models.InvoiceStatusPaid
			invoice.PaidAt = sql.NullTime{Time: time.Now(), Valid: true}
		} else {
			invoice.Status = models.InvoiceStatusPartiallyPaid
		}

		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		// Update client total paid
		if err := tx.Model(&models.Client{}).Where("id = ?", invoice.ClientID).
			Update("total_paid", gorm.Expr("total_paid + ?", payment.Amount)).Error; err != nil {
			return fmt.Errorf("failed to update client: %w", err)
		}

		log.Printf("[Payment] Payment completed: %s for invoice %s", payment.ID, invoice.InvoiceNumber)
		return nil
	})
}

// markPaymentFailed marks a payment as failed
func (s *PaymentService) markPaymentFailed(payment *models.Payment, reason string) error {
	payment.Status = models.PaymentStatusFailed
	payment.FailureReason = reason
	return s.db.Save(payment).Error
}

// markPaymentReversed marks a payment as reversed
func (s *PaymentService) markPaymentReversed(payment *models.Payment) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		payment.Status = models.PaymentStatusRefunded
		if err := tx.Save(payment).Error; err != nil {
			return err
		}

		// Reverse invoice paid amount
		var invoice models.Invoice
		if err := tx.First(&invoice, "id = ?", payment.InvoiceID).Error; err != nil {
			return fmt.Errorf("invoice not found: %w", err)
		}

		invoice.PaidAmount -= payment.Amount
		if invoice.PaidAmount < 0 {
			invoice.PaidAmount = 0
		}
		invoice.Status = models.InvoiceStatusSent

		return tx.Save(&invoice).Error
	})
}

// GetPaymentByInvoiceID retrieves payments for an invoice
func (s *PaymentService) GetPaymentByInvoiceID(invoiceID string) ([]models.Payment, error) {
	var payments []models.Payment
	err := s.db.Where("invoice_id = ?", invoiceID).Order("created_at DESC").Find(&payments).Error
	return payments, err
}

// WebhookPayload represents an incoming payment webhook
type WebhookPayload struct {
	Event         string  `json:"event"`
	CheckoutID    string  `json:"checkout_id"`
	InvoiceNumber string  `json:"invoice_number"`
	Amount        string  `json:"amount"`
	Currency      string  `json:"currency"`
	Reference     string  `json:"reference"`
	CustomerPhone string  `json:"customer_phone"`
	Reason        string  `json:"reason,omitempty"`
}
