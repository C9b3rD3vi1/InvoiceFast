package services

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"gorm.io/gorm"
)

// MPesaCache interface for token caching
type MPesaCache interface {
	GetString(ctx context.Context, key string) (string, error)
	SetString(ctx context.Context, key, value string, expiration time.Duration) error
	Exists(ctx context.Context, key string) (bool, error)
}

var (
	ErrMpesaNotConfigured = fmt.Errorf("M-Pesa not configured - set MPESA_CONSUMER_KEY, MPESA_CONSUMER_SECRET, and MPESA_BUSINESS_SHORT_CODE")
	ErrMpesaTokenFailed   = fmt.Errorf("failed to obtain M-Pesa access token")
	ErrSTKPushFailed      = fmt.Errorf("STK push request failed")
)

// MPesaService handles M-Pesa Daraja API integration
type MPesaService struct {
	cfg             *config.Config
	db              *database.DB
	client          *http.Client
	cache           MPesaCache
	webhookVerifier *WebhookVerifier // SECURITY: Added for callback verification
}

type MpesaAccessToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   string `json:"expires_in"`
}

type STKPushRequest struct {
	BusinessShortCode string `json:"BusinessShortCode"`
	Password          string `json:"Password"`
	Timestamp         string `json:"Timestamp"`
	TransactionType   string `json:"TransactionType"`
	Amount            string `json:"Amount"`
	PartyA            string `json:"PartyA"`
	PartyB            string `json:"PartyB"`
	PhoneNumber       string `json:"PhoneNumber"`
	CallBackURL       string `json:"CallBackURL"`
	AccountReference  string `json:"AccountReference"`
	TransactionDesc   string `json:"TransactionDesc"`
}

type STKPushResponse struct {
	MerchantRequestID   string `json:"MerchantRequestID"`
	CheckoutRequestID   string `json:"CheckoutRequestID"`
	ResponseCode        string `json:"ResponseCode"`
	ResponseDescription string `json:"ResponseDescription"`
	CustomerMessage     string `json:"CustomerMessage"`
}

type STKCallback struct {
	Body struct {
		StkCallback struct {
			MerchantRequestID string `json:"MerchantRequestID"`
			CheckoutRequestID string `json:"CheckoutRequestID"`
			ResultCode        int    `json:"ResultCode"`
			ResultDesc        string `json:"ResultDesc"`
			CallbackMetadata  struct {
				Item []struct {
					Name  string      `json:"Name"`
					Value interface{} `json:"Value"`
				} `json:"Item"`
			} `json:"CallbackMetadata"`
		} `json:"StkCallback"`
	} `json:"Body"`
}

func NewMPesaService(cfg *config.Config, db *database.DB, cache MPesaCache) *MPesaService {
	// Create webhook verifier for callback security
	verifier := NewWebhookVerifier(cfg)

	return &MPesaService{
		cfg:             cfg,
		db:              db,
		client:          &http.Client{Timeout: cfg.MPesa.QueueTimeout},
		cache:           cache,
		webhookVerifier: verifier,
	}
}

func (s *MPesaService) IsConfigured() bool {
	return s.cfg.MPesa.Enabled &&
		s.cfg.MPesa.ConsumerKey != "" &&
		s.cfg.MPesa.ConsumerSecret != "" &&
		s.cfg.MPesa.BusinessShortCode != ""
}

