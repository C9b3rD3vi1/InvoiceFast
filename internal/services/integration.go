package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type IntegrationService struct {
	db *database.DB
}

func NewIntegrationService(db *database.DB) *IntegrationService {
	return &IntegrationService{db: db}
}

type IntegrationConfig struct {
	// WhatsApp (Meta)
	WhatsAppPhoneNumberID string `json:"whatsapp_phone_number_id,omitempty"`
	WhatsAppAccessToken   string `json:"whatsapp_access_token,omitempty"`
	WhatsAppBusinessID    string `json:"whatsapp_business_id,omitempty"`

	// SMS
	SMSProvider     string `json:"sms_provider,omitempty"` // africaastalking, nexmo, twilio
	SMSAPIKey      string `json:"sms_api_key,omitempty"`
	SMSAPISecret   string `json:"sms_api_secret,omitempty"`
	SMSPhoneNumber string `json:"sms_phone_number,omitempty"`
	SMSShortCode   string `json:"sms_short_code,omitempty"`

	// Email - Provider type
	EmailProvider string `json:"email_provider,omitempty"` // smtp, sendgrid, mailgun, ses, postmark

	// Email SMTP
	SMTPHost      string `json:"smtp_host,omitempty"`
	SMTPPort      int    `json:"smtp_port,omitempty"`
	SMTPUsername  string `json:"smtp_username,omitempty"`
	SMTPPassword  string `json:"smtp_password,omitempty"`
	SMTPFromEmail string `json:"smtp_from_email,omitempty"`
	SMTPFromName  string `json:"smtp_from_name,omitempty"`
	SMTPUseTLS    bool   `json:"smtp_use_tls,omitempty"`

	// Email SendGrid
	SendGridAPIKey string `json:"sendgrid_api_key,omitempty"`

	// Email Mailgun
	MailgunAPIKey string `json:"mailgun_api_key,omitempty"`
	MailgunDomain string `json:"mailgun_domain,omitempty"`

	// Email AWS SES
	AWSAccessKeyID     string `json:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `json:"aws_secret_access_key,omitempty"`
	AWSRegion          string `json:"aws_region,omitempty"`

	// Email Postmark
	PostmarkAPIToken string `json:"postmark_api_token,omitempty"`

	// Common
	FromEmail string `json:"from_email,omitempty"`
	FromName  string `json:"from_name,omitempty"`

	// Slack
	SlackWebhookURL string `json:"slack_webhook_url,omitempty"`
	SlackChannel    string `json:"slack_channel,omitempty"`
}

func (s *IntegrationService) GetIntegrations(tenantID string) ([]models.Integration, error) {
	var integrations []models.Integration
	err := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&integrations).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get integrations: %w", err)
	}
	return integrations, nil
}

func (s *IntegrationService) GetIntegrationByProvider(tenantID, provider string) (*models.Integration, error) {
	var integration models.Integration
	err := s.db.Where("tenant_id = ? AND provider = ?", tenantID, provider).First(&integration).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get integration: %w", err)
	}
	return &integration, nil
}

func (s *IntegrationService) isIntegrationConfigured(provider string, config *IntegrationConfig) bool {
	switch provider {
	case "whatsapp":
		return config.WhatsAppPhoneNumberID != "" && config.WhatsAppAccessToken != ""
	case "sms":
		return config.SMSAPIKey != ""
	case "smtp", "email":
		// Check based on provider type
		if config.EmailProvider == "smtp" {
			return config.SMTPHost != "" && config.SMTPUsername != ""
		} else if config.EmailProvider == "sendgrid" {
			return config.SendGridAPIKey != ""
		} else if config.EmailProvider == "mailgun" {
			return config.MailgunAPIKey != ""
		} else if config.EmailProvider == "ses" {
			return config.AWSAccessKeyID != "" && config.AWSSecretAccessKey != ""
		} else if config.EmailProvider == "postmark" {
			return config.PostmarkAPIToken != ""
		}
		return config.SMTPHost != "" || config.SendGridAPIKey != "" || config.MailgunAPIKey != "" || config.AWSAccessKeyID != "" || config.PostmarkAPIToken != ""
	case "slack":
		return config.SlackWebhookURL != ""
	default:
		return false
	}
}

func (s *IntegrationService) SaveIntegration(tenantID, provider, name, description string, config *IntegrationConfig) (*models.Integration, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	encryptedConfig := models.EncryptValue(string(configJSON))

	existing, err := s.GetIntegrationByProvider(tenantID, provider)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		existing.Name = name
		existing.Description = description
		existing.Config = encryptedConfig
		existing.UpdatedAt = time.Now()
		existing.IsConfigured = s.isIntegrationConfigured(provider, config)
		err := s.db.Save(existing).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update integration: %w", err)
		}
		return existing, nil
	}

	isConfigured := s.isIntegrationConfigured(provider, config)
	integration := &models.Integration{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		Provider:     provider,
		Name:         name,
		Description:  description,
		Config:       encryptedConfig,
		IsActive:     true,
		IsConfigured: isConfigured,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err = s.db.Create(integration).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create integration: %w", err)
	}

	return integration, nil
}

func (s *IntegrationService) GetIntegrationConfig(tenantID, provider string) (*IntegrationConfig, error) {
	integration, err := s.GetIntegrationByProvider(tenantID, provider)
	if err != nil {
		return nil, err
	}
	if integration == nil {
		return nil, nil
	}

	decrypted := models.DecryptValue(integration.Config)

	var config IntegrationConfig
	err = json.Unmarshal([]byte(decrypted), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if config.WhatsAppAccessToken != "" {
		config.WhatsAppAccessToken = "********"
	}
	if config.SMSAPISecret != "" {
		config.SMSAPISecret = "********"
	}
	if config.SMTPPassword != "" {
		config.SMTPPassword = "********"
	}

	return &config, nil
}

func (s *IntegrationService) DeleteIntegration(tenantID, integrationID string) error {
	result := s.db.Where("id = ? AND tenant_id = ?", integrationID, tenantID).Delete(&models.Integration{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete integration: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("integration not found")
	}
	return nil
}

func (s *IntegrationService) ToggleIntegration(tenantID, integrationID string, active bool) error {
	result := s.db.Model(&models.Integration{}).
		Where("id = ? AND tenant_id = ?", integrationID, tenantID).
		Updates(map[string]interface{}{
			"is_active": active,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to toggle integration: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("integration not found")
	}
	return nil
}

func (s *IntegrationService) IsWhatsAppConfigured(tenantID string) bool {
	integration, _ := s.GetIntegrationByProvider(tenantID, "whatsapp")
	return integration != nil && integration.IsActive
}

func (s *IntegrationService) IsSMSConfigured(tenantID string) bool {
	integration, _ := s.GetIntegrationByProvider(tenantID, "sms")
	return integration != nil && integration.IsActive
}

func (s *IntegrationService) IsEmailConfigured(tenantID string) bool {
	integration, _ := s.GetIntegrationByProvider(tenantID, "email")
	return integration != nil && integration.IsActive
}