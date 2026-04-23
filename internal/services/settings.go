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
	Business      *BusinessSettings     `json:"business,omitempty"`
	Profile       *ProfileSettings     `json:"profile,omitempty"`
	Invoice       *InvoiceSettings     `json:"invoice,omitempty"`
	Payments      *PaymentSettings     `json:"payments,omitempty"`
	Mpesa         *MpesaSettings      `json:"mpesa,omitempty"`
	KRA           *KRASettings         `json:"kra,omitempty"`
	Branding      *BrandingSettings    `json:"branding,omitempty"`
	Notifications *NotificationSettings `json:"notifications,omitempty"`
	Integrations  interface{}          `json:"integrations,omitempty"`
	Updated       time.Time            `json:"updated_at"`
}

type BusinessSettings struct {
	Name               string `json:"name"`
	Email              string `json:"email"`
	Phone              string `json:"phone"`
	Website            string `json:"website"`
	KRAPIN             string `json:"kra_pin"`
	RegistrationNumber string `json:"registrationNumber"`
	Industry           string `json:"industry"`
	Address            string `json:"address"`
	Country            string `json:"country"`
	Timezone           string `json:"timezone"`
	LogoURL            string `json:"logoUrl"`
	BrandColor         string `json:"brandColor"`
}

type ProfileSettings struct {
	Name             string `json:"name"`
	Email           string `json:"email"`
	JobTitle        string `json:"jobTitle"`
	Phone            string `json:"phone"`
	TwoFactorEnabled bool   `json:"twoFactorEnabled"`
}

type InvoiceSettings struct {
	Prefix            string `json:"prefix"`
	NextNumber        int    `json:"nextNumber"`
	Currency          string `json:"currency"`
	DefaultTaxRate    int    `json:"defaultTaxRate"`
	PaymentTerms      string `json:"paymentTerms"`
	AllowPartialPayments bool `json:"allowPartialPayments"`
	AllowDiscounts   bool   `json:"allowDiscounts"`
	AllowDeposits    bool   `json:"allowDeposits"`
	AutoNumber       bool   `json:"autoNumber"`
}

type CardSettings struct {
	Enabled bool `json:"enabled"`
}

