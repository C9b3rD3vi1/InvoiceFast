package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"invoicefast/internal/config"
)

// WhatsAppService handles WhatsApp messaging via WhatsApp Business API
type WhatsAppService struct {
	cfg        *config.Config
	httpClient *http.Client
}

// WhatsAppMessage represents a WhatsApp message
type WhatsAppMessage struct {
	To        string            `json:"messaging_product"`
	Recipient string            `json:"to"`
	Type      string            `json:"type"`
	Template  *WhatsAppTemplate `json:"template,omitempty"`
	Text      *WhatsAppText     `json:"text,omitempty"`
	Image     *WhatsAppImage    `json:"image,omitempty"`
}

// WhatsAppTemplate for template messages
type WhatsAppTemplate struct {
	Name       string              `json:"name"`
	Language   string              `json:"language"`
	Components []TemplateComponent `json:"components,omitempty"`
}

// TemplateComponent for dynamic content
type TemplateComponent struct {
	Type       string      `json:"type"`
	Parameters []Parameter `json:"parameters,omitempty"`
}

// Parameter for template variables
type Parameter struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// WhatsAppText for simple text messages
type WhatsAppText struct {
	Body string `json:"body"`
}

// WhatsAppImage for image messages
type WhatsAppImage struct {
	ID      string `json:"id,omitempty"`
	Link    string `json:"link,omitempty"`
	Caption string `json:"caption,omitempty"`
}

// WhatsAppResponse from API
type WhatsAppResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

// NewWhatsAppService creates a new WhatsApp service
func NewWhatsAppService(cfg *config.Config) *WhatsAppService {
	return &WhatsAppService{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendInvoice sends invoice via WhatsApp
func (s *WhatsAppService) SendInvoice(phone, invoiceNumber, amount, companyName, link string) error {
	// Use template message for invoices (approved by Meta)
	templateMsg := &WhatsAppTemplate{
		Name:     "invoice_notification", // Need to create this in WhatsApp Business
		Language: "en_US",
		Components: []TemplateComponent{
			{
				Type: "body",
				Parameters: []Parameter{
					{Type: "text", Text: companyName},
					{Type: "text", Text: invoiceNumber},
					{Type: "text", Text: amount},
				},
			},
		},
	}

	msg := &WhatsAppMessage{
		To:       "whatsapp:" + normalizePhone(phone),
		Type:     "template",
		Template: templateMsg,
	}

	return s.Send(msg)
}

// SendPaymentRequest sends payment request via WhatsApp
func (s *WhatsAppService) SendPaymentRequest(phone, invoiceNumber, amount, link string) error {
	msg := &WhatsAppMessage{
		To:   "whatsapp:" + normalizePhone(phone),
		Type: "text",
		Text: &WhatsAppText{
			Body: fmt.Sprintf("💰 Payment Request\n\nInvoice: %s\nAmount: %s\n\nPay now: %s\n\nReply YES to confirm payment",
				invoiceNumber, amount, link),
		},
	}

	return s.Send(msg)
}

// SendReminder sends payment reminder via WhatsApp
func (s *WhatsAppService) SendReminder(phone, invoiceNumber, amount, daysOverdue string) error {
	msg := &WhatsAppMessage{
		To:   "whatsapp:" + normalizePhone(phone),
		Type: "text",
		Text: &WhatsAppText{
			Body: fmt.Sprintf("⏰ Payment Reminder\n\nInvoice: %s\nAmount: %s\nOverdue: %s days\n\nPlease prioritize this payment.",
				invoiceNumber, amount, daysOverdue),
		},
	}

	return s.Send(msg)
}

// SendReceipt sends payment receipt via WhatsApp
func (s *WhatsAppService) SendReceipt(phone, invoiceNumber, amount, receiptNumber string) error {
	msg := &WhatsAppMessage{
		To:   "whatsapp:" + normalizePhone(phone),
		Type: "text",
		Text: &WhatsAppText{
			Body: fmt.Sprintf("✅ Payment Received!\n\nInvoice: %s\nAmount: %s\nReceipt: %s\n\nThank you for your payment!",
				invoiceNumber, amount, receiptNumber),
		},
	}

	return s.Send(msg)
}

// SendThankYou sends thank you message
func (s *WhatsAppService) SendThankYou(phone, invoiceNumber string) error {
	msg := &WhatsAppMessage{
		To:   "whatsapp:" + normalizePhone(phone),
		Type: "text",
		Text: &WhatsAppText{
			Body: fmt.Sprintf("🙏 Thank you!\n\nWe've received your payment for invoice %s.\n\nWe appreciate your business!", invoiceNumber),
		},
	}

	return s.Send(msg)
}

// Send is the main method to send messages
func (s *WhatsAppService) Send(msg *WhatsAppMessage) error {
	// In production, use actual WhatsApp Business API
	// For now, log the message

	fmt.Printf("📱 [WHATSAPP MESSAGE]\n")
	fmt.Printf("To: %s\n", msg.To)
	if msg.Template != nil {
		fmt.Printf("Type: template (%s)\n", msg.Template.Name)
	}
	if msg.Text != nil {
		fmt.Printf("Message: %s\n", msg.Text.Body)
	}
	fmt.Println()

	// Uncomment below for production:
	// return s.sendToAPI(msg)

	return nil
}

// sendToAPI sends message to WhatsApp Business API
func (s *WhatsAppService) sendToAPI(msg *WhatsAppMessage) error {
	// WhatsApp Cloud API endpoint
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages",
		s.cfg.WhatsApp.PhoneNumberID)

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.cfg.WhatsApp.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("WhatsApp API error: %d", resp.StatusCode)
	}

	return nil
}

