package services_test

import (
	"os"
	"testing"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"invoicefast/internal/services"
)

// Helper function to create a test database and service
func setupTestService(t *testing.T) (*services.SettingsService, string) {
	// Initialize encryption for tests
	if os.Getenv("ENCRYPTION_KEY") == "" {
		os.Setenv("ENCRYPTION_KEY", "a-very-long-encryption-key-for-testing-123456789012")
	}
	if err := models.InitEncryption(os.Getenv("ENCRYPTION_KEY")); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	// Setup in-memory SQLite database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate the schema
	mysqlDB := &database.DB{DB: db}
	err = mysqlDB.Migrate()
	require.NoError(t, err)

	// Create settings service
	settingsService := services.NewSettingsService(mysqlDB)

	// Create a test tenant
	tenant, err := settingsService.CreateTenant("Test Tenant", "test")
	require.NoError(t, err)
	tenantID := tenant.ID

	return settingsService, tenantID
}

func TestSettingsService_EmailSMSWhatsapp(t *testing.T) {
	settingsService, tenantID := setupTestService(t)

	// Test Email Settings
	emailReq := services.EmailSettings{
		SMTPHost:     "smtp.example.com",
		SMTPPort:     "587",
		SMTPUsername: "test@example.com",
		SMTPPassword: "secret123",
		FromName:     "Test Sender",
		FromEmail:    "test@example.com",
		Enabled:      true,
	}

	// Save email settings
	err := settingsService.SaveEmailSettings(tenantID, &emailReq)
	assert.NoError(t, err)

	// Retrieve email settings
	retrievedEmail, err := settingsService.GetEmailSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedEmail)
	assert.Equal(t, emailReq.SMTPHost, retrievedEmail.SMTPHost)
	assert.Equal(t, emailReq.SMTPPort, retrievedEmail.SMTPPort)
	assert.Equal(t, emailReq.SMTPUsername, retrievedEmail.SMTPUsername)
	assert.Equal(t, emailReq.FromName, retrievedEmail.FromName)
	assert.Equal(t, emailReq.FromEmail, retrievedEmail.FromEmail)
	assert.Equal(t, emailReq.Enabled, retrievedEmail.Enabled)
	// Password should be encrypted
	assert.NotEqual(t, emailReq.SMTPPassword, retrievedEmail.SMTPPassword)
	assert.NotEmpty(t, retrievedEmail.SMTPPassword)

	// Test SMS Settings
	smsReq := services.SMSSettings{
		Provider:   "africastalking",
		APIKey:     "aks_test123",
		APISecret:  "secret456",
		SenderID:   "INVOICE",
		SMSEndpoint: "https://api.example.com/sms",
		Enabled:    true,
	}

	// Save SMS settings
	err = settingsService.SaveSMSSettings(tenantID, &smsReq)
	assert.NoError(t, err)

	// Retrieve SMS settings
	retrievedSMS, err := settingsService.GetSMSSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedSMS)
	assert.Equal(t, smsReq.Provider, retrievedSMS.Provider)
	assert.Equal(t, smsReq.SenderID, retrievedSMS.SenderID)
	assert.Equal(t, smsReq.SMSEndpoint, retrievedSMS.SMSEndpoint)
	assert.Equal(t, smsReq.Enabled, retrievedSMS.Enabled)
	// Secrets should be encrypted
	assert.NotEqual(t, smsReq.APIKey, retrievedSMS.APIKey)
	assert.NotEqual(t, smsReq.APISecret, retrievedSMS.APISecret)
	assert.NotEmpty(t, retrievedSMS.APIKey)
	assert.NotEmpty(t, retrievedSMS.APISecret)

	// Test WhatsApp Settings
	whatsappReq := services.WhatsAppSettings{
		Provider:     "twilio",
		AccountSID:   "ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		AuthToken:    "your_auth_token",
		FromNumber:   "+1234567890",
		MetaPhoneID:  "123456789012345",
		MetaToken:    "EAAxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		MetaBusinessID: "12345678901234567",
		Enabled:      true,
	}

	// Save WhatsApp settings
	err = settingsService.SaveWhatsAppSettings(tenantID, &whatsappReq)
	assert.NoError(t, err)

	// Retrieve WhatsApp settings
	retrievedWhatsApp, err := settingsService.GetWhatsAppSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedWhatsApp)
	assert.Equal(t, whatsappReq.Provider, retrievedWhatsApp.Provider)
	assert.Equal(t, whatsappReq.FromNumber, retrievedWhatsApp.FromNumber)
	assert.Equal(t, whatsappReq.Enabled, retrievedWhatsApp.Enabled)
	// Secrets should be encrypted
	assert.NotEqual(t, whatsappReq.AccountSID, retrievedWhatsApp.AccountSID)
	assert.NotEqual(t, whatsappReq.AuthToken, retrievedWhatsApp.AuthToken)
	assert.NotEqual(t, whatsappReq.MetaPhoneID, retrievedWhatsApp.MetaPhoneID)
	assert.NotEqual(t, whatsappReq.MetaToken, retrievedWhatsApp.MetaToken)
	assert.NotEqual(t, whatsappReq.MetaBusinessID, retrievedWhatsApp.MetaBusinessID)
	assert.NotEmpty(t, retrievedWhatsApp.AccountSID)
	assert.NotEmpty(t, retrievedWhatsApp.AuthToken)
	assert.NotEmpty(t, retrievedWhatsApp.MetaPhoneID)
	assert.NotEmpty(t, retrievedWhatsApp.MetaToken)
	assert.NotEmpty(t, retrievedWhatsApp.MetaBusinessID)

	// Test that all settings can be retrieved together
	allSettings, err := settingsService.GetSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, allSettings)
	assert.NotNil(t, allSettings.Email)
	assert.NotNil(t, allSettings.SMS)
	assert.NotNil(t, allSettings.WhatsApp)
	assert.Equal(t, emailReq.SMTPHost, allSettings.Email.SMTPHost)
	assert.Equal(t, smsReq.Provider, allSettings.SMS.Provider)
	assert.Equal(t, whatsappReq.Provider, allSettings.WhatsApp.Provider)
}

	// Save email settings
	err := settingsService.SaveEmailSettings(tenantID, &emailReq)
	assert.NoError(t, err)

	// Retrieve email settings
	retrievedEmail, err := settingsService.GetEmailSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedEmail)
	assert.Equal(t, emailReq.SMTPHost, retrievedEmail.SMTPHost)
	assert.Equal(t, emailReq.SMTPPort, retrievedEmail.SMTPPort)
	assert.Equal(t, emailReq.SMTPUsername, retrievedEmail.SMTPUsername)
	assert.Equal(t, emailReq.FromName, retrievedEmail.FromName)
	assert.Equal(t, emailReq.FromEmail, retrievedEmail.FromEmail)
	assert.Equal(t, emailReq.Enabled, retrievedEmail.Enabled)
	// Password should be encrypted
	assert.NotEqual(t, emailReq.SMTPPassword, retrievedEmail.SMTPPassword)
	assert.NotEmpty(t, retrievedEmail.SMTPPassword)

	// Test SMS Settings
	smsReq := services.SMSSettings{
		Provider:   "africastalking",
		APIKey:     "aks_test123",
		APISecret:  "secret456",
		SenderID:   "INVOICE",
		SMSEndpoint: "https://api.example.com/sms",
		Enabled:    true,
	}

	// Save SMS settings
	err = settingsService.SaveSMSSettings(tenantID, &smsReq)
	assert.NoError(t, err)

	// Retrieve SMS settings
	retrievedSMS, err := settingsService.GetSMSSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedSMS)
	assert.Equal(t, smsReq.Provider, retrievedSMS.Provider)
	assert.Equal(t, smsReq.SenderID, retrievedSMS.SenderID)
	assert.Equal(t, smsReq.SMSEndpoint, retrievedSMS.SMSEndpoint)
	assert.Equal(t, smsReq.Enabled, retrievedSMS.Enabled)
	// Secrets should be encrypted
	assert.NotEqual(t, smsReq.APIKey, retrievedSMS.APIKey)
	assert.NotEqual(t, smsReq.APISecret, retrievedSMS.APISecret)
	assert.NotEmpty(t, retrievedSMS.APIKey)
	assert.NotEmpty(t, retrievedSMS.APISecret)

	// Test WhatsApp Settings
	whatsappReq := services.WhatsAppSettings{
		Provider:     "twilio",
		AccountSID:   "ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		AuthToken:    "your_auth_token",
		FromNumber:   "+1234567890",
		MetaPhoneID:  "123456789012345",
		MetaToken:    "EAAxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		MetaBusinessID: "12345678901234567",
		Enabled:      true,
	}

	// Save WhatsApp settings
	err = settingsService.SaveWhatsAppSettings(tenantID, &whatsappReq)
	assert.NoError(t, err)

	// Retrieve WhatsApp settings
	retrievedWhatsApp, err := settingsService.GetWhatsAppSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedWhatsApp)
	assert.Equal(t, whatsappReq.Provider, retrievedWhatsApp.Provider)
	assert.Equal(t, whatsappReq.FromNumber, retrievedWhatsApp.FromNumber)
	assert.Equal(t, whatsappReq.Enabled, retrievedWhatsApp.Enabled)
	// Secrets should be encrypted
	assert.NotEqual(t, whatsappReq.AccountSID, retrievedWhatsApp.AccountSID)
	assert.NotEqual(t, whatsappReq.AuthToken, retrievedWhatsApp.AuthToken)
	assert.NotEqual(t, whatsappReq.MetaPhoneID, retrievedWhatsApp.MetaPhoneID)
	assert.NotEqual(t, whatsappReq.MetaToken, retrievedWhatsApp.MetaToken)
	assert.NotEqual(t, whatsappReq.MetaBusinessID, retrievedWhatsApp.MetaBusinessID)
	assert.NotEmpty(t, retrievedWhatsApp.AccountSID)
	assert.NotEmpty(t, retrievedWhatsApp.AuthToken)
	assert.NotEmpty(t, retrievedWhatsApp.MetaPhoneID)
	assert.NotEmpty(t, retrievedWhatsApp.MetaToken)
	assert.NotEmpty(t, retrievedWhatsApp.MetaBusinessID)

	// Test that all settings can be retrieved together
	allSettings, err := settingsService.GetSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, allSettings)
	assert.NotNil(t, allSettings.Email)
	assert.NotNil(t, allSettings.SMS)
	assert.NotNil(t, allSettings.WhatsApp)
	assert.Equal(t, emailReq.SMTPHost, allSettings.Email.SMTPHost)
	assert.Equal(t, smsReq.Provider, allSettings.SMS.Provider)
	assert.Equal(t, whatsappReq.Provider, allSettings.WhatsApp.Provider)
}

