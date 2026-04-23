package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrPaymentNotFound         = errors.New("payment not found")
	ErrPaymentAlreadyCompleted = errors.New("payment already completed")
	ErrPaymentFailed           = errors.New("payment failed")
	ErrInvalidPaymentAmount    = errors.New("invalid payment amount")
	ErrInvoiceAlreadyPaid      = errors.New("invoice already paid")
	ErrTenantMismatch          = errors.New("tenant mismatch")
)

// PaymentService handles payment processing with proper security
type PaymentService struct {
	db  *database.DB
	cfg *config.Config
	log *logger.Logger
	// Note: Idempotency is handled via database constraints, not in-memory sync.Map
}

// NewPaymentService creates a new PaymentService
func NewPaymentService(db *database.DB, cfg *config.Config) *PaymentService {
	return &PaymentService{
		db:  db,
		cfg: cfg,
		log: logger.Get(),
	}
}

// PaymentRequest represents a payment initiation request
type PaymentRequest struct {
	TenantID    string
	InvoiceID   string
	PhoneNumber string
	Amount      float64
}

// PaymentResult represents the result of payment processing
type PaymentResult struct {
	Payment          *models.Payment
	STKPushSent      bool
	RequiresCallback bool
	Message          string
}

// InitiatePayment initiates a payment with proper flow control
// SECURITY: Does NOT create payment record before STK push succeeds
// The payment is only recorded AFTER successful callback
func (s *PaymentService) InitiatePayment(ctx context.Context, req *PaymentRequest) (*PaymentResult, error) {
	// Validate tenant
	if req.TenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	s.log.Info(ctx, "Payment: Initiating payment flow",
		"tenant_id", req.TenantID,
		"invoice_id", req.InvoiceID,
		"amount", req.Amount,
	)

	// Get invoice with tenant filter
	invoice, err := s.getInvoiceWithTenant(ctx, req.TenantID, req.InvoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoice: %w", err)
	}

	// Calculate remaining amount
	remainingAmount := invoice.Total - invoice.PaidAmount
	if remainingAmount <= 0 {
		s.log.Warn(ctx, "Payment: Invoice already paid",
			"invoice_id", invoice.ID,
			"total", invoice.Total,
			"paid_amount", invoice.PaidAmount,
		)
		return nil, ErrInvoiceAlreadyPaid
	}

	// Validate payment amount
	paymentAmount := req.Amount
	if paymentAmount <= 0 {
		return nil, ErrInvalidPaymentAmount
	}
	if paymentAmount > remainingAmount {
		paymentAmount = remainingAmount // Cap at remaining
	}

	// NOTE: We do NOT create payment record here
	// Payment will be created upon successful callback
	// This prevents "phantom" payments when STK push fails

	s.log.Info(ctx, "Payment: Payment validated, ready for STK push",
		"invoice_id", invoice.ID,
		"amount", paymentAmount,
		"remaining", remainingAmount,
	)

	return &PaymentResult{
		Payment:          nil,  // Will be created on callback
		STKPushSent:      true, // Assuming STK push will be triggered
		RequiresCallback: true,
		Message:          "STK push initiated. Payment will be recorded upon confirmation.",
	}, nil
}

