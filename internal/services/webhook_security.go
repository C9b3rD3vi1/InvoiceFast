package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"invoicefast/internal/config"
)

// WebhookSecurity handles secure webhook processing
type WebhookSecurity struct {
	cfg        *config.IntasendConfig
	httpClient *http.Client
}

// NewWebhookSecurity creates a new webhook security handler
func NewWebhookSecurity(cfg *config.IntasendConfig) *WebhookSecurity {
	return &WebhookSecurity{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// VerifySignature verifies the HMAC signature of a webhook payload
// This is CRITICAL for payment security - prevents fraud
func (ws *WebhookSecurity) VerifySignature(payload []byte, signature string) error {
	if ws.cfg.WebhookSecret == "" {
		return fmt.Errorf("webhook secret not configured - cannot verify signature")
	}

	if signature == "" {
		return fmt.Errorf("missing signature header")
	}

	// Compute HMAC-SHA256 of the payload
	mac := hmac.New(sha256.New, []byte(ws.cfg.WebhookSecret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expectedMAC)) != 1 {
		return fmt.Errorf("invalid webhook signature - possible tampering detected")
	}

	return nil
}

// VerifyTimestamp checks if the webhook timestamp is within acceptable range
// Prevents replay attacks
func (ws *WebhookSecurity) VerifyTimestamp(timestamp string, maxAge time.Duration) error {
	if timestamp == "" {
		return fmt.Errorf("missing timestamp")
	}

	ts, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fmt.Errorf("invalid timestamp format: %w", err)
	}

	age := time.Since(ts)
	if age > maxAge {
		return fmt.Errorf("webhook timestamp too old: %v (max: %v)", age, maxAge)
	}

	// Also check it's not in the future
	if ts.After(time.Now().Add(5 * time.Minute)) {
		return fmt.Errorf("webhook timestamp is in the future")
	}

	return nil
}

// ParseWebhookPayload safely parses webhook payload with validation
func (ws *WebhookSecurity) ParseWebhookPayload(payload []byte, signature, timestamp string) (*IntasendWebhookEvent, error) {
	// 1. Verify signature
	if err := ws.VerifySignature(payload, signature); err != nil {
		log.Printf("[SECURITY] Webhook signature verification failed: %v", err)
		return nil, fmt.Errorf("security: %w", err)
	}

	// 2. Verify timestamp (prevent replay attacks)
	if err := ws.VerifyTimestamp(timestamp, 5*time.Minute); err != nil {
		log.Printf("[SECURITY] Webhook timestamp verification failed: %v", err)
		return nil, fmt.Errorf("security: %w", err)
	}

	// 3. Parse the payload
	var event IntasendWebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	// 4. Validate event type
	if err := ws.validateEventType(event.Event); err != nil {
		return nil, err
	}

	// 5. Validate amounts and IDs
	if err := ws.validatePayloadContent(&event); err != nil {
		return nil, err
	}

	return &event, nil
}

// validateEventType ensures we only process known event types
func (ws *WebhookSecurity) validateEventType(eventType string) error {
	allowedEvents := map[string]bool{
		"payment_successful":       true,
		"payment_failed":           true,
		"payment_reversed":          true,
		"refund_processed":         true,
		"collection_completed":     true,
		"collection_failed":        true,
		"invoice_payment_signed":   true,
		"chargeback":               true,
		"payout_processed":         true,
	}

	if !allowedEvents[eventType] {
		return fmt.Errorf("unknown event type: %s", eventType)
	}

	return nil
}

// validatePayloadContent validates the content of the webhook
func (ws *WebhookSecurity) validatePayloadContent(event *IntasendWebhookEvent) error {
	// Validate collection ID exists
	if event.Collection.ID == "" {
		return fmt.Errorf("missing collection ID")
	}

	// Validate amount is positive for successful payments
	if event.Event == "payment_successful" || event.Event == "collection_completed" {
		if event.Collection.Amount <= 0 {
			return fmt.Errorf("invalid amount for successful payment: %d", event.Collection.Amount)
		}
	}

	return nil
}

// IdempotencyKey generates an idempotency key for webhook processing
func (ws *WebhookSecurity) GenerateIdempotencyKey(event *IntasendWebhookEvent) string {
	// Use collection ID as idempotency key
	return fmt.Sprintf("webhook:%s:%s", event.Collection.ID, event.Event)
}

// WebhookSignatureMiddleware is a Gin middleware for webhook verification
func WebhookSignatureMiddleware(cfg *config.IntasendConfig) func(http.Handler) http.Handler {
	ws := NewWebhookSecurity(cfg)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Read the body
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			// Restore body for downstream handlers
			r.Body = io.NopCloser(bytes.NewBuffer(body))

			// Get signature header
			signature := r.Header.Get("X-IntaSend-Signature")
			if signature == "" {
				signature = r.Header.Get("X-Signature")
			}
			timestamp := r.Header.Get("X-Timestamp")

			// Verify signature
			if err := ws.VerifySignature(body, signature); err != nil {
				log.Printf("[SECURITY] Rejected webhook: %v", err)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Verify timestamp (prevents replay attacks)
			if err := ws.VerifyTimestamp(timestamp, 5*time.Minute); err != nil {
				log.Printf("[SECURITY] Rejected webhook: %v", err)
				http.Error(w, "Request expired", http.StatusUnauthorized)
				return
			}

			// Add verified flag to context
			ctx := context.WithValue(r.Context(), "webhook_verified", true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// HashPayload creates a hash of the payload for logging (without exposing sensitive data)
func (ws *WebhookSecurity) HashPayload(payload []byte) string {
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:8]) // Only first 8 bytes for logging
}