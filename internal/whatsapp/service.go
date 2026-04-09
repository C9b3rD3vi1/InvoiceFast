package whatsapp

import (
	"fmt"
	"os"
	"strings"
)

// WhatsAppService handles WhatsApp messaging
type WhatsAppService struct {
	fromNumber     string
	useTwilio      bool
	accountSID     string
	authToken      string
	metaPhoneID    string
	metaToken      string
	metaBusinessID string
}

// NewWhatsAppService creates a new WhatsApp service
func NewWhatsAppService() *WhatsAppService {
	return &WhatsAppService{
		fromNumber:     os.Getenv("TWILIO_PHONE_NUMBER"),
		useTwilio:      os.Getenv("WHATSAPP_PROVIDER") == "twilio",
		accountSID:     os.Getenv("TWILIO_ACCOUNT_SID"),
		authToken:      os.Getenv("TWILIO_AUTH_TOKEN"),
		metaPhoneID:    os.Getenv("META_PHONE_NUMBER_ID"),
		metaToken:      os.Getenv("META_ACCESS_TOKEN"),
		metaBusinessID: os.Getenv("META_BUSINESS_ACCOUNT_ID"),
	}
}

// SendMessage sends a WhatsApp message
func (s *WhatsAppService) SendMessage(to, message string) error {
	// Format phone number
	to = formatWhatsAppNumber(to)
	from := formatWhatsAppNumber(s.fromNumber)

	if s.useTwilio && s.accountSID != "" && s.authToken != "" {
		return s.sendTwilioMessage(from, to, message)
	}

	// For demo/sandbox, just log the message
	fmt.Printf("[WhatsApp Demo] To: %s, From: %s, Message: %s\n", to, from, message)
	return nil
}

func (s *WhatsAppService) sendTwilioMessage(from, to, message string) error {
	// Twilio implementation would go here
	// For now, return nil to allow the service to work
	return nil
}

// SendInvoiceNotification sends invoice notification
func (s *WhatsAppService) SendInvoiceNotification(to string, data map[string]string) error {
	message := fmt.Sprintf(`Hello %s,

You have received an invoice from %s.

Invoice #: %s
Amount: %s %s
Due Date: %s

View and pay: %s

Thank you for your business!`,
		data["client_name"],
		data["company_name"],
		data["invoice_number"],
		data["currency"],
		data["total"],
		data["due_date"],
		data["payment_link"],
	)

	return s.SendMessage(to, message)
}

// SendPaymentReminder sends payment reminder
func (s *WhatsAppService) SendPaymentReminder(to string, data map[string]string) error {
	message := fmt.Sprintf(`Hello %s,

This is a friendly reminder that invoice #%s is %s day(s) overdue.

Amount Due: %s %s

Please make payment as soon as possible.

Pay now: %s

Thank you!`,
		data["client_name"],
		data["invoice_number"],
		data["days_overdue"],
		data["currency"],
		data["total"],
		data["payment_link"],
	)

	return s.SendMessage(to, message)
}

// SendPaymentConfirmation sends payment confirmation
func (s *WhatsAppService) SendPaymentConfirmation(to string, data map[string]string) error {
	message := fmt.Sprintf(`Hello %s,

Thank you! We have received your payment.

Invoice #: %s
Amount Paid: %s %s
Payment Method: %s

Your receipt has been sent to your email.

Thank you for your business!`,
		data["client_name"],
		data["invoice_number"],
		data["currency"],
		data["amount_paid"],
		data["payment_method"],
	)

	return s.SendMessage(to, message)
}

// SendBulkMessages sends bulk WhatsApp messages
func (s *WhatsAppService) SendBulkMessages(recipients []string, message string) []error {
	errors := make([]error, 0)

	for _, to := range recipients {
		err := s.SendMessage(to, message)
		if err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

func formatWhatsAppNumber(number string) string {
	// Remove any whitespace
	number = strings.TrimSpace(number)

	// Add whatsapp: prefix if not present
	if !strings.HasPrefix(number, "whatsapp:") {
		number = "whatsapp:" + number
	}

	return number
}