func TestSettingsService_EncryptionDecryption(t *testing.T) {
	settingsService, tenantID := setupTestService(t)

	// Test with various sensitive values
	settings := &services.TenantSettings{
		Email: &services.EmailSettings{
			SMTPPassword: "email_password_123",
		},
		SMS: &services.SMSSettings{
			APIKey:    "sms_api_key_456",
			APISecret: "sms_api_secret_789",
		},
		WhatsApp: &services.WhatsAppSettings{
			AccountSID:   "ACcount_sid_123",
			AuthToken:    "auth_token_456",
			MetaPhoneID:  "meta_phone_id_789",
			MetaToken:    "meta_token_123",
			MetaBusinessID: "meta_business_id_456",
		},
		Updated: time.Now(),
	}

	// Encrypt settings - we can't test this directly as it's unexported
	// Instead test through the save/get flow which uses encryption internally

	// Save settings (this will encrypt internally)
	err := settingsService.SaveSettings(tenantID, settings)
	assert.NoError(t, err)

	// Retrieve settings (this will decrypt internally)
	retrievedSettings, err := settingsService.GetSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedSettings)
	assert.Equal(t, "email_password_123", retrievedSettings.Email.SMTPPassword)
	assert.Equal(t, "sms_api_key_456", retrievedSettings.SMS.APIKey)
	assert.Equal(t, "sms_api_secret_789", retrievedSettings.SMS.APISecret)
	assert.Equal(t, "ACcount_sid_123", retrievedSettings.WhatsApp.AccountSID)
	assert.Equal(t, "auth_token_456", retrievedSettings.WhatsApp.AuthToken)
	assert.Equal(t, "meta_phone_id_789", retrievedSettings.WhatsApp.MetaPhoneID)
	assert.Equal(t, "meta_token_123", retrievedSettings.WhatsApp.MetaToken)
	assert.Equal(t, "meta_business_id_456", retrievedSettings.WhatsApp.MetaBusinessID)
}