type PaymentSettings struct {
	Mpesa *MpesaSettings `json:"mpesa"`
	Card  CardSettings   `json:"card"`
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

	settings := &TenantSettings{
		Updated:   tenant.UpdatedAt,
		Business:  &BusinessSettings{},
		Profile:   &ProfileSettings{},
		Invoice:   &InvoiceSettings{
			Prefix:            "INV",
			NextNumber:        1,
			Currency:          "KES",
			DefaultTaxRate:    16,
			PaymentTerms:      "30",
			AllowPartialPayments: true,
			AllowDiscounts:   true,
			AllowDeposits:    false,
			AutoNumber:       true,
		},
		Payments: &PaymentSettings{
			Mpesa: &MpesaSettings{},
			Card:  CardSettings{Enabled: false},
		},
	}

	// Parse tenant settings
	if tenant.Settings != "" {
		if err := json.Unmarshal([]byte(tenant.Settings), settings); err != nil {
			return nil, fmt.Errorf("failed to parse settings: %w", err)
		}
	}

	// Populate business from tenant columns FIRST (primary source)
	if settings.Business == nil {
		settings.Business = &BusinessSettings{}
	}
	// Tenant table is the primary source for business info
	if settings.Business.Name == "" {
		settings.Business.Name = tenant.Name
	}
	if settings.Business.Email == "" {
		settings.Business.Email = tenant.Email
	}
	if settings.Business.Phone == "" {
		settings.Business.Phone = tenant.Phone
	}
	if settings.Business.Website == "" {
		settings.Business.Website = tenant.Website
	}
	if settings.Business.Country == "" {
		settings.Business.Country = tenant.Country
	}
	if settings.Business.Timezone == "" {
		settings.Business.Timezone = tenant.Timezone
	}

	// Merge branding into business
	if settings.Branding != nil {
		if settings.Branding.LogoURL != "" {
			settings.Business.LogoURL = settings.Branding.LogoURL
		}
		if settings.Branding.BrandColor != "" {
			settings.Business.BrandColor = settings.Branding.BrandColor
		}
	}

	// Get user profile
	var user models.User
	err = s.db.Where("tenant_id = ?", tenantID).First(&user).Error
	if err == nil {
		settings.Profile.Name = user.Name
		settings.Profile.Email = user.Email
		settings.Profile.Phone = user.Phone
		settings.Profile.TwoFactorEnabled = user.TwoFactorEnabled
	}

	// Merge invoice defaults (never overwrite with empty values)
	if settings.Invoice == nil {
		settings.Invoice = &InvoiceSettings{}
	}
	if settings.Invoice.Prefix == "" {
		settings.Invoice.Prefix = "INV"
	}
	if settings.Invoice.NextNumber == 0 {
		settings.Invoice.NextNumber = 1
	}
	if settings.Invoice.Currency == "" {
		settings.Invoice.Currency = "KES"
	}
	if settings.Invoice.DefaultTaxRate == 0 {
		settings.Invoice.DefaultTaxRate = 16
	}
	if settings.Invoice.PaymentTerms == "" {
		settings.Invoice.PaymentTerms = "30"
	}

	// Merge payments
	if settings.Payments == nil {
		settings.Payments = &PaymentSettings{
			Mpesa: &MpesaSettings{},
			Card:  CardSettings{Enabled: false},
		}
	}
	if settings.Mpesa != nil {
		settings.Payments.Mpesa = settings.Mpesa
	}

	// Mask secrets for UI display
	s.MaskSecrets(settings)

	// Get integrations count for overview
	var integrationCount int64
	s.db.Model(&models.Integration{}).Where("tenant_id = ? AND is_active = ?", tenantID, true).Count(&integrationCount)
	if integrationCount > 0 {
		settings.Integrations = []interface{}{}
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

	// Get current settings and merge
	var tenant models.Tenant
	if err := s.db.First(&tenant, "id = ?", tenantID).Error; err != nil {
		return fmt.Errorf("tenant not found")
	}

	// Parse existing settings
	var existing TenantSettings
	if tenant.Settings != "" {
		if err := json.Unmarshal([]byte(tenant.Settings), &existing); err == nil {
			// Merge: only overwrite if new value is non-empty
			if settings.Business != nil && existing.Business != nil {
				settings.Business = mergeBusiness(settings.Business, existing.Business)
			}
			if settings.Invoice == nil {
				settings.Invoice = existing.Invoice
			} else if existing.Invoice != nil {
				if settings.Invoice.Prefix == "" {
					settings.Invoice.Prefix = existing.Invoice.Prefix
				}
				if settings.Invoice.NextNumber == 0 {
					settings.Invoice.NextNumber = existing.Invoice.NextNumber
				}
				if settings.Invoice.Currency == "" {
					settings.Invoice.Currency = existing.Invoice.Currency
				}
			}
			if settings.Payments == nil {
				settings.Payments = existing.Payments
			}
			if settings.Mpesa == nil {
				settings.Mpesa = existing.Mpesa
			}
			if settings.KRA == nil {
				settings.KRA = existing.KRA
			}
			if settings.Notifications == nil {
				settings.Notifications = existing.Notifications
			}
		}
	}

	settingsJSON, err = json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	// Update tenant record with both settings JSON and business fields
	updates := map[string]interface{}{
		"settings": string(settingsJSON),
	}
	if settings.Business != nil {
		if settings.Business.Name != "" {
			updates["name"] = settings.Business.Name
		}
		if settings.Business.Email != "" {
			updates["email"] = settings.Business.Email
		}
		if settings.Business.Phone != "" {
			updates["phone"] = settings.Business.Phone
		}
		if settings.Business.Website != "" {
			updates["website"] = settings.Business.Website
		}
		if settings.Business.Country != "" {
			updates["country"] = settings.Business.Country
		}
		if settings.Business.Timezone != "" {
			updates["timezone"] = settings.Business.Timezone
		}
	}

	result := s.db.Model(&models.Tenant{}).
		Where("id = ?", tenantID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to save settings: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

func mergeBusiness(incoming, existing *BusinessSettings) *BusinessSettings {
	if incoming == nil {
		return existing
	}
	if existing == nil {
		return incoming
	}
	if incoming.Name == "" {
		incoming.Name = existing.Name
	}
	if incoming.Email == "" {
		incoming.Email = existing.Email
	}
	if incoming.Phone == "" {
		incoming.Phone = existing.Phone
	}
	if incoming.Website == "" {
		incoming.Website = existing.Website
	}
	if incoming.KRAPIN == "" {
		incoming.KRAPIN = existing.KRAPIN
	}
	if incoming.RegistrationNumber == "" {
		incoming.RegistrationNumber = existing.RegistrationNumber
	}
	if incoming.Industry == "" {
		incoming.Industry = existing.Industry
	}
	if incoming.Address == "" {
		incoming.Address = existing.Address
	}
	if incoming.LogoURL == "" {
		incoming.LogoURL = existing.LogoURL
	}
	if incoming.BrandColor == "" {
		incoming.BrandColor = existing.BrandColor
	}
	return incoming
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
