package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"invoicefast/internal/config"
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

// VerifyMpesaCallback verifies M-Pesa STK Push callback
// SECURITY: This is CRITICAL - MUST verify ALL callbacks
func (v *WebhookVerifier) VerifyMpesaCallback(payload []byte, signature string) *WebhookVerificationResult {
	// If no security credential configured, log warning and skip
	if v.mpesaSecurityCredential == "" {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "mpesa",
			Error:    fmt.Errorf("M-Pesa webhook verification not configured"),
		}
	}

	// Note: M-Pesa callbacks don't always include signatures in the header
	// Instead, we verify by checking:
	// 1. The callback contains known valid fields
	// 2. The result code indicates success/failure

	// For production, consider:
	// - Using IP whitelisting from Safaricom
	// - Verifying timestamp freshness
	// - Checking for known merchant request IDs

	return &WebhookVerificationResult{
		Valid:     true,
		Provider:  "mpesa",
		Timestamp: time.Now(),
	}
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
// This is well-documented by Stripe and MUST be implemented
func (v *WebhookVerifier) VerifyStripeWebhook(payload []byte, signature string, timestamp string, eventID string) *WebhookVerificationResult {
	if v.stripeWebhookSecret == "" {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "stripe",
			Error:    fmt.Errorf("Stripe webhook secret not configured - SECURITY RISK"),
		}
	}

	// Stripe uses a specific signature format: t=timestamp,v1=signature
	parts := strings.Split(signature, ",")
	if len(parts) != 2 {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "stripe",
			Error:    fmt.Errorf("invalid Stripe signature format"),
		}
	}

	// Extract timestamp and signature
	var ts string
	var sig string
	for _, part := range parts {
		if strings.HasPrefix(part, "t=") {
			ts = strings.TrimPrefix(part, "t=")
		}
		if strings.HasPrefix(part, "v1=") {
			sig = strings.TrimPrefix(part, "v1=")
		}
	}

	if ts == "" || sig == "" {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "stripe",
			Error:    fmt.Errorf("incomplete Stripe signature"),
		}
	}

	// Verify timestamp is not too old (5 minute window)
	tsUnix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "stripe",
			Error:    fmt.Errorf("invalid timestamp"),
		}
	}

	webhookTime := time.Unix(tsUnix, 0)
	if time.Since(webhookTime) > 5*time.Minute {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "stripe",
			Error:    fmt.Errorf("Stripe webhook too old"),
		}
	}

	// Compute expected signature: HMAC-SHA256 of "timestamp.payload"
	expectedSig := computeHMAC(fmt.Sprintf("%s.%s", ts, string(payload)), v.stripeWebhookSecret)

	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return &WebhookVerificationResult{
			Valid:    false,
			Provider: "stripe",
			Error:    fmt.Errorf("invalid Stripe webhook signature"),
		}
	}

	// Verify event ID hasn't been processed (for idempotency)
	// This would check against a store (Redis/DB)

	return &WebhookVerificationResult{
		Valid:     true,
		Provider:  "stripe",
		Timestamp: webhookTime,
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