func TestSettingsService_MaskSecrets(t *testing.T) {
	settingsService, _ := setupTestService(t)

	// Test masking
	settings := &services.TenantSettings{
		Email: &services.EmailSettings{
			SMTPPassword: "email_password_123",
		},
		SMS: &services.SMSSettings{
			APIKey:    "sms_api_key_456",
			APISecret: "sms_api_secret_789",
		},
		WhatsApp: &services.WhatsAppSettings{
			AccountSID:   "ACcount_sid_123",
			AuthToken:    "auth_token_456",
			MetaPhoneID:  "meta_phone_id_789",
			MetaToken:    "meta_token_123",
			MetaBusinessID: "meta_business_id_456",
		},
		Updated: time.Now(),
	}

	// Apply masking
	settingsService.MaskSecrets(settings)

	// Verify masking occurred
	assert.Equal(t, "********", settings.Email.SMTPPassword)
	assert.Equal(t, "********", settings.SMS.APIKey)
	assert.Equal(t, "********", settings.SMS.APISecret)
	assert.Equal(t, "********", settings.WhatsApp.AccountSID)
	assert.Equal(t, "********", settings.WhatsApp.AuthToken)
	assert.Equal(t, "********", settings.WhatsApp.MetaPhoneID)
	assert.Equal(t, "********", settings.WhatsApp.MetaToken)
	assert.Equal(t, "********", settings.WhatsApp.MetaBusinessID)
}