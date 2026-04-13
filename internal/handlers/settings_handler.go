package handlers

import (
	"fmt"
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

	var req services.TenantSettings
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Branding != nil {
		if strings.TrimSpace(req.Branding.CompanyName) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Company name is required"})
		}
		if len([]rune(req.Branding.CompanyName)) > 100 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Company name must be less than 100 characters"})
		}
		if req.Branding.BrandColor != "" && !isValidHexColor(req.Branding.BrandColor) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid brand color format"})
		}
	}

	if err := h.settingsService.SaveSettings(tenantID, &req); err != nil {
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