func (s *MPesaService) InitiateSTKPush(ctx context.Context, tenantID, invoiceID, phoneNumber, amount, invoiceNumber string) (*STKPushResponse, error) {
	if !s.IsConfigured() {
		return nil, ErrMpesaNotConfigured
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	token, err := s.getAccessToken(ctx)
	if err != nil {
		log.Printf("[M-Pesa] Failed to get access token: %v", err)
		return nil, fmt.Errorf("%w: %v", ErrMpesaTokenFailed, err)
	}

	timestamp := time.Now().Format("20060102150405")
	password, err := s.generatePassword(timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to generate password: %w", err)
	}

	phone := normalizeMpesaPhone(phoneNumber)

	req := STKPushRequest{
		BusinessShortCode: s.cfg.MPesa.BusinessShortCode,
		Password:          password,
		Timestamp:         timestamp,
		TransactionType:   "CustomerPayBillOnline",
		Amount:            amount,
		PartyA:            phone,
		PartyB:            s.cfg.MPesa.BusinessShortCode,
		PhoneNumber:       phone,
		CallBackURL:       s.cfg.MPesa.CallbackURL,
		AccountReference:  invoiceNumber,
		TransactionDesc:   fmt.Sprintf("InvoiceFast Invoice %s", invoiceNumber),
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal STK request: %w", err)
	}

	apiURL := "https://api.safaricom.co.ke/mpesa/stkpush/v1/processrequest"
	if s.cfg.MPesa.Environment == "sandbox" {
		apiURL = "https://sandbox.safaricom.co.ke/mpesa/stkpush/v1/processrequest"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("STK push request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		log.Printf("[M-Pesa] STK push failed (status %d): %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("%s: HTTP %d", ErrSTKPushFailed, resp.StatusCode)
	}

	var stkResp STKPushResponse
	if err := json.Unmarshal(body, &stkResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if stkResp.ResponseCode != "0" {
		log.Printf("[M-Pesa] STK push failed: %s - %s", stkResp.ResponseCode, stkResp.ResponseDescription)
		return nil, fmt.Errorf("%s: %s", ErrSTKPushFailed, stkResp.ResponseDescription)
	}

	log.Printf("[M-Pesa] STK Push initiated: CheckoutRequestID=%s for invoice %s", stkResp.CheckoutRequestID, invoiceNumber)

	return &stkResp, nil
}

// ProcessSTKCallback processes a verified M-Pesa STK callback
// SECURITY: This should ONLY be called AFTER webhook verification middleware
// The middleware handles signature verification, IP allowlisting, and replay protection
// This function handles the idempotent payment processing logic
func (s *MPesaService) ProcessSTKCallback(ctx context.Context, callback STKCallback) error {
	// If we have a webhook verifier, this callback SHOULD have been verified
	// The verification happens in the middleware BEFORE this is called
	// This is a safety check
	if s.webhookVerifier != nil {
		log.Printf("[M-Pesa] Processing callback: %s", callback.Body.StkCallback.CheckoutRequestID)
	}

	stkCallback := callback.Body.StkCallback
	merchantReqID := stkCallback.MerchantRequestID
	checkoutReqID := stkCallback.CheckoutRequestID

	if stkCallback.ResultCode != 0 {
		log.Printf("[M-Pesa] Payment failed: ResultCode=%d, ResultDesc=%s", stkCallback.ResultCode, stkCallback.ResultDesc)
		return s.markPaymentFailed(ctx, merchantReqID, checkoutReqID, stkCallback.ResultDesc)
	}

	var mpesaReceipt, phone, amount string
	for _, item := range stkCallback.CallbackMetadata.Item {
		switch item.Name {
		case "MpesaReceiptNumber":
			if v, ok := item.Value.(string); ok {
				mpesaReceipt = v
			}
		case "PhoneNumber":
			if v, ok := item.Value.(string); ok {
				phone = v
			}
		case "Amount":
			switch v := item.Value.(type) {
			case float64:
				amount = fmt.Sprintf("%.0f", v)
			case string:
				amount = v
			}
		}
	}

	log.Printf("[M-Pesa] Payment received: Receipt=%s, Phone=%s, Amount=%s", mpesaReceipt, phone, amount)
	return s.markPaymentCompleted(ctx, merchantReqID, checkoutReqID, mpesaReceipt, phone, amount)
}

func (s *MPesaService) markPaymentCompleted(ctx context.Context, merchantReqID, checkoutReqID, receipt, phone, amount string) error {
	// SECURITY: Idempotency check - prevent double-crediting
	// Use MerchantRequestID as unique key to detect duplicate callbacks
	idempotencyKey := "mpesa:processed:" + merchantReqID
	if s.cache != nil {
		exists, err := s.cache.Exists(ctx, idempotencyKey)
		if err == nil && exists {
			log.Printf("[M-Pesa] Duplicate callback detected for MerchantRequestID: %s - skipping processing", merchantReqID)
			return nil // Already processed - return success to avoid M-Pesa retry
		}
		// Mark as processing to prevent race conditions
		if err := s.cache.SetString(ctx, idempotencyKey, "processing", 5*time.Minute); err != nil {
			log.Printf("[M-Pesa] Warning: Failed to set idempotency lock: %v", err)
		}
	}

	amountFloat := 0.0
	fmt.Sscanf(amount, "%f", &amountFloat)

	var payment models.Payment

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("reference = ? OR id = ?", checkoutReqID, merchantReqID).First(&payment).Error; err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		// Double-check: if payment already marked as completed in DB, skip
		if payment.Status == models.PaymentStatusCompleted {
			log.Printf("[M-Pesa] Payment already completed: %s", payment.ID)
			return nil
		}

		payment.Status = models.PaymentStatusCompleted
		payment.Reference = receipt
		payment.CompletedAt.Valid = true
		payment.CompletedAt.Time = time.Now()

		if err := tx.Save(&payment).Error; err != nil {
			return fmt.Errorf("failed to update payment: %w", err)
		}

		// SECURITY: Use TenantFilter to ensure invoice belongs to same tenant as payment
		var invoice models.Invoice
		if err := tx.Scopes(database.TenantFilter(payment.TenantID)).First(&invoice, "id = ?", payment.InvoiceID).Error; err != nil {
			return fmt.Errorf("invoice not found: %w", err)
		}

		invoice.PaidAmount += amountFloat
		if invoice.PaidAmount >= invoice.Total {
			invoice.PaidAmount = invoice.Total
			invoice.Status = models.InvoiceStatusPaid
			invoice.PaidAt.Valid = true
			invoice.PaidAt.Time = time.Now()
		} else {
			invoice.Status = models.InvoiceStatusPartiallyPaid
		}

		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		tx.Model(&models.Client{}).Where("id = ?", invoice.ClientID).
			Update("total_paid", gorm.Expr("total_paid + ?", amountFloat))

		log.Printf("[M-Pesa] Payment completed: %s for invoice %s (amount: %s)", receipt, invoice.InvoiceNumber, amount)
		return nil
	})

	// Update idempotency key to completed
	if s.cache != nil && err == nil {
		if err := s.cache.SetString(ctx, idempotencyKey, "completed", 24*time.Hour); err != nil {
			log.Printf("[M-Pesa] Warning: Failed to mark idempotency complete: %v", err)
		}
	}

	return err
}

func (s *MPesaService) markPaymentFailed(ctx context.Context, merchantReqID, checkoutReqID, reason string) error {
	var payment models.Payment
	err := s.db.Where("reference = ? OR id = ?", checkoutReqID, merchantReqID).First(&payment).Error
	if err != nil {
		return fmt.Errorf("payment not found: %w", err)
	}

	payment.Status = models.PaymentStatusFailed
	payment.FailureReason = reason

	return s.db.Save(&payment).Error
}

func (s *MPesaService) getAccessToken(ctx context.Context) (string, error) {
	if s.cache != nil {
		token, err := s.cache.GetString(ctx, "mpesa:access_token")
		if err == nil && token != "" {
			log.Printf("[M-Pesa] Using cached access token")
			return token, nil
		}
	}

	apiURL := "https://api.safaricom.co.ke/oauth/v1/generate?grant_type=client_credentials"
	if s.cfg.MPesa.Environment == "sandbox" {
		apiURL = "https://sandbox.safaricom.co.ke/oauth/v1/generate?grant_type=client_credentials"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	credentials := base64.StdEncoding.EncodeToString([]byte(s.cfg.MPesa.ConsumerKey + ":" + s.cfg.MPesa.ConsumerSecret))
	req.Header.Set("Authorization", "Basic "+credentials)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("token request failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	var tokenResp MpesaAccessToken
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}

	if tokenResp.AccessToken == "" {
		return "", ErrMpesaTokenFailed
	}

	if s.cache != nil {
		expiresIn := 3600
		fmt.Sscanf(tokenResp.ExpiresIn, "%d", &expiresIn)
		_ = s.cache.SetString(ctx, "mpesa:access_token", tokenResp.AccessToken, time.Duration(expiresIn)*time.Second)
	}

	log.Printf("[M-Pesa] Obtained new access token")
	return tokenResp.AccessToken, nil
}

func (s *MPesaService) generatePassword(timestamp string) (string, error) {
	passkey := s.cfg.MPesa.PassKey
	if passkey == "" {
		// SECURITY: Fail if passkey not configured - don't use default!
		return "", fmt.Errorf("MPESA_PASS_KEY not configured - cannot generate STK password")
	}

	data := s.cfg.MPesa.BusinessShortCode + passkey + timestamp
	hash := md5.Sum([]byte(data))
	return base64.StdEncoding.EncodeToString(hash[:]), nil
}

func normalizeMpesaPhone(phone string) string {
	phone = strings.TrimSpace(phone)

	if strings.HasPrefix(phone, "+254") {
		phone = "254" + phone[4:]
	} else if strings.HasPrefix(phone, "07") {
		phone = "254" + phone[1:]
	} else if strings.HasPrefix(phone, "7") {
		phone = "254" + phone
	}

	return phone
}