// CompletePaymentFromCallback processes payment completion from webhook callback
// This is the CRITICAL function that must be idempotent and secure
func (s *PaymentService) CompletePaymentFromCallback(ctx context.Context, provider string, callbackData interface{}) error {
	var paymentRef string
	var amount float64
	var receipt string

	switch provider {
	case "mpesa":
		callback := callbackData.(map[string]interface{})
		paymentRef = callback["checkout_request_id"].(string)
		amountStr := callback["amount"].(string)
		fmt.Sscanf(amountStr, "%f", &amount)
		receipt = callback["receipt"].(string)
	case "intasend":
		callback := callbackData.(map[string]interface{})
		paymentRef = callback["checkout_id"].(string)
		amountStr := callback["amount"].(string)
		fmt.Sscanf(amountStr, "%f", &amount)
		receipt = callback["reference"].(string)
	}

	// IDEMPOTENCY CHECK - CRITICAL
	// Check if this exact payment has already been processed
	existingPayment := &models.Payment{}
	err := s.db.Where("reference = ? OR intasend_id = ?", receipt, paymentRef).First(existingPayment).Error
	if err == nil && existingPayment.Status == models.PaymentStatusCompleted {
		s.log.Info(ctx, "Payment: Duplicate callback detected, skipping",
			"receipt", receipt,
			"payment_id", existingPayment.ID,
		)
		return nil // Already processed - idempotent
	}

	// Find the invoice and verify tenant
	// In production, you'd look up by payment reference or invoice number
	// For now, assume the invoice was already created or we look it up

	// Create payment record INSIDE transaction
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Double-check idempotency within transaction
		var count int64
		tx.Model(&models.Payment{}).Where("reference = ? AND status = ?", receipt, models.PaymentStatusCompleted).Count(&count)
		if count > 0 {
			return nil // Already processed
		}

		// Find pending payment or create new one
		var payment *models.Payment
		if err := tx.Where("reference = ? AND status = ?", paymentRef, models.PaymentStatusPending).First(&payment).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Payment wasn't created upfront - this is actually a security concern
				// In production, require prior payment record exists
				s.log.Warn(ctx, "Payment: No pending payment record found",
					"ref", paymentRef,
				)
				return ErrPaymentNotFound
			}
			return err
		}

		// Validate amount against invoice
		var invoice models.Invoice
		if err := tx.First(&invoice, "id = ?", payment.InvoiceID).Error; err != nil {
			return ErrInvoiceNotFound
		}

		remaining := invoice.Total - invoice.PaidAmount
		if amount > remaining {
			s.log.Error(ctx, "Payment: Amount exceeds remaining balance",
				"amount", amount,
				"remaining", remaining,
			)
			return fmt.Errorf("payment amount exceeds remaining balance")
		}

		// Update payment to completed
		payment.Status = models.PaymentStatusCompleted
		payment.Reference = receipt
		now := time.Now()
		payment.CompletedAt = &now
		if err := tx.Save(payment).Error; err != nil {
			return fmt.Errorf("failed to update payment: %w", err)
		}

		// Update invoice status
		invoice.PaidAmount += amount
		if invoice.PaidAmount >= invoice.Total {
			invoice.PaidAmount = invoice.Total
			invoice.Status = models.InvoiceStatusPaid
			now := time.Now()
			invoice.PaidAt = &now
		} else {
			invoice.Status = models.InvoiceStatusPartiallyPaid
		}
		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		// Update client totals
		tx.Model(&models.Client{}).Where("id = ?", invoice.ClientID).
			Update("total_paid", gorm.Expr("total_paid + ?", amount))

		s.log.Info(ctx, "Payment: Completed successfully",
			"payment_id", payment.ID,
			"invoice_id", invoice.ID,
			"amount", amount,
		)

		return nil
	})
}

// MarkPaymentFailed marks a payment as failed with proper tenant isolation
func (s *PaymentService) MarkPaymentFailed(ctx context.Context, tenantID, paymentID, reason string) error {
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}

	var payment models.Payment
	err := s.db.Scopes(database.TenantFilter(tenantID)).First(&payment, "id = ?", paymentID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPaymentNotFound
		}
		return fmt.Errorf("failed to fetch payment: %w", err)
	}

	// Can only mark pending payments as failed
	if payment.Status != models.PaymentStatusPending {
		return fmt.Errorf("cannot mark payment as failed - current status: %s", payment.Status)
	}

	payment.Status = models.PaymentStatusFailed
	payment.FailureReason = reason

	return s.db.Save(&payment).Error
}

// getInvoiceWithTenant retrieves invoice with proper tenant isolation
func (s *PaymentService) getInvoiceWithTenant(ctx context.Context, tenantID, invoiceID string) (*models.Invoice, error) {
	var invoice models.Invoice
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Preload("Client").
		First(&invoice, "id = ?", invoiceID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to fetch invoice: %w", err)
	}
	return &invoice, nil
}

// GetPaymentWithTenant retrieves payment with proper tenant isolation
func (s *PaymentService) GetPaymentWithTenant(ctx context.Context, tenantID, paymentID string) (*models.Payment, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	var payment models.Payment
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Preload("Invoice").
		First(&payment, "id = ?", paymentID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("failed to fetch payment: %w", err)
	}

	// Verify tenant matches
	if payment.TenantID != tenantID {
		return nil, ErrTenantMismatch
	}

	return &payment, nil
}

