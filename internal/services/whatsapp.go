package services

import (
	"fmt"
	"net/url"
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

// WhatsAppResult contains the result of sending WhatsApp message
type WhatsAppResult struct {
	Sent    bool   `json:"sent"`
	URL     string `json:"url,omitempty"`      // wa.me URL for manual sending
	Message string `json:"message,omitempty"`  // The message that would be sent
	Phone   string `json:"phone"`              // Formatted phone number
	PDFData []byte `json:"-"`                  // PDF file data (for download)
	PDFName string `json:"pdf_name,omitempty"` // PDF filename
	HasPDF  bool   `json:"has_pdf"`            // Whether PDF data is available
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

// IsConfigured checks if WhatsApp is properly configured
func (s *WhatsAppService) IsConfigured() bool {
	return s.useTwilio && s.accountSID != "" && s.authToken != ""
}

// GetWaMeURL generates a wa.me URL for manual sending
func (s *WhatsAppService) GetWaMeURL(phone, message string) string {
	formattedPhone := formatPhoneNumber(phone)
	encodedMessage := url.QueryEscape(message)
	return fmt.Sprintf("https://wa.me/%s?text=%s", formattedPhone, encodedMessage)
}

// GetWaMeURLWithPDF generates a wa.me URL and includes PDF info
func (s *WhatsAppService) GetWaMeURLWithPDF(phone, message, pdfDownloadURL string) string {
	formattedPhone := formatPhoneNumber(phone)
	pdfNote := fmt.Sprintf("\n\n📎 Download Invoice PDF: %s", pdfDownloadURL)
	fullMessage := message + pdfNote
	encodedMessage := url.QueryEscape(fullMessage)
	return fmt.Sprintf("https://wa.me/%s?text=%s", formattedPhone, encodedMessage)
}

// SendInvoiceNotification sends invoice notification and returns result
func (s *WhatsAppService) SendInvoiceNotification(to string, data map[string]string) *WhatsAppResult {
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

	return s.Send(to, message)
}

// SendPaymentReminder sends payment reminder and returns result
func (s *WhatsAppService) SendPaymentReminder(to string, data map[string]string) *WhatsAppResult {
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

	return s.Send(to, message)
}

// SendPaymentConfirmation sends payment confirmation and returns result
func (s *WhatsAppService) SendPaymentConfirmation(to string, data map[string]string) *WhatsAppResult {
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

	return s.Send(to, message)
}

// Send sends a WhatsApp message (via API or returns wa.me URL)
func (s *WhatsAppService) Send(to, message string) *WhatsAppResult {
	formattedPhone := formatPhoneNumber(to)

	// If not configured, return wa.me URL for manual sending
	if !s.IsConfigured() {
		return &WhatsAppResult{
			Sent:    false,
			URL:     s.GetWaMeURL(to, message),
			Message: message,
			Phone:   formattedPhone,
		}
	}

	// Send via configured provider
	var err error
	if s.useTwilio {
		err = s.sendTwilioMessage(formatWhatsAppNumber(s.fromNumber), formatWhatsAppNumber(to), message)
	} else {
		err = s.sendMetaMessage(to, message)
	}

	if err != nil {
		return &WhatsAppResult{
			Sent:    false,
			URL:     s.GetWaMeURL(to, message),
			Message: message,
			Phone:   formattedPhone,
		}
	}

	return &WhatsAppResult{
		Sent:    true,
		Message: message,
		Phone:   formattedPhone,
	}
}

// SendWithPDF sends a WhatsApp message with PDF attachment
func (s *WhatsAppService) SendWithPDF(to, message string, pdfData []byte, pdfName string) *WhatsAppResult {
	formattedPhone := formatPhoneNumber(to)

	// If not configured, return wa.me URL with PDF download note
	if !s.IsConfigured() {
		pdfDownloadURL := fmt.Sprintf("https://invoice.simuxtech.com/api/invoices/download/%s.pdf", pdfName)
		return &WhatsAppResult{
			Sent:    false,
			URL:     s.GetWaMeURLWithPDF(to, message, pdfDownloadURL),
			Message: message,
			Phone:   formattedPhone,
			PDFData: pdfData,
			PDFName: pdfName,
			HasPDF:  len(pdfData) > 0,
		}
	}

	// Send via configured provider with PDF
	var sendErr error
	if s.useTwilio {
		sendErr = s.sendTwilioMessageWithMedia(formatWhatsAppNumber(s.fromNumber), formatWhatsAppNumber(to), message, pdfData, pdfName)
	} else {
		sendErr = s.sendMetaMessageWithMedia(to, message, pdfData, pdfName)
	}

	if sendErr != nil {
		pdfDownloadURL := fmt.Sprintf("https://invoice.simuxtech.com/api/invoices/download/%s.pdf", pdfName)
		return &WhatsAppResult{
			Sent:    false,
			URL:     s.GetWaMeURLWithPDF(to, message, pdfDownloadURL),
			Message: message,
			Phone:   formattedPhone,
			PDFData: pdfData,
			PDFName: pdfName,
			HasPDF:  len(pdfData) > 0,
		}
	}

	return &WhatsAppResult{
		Sent:    true,
		Message: message,
		Phone:   formattedPhone,
		PDFData: pdfData,
		PDFName: pdfName,
		HasPDF:  true,
	}
}

// SendMessage sends a WhatsApp message (legacy method for compatibility)
func (s *WhatsAppService) SendMessage(to, message string) error {
	result := s.Send(to, message)
	if !result.Sent {
		return fmt.Errorf("WhatsApp not configured")
	}
	return nil
}

func (s *WhatsAppService) sendTwilioMessage(from, to, message string) error {
	// Twilio implementation would go here
	return nil
}

func (s *WhatsAppService) sendTwilioMessageWithMedia(from, to, message string, mediaData []byte, mediaName string) error {
	// Twilio WhatsApp with media support
	// In production, upload media to Twilio and send with media URL
	return fmt.Errorf("Twilio media sending not implemented")
}

func (s *WhatsAppService) sendMetaMessage(to, message string) error {
	// Meta WhatsApp Business API implementation would go here
	return nil
}

func (s *WhatsAppService) sendMetaMessageWithMedia(to, message string, mediaData []byte, mediaName string) error {
	// Meta WhatsApp Business API with media support
	// In production:
	// 1. Upload media to Meta Graph API
	// 2. Get media ID
	// 3. Send message with media ID
	return fmt.Errorf("Meta media sending not implemented")
}

// SendBulkMessages sends bulk WhatsApp messages
func (s *WhatsAppService) SendBulkMessages(recipients []string, message string) []*WhatsAppResult {
	results := make([]*WhatsAppResult, len(recipients))
	for i, to := range recipients {
		results[i] = s.Send(to, message)
	}
	return results
}

func formatWhatsAppNumber(number string) string {
	number = strings.TrimSpace(number)
	if !strings.HasPrefix(number, "whatsapp:") {
		number = "whatsapp:" + number
	}
	return number
}
