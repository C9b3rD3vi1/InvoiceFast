package services

import (
	"fmt"
	"log"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

type ThankYouMessageService struct {
	db           *database.DB
	emailService *EmailService
}

func NewThankYouMessageService(db *database.DB, email *EmailService) *ThankYouMessageService {
	return &ThankYouMessageService{
		db:           db,
		emailService: email,
	}
}

func (s *ThankYouMessageService) SendThankYou(invoiceID string) error {
	var invoice models.Invoice
	if err := s.db.Preload("Client").First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return fmt.Errorf("invoice not found: %w", err)
	}

	if invoice.ClientID == "" {
		return fmt.Errorf("client not found for invoice")
	}

	client, err := s.GetClient(invoice.ClientID)
	if err != nil {
		return fmt.Errorf("client not found: %w", err)
	}

	companyName := s.getCompanyName(invoice.TenantID)

	if client.Email != "" {
		s.sendEmailThankYou(client.Email, &invoice, companyName)
	}

	log.Printf("Thank you message sent for invoice %s to client %s", invoice.InvoiceNumber, client.Name)
	return nil
}

func (s *ThankYouMessageService) GetClient(clientID string) (*models.Client, error) {
	var client models.Client
	if err := s.db.First(&client, "id = ?", clientID).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

func (s *ThankYouMessageService) sendEmailThankYou(to string, invoice *models.Invoice, companyName string) {
	if s.emailService == nil {
		return
	}

	subject := fmt.Sprintf("Payment Received - Invoice %s", invoice.InvoiceNumber)
	body := fmt.Sprintf(`
		<html>
		<body>
			<h2>Thank You for Your Payment!</h2>
			<p>Dear %s,</p>
			<p>We have received your payment for Invoice <strong>%s</strong>.</p>
			<p>Amount: %s %.2f</p>
			<p>Thank you for your business. We appreciate your prompt payment.</p>
			<br>
			<p>Best regards,<br>%s</p>
		</body>
		</html>
	`, invoice.Client.Name, invoice.InvoiceNumber, invoice.Currency, invoice.PaidAmount, companyName)

	s.emailService.Send(EmailRequest{
		To:      []string{to},
		Subject: subject,
		Body:    body,
		IsHTML:  true,
	})
}

func (s *ThankYouMessageService) getCompanyName(tenantID string) string {
	var tenant models.Tenant
	if err := s.db.First(&tenant, "id = ?", tenantID).Error; err != nil {
		return "InvoiceFast"
	}
	return tenant.Name
}

type ThankYouConfig struct {
	Enabled       bool   `json:"enabled"`
	SendEmail     bool   `json:"send_email"`
	EmailTemplate string `json:"email_template"`
}

func (s *ThankYouMessageService) GetConfig(tenantID string) (*ThankYouConfig, error) {
	return &ThankYouConfig{
		Enabled:   true,
		SendEmail: true,
	}, nil
}

func (s *ThankYouMessageService) UpdateConfig(tenantID string, config *ThankYouConfig) error {
	return nil
}