// WhatsAppTemplates for pre-approved templates
// These need to be created in WhatsApp Business Manager
var WhatsAppTemplates = map[string]string{
	"invoice_notification": "invoice_notification",
	"payment_request":      "payment_request",
	"payment_reminder":     "payment_reminder",
	"payment_received":     "payment_received",
	"thank_you":            "thank_you",
}

// Mock service for development
type MockWhatsAppService struct{}

func NewMockWhatsAppService() *MockWhatsAppService {
	return &MockWhatsAppService{}
}

func (s *MockWhatsAppService) SendInvoice(phone, invoiceNumber, amount, companyName, link string) error {
	fmt.Printf("📱 [MOCK WHATSAPP - Invoice]\n")
	fmt.Printf("To: %s\n", phone)
	fmt.Printf("Invoice: %s, Amount: %s, Company: %s\n\n", invoiceNumber, amount, companyName)
	return nil
}

func (s *MockWhatsAppService) SendPaymentRequest(phone, invoiceNumber, amount, link string) error {
	fmt.Printf("📱 [MOCK WHATSAPP - Payment Request]\n")
	fmt.Printf("To: %s\n", phone)
	fmt.Printf("Invoice: %s, Amount: %s\n\n", invoiceNumber, amount)
	return nil
}

func (s *MockWhatsAppService) SendReminder(phone, invoiceNumber, amount, daysOverdue string) error {
	fmt.Printf("📱 [MOCK WHATSAPP - Reminder]\n")
	fmt.Printf("To: %s\n", phone)
	fmt.Printf("Invoice: %s, Amount: %s, Days Overdue: %s\n\n", invoiceNumber, amount, daysOverdue)
	return nil
}

func (s *MockWhatsAppService) SendReceipt(phone, invoiceNumber, amount, receiptNumber string) error {
	fmt.Printf("📱 [MOCK WHATSAPP - Receipt]\n")
	fmt.Printf("To: %s\n", phone)
	fmt.Printf("Invoice: %s, Amount: %s, Receipt: %s\n\n", invoiceNumber, amount, receiptNumber)
	return nil
}

func (s *MockWhatsAppService) SendThankYou(phone, invoiceNumber string) error {
	fmt.Printf("📱 [MOCK WHATSAPP - Thank You]\n")
	fmt.Printf("To: %s\n", phone)
	fmt.Printf("Invoice: %s\n\n", invoiceNumber)
	return nil
}
