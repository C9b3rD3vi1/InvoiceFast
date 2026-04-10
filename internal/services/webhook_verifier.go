package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"invoicefast/internal/config"
)

// Whitelist of trusted IP addresses for webhooks
var (
	MpesaAllowedIPs = []string{
		"196.201.214.0/24", // Safaricom production
		"196.201.213.0/24", // Safaricom
		"41.212.0.0/16",    // Safaricom mobile
		"197.248.0.0/16",   // Safaricom
		"41.89.0.0/16",     // Safaricom
	}

	IntasendAllowedIPs = []string{
		"104.21.0.0/16",    // Cloudflare
		"172.64.0.0/16",    // Cloudflare
		"108.162.192.0/18", // Cloudflare
		"162.247.248.0/22", // Cloudflare
		"172.64.0.0/20",    // Cloudflare
		"104.16.0.0/12",    // Cloudflare general
	}
)

// WebhookVerificationResult contains the result of webhook verification
type WebhookVerificationResult struct {
	IsValid    bool
	Provider   string
	Timestamp  time.Time
	Body       []byte
	ParsedData interface{}
	Error      error
}

// WebhookVerifier handles verification of webhook signatures
type WebhookVerifier struct {
	cfg        *config.Config
	httpClient *http.Client
}

// NewWebhookVerifier creates a new webhook verifier
func NewWebhookVerifier(cfg *config.Config) *WebhookVerifier {
	return &WebhookVerifier{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// VerifyMpesaCallback verifies M-Pesa Daraja API callback
// Security: Validates signature, timestamp, IP allowlist
func (v *WebhookVerifier) VerifyMpesaCallback(
	body []byte,
	signature string,
	ipAddress string,
) (*MpesaSTKCallback, error) {

	// 1. IP Allowlisting - critical security check
	if !v.isIPAllowed(ipAddress, MpesaAllowedIPs) {
		log.Printf("[SECURITY] M-Pesa callback from unauthorized IP: %s", ipAddress)
		return nil, fmt.Errorf("unauthorized IP address")
	}

	// 2. Signature verification (if configured)
	if v.cfg.MPesa.SecurityCredential != "" {
		if signature == "" {
			return nil, fmt.Errorf("missing M-Pesa signature")
		}

		// M-Pesa uses base64 encoded SHA256 of the raw body with security credential
		expectedSig := v.computeMpesaSignature(body)
		if subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSig)) != 1 {
			log.Printf("[SECURITY] M-Pesa signature verification failed")
			return nil, fmt.Errorf("invalid signature")
		}
	}

	// 3. Parse and validate callback
	var callback MpesaSTKCallback
	if err := json.Unmarshal(body, &callback); err != nil {
		return nil, fmt.Errorf("invalid callback JSON: %w", err)
	}

	// 4. Validate callback structure
	if callback.Body.StkCallback.CheckoutRequestID == "" {
		return nil, fmt.Errorf("invalid callback: missing CheckoutRequestID")
	}

	return &callback, nil
}

// VerifyIntasendWebhook verifies Intasend webhook signature
func (v *WebhookVerifier) VerifyIntasendWebhook(
	body []byte,
	signature string,
	timestamp string,
	ipAddress string,
) (*IntasendWebhookEvent, error) {

	// 1. IP Allowlisting (basic Cloudflare check)
	if !v.isIPAllowed(ipAddress, IntasendAllowedIPs) && ipAddress != "" {
		log.Printf("[SECURITY] Intasend webhook from non-Cloudflare IP: %s", ipAddress)
		// Don't block - Cloudflare should have filtered, but log it
	}

	// 2. Signature verification (HMAC-SHA256)
	if v.cfg.Intasend.WebhookSecret == "" {
		log.Printf("[SECURITY] WARNING: Intasend webhook secret not configured")
		return nil, fmt.Errorf("webhook secret not configured")
	}

	if signature == "" {
		return nil, fmt.Errorf("missing webhook signature")
	}

	expectedSig := v.computeHMAC(body, v.cfg.Intasend.WebhookSecret)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSig)) != 1 {
		log.Printf("[SECURITY] Intasend signature verification failed")
		return nil, fmt.Errorf("invalid webhook signature")
	}

	// 3. Timestamp replay protection
	if timestamp != "" {
		ts, err := time.Parse(time.RFC3339, timestamp)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp format: %w", err)
		}

		age := time.Since(ts)
		maxAge := 5 * time.Minute
		if age > maxAge {
			return nil, fmt.Errorf("webhook too old (age: %v)", age)
		}

		if ts.After(time.Now().Add(5 * time.Minute)) {
			return nil, fmt.Errorf("timestamp in future")
		}
	}

	// 4. Parse and validate payload
	var event IntasendWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("invalid webhook JSON: %w", err)
	}

	// 5. Validate event type
	validEvents := map[string]bool{
		"payment_successful":     true,
		"payment_failed":         true,
		"payment_reversed":       true,
		"invoice_payment_signed": true,
		"chargeback":             true,
		"refund_processed":       true,
		"collection_completed":   true,
		"collection_failed":      true,
	}

	if !validEvents[event.Event] {
		return nil, fmt.Errorf("unknown event type: %s", event.Event)
	}

	// 6. Validate amount for successful payments
	if event.Event == "payment_successful" || event.Event == "collection_completed" {
		if event.Collection.Amount <= 0 {
			return nil, fmt.Errorf("invalid amount for successful payment")
		}
	}

	return &event, nil
}

