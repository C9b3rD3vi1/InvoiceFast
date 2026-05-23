package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"invoicefast/internal/circuitbreaker"
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
	baseURL       string
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
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://invoice.simuxtech.com"
	}
	return &WhatsAppService{
		fromNumber:     os.Getenv("TWILIO_PHONE_NUMBER"),
		useTwilio:      os.Getenv("WHATSAPP_PROVIDER") == "twilio",
		accountSID:     os.Getenv("TWILIO_ACCOUNT_SID"),
		authToken:      os.Getenv("TWILIO_AUTH_TOKEN"),
		metaPhoneID:    os.Getenv("META_PHONE_NUMBER_ID"),
		metaToken:      os.Getenv("META_ACCESS_TOKEN"),
		metaBusinessID: os.Getenv("META_BUSINESS_ACCOUNT_ID"),
		baseURL:       baseURL,
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

	pdfDownloadURL := fmt.Sprintf("%s/api/invoices/download/%s.pdf", s.baseURL, pdfName)

	// If not configured, return wa.me URL with PDF download note
	if !s.IsConfigured() {
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
		pdfDownloadURL = fmt.Sprintf("%s/api/invoices/download/%s.pdf", s.baseURL, pdfName)
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
	twilioURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", s.accountSID)
	data := url.Values{}
	data.Set("From", from)
	data.Set("To", to)
	data.Set("Body", message)
	data.Set("Provider", "whatsapp")

	req, err := http.NewRequest("POST", twilioURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("twilio request create failed: %w", err)
	}
	req.SetBasicAuth(s.accountSID, s.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}

	// Execute Twilio API call with circuit breaker protection
	_, err = circuitbreaker.WhatsAppCircuit().ExecuteWithResult(context.Background(), func(ctx context.Context) (interface{}, error) {
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("twilio request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("twilio API error: status=%d body=%s", resp.StatusCode, string(body))
		}
		return nil, nil
	})
	return err
}

func (s *WhatsAppService) sendTwilioMessageWithMedia(from, to, message string, mediaData []byte, mediaName string) error {
	// Upload media to a temporary URL and include in Twilio message
	mediaURL, err := s.uploadMediaToCDN(mediaData, mediaName)
	if err != nil {
		return fmt.Errorf("media upload failed: %w", err)
	}

	twilioURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", s.accountSID)
	data := url.Values{}
	data.Set("From", from)
	data.Set("To", to)
	data.Set("Body", message)
	data.Set("MediaUrl", mediaURL)
	data.Set("Provider", "whatsapp")

	req, err := http.NewRequest("POST", twilioURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("twilio media request create failed: %w", err)
	}
	req.SetBasicAuth(s.accountSID, s.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("twilio media request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twilio media API error: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

func (s *WhatsAppService) uploadMediaToCDN(data []byte, name string) (string, error) {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://invoice.simuxtech.com"
	}
	return fmt.Sprintf("%s/api/invoices/download/%s", baseURL, name), nil
}

type whatsappTextObject struct {
	Body       string `json:"body"`
	PreviewURL bool   `json:"preview_url"`
}

func (s *WhatsAppService) sendMetaMessage(to, message string) error {
	if s.metaPhoneID == "" || s.metaToken == "" {
		return fmt.Errorf("Meta WhatsApp not configured: missing phone ID or access token")
	}

	type messagePayload struct {
		MessagingProduct string            `json:"messaging_product"`
		RecipientType    string            `json:"recipient_type"`
		To               string            `json:"to"`
		Type             string            `json:"type"`
		Text             whatsappTextObject `json:"text"`
	}

	payload := messagePayload{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               to,
		Type:             "text",
		Text:             whatsappTextObject{Body: message, PreviewURL: false},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("meta marshal failed: %w", err)
	}

	apiURL := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", s.metaPhoneID)
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("meta request create failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.metaToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("meta request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("meta API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (s *WhatsAppService) sendMetaMessageWithMedia(to, message string, mediaData []byte, mediaName string) error {
	if s.metaPhoneID == "" || s.metaToken == "" {
		return fmt.Errorf("Meta WhatsApp not configured: missing phone ID or access token")
	}

	// Step 1: Upload media to Meta servers
	mediaID, err := s.uploadMetaMedia(mediaData, mediaName)
	if err != nil {
		return fmt.Errorf("meta media upload failed: %w", err)
	}

	// Step 2: Send message with media ID
	type mediaObject struct {
		ID string `json:"id"`
	}
	type messagePayload struct {
		MessagingProduct string     `json:"messaging_product"`
		RecipientType    string     `json:"recipient_type"`
		To               string     `json:"to"`
		Type             string     `json:"type"`
		Text             *whatsappTextObject `json:"text,omitempty"`
		Document         *mediaObject `json:"document,omitempty"`
	}

	payload := messagePayload{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               to,
		Type:             "document",
		Text:             &whatsappTextObject{Body: message, PreviewURL: false},
		Document:         &mediaObject{ID: mediaID},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("meta media marshal failed: %w", err)
	}

	apiURL := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", s.metaPhoneID)
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("meta media request create failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.metaToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("meta media request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("meta media API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (s *WhatsAppService) uploadMetaMedia(data []byte, fileName string) (string, error) {
	uploadURL := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/media", s.metaPhoneID)

	// Use multipart form upload per Meta API
	var buf bytes.Buffer
	writer := io.Writer(&buf)
	if _, err := fmt.Fprintf(writer, "{\"messaging_product\":\"whatsapp\",\"file\":\"%s\"}", fileName); err != nil {
		return "", err
	}

	// Meta requires multipart/form-data upload
	boundary := "----Boundary" + fmt.Sprintf("%d", time.Now().UnixNano())
	fullBody := fmt.Sprintf("--%s\r\nContent-Disposition: form-data; name=\"messaging_product\"\r\n\r\nwhatsapp\r\n--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\nContent-Type: application/pdf\r\n\r\n", boundary, boundary, fileName)
	fullBody += string(data)
	fullBody += fmt.Sprintf("\r\n--%s--\r\n", boundary)

	req, err := http.NewRequest("POST", uploadURL, strings.NewReader(fullBody))
	if err != nil {
		return "", fmt.Errorf("meta upload request failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.metaToken)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("meta upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("meta upload API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("meta upload decode failed: %w", err)
	}
	return result.ID, nil
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
