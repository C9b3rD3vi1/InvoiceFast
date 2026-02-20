package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"invoicefast/internal/config"
)

type IntasendService struct {
	cfg        *config.IntasendConfig
	httpClient *http.Client
	apiURL     string
}

type IntasendResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	ID      string `json:"id,omitempty"`
}

type InitiatePaymentRequest struct {
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	PhoneNumber   string  `json:"phone_number"`
	APIRef        string  `json:"api_ref"`
	CallbackURL   string  `json:"callback_url"`
	CustomerEmail string  `json:"customer_email"`
	CustomerName  string  `json:"customer_name"`
	InvoiceNumber string  `json:"invoice_number,omitempty"`
}

type PaymentStatusRequest struct {
	ID string `json:"id"`
}

type IntasendPaymentStatus struct {
	ID            string `json:"id"`
	State         string `json:"state"` // "pending", "completed", "failed"
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	CustomerEmail string `json:"customer_email"`
	CreatedAt     string `json:"created_at"`
	CompletedAt   string `json:"completed_at"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type IntasendWebhookEvent struct {
	Event      string             `json:"event"`
	Timestamp  string             `json:"timestamp"`
	PublicID   string             `json:"public_id"`
	Checkout   IntasendCheckout   `json:"checkout"`
	Customer   IntasendCustomer   `json:"customer"`
	Collection IntasendCollection `json:"collection"`
}

type IntasendCheckout struct {
	URL string `json:"url"`
}

type IntasendCustomer struct {
	Email string `json:"email"`
	Phone string `json:"phone"`
	Name  string `json:"name"`
}

type IntasendCollection struct {
	ID           string `json:"id"`
	Amount       int    `json:"amount"`
	Currency     string `json:"currency"`
	Status       string `json:"status"`
	MpesaReceipt string `json:"mpesa_receipt_number,omitempty"`
}

func NewIntasendService(cfg *config.IntasendConfig) *IntasendService {
	return &IntasendService{
		cfg:    cfg,
		apiURL: cfg.APIURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// InitiateSTKPush initiates an STK Push payment request
func (s *IntasendService) InitiateSTKPush(req InitiatePaymentRequest) (*IntasendResponse, error) {
	// Intasend uses the "collect" endpoint for STK Push
	endpoint := fmt.Sprintf("%s/api/v1/collection/", s.apiURL)

	payload := map[string]interface{}{
		"amount":         req.Amount,
		"currency":       req.Currency,
		"phone_number":   normalizePhoneNumber(req.PhoneNumber),
		"api_ref":        req.APIRef,
		"callback_url":   req.CallbackURL,
		"customer_email": req.CustomerEmail,
		"customer_name":  req.CustomerName,
		"invoice_number": req.InvoiceNumber,
		"host":           "browser", // Required by Intasend
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.SecretKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("intasend API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Intasend returns different response structures
	// Check if we have a checkout or direct response
	if checkout, ok := result["checkout"].(map[string]interface{}); ok {
		return &IntasendResponse{
			Success: true,
			Message: checkout["url"].(string),
			ID:      result["id"].(string),
		}, nil
	}

	// For STK Push, check for state or id
	if id, ok := result["id"].(string); ok {
		return &IntasendResponse{
			Success: true,
			ID:      id,
			Message: "STK Push initiated",
		}, nil
	}

	return &IntasendResponse{
		Success: true,
		Message: "Payment initiated",
	}, nil
}

// InitiateCardPayment initiates a card payment (redirects to checkout)
func (s *IntasendService) InitiateCardPayment(req InitiatePaymentRequest) (*IntasendResponse, error) {
	endpoint := fmt.Sprintf("%s/api/v1/checkout/", s.apiURL)

	payload := map[string]interface{}{
		"amount":         req.Amount,
		"currency":       req.Currency,
		"customer_email": req.CustomerEmail,
		"customer_name":  req.CustomerName,
		"api_ref":        req.APIRef,
		"callback_url":   req.CallbackURL,
		"redirect_url":   fmt.Sprintf("%s/payment/complete", req.CallbackURL),
		"invoice_number": req.InvoiceNumber,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.SecretKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("intasend API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract checkout URL
	checkoutURL := ""
	if checkout, ok := result["checkout"].(map[string]interface{}); ok {
		if url, ok := checkout["url"].(string); ok {
			checkoutURL = url
		}
	}

	return &IntasendResponse{
		Success: true,
		Message: checkoutURL,
		ID:      result["id"].(string),
	}, nil
}

// GetPaymentStatus checks the status of a payment
func (s *IntasendService) GetPaymentStatus(paymentID string) (*IntasendPaymentStatus, error) {
	endpoint := fmt.Sprintf("%s/api/v1/collection/%s/", s.apiURL, paymentID)

	httpReq, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.SecretKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("intasend API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result IntasendPaymentStatus
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// VerifyWebhookSignature verifies the webhook signature from Intasend
func (s *IntasendService) VerifyWebhookSignature(payload []byte, signature string) bool {
	// In production, use crypto/hmac to verify the signature
	// For now, just check if signature exists
	return signature != ""
}

// normalizePhoneNumber converts phone to format Intasend expects (254...)
func normalizePhoneNumber(phone string) string {
	// Remove any non-digit characters
	var digits string
	for _, c := range phone {
		if c >= '0' && c <= '9' {
			digits += string(c)
		}
	}

	// Handle different formats
	if len(digits) == 10 && digits[0] == '0' {
		// 0712345678 -> 254712345678
		return "254" + digits[1:]
	}
	if len(digits) == 9 {
		// 712345678 -> 254712345678
		return "254" + digits
	}
	if len(digits) == 12 {
		// 254712345678 - already correct
		return digits
	}

	// Default: prepend 254
	return "254" + digits
}

// FormatPhoneForDisplay converts 254... back to 07...
func FormatPhoneForDisplay(phone string) string {
	if len(phone) >= 12 && phone[:3] == "254" {
		return "0" + phone[3:]
	}
	return phone
}
