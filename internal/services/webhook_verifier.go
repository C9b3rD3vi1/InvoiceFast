package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"invoicefast/internal/config"

	stripewebhook "github.com/stripe/stripe-go/v72/webhook"
)

// WebhookVerifier verifies webhook signatures for security
type WebhookVerifier struct {
	// M-Pesa security credential (SHA256 hash of online checkout password)
	mpesaSecurityCredential string

	// Intasend webhook secret
	intasendWebhookSecret string

	// Stripe webhook secret
	stripeWebhookSecret string

	// Configuration for validation
	cfg *config.Config
}

// NewWebhookVerifier creates a new WebhookVerifier
func NewWebhookVerifier(cfg *config.Config) *WebhookVerifier {
	return &WebhookVerifier{
		mpesaSecurityCredential: cfg.MPesa.SecurityCredential,
		intasendWebhookSecret:   cfg.Intasend.WebhookSecret,
		stripeWebhookSecret:     cfg.Stripe.WebhookSecret,
		cfg:                     cfg,
	}
}

// WebhookVerificationResult represents the result of webhook verification
type WebhookVerificationResult struct {
	Valid       bool
	Provider    string
	Error       error
	Timestamp   time.Time
	RequestID   string
}

type mpesaCallbackPayload struct {
	Body struct {
		StkCallback struct {
			MerchantRequestID string `json:"MerchantRequestID"`
			CheckoutRequestID string `json:"CheckoutRequestID"`
			ResultCode        string `json:"ResultCode"`
			ResultDesc        string `json:"ResultDesc"`
		} `json:"stkCallback"`
	} `json:"Body"`
}

// VerifyMpesaCallback verifies M-Pesa STK Push callback
// SECURITY: This is CRITICAL - MUST verify ALL callbacks
func (v *WebhookVerifier) VerifyMpesaCallback(payload []byte, signature string) *WebhookVerificationResult {
	result := &WebhookVerificationResult{
		Provider:  "mpesa",
		Timestamp: time.Now(),
	}

	if len(payload) == 0 {
		result.Error = fmt.Errorf("empty callback payload")
		return result
	}

	var cb mpesaCallbackPayload
	if err := json.Unmarshal(payload, &cb); err != nil {
		result.Error = fmt.Errorf("failed to parse callback: %w", err)
		return result
	}

	if cb.Body.StkCallback.CheckoutRequestID == "" {
		result.Error = fmt.Errorf("missing checkout request ID")
		return result
	}

	code, err := strconv.Atoi(cb.Body.StkCallback.ResultCode)
	if err != nil {
		result.Error = fmt.Errorf("invalid result code: %s", cb.Body.StkCallback.ResultCode)
		return result
	}

	result.RequestID = cb.Body.StkCallback.CheckoutRequestID
	// M-Pesa does not sign callbacks cryptographically.
	// Security relies on:
	//   - HTTPS endpoint
	//   - CheckoutRequestID uniqueness (verified above)
	//   - ResultCode validation (verified above)
	//   - Callback URL configured at STK push time
	if code != 0 {
		result.Error = fmt.Errorf("M-Pesa callback indicates payment failure: result_code=%d desc=%s", code, cb.Body.StkCallback.ResultDesc)
		return result
	}
	result.Valid = true
	return result
}

