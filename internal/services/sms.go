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

// SMSService handles SMS sending for critical alerts
type SMSService struct {
	cfg        *config.SMSConfig
	httpClient *http.Client
}

// NewSMSService creates a new SMS service
func NewSMSService(cfg *config.SMSConfig) *SMSService {
	if cfg == nil {
		cfg = &config.SMSConfig{Enabled: false}
	}

	return &SMSService{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SMSMessage represents an SMS to be sent
type SMSMessage struct {
	To      string `json:"to"`
	Message string `json:"message"`
}

// Send sends an SMS message
func (s *SMSService) Send(to, message string) error {
	if !s.cfg.Enabled {
		fmt.Printf("[SMS MOCK] To: %s, Message: %s\n", to, message)
		return nil
	}

	switch s.cfg.Provider {
	case "africastalking":
		return s.sendViaAfricaStalking(to, message)
	case "twilio":
		return s.sendViaTwilio(to, message)
	case "bulk":
		return s.sendViaBulkAPI(to, message)
	default:
		// Default to Africa Stalking
		return s.sendViaAfricaStalking(to, message)
	}
}

// sendViaAfricaStalking sends SMS via Africa's Talking API
func (s *SMSService) sendViaAfricaStalking(to, message string) error {
	// Format phone number
	phone := formatPhoneNumber(to)
	if phone == "" {
		return fmt.Errorf("invalid phone number: %s", to)
	}

	// Africa's Talking USSD/SMS endpoint
	url := "https://api.africastalking.com/version1/messaging"

	data := map[string]string{
		"username": s.cfg.SenderID,
		"to":       phone,
		"message":  message,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal SMS: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apiKey", s.cfg.APIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send SMS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SMS API error (status %d): %s", resp.StatusCode, string(body))
	}

	fmt.Printf("[SMS] Sent to %s: %s\n", phone, message)
	return nil
}

// sendViaTwilio sends SMS via Twilio API
func (s *SMSService) sendViaTwilio(to, message string) error {
	phone := formatPhoneNumber(to)
	if phone == "" {
		return fmt.Errorf("invalid phone number: %s", to)
	}

	url := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", s.cfg.APIKey)

	data := map[string]string{
		"To":   phone,
		"From": s.cfg.SenderID,
		"Body": message,
	}

	jsonData, _ := json.Marshal(data)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.cfg.APIKey, s.cfg.APISecret)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send SMS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Twilio error: %s", string(body))
	}

	return nil
}

// sendViaBulkAPI sends SMS via generic bulk SMS API
func (s *SMSService) sendViaBulkAPI(to, message string) error {
	if s.cfg.SMSEndpoint == "" {
		return fmt.Errorf("SMS endpoint not configured")
	}

	phone := formatPhoneNumber(to)

	payload := map[string]interface{}{
		"sender":  s.cfg.SenderID,
		"phone":   phone,
		"message": message,
	}

	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", s.cfg.SMSEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send SMS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SMS API error: %s", string(body))
	}

	return nil
}

// formatPhoneNumber formats phone number to international format
func formatPhoneNumber(phone string) string {
	var digits string
	for _, c := range phone {
		if c >= '0' && c <= '9' {
			digits += string(c)
		}
	}

	// Already international
	if len(digits) == 12 && digits[:3] == "254" {
		return "+" + digits
	}
	// Local format (07xx...)
	if len(digits) == 10 && digits[0] == '0' {
		return "+254" + digits[1:]
	}
	// Short format (7xx...)
	if len(digits) == 9 && digits[0] >= '7' {
		return "+254" + digits
	}

	return ""
}

// SendPaymentAlert sends a payment notification SMS
func (s *SMSService) SendPaymentAlert(phone, invoiceNumber, amount string) error {
	msg := fmt.Sprintf("Payment received: Invoice #%s of %s. Thank you!", invoiceNumber, amount)
	return s.Send(phone, msg)
}

// SendInvoiceAlert sends an invoice notification SMS
func (s *SMSService) SendInvoiceAlert(phone, invoiceNumber, amount, dueDate string) error {
	msg := fmt.Sprintf("New invoice #%s for %s. Due: %s", invoiceNumber, amount, dueDate)
	return s.Send(phone, msg)
}

// SendOverdueAlert sends an overdue reminder SMS
func (s *SMSService) SendOverdueAlert(phone, invoiceNumber, amount string) error {
	msg := fmt.Sprintf("Reminder: Invoice #%s for %s is overdue. Please pay immediately.", invoiceNumber, amount)
	return s.Send(phone, msg)
}
