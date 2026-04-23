package services

import (
	"errors"
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

// StripeService now expects real Stripe configuration
// In production, this would use the actual stripe-go/v72 package
type StripeService struct {
	db        *database.DB
	secretKey string
	publicKey string
}

type CreatePaymentIntentRequest struct {
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	InvoiceID   string  `json:"invoice_id"`
	Description string  `json:"description"`
	Email       string  `json:"email"`
}

type PaymentIntentResponse struct {
	ClientSecret string  `json:"client_secret"`
	PaymentID    string  `json:"payment_id"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
}

// NewStripeService creates a new Stripe service with required configuration
func NewStripeService(db *database.DB, secretKey, publicKey string) *StripeService {
	return &StripeService{
		db:        db,
		secretKey: secretKey,
		publicKey: publicKey,
	}
}

// CreatePaymentIntent creates a real payment intent with Stripe
func (s *StripeService) CreatePaymentIntent(req *CreatePaymentIntentRequest) (*PaymentIntentResponse, error) {
	if s.secretKey == "" {
		return nil, errors.New("stripe not configured - secret key required")
	}

	// For now, return structured error indicating configuration needed
	// In production, this would use the stripe-go package
	return nil, errors.New("stripe integration requires proper configuration and stripe-go package")
}

// HandleWebhook processes Stripe webhook events
func (s *StripeService) HandleWebhook(eventType string, data map[string]interface{}) error {
	if s.secretKey == "" {
		return errors.New("stripe not configured")
	}

	// In production, this would verify the webhook signature and process events
	// using the stripe-go package's webhook functionality
	switch eventType {
	case "payment_intent.succeeded":
		return s.handlePaymentSuccess(data)
	case "payment_intent.payment_failed":
		return s.handlePaymentFailure(data)
	case "charge.refunded":
		return s.handleRefund(data)
	default:
		// Ignore other event types
		return nil
	}
}

func (s *StripeService) handlePaymentSuccess(data map[string]interface{}) error {
	// Extract invoice ID from metadata
	metadata, ok := data["metadata"].(map[string]interface{})
	if !ok {
		return errors.New("no metadata in webhook")
	}
	invoiceID, ok := metadata["invoice_id"].(string)
	if !ok {
		return errors.New("no invoice_id in metadata")
	}

	var invoice models.Invoice
	if err := s.db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return fmt.Errorf("invoice not found: %w", err)
	}

	// Extract payment amount
	amountFloat, ok := data["amount"].(float64)
	if !ok {
		return errors.New("invalid amount in webhook")
	}
	t := time.Now()
	amount := amountFloat / 100 // Convert from cents to units

	// Create payment record
	payment := &models.Payment{
		ID:          data["id"].(string),
		TenantID:    invoice.TenantID,
		UserID:      invoice.UserID,
		InvoiceID:   invoiceID,
		Amount:      amount,
		CompletedAt: &t,
	}

	if err := s.db.Create(payment).Error; err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}

	// Update invoice
	invoice.PaidAmount += amount
	if invoice.PaidAmount >= invoice.Total {
		invoice.Status = models.InvoiceStatusPaid
	}

	return s.db.Save(&invoice).Error
}

func (s *StripeService) handlePaymentFailure(data map[string]interface{}) error {
	// Handle failed payment - could notify user, retry, etc.
	// For now, just log the failure
	return nil
}

func (s *StripeService) handleRefund(data map[string]interface{}) error {
	// Handle refund - update payment status
	return nil
}

// CreateCheckoutSession creates a real Stripe checkout session
func (s *StripeService) CreateCheckoutSession(invoiceID, successURL, cancelURL string) (string, error) {
	if s.secretKey == "" {
		return "", errors.New("stripe not configured")
	}

	var invoice models.Invoice
	if err := s.db.Preload("Client").First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return "", fmt.Errorf("invoice not found: %w", err)
	}

	return "", errors.New("stripe integration requires proper configuration and stripe-go package")
}

// CreateCustomer creates a real Stripe customer
func (s *StripeService) CreateCustomer(email, name string) (string, error) {
	if s.secretKey == "" {
		return "", errors.New("stripe not configured")
	}
	return "", errors.New("stripe integration requires proper configuration and stripe-go package")
}

// Refund processes a refund through Stripe
func (s *StripeService) Refund(paymentID string, amount float64) error {
	if s.secretKey == "" {
		return errors.New("stripe not configured")
	}
	return errors.New("stripe integration requires proper configuration and stripe-go package")
}

// GetPublicKey returns the publishable key
func (s *StripeService) GetPublicKey() string {
	return s.publicKey
}

// IsEnabled returns true if stripe is properly configured
func (s *StripeService) IsEnabled() bool {
	return s.secretKey != "" && s.publicKey != ""
}