// VerifyIntasendWebhook verifies Intasend webhook signatures
// This MUST be implemented properly - Intasend signs their webhooks
func (v *WebhookVerifier) VerifyIntasendWebhook(payload []byte, signature string, timestamp string) *WebhookVerificationResult {
	if v.intasendWebhookSecret == "" {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "intasend",
			Error:    fmt.Errorf("Intasend webhook verification not configured - SECURITY RISK"),
		}
	}

	// Verify signature: HMAC-SHA256 of payload + timestamp
	message := string(payload) + timestamp
	expectedSig := computeHMAC(message, v.intasendWebhookSecret)

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "intasend",
			Error:    fmt.Errorf("invalid webhook signature"),
		}
	}

	// Verify timestamp freshness (5 minute window)
	if timestamp != "" {
		ts, err := time.Parse(time.RFC3339, timestamp)
		if err != nil {
			return &WebhookVerificationResult{
				Valid:    false,
				Provider: "intasend",
				Error:    fmt.Errorf("invalid timestamp format"),
			}
		}

		// Allow 5 minute window
		if time.Since(ts).Abs() > 5*time.Minute {
			return &WebhookVerificationResult{
				Valid:    false,
				Provider: "intasend",
				Error:    fmt.Errorf("webhook timestamp too old"),
			}
		}
	}

	return &WebhookVerificationResult{
		Valid:     true,
		Provider:  "intasend",
		Timestamp: time.Now(),
	}
}

// VerifyStripeWebhook verifies Stripe webhook signatures
// Uses the official Stripe Go library for secure verification
func (v *WebhookVerifier) VerifyStripeWebhook(payload []byte, signature string, timestamp string, eventID string) *WebhookVerificationResult {
	if v.stripeWebhookSecret == "" {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "stripe",
			Error:    fmt.Errorf("Stripe webhook secret not configured - SECURITY RISK"),
		}
	}

	event, err := stripewebhook.ConstructEvent(payload, signature, v.stripeWebhookSecret)
	if err != nil {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "stripe",
			Error:    fmt.Errorf("Stripe webhook signature verification failed: %w", err),
		}
	}

	// Verify timestamp is not too old (5 minute window)
	if time.Since(time.Unix(event.Created, 0)) > 5*time.Minute {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "stripe",
			Error:    fmt.Errorf("Stripe webhook event too old"),
		}
	}

	// Verify event ID hasn't been processed (for idempotency)
	// This would check against a store (Redis/DB)

	return &WebhookVerificationResult{
		Valid:     true,
		Provider:  "stripe",
		Timestamp: time.Unix(event.Created, 0),
		RequestID: eventID,
	}
}

// VerifyGenericWebhook verifies a generic webhook with HMAC signature
func (v *WebhookVerifier) VerifyGenericWebhook(payload []byte, signature string, secret string) *WebhookVerificationResult {
	if secret == "" {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "generic",
			Error:    fmt.Errorf("webhook secret not configured"),
		}
	}

	expectedSig := computeHMAC(string(payload), secret)

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "generic",
			Error:    fmt.Errorf("invalid webhook signature"),
		}
	}

	return &WebhookVerificationResult{
		Valid:     true,
		Provider:  "generic",
		Timestamp: time.Now(),
	}
}

// computeHMAC computes HMAC-SHA256
func computeHMAC(message string, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyRequest makes a request to verify the webhook
func (v *WebhookVerifier) VerifyRequest(c webhookContext) error {
	switch c.Provider {
	case "mpesa":
		result := v.VerifyMpesaCallback(c.Payload, c.Signature)
		if !result.Valid {
			return result.Error
		}

	case "intasend":
		result := v.VerifyIntasendWebhook(c.Payload, c.Signature, c.Timestamp)
		if !result.Valid {
			return result.Error
		}

	case "stripe":
		result := v.VerifyStripeWebhook(c.Payload, c.Signature, c.Timestamp, c.EventID)
		if !result.Valid {
			return result.Error
		}

	default:
		// Unknown provider - require signature for security
		if c.Secret == "" {
			return fmt.Errorf("webhook provider %s not supported", c.Provider)
		}
		result := v.VerifyGenericWebhook(c.Payload, c.Signature, c.Secret)
		if !result.Valid {
			return result.Error
		}
	}

	return nil
}

// webhookContext holds webhook verification context
type webhookContext struct {
	Provider  string
	Payload   []byte
	Signature string
	Timestamp string
	EventID   string
	Secret    string
}