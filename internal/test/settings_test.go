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

func setupTestService(t *testing.T) (*services.SettingsService, string) {
	if os.Getenv("ENCRYPTION_KEY") == "" {
		os.Setenv("ENCRYPTION_KEY", "a-very-long-encryption-key-for-testing-123456789012")
	}
	if err := models.InitEncryption(os.Getenv("ENCRYPTION_KEY")); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	mysqlDB := &database.DB{DB: db}
	err = mysqlDB.Migrate()
	require.NoError(t, err)

	settingsService := services.NewSettingsService(mysqlDB)

	tenant, err := settingsService.CreateTenant("Test Tenant", "test")
	require.NoError(t, err)
	tenantID := tenant.ID

	return settingsService, tenantID
}

func TestSettingsService_SaveAndGetSettings(t *testing.T) {
	settingsService, tenantID := setupTestService(t)

	settings := &services.TenantSettings{
		Business: &services.BusinessSettings{
			Name:    "Test Corp",
			Email:   "corp@test.com",
			Phone:   "+254700000000",
			Address: "Nairobi",
		},
		Profile: &services.ProfileSettings{
			Name:  "Admin",
			Email: "admin@test.com",
		},
		Invoice: &services.InvoiceSettings{
			Prefix:   "INV",
			Currency: "KES",
		},
		Mpesa: &services.MpesaSettings{
			ConsumerKey:    "ck_test",
			ConsumerSecret: "cs_secret",
		},
		KRA: &services.KRASettings{
			APIKey: "kra_key",
		},
		Notifications: &services.NotificationSettings{},
		Updated:       time.Now(),
	}

	err := settingsService.SaveSettings(tenantID, settings)
	assert.NoError(t, err)

	retrieved, err := settingsService.GetSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "Test Corp", retrieved.Business.Name)
	assert.Equal(t, "INV", retrieved.Invoice.Prefix)
}

func TestSettingsService_EncryptionDecryption(t *testing.T) {
	settingsService, tenantID := setupTestService(t)

	settings := &services.TenantSettings{
		Mpesa: &services.MpesaSettings{
			ConsumerKey:    "ck_encrypt_test",
			ConsumerSecret: "cs_encrypt_secret",
			Passkey:        "passkey_123",
		},
		KRA: &services.KRASettings{
			APIKey:        "kra_encrypt_key",
			RSAPrivateKey: "rsa_private_key",
		},
		Notifications: &services.NotificationSettings{},
		Updated:       time.Now(),
	}

	err := settingsService.SaveSettings(tenantID, settings)
	assert.NoError(t, err)

	retrieved, err := settingsService.GetSettings(tenantID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "ck_encrypt_test", retrieved.Mpesa.ConsumerKey)
	assert.Equal(t, "cs_encrypt_secret", retrieved.Mpesa.ConsumerSecret)
	assert.Equal(t, "passkey_123", retrieved.Mpesa.Passkey)
	assert.Equal(t, "kra_encrypt_key", retrieved.KRA.APIKey)
	assert.Equal(t, "rsa_private_key", retrieved.KRA.RSAPrivateKey)
}

func TestSettingsService_MaskSecrets(t *testing.T) {
	settingsService, _ := setupTestService(t)

	settings := &services.TenantSettings{
		Mpesa: &services.MpesaSettings{
			ConsumerSecret: "cs_secret",
			Passkey:        "passkey_123",
		},
		KRA: &services.KRASettings{
			APIKey:        "kra_key",
			RSAPrivateKey: "rsa_private",
		},
		Notifications: &services.NotificationSettings{},
	}

	settingsService.MaskSecrets(settings)

	assert.Equal(t, "********", settings.Mpesa.ConsumerSecret)
	assert.Equal(t, "********", settings.Mpesa.Passkey)
	assert.Equal(t, "********", settings.KRA.APIKey)
	assert.Equal(t, "********", settings.KRA.RSAPrivateKey)
}
