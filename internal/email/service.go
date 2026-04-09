package email

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"sync"

	"invoicefast/internal/models"
)

// EmailService handles email sending
type EmailService struct {
	smtpHost     string
	smtpPort     int
	username     string
	password     string
	fromName     string
	fromEmail    string
	useTLS       bool
	asyncQueue   chan *EmailJob
	workerCount  int
	wg           sync.WaitGroup
}

// EmailJob represents an email job in the queue
type EmailJob struct {
	To          string
	Subject     string
	BodyHTML    string
	BodyText    string
	Attachments []string
	Result      chan error
}

// EmailTemplate represents an email template
type EmailTemplate struct {
	Name        string
	Subject     string
	BodyHTML    string
	BodyText    string
	Variables   []string
}

// NewEmailService creates a new email service
func NewEmailService(host string, port int, username, password, fromName, fromEmail string, useTLS bool) *EmailService {
	service := &EmailService{
		smtpHost:   host,
		smtpPort:   port,
		username:   username,
		password:   password,
		fromName:   fromName,
		fromEmail:  fromEmail,
		useTLS:     useTLS,
		asyncQueue: make(chan *EmailJob, 1000),
		workerCount: 5,
	}
	
	// Start worker pool
	for i := 0; i < service.workerCount; i++ {
		service.wg.Add(1)
		go service.worker()
	}
	
	return service
}

// worker processes email jobs from the queue
func (s *EmailService) worker() {
	defer s.wg.Done()
	
	for job := range s.asyncQueue {
		job.Result <- s.sendEmail(job.To, job.Subject, job.BodyHTML, job.BodyText, job.Attachments)
	}
}

// Send sends an email synchronously
func (s *EmailService) Send(ctx context.Context, to, subject, bodyHTML, bodyText string, attachments []string) error {
	result := make(chan error, 1)
	
	job := &EmailJob{
		To:          to,
		Subject:     subject,
		BodyHTML:    bodyHTML,
		BodyText:    bodyText,
		Attachments: attachments,
		Result:      result,
	}
	
	select {
	case s.asyncQueue <- job:
		return <-result
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendAsync sends an email asynchronously
func (s *EmailService) SendAsync(to, subject, bodyHTML, bodyText string, attachments []string) {
	job := &EmailJob{
		To:          to,
		Subject:     subject,
		BodyHTML:    bodyHTML,
		BodyText:    bodyText,
		Attachments: attachments,
		Result:      make(chan error, 1),
	}
	s.asyncQueue <- job
}

// sendEmail performs the actual email sending
func (s *EmailService) sendEmail(to, subject, bodyHTML, bodyText string, attachments []string) error {
	var msg strings.Builder
	
	from := fmt.Sprintf("%s <%s>", s.fromName, s.fromEmail)
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: multipart/mixed; boundary=boundary\r\n\r\n")
	
	// Text part
	msg.WriteString("--boundary\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	msg.WriteString(bodyText + "\r\n")
	
	// HTML part
	msg.WriteString("--boundary\r\n")
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
	msg.WriteString(bodyHTML + "\r\n")
	
	msg.WriteString("--boundary--\r\n")
	
	addr := fmt.Sprintf("%s:%d", s.smtpHost, s.smtpPort)
	
	var err error
	if s.useTLS {
		err = smtp.SendMail(addr, s.auth(), s.fromEmail, []string{to}, []byte(msg.String()))
	} else {
		err = smtp.SendMail(addr, s.auth(), s.fromEmail, []string{to}, []byte(msg.String()))
	}
	
	return err
}

// auth returns the SMTP authentication
func (s *EmailService) auth() smtp.Auth {
	return smtp.PlainAuth("", s.username, s.password, s.smtpHost)
}

// Close stops the email workers
func (s *EmailService) Close() {
	close(s.asyncQueue)
	s.wg.Wait()
}

// Template rendering functions
func (s *EmailService) RenderInvoiceEmail(invoice *models.Invoice, client *models.Client, user *models.User) (string, string, error) {
	subject := fmt.Sprintf("Invoice %s from %s", invoice.InvoiceNumber, user.CompanyName)
	
	variables := map[string]string{
		"invoice_number": invoice.InvoiceNumber,
		"invoice_date":   invoice.CreatedAt.Format("Jan 02, 2006"),
		"due_date":       invoice.DueDate.Format("Jan 02, 2006"),
		"client_name":     client.Name,
		"client_email":   client.Email,
		"company_name":   user.CompanyName,
		"company_email":  user.Email,
		"company_phone":  user.Phone,
		"total":          fmt.Sprintf("%.2f", invoice.Total),
		"currency":       invoice.Currency,
		"payment_link":   fmt.Sprintf("https://invoice.simuxtech.com/invoice/%s", invoice.MagicToken),
		"brand_color":    "#2563eb",
	}
	
	// Simple template rendering (in production, use html/template)
	bodyHTML := s.renderTemplate(invoiceTemplateHTML, variables)
	_ = s.renderTemplate(invoiceTemplateText, variables) // Plain text version
	
	return subject, bodyHTML, nil
}

func (s *EmailService) renderTemplate(template string, vars map[string]string) string {
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

// Templates (simplified versions - use separate files in production)
var invoiceTemplateHTML = `
<h1>Invoice {{invoice_number}}</h1>
<p>Dear {{client_name}},</p>
<p>Please find your invoice attached.</p>
<p><strong>Amount:</strong> {{currency}} {{total}}</p>
<p><a href="{{payment_link}}">Pay Now</a></p>
`

var invoiceTemplateText = `
Invoice {{invoice_number}}

Dear {{client_name}},

Please find your invoice details below.

Amount: {{currency}} {{total}}
Due Date: {{due_date}}

Pay now: {{payment_link}}
`