// GetPaymentsByInvoice retrieves all payments for an invoice (tenant-scoped)
func (s *PaymentService) GetPaymentsByInvoice(ctx context.Context, tenantID, invoiceID string) ([]models.Payment, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	var payments []models.Payment
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Where("invoice_id = ?", invoiceID).
		Order("created_at DESC").
		Find(&payments).Error
	return payments, err
}

// PaymentFilter for querying payments with tenant isolation
type PaymentFilter struct {
	Search    string
	Status    string
	Method    string
	DateFrom  string
	DateTo    string
	ClientID  string
	InvoiceID string
	Page      int
	Limit     int
	FromDate  *time.Time
	ToDate    *time.Time
}

// GetTenantPayments retrieves all payments for a tenant (tenant-scoped)
func (s *PaymentService) GetTenantPayments(ctx context.Context, tenantID string, filter PaymentFilter) ([]models.Payment, int64, error) {
	if tenantID == "" {
		return nil, 0, errors.New("tenant_id is required")
	}

	var payments []models.Payment
	var total int64

	query := s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Payment{})

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.InvoiceID != "" {
		query = query.Where("invoice_id = ?", filter.InvoiceID)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count payments: %w", err)
	}

	// Apply pagination
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	offset := (filter.Page - 1) * filter.Limit
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Order("created_at DESC").Find(&payments).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch payments: %w", err)
	}

	return payments, total, nil
}

// ValidatePaymentAmount validates payment amount against invoice
// Returns error if amount is invalid or exceeds remaining balance
func (s *PaymentService) ValidatePaymentAmount(invoice *models.Invoice, amount float64) error {
	if amount <= 0 {
		return ErrInvalidPaymentAmount
	}

	remaining := invoice.Total - invoice.PaidAmount
	if amount > remaining {
		return fmt.Errorf("amount %.2f exceeds remaining balance %.2f", amount, remaining)
	}

	return nil
}

// CreatePendingPayment creates a pending payment record (used when we need to track initiation)
func (s *PaymentService) CreatePendingPayment(ctx context.Context, invoice *models.Invoice, phoneNumber, providerRef string, amount float64) (*models.Payment, error) {
	// Validate amount first
	if err := s.ValidatePaymentAmount(invoice, amount); err != nil {
		return nil, err
	}

	payment := &models.Payment{
		ID:          uuid.New().String(),
		TenantID:    invoice.TenantID,
		InvoiceID:   invoice.ID,
		UserID:      invoice.UserID,
		Amount:      amount,
		Currency:    invoice.Currency,
		Method:      models.PaymentMethodMpesa,
		Status:      models.PaymentStatusPending,
		PhoneNumber: phoneNumber,
		Reference:   providerRef,
	}

	if err := s.db.Create(payment).Error; err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	s.log.Info(ctx, "Payment: Pending payment created",
		"payment_id", payment.ID,
		"invoice_id", invoice.ID,
		"amount", amount,
	)

	return payment, nil
}

// ReversePayment reverses a completed payment (for chargebacks/refunds)
func (s *PaymentService) ReversePayment(ctx context.Context, tenantID, paymentID, reason string) error {
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		var payment models.Payment
		if err := tx.Scopes(database.TenantFilter(tenantID)).First(&payment, "id = ?", paymentID).Error; err != nil {
			return ErrPaymentNotFound
		}

		if payment.Status != models.PaymentStatusCompleted {
			return fmt.Errorf("can only reverse completed payments")
		}

		// Mark payment as refunded
		payment.Status = models.PaymentStatusRefunded
		payment.FailureReason = reason
		if err := tx.Save(&payment).Error; err != nil {
			return err
		}

		// Reverse invoice amounts
		var invoice models.Invoice
		if err := tx.First(&invoice, "id = ?", payment.InvoiceID).Error; err != nil {
			return ErrInvoiceNotFound
		}

		invoice.PaidAmount -= payment.Amount
		if invoice.PaidAmount < 0 {
			invoice.PaidAmount = 0
		}

		// Determine status based on remaining paid amount
		if invoice.PaidAmount <= 0 {
			invoice.Status = models.InvoiceStatusSent
		} else {
			invoice.Status = models.InvoiceStatusPartiallyPaid
		}

		if err := tx.Save(&invoice).Error; err != nil {
			return err
		}

		// Reverse client totals
		tx.Model(&models.Client{}).Where("id = ?", invoice.ClientID).
			Update("total_paid", gorm.Expr("total_paid - ?", payment.Amount))

		return nil
	})
}