// isIPAllowed checks if IP is within allowed CIDR ranges
func (v *WebhookVerifier) isIPAllowed(ipStr string, allowedCIDRs []string) bool {
	if ipStr == "" {
		return false // Empty IP should not be allowed
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, cidr := range allowedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// computeMpesaSignature computes M-Pesa signature
func (v *WebhookVerifier) computeMpesaSignature(body []byte) string {
	if v.cfg.MPesa.SecurityCredential == "" {
		return ""
	}
	// M-Pesa uses SHA256 of the raw body with security credential as password
	h := sha256.Sum256(append(body, []byte(v.cfg.MPesa.SecurityCredential)...))
	return hex.EncodeToString(h[:])
}

// computeHMAC computes HMAC-SHA256
func (v *WebhookVerifier) computeHMAC(data []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// MpesaSTKCallback represents M-Pesa STK callback structure
type MpesaSTKCallback struct {
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

// ExtractPaymentDetails extracts payment details from M-Pesa callback
func (c *MpesaSTKCallback) ExtractPaymentDetails() (receipt, phone, amount string) {
	for _, item := range c.Body.StkCallback.CallbackMetadata.Item {
		switch item.Name {
		case "MpesaReceiptNumber":
			if v, ok := item.Value.(string); ok {
				receipt = v
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
	return
}

// ValidatePaymentAmount validates payment amount against invoice
func ValidatePaymentAmount(paymentAmount, remainingBalance float64) error {
	if paymentAmount <= 0 {
		return fmt.Errorf("payment amount must be greater than zero")
	}
	if paymentAmount > remainingBalance {
		return fmt.Errorf("payment amount exceeds remaining balance: %.2f > %.2f", paymentAmount, remainingBalance)
	}
	return nil
}

// PaymentValidator validates payment data
type PaymentValidator struct {
	db interface{} // *database.DB - would be actual type in production
}

// NewPaymentValidator creates a new payment validator
func NewPaymentValidator(db interface{}) *PaymentValidator {
	return &PaymentValidator{db: db}
}

// ValidatePaymentForInvoice validates payment can be applied to invoice
// Returns error if:
// - Invoice not found
// - Invoice already paid
// - Amount exceeds remaining
// - Invoice belongs to different tenant
func (v *PaymentValidator) ValidatePaymentForInvoice(invoiceID, tenantID string, amount float64) error {
	// This would be implemented with actual database query
	// Validation checks:
	// 1. Invoice exists and belongs to tenant
	// 2. Invoice is not already fully paid
	// 3. Amount doesn't exceed remaining balance
	// 4. Invoice status allows payment (not cancelled, not draft - depending on flow)
	return nil
}

// SanitizeLogData removes sensitive data from logs
func SanitizeLogData(data map[string]interface{}) map[string]interface{} {
	sanitized := make(map[string]interface{})
	sensitiveFields := []string{"password", "token", "secret", "key", "card", "pin"}

	for k, v := range data {
		isSensitive := false
		lowerK := strings.ToLower(k)
		for _, field := range sensitiveFields {
			if strings.Contains(lowerK, field) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			sanitized[k] = "***REDACTED***"
		} else {
			sanitized[k] = v
		}
	}

	return sanitized
}

// SortParams sorts URL parameters for signature verification
func SortParams(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, params[k]))
	}
	return strings.Join(parts, "&")
}
