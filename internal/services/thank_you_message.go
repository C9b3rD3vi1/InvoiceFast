package services

import (
	"context"
	"fmt"

	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/models"
)

type ThankYouMessageService struct {
	db               *database.DB
	emailService    *EmailService
	notificationSvc *NotificationService
}

func NewThankYouMessageService(db *database.DB, deps *ServiceDependencies) *ThankYouMessageService {
	return &ThankYouMessageService{
		db:               db,
		emailService:     deps.Email,
		notificationSvc: deps.Notification,
	}
}

// SendThankYou sends a thank you message to the client - expects invoice to already be loaded with TenantID
func (s *ThankYouMessageService) SendThankYou(invoice *models.Invoice) error {
	if invoice == nil || invoice.ID == "" {
		return fmt.Errorf("invoice is required")
	}

	if invoice.ClientID == "" {
		return fmt.Errorf("client not found for invoice")
	}

	tenantID := invoice.TenantID
	client, err := s.GetClient(tenantID, invoice.ClientID)
	if err != nil {
		return fmt.Errorf("client not found: %w", err)
	}

	companyName := s.getCompanyName(tenantID)

	if client.Email != "" {
		s.sendEmailThankYou(client.Email, invoice, companyName)
	}

	logger.Get().Info(context.Background(), "Thank you message sent for invoice", "invoice_number", invoice.InvoiceNumber, "client_name", client.Name)
	return nil
}

func (s *ThankYouMessageService) GetClient(tenantID, clientID string) (*models.Client, error) {
	var client models.Client
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&client, "id = ?", clientID).Error; err != nil {
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
	`, invoice.Client.Name, invoice.InvoiceNumber, invoice.Currency, invoice.PaidAmount.Float64(), companyName)

	billingName, billingEmail := s.emailService.sender("billing")
	s.emailService.Send(EmailRequest{
		FromName:  billingName,
		FromEmail: billingEmail,
		To:        []string{to},
		Subject:   subject,
		Body:      body,
		IsHTML:    true,
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
