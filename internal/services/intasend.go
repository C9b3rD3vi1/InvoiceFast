package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"invoicefast/internal/logger"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrIntaSendNotConfigured = errors.New("intasend not configured")
var ErrInvalidSignature = errors.New("invalid webhook signature")
var ErrAlreadyProcessed = errors.New("payment already processed")

type IntasendService struct {
	cfg        *config.IntasendConfig
	db         *database.DB
	httpClient *http.Client
	notifySvc  *NotificationService
	apiKey    string
	apiURL    string
	pubKey    string
}

func NewIntasendServiceWithDB(db *database.DB, cfg *config.IntasendConfig, notifySvc *NotificationService) *IntasendService {
	if cfg == nil {
		return &IntasendService{db: db}
	}

	return &IntasendService{
		cfg:        cfg,
		db:         db,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		notifySvc:  notifySvc,
		apiKey:    cfg.APIKey,
		apiURL:    cfg.APIURL,
		pubKey:   cfg.PublicKey,
	}
}

type InitiatePaymentRequest struct {
	Amount         float64 `json:"amount"`
	Currency      string  `json:"currency"`
	PhoneNumber   string  `json:"phone_number"`
	APIRef        string  `json:"api_ref"`
	CallbackURL   string  `json:"callback_url"`
	CustomerEmail string  `json:"customer_email"`
	CustomerName  string  `json:"customer_name"`
	InvoiceNumber string  `json:"invoice_number,omitempty"`
}

type IntasendCheckoutResponse struct {
	URL string `json:"url"`
	ID  string `json:"id"`
}

type IntasendWebhookPayload struct {
	Event      string `json:"event"`
	Timestamp string `json:"timestamp"`
	PublicID  string `json:"public_id"`

	Checkout struct {
		ID     string `json:"id"`
		APIRef string `json:"api_ref,omitempty"`
	} `json:"checkout"`

	Collection struct {
		ID            string `json:"id"`
		Amount       int    `json:"amount"`
		Currency     string `json:"currency"`
		Status       string `json:"status"`
		MpesaReceipt string `json:"mpesa_receipt_number,omitempty"`
	} `json:"collection"`

	Customer struct {
		Email string `json:"email"`
		Phone string `json:"phone"`
		Name  string `json:"name"`
	} `json:"customer"`
}

type IntasendWebhookEvent struct {
	Event      string `json:"event"`
	Timestamp string `json:"timestamp"`
	PublicID  string `json:"public_id"`

	Checkout struct {
		ID string `json:"id"`
	} `json:"checkout"`

	Collection struct {
		ID            string `json:"id"`
		Amount       int    `json:"amount"`
		Currency     string `json:"currency"`
		Status       string `json:"status"`
		MpesaReceipt string `json:"mpesa_receipt_number,omitempty"`
	} `json:"collection"`

	Customer struct {
		Email string `json:"email"`
		Phone string `json:"phone"`
		Name  string `json:"name"`
	} `json:"customer"`
}

func (s *IntasendService) IsConfigured() bool {
	return s.cfg != nil && s.apiKey != "" && s.apiURL != ""
}

