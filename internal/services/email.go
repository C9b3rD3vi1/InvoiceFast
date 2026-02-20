package services

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
	"time"

	"invoicefast/internal/config"
)

// EmailService handles sending emails
type EmailService struct {
	cfg *config.Config
}

// EmailRequest represents an email to send
type EmailRequest struct {
	To          []string
	Subject     string
	Body        string
	IsHTML      bool
	Attachments []Attachment
}

// Attachment represents an email attachment
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// NewEmailService creates a new email service
func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{cfg: cfg}
}

// Send sends an email
func (s *EmailService) Send(req EmailRequest) error {
	if s.cfg.Mail.SMTPHost == "" {
		return fmt.Errorf("SMTP not configured")
	}

	// Build email headers
	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s <%s>\n", s.cfg.Mail.FromName, s.cfg.Mail.FromEmail))
	msg.WriteString(fmt.Sprintf("To: %s\n", strings.Join(req.To, ",")))
	msg.WriteString(fmt.Sprintf("Subject: %s\n", req.Subject))
	msg.WriteString("MIME-Version: 1.0\n")

	if len(req.Attachments) > 0 {
		msg.WriteString("Content-Type: multipart/mixed; boundary=boundary\n\n")
	} else {
		if req.IsHTML {
			msg.WriteString("Content-Type: text/html; charset=UTF-8\n\n")
		} else {
			msg.WriteString("Content-Type: text/plain; charset=UTF-8\n\n")
		}
	}

	// Email body
	if req.IsHTML {
		msg.WriteString(req.Body)
	} else {
		// Convert plain text to HTML for display
		htmlBody := fmt.Sprintf("<pre style='font-family: Arial, sans-serif;'>%s</pre>", req.Body)
		msg.WriteString(htmlBody)
	}

	// Add attachments
	for _, att := range req.Attachments {
		msg.WriteString("\n--boundary\n")
		msg.WriteString(fmt.Sprintf("Content-Type: %s; name=%s\n", att.ContentType, att.Filename))
		msg.WriteString("Content-Transfer-Encoding: base64\n\n")
		msg.Write(att.Data)
	}

	if len(req.Attachments) > 0 {
		msg.WriteString("\n--boundary--\n")
	}

	// Connect and send
	addr := fmt.Sprintf("%s:%s", s.cfg.Mail.SMTPHost, s.cfg.Mail.SMTPPort)

	auth := smtp.PlainAuth("", s.cfg.Mail.SMTPUsername, s.cfg.Mail.SMTPPassword, s.cfg.Mail.SMTPHost)

	err := smtp.SendMail(addr, auth, s.cfg.Mail.FromEmail, req.To, msg.Bytes())
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// SendInvoiceEmail sends an invoice to a client
func (s *EmailService) SendInvoiceEmail(invoice *InvoiceEmailData) error {
	body, err := renderInvoiceEmail(invoice)
	if err != nil {
		return err
	}

	req := EmailRequest{
		To:      []string{invoice.ClientEmail},
		Subject: fmt.Sprintf("Invoice %s from %s", invoice.InvoiceNumber, invoice.CompanyName),
		Body:    body,
		IsHTML:  true,
	}

	return s.Send(req)
}

// SendPaymentReminder sends a payment reminder
func (s *EmailService) SendPaymentReminder(reminder *ReminderEmailData) error {
	body, err := renderReminderEmail(reminder)
	if err != nil {
		return err
	}

	req := EmailRequest{
		To:      []string{reminder.ClientEmail},
		Subject: fmt.Sprintf("Payment Reminder: Invoice %s", reminder.InvoiceNumber),
		Body:    body,
		IsHTML:  true,
	}

	return s.Send(req)
}

// SendPaymentReceipt sends a payment receipt
func (s *EmailService) SendPaymentReceipt(receipt *ReceiptEmailData) error {
	body, err := renderReceiptEmail(receipt)
	if err != nil {
		return err
	}

	req := EmailRequest{
		To:      []string{receipt.ClientEmail},
		Subject: fmt.Sprintf("Payment Receipt for Invoice %s", receipt.InvoiceNumber),
		Body:    body,
		IsHTML:  true,
	}

	return s.Send(req)
}

// InvoiceEmailData for invoice email template
type InvoiceEmailData struct {
	CompanyName   string
	CompanyEmail  string
	ClientName    string
	ClientEmail   string
	InvoiceNumber string
	InvoiceLink   string
	Amount        float64
	Currency      string
	DueDate       string
}

// ReminderEmailData for reminder email template
type ReminderEmailData struct {
	CompanyName   string
	ClientName    string
	ClientEmail   string
	InvoiceNumber string
	InvoiceLink   string
	Amount        float64
	Currency      string
	DueDate       string
	DaysOverdue   int
}

// ReceiptEmailData for receipt email template
type ReceiptEmailData struct {
	CompanyName   string
	ClientName    string
	ClientEmail   string
	InvoiceNumber string
	Amount        float64
	Currency      string
	ReceiptNumber string
	PaymentMethod string
	Reference     string
	PaymentDate   string
}

// Email templates
func renderInvoiceEmail(data *InvoiceEmailData) (string, error) {
	const templateStr = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Invoice</title>
    <style>
        body { font-family: 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #2563eb, #1d4ed8); color: white; padding: 30px; border-radius: 8px 8px 0 0; }
        .logo { font-size: 24px; font-weight: bold; }
        .content { background: #f9fafb; padding: 30px; border: 1px solid #e5e7eb; }
        .invoice-box { background: white; padding: 20px; border-radius: 8px; margin: 20px 0; }
        .invoice-number { font-size: 20px; font-weight: bold; color: #2563eb; }
        .amount { font-size: 32px; font-weight: bold; margin: 20px 0; }
        .btn { display: inline-block; background: #2563eb; color: white; padding: 14px 28px; text-decoration: none; border-radius: 6px; font-weight: 600; }
        .footer { text-align: center; padding: 20px; color: #666; font-size: 12px; }
    </style>
</head>
<body>
    <div class="header">
        <div class="logo">{{.CompanyName}}</div>
    </div>
    <div class="content">
        <p>Hello {{.ClientName}},</p>
        <p>Please find attached invoice <strong>{{.InvoiceNumber}}</strong>.</p>
        
        <div class="invoice-box">
            <div class="invoice-number">{{.InvoiceNumber}}</div>
            <div class="amount">{{.Currency}} {{printf "%.2f" .Amount}}</div>
            <p><strong>Due Date:</strong> {{.DueDate}}</p>
        </div>
        
        <p>
            <a href="{{.InvoiceLink}}" class="btn">View & Pay Invoice</a>
        </p>
        
        <p>If you have any questions, please don't hesitate to contact us.</p>
        
        <p>Best regards,<br>{{.CompanyName}}</p>
    </div>
    <div class="footer">
        <p>This email was sent by InvoiceFast</p>
    </div>
</body>
</html>`

	tmpl, err := template.New("invoice").Funcs(template.FuncMap{
		"printf": func(format string, args ...interface{}) string {
			return fmt.Sprintf(format, args...)
		},
	}).Parse(templateStr)

	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func renderReminderEmail(data *ReminderEmailData) (string, error) {
	urgency := ""
	if data.DaysOverdue > 30 {
		urgency = "This is your final reminder."
	} else if data.DaysOverdue > 14 {
		urgency = "Please treat this matter as urgent."
	}

	const templateStr = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Payment Reminder</title>
    <style>
        body { font-family: 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #f59e0b, #d97706); color: white; padding: 30px; border-radius: 8px 8px 0 0; }
        .content { background: #f9fafb; padding: 30px; border: 1px solid #e5e7eb; }
        .amount { font-size: 32px; font-weight: bold; margin: 20px 0; color: #dc2626; }
        .btn { display: inline-block; background: #2563eb; color: white; padding: 14px 28px; text-decoration: none; border-radius: 6px; font-weight: 600; }
        .footer { text-align: center; padding: 20px; color: #666; font-size: 12px; }
    </style>
</head>
<body>
    <div class="header">
        <h2>Payment Reminder</h2>
    </div>
    <div class="content">
        <p>Hello {{.ClientName}},</p>
        <p>This is a friendly reminder that invoice <strong>{{.InvoiceNumber}}</strong> is {{.DaysOverdue}} days overdue.</p>
        
        <div class="amount">{{.Currency}} {{printf "%.2f" .Amount}}</div>
        
        <p>{{.urgency}}</p>
        
        <p>
            <a href="{{.InvoiceLink}}" class="btn">Pay Now</a>
        </p>
        
        <p>If you've already made the payment, please ignore this reminder.</p>
        
        <p>Best regards,<br>{{.CompanyName}}</p>
    </div>
    <div class="footer">
        <p>Sent by InvoiceFast</p>
    </div>
</body>
</html>`

	tmpl, err := template.New("reminder").Funcs(template.FuncMap{
		"printf": func(format string, args ...interface{}) string {
			return fmt.Sprintf(format, args...)
		},
	}).Parse(templateStr)

	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func renderReceiptEmail(data *ReceiptEmailData) (string, error) {
	const templateStr = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Payment Receipt</title>
    <style>
        body { font-family: 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #22c55e, #16a34a); color: white; padding: 30px; border-radius: 8px 8px 0 0; }
        .content { background: #f9fafb; padding: 30px; border: 1px solid #e5e7eb; }
        .amount { font-size: 32px; font-weight: bold; margin: 20px 0; color: #22c55e; }
        .receipt-box { background: white; padding: 20px; border-radius: 8px; margin: 20px 0; }
        .footer { text-align: center; padding: 20px; color: #666; font-size: 12px; }
    </style>
</head>
<body>
    <div class="header">
        <h2>✅ Payment Received</h2>
    </div>
    <div class="content">
        <p>Hello {{.ClientName}},</p>
        <p>Thank you! We have received your payment.</p>
        
        <div class="receipt-box">
            <div class="amount">{{.Currency}} {{printf "%.2f" .Amount}}</div>
            <p><strong>Receipt:</strong> {{.ReceiptNumber}}</p>
            <p><strong>Invoice:</strong> {{.InvoiceNumber}}</p>
            <p><strong>Method:</strong> {{.PaymentMethod}}</p>
            <p><strong>Reference:</strong> {{.Reference}}</p>
            <p><strong>Date:</strong> {{.PaymentDate}}</p>
        </div>
        
        <p>Your payment has been processed successfully.</p>
        
        <p>Best regards,<br>{{.CompanyName}}</p>
    </div>
    <div class="footer">
        <p>Receipt generated by InvoiceFast</p>
    </div>
</body>
</html>`

	tmpl, err := template.New("receipt").Funcs(template.FuncMap{
		"printf": func(format string, args ...interface{}) string {
			return fmt.Sprintf(format, args...)
		},
	}).Parse(templateStr)

	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Mock email for development
func (s *EmailService) MockSend(req EmailRequest) error {
	fmt.Printf("📧 [MOCK EMAIL]\n")
	fmt.Printf("To: %s\n", strings.Join(req.To, ", "))
	fmt.Printf("Subject: %s\n", req.Subject)
	fmt.Printf("---\n")

	if req.IsHTML {
		// Extract text from HTML for display
		text := strings.ReplaceAll(req.Body, "<br>", "\n")
		text = strings.ReplaceAll(text, "<p>", "\n")
		text = strings.ReplaceAll(text, "</p>", "")
		text = strings.ReplaceAll(text, "<strong>", "")
		text = strings.ReplaceAll(text, "</strong>", "")
		text = strings.ReplaceAll(text, "&nbsp;", " ")
		fmt.Printf("%s\n", text)
	}

	fmt.Printf("---\n\n")
	return nil
}

// QueueEmail adds email to queue (for async processing)
type EmailQueue struct {
	emails   chan EmailRequest
	stopChan chan bool
}

func NewEmailQueue(workerCount int) *EmailQueue {
	q := &EmailQueue{
		emails:   make(chan EmailRequest, 1000),
		stopChan: make(chan bool),
	}

	// Start workers
	for i := 0; i < workerCount; i++ {
		go q.worker()
	}

	return q
}

func (q *EmailQueue) worker() {
	for {
		select {
		case email := <-q.emails:
			// Process email (would call EmailService.Send in production)
			fmt.Printf("Processing email to: %s\n", email.To)
		case <-q.stopChan:
			return
		}
	}
}

func (q *EmailQueue) Enqueue(email EmailRequest) {
	q.emails <- email
}

func (q *EmailQueue) Stop() {
	close(q.stopChan)
}

// FormatDate formats date for emails
func FormatDate(t time.Time) string {
	return t.Format("02 Jan 2006")
}
