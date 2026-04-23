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

type SettingsService struct {
	db *database.DB
}

func NewSettingsService(db *database.DB) *SettingsService {
	return &SettingsService{db: db}
}

type TenantSettings struct {
	Mpesa         *MpesaSettings        `json:"mpesa,omitempty"`
	KRA           *KRASettings          `json:"kra,omitempty"`
	Branding      *BrandingSettings     `json:"branding,omitempty"`
	Notifications *NotificationSettings `json:"notifications,omitempty"`
	Updated       time.Time             `json:"updated_at"`
}

type NotificationSettings struct {
	Email   []NotificationEvent `json:"email"`
	SMS     []NotificationEvent `json:"sms"`
	Slack   []NotificationEvent `json:"slack"`
	Webhook []NotificationEvent `json:"webhook"`
}

type NotificationEvent struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type MpesaSettings struct {
	ConsumerKey    string `json:"consumer_key"`
	ConsumerSecret string `json:"-"` // Encrypted at rest
	Shortcode      string `json:"shortcode"`
	Passkey        string `json:"-"` // Encrypted at rest
	Enabled        bool   `json:"enabled"`
}

type KRASettings struct {
	VendorID      string `json:"vendor_id"`
	APIKey        string `json:"-"` // Encrypted at rest
	RSAPrivateKey string `json:"-"` // Encrypted at rest
	LiveMode      bool   `json:"live_mode"`
	Enabled       bool   `json:"enabled"`
}

type BrandingSettings struct {
	LogoURL     string `json:"logo_url"`
	BrandColor  string `json:"brand_color"`
	CompanyName string `json:"company_name"`
}

// SECURITY: Encrypt sensitive fields before saving to database
// Uses AES-256-GCM from models package
func (s *SettingsService) encryptSettings(settings *TenantSettings) error {
	if settings.Mpesa != nil {
		if settings.Mpesa.ConsumerSecret != "" {
			settings.Mpesa.ConsumerSecret = models.EncryptValue(settings.Mpesa.ConsumerSecret)
		}
		if settings.Mpesa.Passkey != "" {
			settings.Mpesa.Passkey = models.EncryptValue(settings.Mpesa.Passkey)
		}
	}
	if settings.KRA != nil {
		if settings.KRA.APIKey != "" {
			settings.KRA.APIKey = models.EncryptValue(settings.KRA.APIKey)
		}
		if settings.KRA.RSAPrivateKey != "" {
			settings.KRA.RSAPrivateKey = models.EncryptValue(settings.KRA.RSAPrivateKey)
		}
	}
	return nil
}

// SECURITY: Decrypt sensitive fields after reading from database
func (s *SettingsService) decryptSettings(settings *TenantSettings) error {
	if settings.Mpesa != nil {
		if settings.Mpesa.ConsumerSecret != "" {
			settings.Mpesa.ConsumerSecret = models.DecryptValue(settings.Mpesa.ConsumerSecret)
		}
		if settings.Mpesa.Passkey != "" {
			settings.Mpesa.Passkey = models.DecryptValue(settings.Mpesa.Passkey)
		}
	}
	if settings.KRA != nil {
		if settings.KRA.APIKey != "" {
			settings.KRA.APIKey = models.DecryptValue(settings.KRA.APIKey)
		}
		if settings.KRA.RSAPrivateKey != "" {
			settings.KRA.RSAPrivateKey = models.DecryptValue(settings.KRA.RSAPrivateKey)
		}
	}
	return nil
}

// MaskSecrets masks sensitive values for UI display
func (s *SettingsService) MaskSecrets(settings *TenantSettings) {
	if settings.Mpesa != nil {
		if settings.Mpesa.ConsumerSecret != "" {
			settings.Mpesa.ConsumerSecret = "********"
		}
		if settings.Mpesa.Passkey != "" {
			settings.Mpesa.Passkey = "********"
		}
	}
	if settings.KRA != nil {
		if settings.KRA.APIKey != "" {
			settings.KRA.APIKey = "********"
		}
		if settings.KRA.RSAPrivateKey != "" {
			settings.KRA.RSAPrivateKey = "********"
		}
	}
}