func (s *IntasendService) InitiatePayment(req *InitiatePaymentRequest) (*IntasendCheckoutResponse, error) {
	if !s.IsConfigured() {
		return nil, ErrIntaSendNotConfigured
	}

	payload := map[string]interface{}{
		"amount":          req.Amount,
		"currency":       req.Currency,
		"phone_number":   req.PhoneNumber,
		"api_ref":        req.APIRef,
		"callback_url":   req.CallbackURL,
		"customer_email": req.CustomerEmail,
		"customer_name": req.CustomerName,
	}

	if req.InvoiceNumber != "" {
		payload["invoice_number"] = req.InvoiceNumber
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", s.apiURL+"/api/v1/checkout/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("intasend API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		ID      string `json:"id"`
		URL     string `json:"url"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	if !result.Success {
		return nil, errors.New(result.Message)
	}

	return &IntasendCheckoutResponse{
		URL: result.URL,
		ID:  result.ID,
	}, nil
}

func (s *IntasendService) HandleWebhook(payload []byte, signature string, sourceIP string) error {
	if !s.IsConfigured() {
		logger.Get().Warn(context.Background(), "Webhook received but service not configured")
		return nil
	}

	// Verify signature if secret is configured
	if s.cfg.WebhookSecret != "" {
		expectedSig := computeHMAC(string(payload), s.cfg.WebhookSecret)
		if signature != expectedSig {
			logger.Get().Error(context.Background(), "Invalid webhook signature", "source_ip", sourceIP)
			return ErrInvalidSignature
		}
	}

	var event IntasendWebhookPayload
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("invalid webhook payload: %w", err)
	}

	checkoutID := event.Checkout.ID
	tenantID := event.Customer.Email

	alreadyProcessed, err := s.isAlreadyProcessed(checkoutID)
	if err == nil && alreadyProcessed {
		logger.Get().Info(context.Background(), "Duplicate webhook for checkout - skipping", "checkout_id", checkoutID)
		return nil
	}

	switch event.Event {
	case "checkout.complete":
		return s.handlePaymentSuccess(checkoutID, tenantID, event)
	case "checkout.failed":
		return s.handlePaymentFailed(checkoutID, tenantID, event)
	case "checkout.pending":
		return s.handlePaymentPending(checkoutID, tenantID, event)
	}

	logger.Get().Warn(context.Background(), "Unhandled webhook event", "event", event.Event)
	return nil
}

func (s *IntasendService) handlePaymentSuccess(checkoutID, tenantID string, event IntasendWebhookPayload) error {
	amount := models.Money(event.Collection.Amount)
	currency := event.Collection.Currency

	existingPayment, err := s.findPaymentByRef(checkoutID)
	if err == nil && existingPayment != nil {
		if existingPayment.Status == models.PaymentStatusCompleted {
			logger.Get().Info(context.Background(), "Payment already processed for checkout", "checkout_id", checkoutID)
			return nil
		}
	}

	var invoice models.Invoice
	if s.db != nil && tenantID != "" && event.Checkout.APIRef != "" {
		s.db.Scopes(database.TenantFilter(tenantID)).First(&invoice, "invoice_number = ? OR id = ?", event.Checkout.APIRef, event.Checkout.APIRef)
	}

	payment := &models.Payment{
		ID:            uuid.New().String(),
		TenantID:      tenantID,
		InvoiceID:     invoice.ID,
		Amount:        amount,
		Currency:       currency,
		Method:        models.PaymentMethodIntasend,
		Status:        models.PaymentStatusCompleted,
		Reference:     event.Collection.MpesaReceipt,
		IntasendID:    event.Collection.ID,
	}
	now := time.Now()
	payment.CompletedAt = &now

	if err := s.db.Create(payment).Error; err != nil {
		return fmt.Errorf("failed to record payment: %w", err)
	}

	if invoice.ID != "" {
		invoice.PaidAmount += amount
		if invoice.PaidAmount >= invoice.Total {
			invoice.Status = models.InvoiceStatusPaid
			invoice.PaidAt = &now
		} else {
			invoice.Status = models.InvoiceStatusPartiallyPaid
		}
		s.db.Save(&invoice)
	}

	s.recordIdempotency(tenantID, checkoutID, "payment", payment.ID)
	s.sendNotification(tenantID, EventPaymentReceived, fmt.Sprintf("Payment of %.2f %s received via M-Pesa", amount.Float64(), currency))

	logger.Get().Info(context.Background(), "Payment succeeded", "checkout_id", checkoutID, "currency", currency, "amount", amount.Float64())
	return nil
}

func (s *IntasendService) handlePaymentFailed(checkoutID, tenantID string, event IntasendWebhookPayload) error {
	logger.Get().Warn(context.Background(), "Payment failed for checkout", "checkout_id", checkoutID)

	existingPayment, _ := s.findPaymentByRef(checkoutID)
	if existingPayment != nil {
		existingPayment.Status = models.PaymentStatusFailed
		s.db.Save(existingPayment)
	}

	s.sendNotification(tenantID, EventPaymentFailed, "Payment failed")
	return nil
}

func (s *IntasendService) handlePaymentPending(checkoutID, tenantID string, event IntasendWebhookPayload) error {
	logger.Get().Info(context.Background(), "Payment pending for checkout", "checkout_id", checkoutID)
	return nil
}

func (s *IntasendService) isAlreadyProcessed(checkoutID string) (bool, error) {
	var payment models.Payment
	err := s.db.Where("reference = ? OR intasend_id = ?", checkoutID, checkoutID).First(&payment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return payment.Status == models.PaymentStatusCompleted, nil
}

func (s *IntasendService) findPaymentByRef(ref string) (*models.Payment, error) {
	var payment models.Payment
	err := s.db.Where("reference = ? OR intasend_id = ?", ref, ref).First(&payment).Error
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

func (s *IntasendService) recordIdempotency(tenantID, key, eventType, reference string) error {
	// Using Payment model for idempotency tracking
	var existing models.Payment
	if err := s.db.Where("reference = ? OR intasend_id = ?", key, key).First(&existing).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Record doesn't exist, which is what we want for idempotency check
		return nil
	}
	// Record exists, check if it's already processed
	if existing.Status == models.PaymentStatusCompleted {
		return errors.New("already processed")
	}
	return nil
}

func (s *IntasendService) sendNotification(tenantID, eventType, message string) {
	if s.notifySvc == nil {
		return
	}

	s.notifySvc.Send(context.Background(), &NotificationRequest{
		TenantID:  tenantID,
		UserID:   tenantID,
		EventType: eventType,
		Channels: []string{ChannelEmail},
		Subject:  "Payment Notification",
		Body:     message,
	})
}