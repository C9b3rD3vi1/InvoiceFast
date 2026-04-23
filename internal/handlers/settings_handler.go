package handlers

import (
	"fmt"
	"strconv"
	"strings"

	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// SettingsHandler handles settings API endpoints
type SettingsHandler struct {
	settingsService *services.SettingsService
}

// NewSettingsHandler creates SettingsHandler
func NewSettingsHandler(settingsSvc *services.SettingsService) *SettingsHandler {
	return &SettingsHandler{
		settingsService: settingsSvc,
	}
}

// GetSettings returns tenant settings
func (h *SettingsHandler) GetSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	settings, err := h.settingsService.GetSettings(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.settingsService.MaskSecrets(settings)

	return c.JSON(settings)
}

// SaveSettings saves tenant settings
func (h *SettingsHandler) SaveSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var reqBody map[string]interface{}
	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	// Build settings from flexible input (frontend sends nested like {invoice: {...}}, {business: {...}})
	settings := &services.TenantSettings{}

	// Handle business/branding
	if business, ok := reqBody["business"].(map[string]interface{}); ok {
		settings.Branding = &services.BrandingSettings{}
		if name, ok := business["name"].(string); ok {
			settings.Branding.CompanyName = name
		}
		if logo, ok := business["logoUrl"].(string); ok {
			settings.Branding.LogoURL = logo
		}
		if color, ok := business["brandColor"].(string); ok {
			settings.Branding.BrandColor = color
		}
	}

	// Handle branding directly
	if branding, ok := reqBody["branding"].(map[string]interface{}); ok {
		if settings.Branding == nil {
			settings.Branding = &services.BrandingSettings{}
		}
		if name, ok := branding["company_name"].(string); ok {
			settings.Branding.CompanyName = name
		}
		if logo, ok := branding["logo_url"].(string); ok {
			settings.Branding.LogoURL = logo
		}
		if color, ok := branding["brand_color"].(string); ok {
			settings.Branding.BrandColor = color
		}
	}

	// Validate branding
	if settings.Branding != nil {
		if strings.TrimSpace(settings.Branding.CompanyName) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Company name is required"})
		}
		if len([]rune(settings.Branding.CompanyName)) > 100 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Company name must be less than 100 characters"})
		}
		if settings.Branding.BrandColor != "" && !isValidHexColor(settings.Branding.BrandColor) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid brand color format"})
		}
	}

	// Handle invoice settings
	if invoice, ok := reqBody["invoice"].(map[string]interface{}); ok {
		settings.Invoice = &services.InvoiceSettings{}
		if prefix, ok := invoice["prefix"].(string); ok {
			settings.Invoice.Prefix = prefix
		}
		if nextNum, ok := invoice["nextNumber"].(float64); ok {
			settings.Invoice.NextNumber = int(nextNum)
		}
		if currency, ok := invoice["currency"].(string); ok {
			settings.Invoice.Currency = currency
		}
		if taxRate, ok := invoice["defaultTaxRate"].(string); ok {
			// Handle string
			if f, err := strconv.ParseFloat(taxRate, 64); err == nil {
				settings.Invoice.DefaultTaxRate = int(f)
			}
		} else if taxRate, ok := invoice["defaultTaxRate"].(float64); ok {
			settings.Invoice.DefaultTaxRate = int(taxRate)
		}
		if terms, ok := invoice["paymentTerms"].(string); ok {
			settings.Invoice.PaymentTerms = terms
		}
		if partial, ok := invoice["allowPartialPayments"].(bool); ok {
			settings.Invoice.AllowPartialPayments = partial
		}
		if discount, ok := invoice["allowDiscounts"].(bool); ok {
			settings.Invoice.AllowDiscounts = discount
		}
		if deposit, ok := invoice["allowDeposits"].(bool); ok {
			settings.Invoice.AllowDeposits = deposit
		}
		if auto, ok := invoice["autoNumber"].(bool); ok {
			settings.Invoice.AutoNumber = auto
		}
	}

	// Handle payments settings
	if payments, ok := reqBody["payments"].(map[string]interface{}); ok {
		settings.Payments = &services.PaymentSettings{
			Mpesa: &services.MpesaSettings{},
			Card:  services.CardSettings{Enabled: false},
		}
		if mpesa, ok := payments["mpesa"].(map[string]interface{}); ok {
			settings.Payments.Mpesa = &services.MpesaSettings{}
			if key, ok := mpesa["consumerKey"].(string); ok {
				settings.Payments.Mpesa.ConsumerKey = key
			}
			if secret, ok := mpesa["consumerSecret"].(string); ok {
				settings.Payments.Mpesa.ConsumerSecret = secret
			}
			if code, ok := mpesa["shortcode"].(string); ok {
				settings.Payments.Mpesa.Shortcode = code
			}
			if bsCode, ok := mpesa["businessShortcode"].(string); ok {
				settings.Payments.Mpesa.Shortcode = bsCode
			}
			if enabled, ok := mpesa["enabled"].(bool); ok {
				settings.Payments.Mpesa.Enabled = enabled
			}
		}
		if card, ok := payments["card"].(map[string]interface{}); ok {
			if enabled, ok := card["enabled"].(bool); ok {
				settings.Payments.Card.Enabled = enabled
			}
		}
	}

	// Handle notifications - simple boolean format from frontend
	if notifications, ok := reqBody["notifications"].(map[string]interface{}); ok {
		settings.Notifications = &services.NotificationSettings{}
		
		// Handle simple boolean format { email: true, sms: false, whatsapp: false }
		if emailEnabled, ok := notifications["email"].(bool); ok {
			settings.Notifications.Email = []services.NotificationEvent{
				{Key: "invoice_created", Label: "Invoice Created", Enabled: emailEnabled},
				{Key: "payment_received", Label: "Payment Received", Enabled: emailEnabled},
				{Key: "payment_due", Label: "Payment Due Reminder", Enabled: emailEnabled},
				{Key: "invoice_overdue", Label: "Invoice Overdue", Enabled: emailEnabled},
			}
		}
		if smsEnabled, ok := notifications["sms"].(bool); ok {
			settings.Notifications.SMS = []services.NotificationEvent{
				{Key: "invoice_created", Label: "Invoice Created", Enabled: smsEnabled},
				{Key: "payment_received", Label: "Payment Received", Enabled: smsEnabled},
				{Key: "payment_due", Label: "Payment Due Reminder", Enabled: smsEnabled},
			}
		}
		if whatsappEnabled, ok := notifications["whatsapp"].(bool); ok {
			settings.Notifications.Slack = []services.NotificationEvent{
				{Key: "invoice_created", Label: "Invoice Created", Enabled: whatsappEnabled},
				{Key: "payment_received", Label: "Payment Received", Enabled: whatsappEnabled},
			}
		}
	}

	if err := h.settingsService.SaveSettings(tenantID, settings); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "saved"})
}

func isValidHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	_, err := fmt.Sscanf(s[1:], "%x", new(int))
	return err == nil
}

// SaveSettingsMpesa saves M-Pesa settings
func (h *SettingsHandler) SaveSettingsMpesa(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req struct {
		ConsumerKey    string `json:"consumer_key"`
		ConsumerSecret string `json:"consumer_secret"`
		Shortcode      string `json:"shortcode"`
		Passkey        string `json:"passkey"`
		Enabled        bool   `json:"enabled"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.ConsumerKey == "" || req.Shortcode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Consumer Key and Shortcode are required"})
	}

	settings := &services.MpesaSettings{
		ConsumerKey:    req.ConsumerKey,
		ConsumerSecret: req.ConsumerSecret,
		Shortcode:      req.Shortcode,
		Passkey:        req.Passkey,
		Enabled:        req.Enabled,
	}

	if err := h.settingsService.SaveMpesaSettings(tenantID, settings); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "saved"})
}

// GetMpesaSettings returns M-Pesa settings
func (h *SettingsHandler) GetMpesaSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	settings, err := h.settingsService.GetMpesaSettings(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.settingsService.MaskSecrets(&services.TenantSettings{Mpesa: settings})

	return c.JSON(settings)
}

// SaveSettingsKRA saves KRA settings
func (h *SettingsHandler) SaveSettingsKRA(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req struct {
		VendorID string `json:"vendor_id"`
		APIKey   string `json:"api_key"`
		LiveMode bool   `json:"live_mode"`
		Enabled  bool   `json:"enabled"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.VendorID == "" || req.APIKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Vendor ID and API Key are required"})
	}

	settings := &services.KRASettings{
		VendorID: req.VendorID,
		APIKey:   req.APIKey,
		LiveMode: req.LiveMode,
		Enabled:  req.Enabled,
	}

	if err := h.settingsService.SaveKRASettings(tenantID, settings); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "saved"})
}

// GetKRASettings returns KRA settings
func (h *SettingsHandler) GetKRASettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	settings, err := h.settingsService.GetKRASettings(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(settings)
}

// SaveBranding saves branding settings
func (h *SettingsHandler) SaveBranding(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req struct {
		CompanyName string `json:"company_name"`
		LogoURL     string `json:"logo_url"`
		BrandColor  string `json:"brand_color"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	settings := &services.BrandingSettings{
		CompanyName: req.CompanyName,
		LogoURL:     req.LogoURL,
		BrandColor:  req.BrandColor,
	}

	if err := h.settingsService.SaveBrandingSettings(tenantID, settings); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "saved"})
}

// GetNotificationSettings returns notification settings
func (h *SettingsHandler) GetNotificationSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	settings, err := h.settingsService.GetNotificationSettings(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(settings)
}

// SaveNotificationSettings saves notification settings
func (h *SettingsHandler) SaveNotificationSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req services.NotificationSettings
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if err := h.settingsService.SaveNotificationSettings(tenantID, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "saved"})
}