func (s *SettingsService) GetSettings(tenantID string) (*TenantSettings, error) {
	var tenant models.Tenant
	err := s.db.First(&tenant, "id = ?", tenantID).Error
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}

	settings := &TenantSettings{Updated: tenant.UpdatedAt}
	if tenant.Settings != "" {
		if err := json.Unmarshal([]byte(tenant.Settings), settings); err != nil {
			return nil, fmt.Errorf("failed to parse settings: %w", err)
		}
		// Decrypt stored secrets for internal use
		_ = s.decryptSettings(settings)
		// Mask secrets before returning to caller (UI)
		s.MaskSecrets(settings)
	}
	return settings, nil
}

func (s *SettingsService) SaveSettings(tenantID string, settings *TenantSettings) error {
	settings.Updated = time.Now()

	// Encrypt sensitive fields before saving
	if err := s.encryptSettings(settings); err != nil {
		return fmt.Errorf("failed to encrypt settings: %w", err)
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

result := s.db.Model(&models.Tenant{}).
		Where("id = ?", tenantID).
		Update("settings", string(settingsJSON))

	if result.Error != nil {
		return fmt.Errorf("failed to save settings: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

func (s *SettingsService) SaveMpesaSettings(tenantID string, mpesa *MpesaSettings) error {
	settings, err := s.GetSettings(tenantID)
	if err != nil {
		settings = &TenantSettings{}
	}
	settings.Mpesa = mpesa
	return s.SaveSettings(tenantID, settings)
}

func (s *SettingsService) SaveKRASettings(tenantID string, kra *KRASettings) error {
	settings, err := s.GetSettings(tenantID)
	if err != nil {
		settings = &TenantSettings{}
	}
	settings.KRA = kra
	return s.SaveSettings(tenantID, settings)
}

func (s *SettingsService) SaveBrandingSettings(tenantID string, branding *BrandingSettings) error {
	settings, err := s.GetSettings(tenantID)
	if err != nil {
		settings = &TenantSettings{}
	}
	settings.Branding = branding
	return s.SaveSettings(tenantID, settings)
}

func (s *SettingsService) GetMpesaSettings(tenantID string) (*MpesaSettings, error) {
	settings, err := s.GetSettings(tenantID)
	if err != nil {
		return nil, err
	}
	if settings.Mpesa == nil {
		return &MpesaSettings{}, nil
	}
	return settings.Mpesa, nil
}

func (s *SettingsService) GetKRASettings(tenantID string) (*KRASettings, error) {
	settings, err := s.GetSettings(tenantID)
	if err != nil {
		return nil, err
	}
	if settings.KRA == nil {
		return &KRASettings{}, nil
	}
	return settings.KRA, nil
}

func (s *SettingsService) IsMpesaConfigured(tenantID string) bool {
	settings, err := s.GetMpesaSettings(tenantID)
	if err != nil {
		return false
	}
	return settings.ConsumerKey != "" && settings.Shortcode != "" && settings.Enabled
}

func (s *SettingsService) IsKRAConfigured(tenantID string) bool {
	settings, err := s.GetKRASettings(tenantID)
	if err != nil {
		return false
	}
	return settings.VendorID != "" && settings.Enabled
}

func GenerateTenantID() string {
	return uuid.New().String()
}

func (s *SettingsService) CreateTenant(name, subdomain string) (*models.Tenant, error) {
	tenant := &models.Tenant{
		ID:        GenerateTenantID(),
		Name:      name,
		Subdomain: subdomain,
		Plan:      "free",
		IsActive:  true,
	}

	if err := s.db.Create(tenant).Error; err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}
	return tenant, nil
}

func (s *SettingsService) GetTenantBySubdomain(subdomain string) (*models.Tenant, error) {
	var tenant models.Tenant
	err := s.db.Where("subdomain = ? AND is_active = ?", subdomain, true).First(&tenant).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("tenant not found")
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &tenant, nil
}

func (s *SettingsService) GetNotificationSettings(tenantID string) (*NotificationSettings, error) {
	settings, err := s.GetSettings(tenantID)
	if err != nil {
		return nil, err
	}
	if settings.Notifications == nil {
		return &NotificationSettings{}, nil
	}
	return settings.Notifications, nil
}

func (s *SettingsService) SaveNotificationSettings(tenantID string, notif *NotificationSettings) error {
	settings, err := s.GetSettings(tenantID)
	if err != nil {
		settings = &TenantSettings{}
	}
	settings.Notifications = notif
	return s.SaveSettings(tenantID, settings)
}