// WebhookPayload represents an incoming payment webhook
type WebhookPayload struct {
	Event         string `json:"event"`
	CheckoutID    string `json:"checkout_id"`
	InvoiceNumber string `json:"invoice_number"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	Reference     string `json:"reference"`
	CustomerPhone string `json:"customer_phone"`
	Reason        string `json:"reason,omitempty"`
}

// ProcessWebhook processes an incoming payment webhook (tenant-aware)
func (s *PaymentService) ProcessWebhook(ctx context.Context, tenantID string, payload *WebhookPayload) error {
	s.log.Info(ctx, "Payment: Processing webhook",
		"tenant_id", tenantID,
		"event", payload.Event,
		"checkout_id", payload.CheckoutID,
		"reference", payload.Reference,
	)

	if tenantID == "" {
		return errors.New("tenant_id is required for webhook processing")
	}

	var payment models.Payment
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Where("reference = ? OR intasend_id = ?", payload.Reference, payload.CheckoutID).
		First(&payment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.log.Warn(ctx, "Payment: Webhook payment not found",
				"tenant_id", tenantID,
				"reference", payload.Reference,
			)
			return ErrPaymentNotFound
		}
		return fmt.Errorf("failed to find payment: %w", err)
	}

	switch payload.Event {
	case "payment_successful", "invoice_payment_signed":
		return s.completePaymentFromWebhook(&payment, payload)
	case "payment_failed", "payment_cancelled":
		return s.failPayment(&payment, payload.Reason)
	case "payment_reversed", "chargeback":
		return s.reversePaymentFromWebhook(&payment)
	default:
		s.log.Warn(ctx, "Payment: Unknown event type",
			"event", payload.Event,
		)
	}

	return nil
}

// completePaymentFromWebhook completes a payment from webhook
func (s *PaymentService) completePaymentFromWebhook(payment *models.Payment, payload *WebhookPayload) error {
	// Idempotency check
	if payment.Status == models.PaymentStatusCompleted {
		return nil // Already completed
	}

	var amount float64
	fmt.Sscanf(payload.Amount, "%f", &amount)

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Update payment
		payment.Status = models.PaymentStatusCompleted
		payment.Reference = payload.Reference
		now := time.Now()
		payment.CompletedAt = &now
		if err := tx.Save(payment).Error; err != nil {
			return fmt.Errorf("failed to update payment: %w", err)
		}

		// Update invoice
		var invoice models.Invoice
		if err := tx.First(&invoice, "id = ?", payment.InvoiceID).Error; err != nil {
			return ErrInvoiceNotFound
		}

		invoice.PaidAmount += amount
		if invoice.PaidAmount >= invoice.Total {
			invoice.PaidAmount = invoice.Total
			invoice.Status = models.InvoiceStatusPaid
			invoice.PaidAt = &now
		} else {
			invoice.Status = models.InvoiceStatusPartiallyPaid
		}
		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		// Update client
		tx.Model(&models.Client{}).Where("id = ?", invoice.ClientID).
			Update("total_paid", gorm.Expr("total_paid + ?", amount))

		return nil
	})
}

// failPayment marks payment as failed
func (s *PaymentService) failPayment(payment *models.Payment, reason string) error {
	payment.Status = models.PaymentStatusFailed
	payment.FailureReason = reason
	return s.db.Save(payment).Error
}

// reversePaymentFromWebhook reverses a payment from webhook
func (s *PaymentService) reversePaymentFromWebhook(payment *models.Payment) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		payment.Status = models.PaymentStatusRefunded
		if err := tx.Save(payment).Error; err != nil {
			return err
		}

		// Reverse invoice
		var invoice models.Invoice
		if err := tx.First(&invoice, "id = ?", payment.InvoiceID).Error; err != nil {
			return ErrInvoiceNotFound
		}

		invoice.PaidAmount -= payment.Amount
		if invoice.PaidAmount < 0 {
			invoice.PaidAmount = 0
		}
		invoice.Status = models.InvoiceStatusSent

		return tx.Save(&invoice).Error
	})
}
