package handlers

import (
	"errors"
	"strings"

	"invoicefast/internal/database"
	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type OnboardingHandler struct {
	authService     *services.AuthService
	invoiceService  *services.InvoiceService
	clientService   *services.ClientService
	settingsService *services.SettingsService
	layoutService   *services.LayoutService
	db              *database.DB
}

func NewOnboardingHandler(authSvc *services.AuthService, invSvc *services.InvoiceService, clientSvc *services.ClientService, settingsSvc *services.SettingsService, db *database.DB) *OnboardingHandler {
	return &OnboardingHandler{
		authService:     authSvc,
		invoiceService:  invSvc,
		clientService:   clientSvc,
		settingsService: settingsSvc,
		layoutService:   services.NewLayoutService(),
		db:              db,
	}
}

func (h *OnboardingHandler) ServeOnboardingPage(c *fiber.Ctx) error {
	return h.layoutService.RenderPublicWithShell(c, "./views/onboarding.html", "Get Started")
}

type VerifyEmailRequest struct {
	Code string `json:"code"`
}

func (h *OnboardingHandler) HandleVerifyEmail(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID := middleware.GetTenantID(c)

	var req VerifyEmailRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if err := h.authService.VerifyEmailCode(userID, req.Code); err != nil {
		if errors.Is(err, services.ErrVerificationInvalid) || errors.Is(err, services.ErrVerificationExpired) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "verification failed"})
	}

	// Mark email_verified in onboarding progress
	if tenantID != "" {
		if err := h.markOnboardingStep(tenantID, func(p *services.OnboardingProgress) {
			p.EmailVerified = true
		}); err != nil {
			// non-fatal
		}
	}

	return c.JSON(fiber.Map{"status": "verified"})
}

func (h *OnboardingHandler) HandleResendCode(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if err := h.authService.SendVerificationCode(userID, req.Email); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "sent"})
}

type BusinessProfileRequest struct {
	BusinessName string `json:"business_name"`
	BusinessType string `json:"business_type"`
}

func (h *OnboardingHandler) HandleBusinessProfile(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req BusinessProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	name := strings.TrimSpace(req.BusinessName)
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "business name is required"})
	}

	_, err := h.authService.UpdateUser(tenantID, userID, &services.UpdateUserRequest{
		Name:        &name,
		CompanyName: &name,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save profile"})
	}

	var tenant models.Tenant
	if err := h.db.First(&tenant, "id = ?", tenantID).Error; err == nil {
		updates := map[string]interface{}{}
		if tenant.Name == "" {
			updates["name"] = name
		}
		if len(updates) > 0 {
			h.db.Model(&tenant).Updates(updates)
		}
	}

	if err := h.saveBusinessType(tenantID, req.BusinessType); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save business type"})
	}

	// Mark business_profile in onboarding progress
	if err := h.markOnboardingStep(tenantID, func(p *services.OnboardingProgress) {
		p.BusinessProfile = true
	}); err != nil {
		// non-fatal
	}

	return c.JSON(fiber.Map{"status": "saved"})
}

func (h *OnboardingHandler) saveBusinessType(tenantID, bizType string) error {
	settings, err := h.settingsService.GetSettings(tenantID)
	if err != nil {
		settings = &services.TenantSettings{}
	}
	if settings.Business == nil {
		settings.Business = &services.BusinessSettings{}
	}
	settings.Business.Industry = bizType
	return h.settingsService.SaveSettings(tenantID, settings)
}

type CreateInvoiceRequest struct {
	ClientName  string  `json:"client_name"`
	ClientEmail string  `json:"client_email"`
	Description string  `json:"description"`
	ItemDesc    string  `json:"item_desc"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
}

func (h *OnboardingHandler) HandleCreateInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req CreateInvoiceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if strings.TrimSpace(req.ClientName) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "client name is required"})
	}
	if req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "amount must be greater than 0"})
	}

	currency := strings.ToUpper(req.Currency)
	if currency == "" {
		currency = "KES"
	}

	clientRec, err := h.clientService.CreateClient(tenantID, userID, &services.CreateClientRequest{
		Name:  strings.TrimSpace(req.ClientName),
		Email: strings.TrimSpace(req.ClientEmail),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create client"})
	}

	itemDesc := strings.TrimSpace(req.ItemDesc)
	if itemDesc == "" {
		itemDesc = "Services"
	}

	invoice, err := h.invoiceService.CreateInvoice(tenantID, userID, clientRec.ID, &services.CreateInvoiceRequest{
		ClientID: clientRec.ID,
		Currency: currency,
		Items: []services.InvoiceItemRequest{
			{
				Description:  itemDesc,
				Quantity:     1,
				UnitPrice:    req.Amount,
				Unit:         "unit",
			},
		},
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create invoice"})
	}

	// Mark first_invoice in onboarding progress
	if err := h.markOnboardingStep(tenantID, func(p *services.OnboardingProgress) {
		p.FirstInvoice = true
	}); err != nil {
		// non-fatal
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"invoice_id":          invoice.ID,
		"invoice_number":      invoice.InvoiceNumber,
		"client_id":           clientRec.ID,
		"client_name":         clientRec.Name,
		"amount":              req.Amount,
		"currency":            currency,
	})
}

type SavePaymentRequest struct {
	PaybillNumber string `json:"paybill_number"`
	AccountNumber string `json:"account_number"`
}

func (h *OnboardingHandler) HandleSavePayment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req SavePaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	paybill := strings.TrimSpace(req.PaybillNumber)
	if paybill == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "paybill number is required"})
	}

	mpesa := &services.MpesaSettings{
		Shortcode: paybill,
		Enabled:   true,
	}
	if err := h.settingsService.SaveMpesaSettings(tenantID, mpesa); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save payment settings"})
	}

	// Mark mpesa_setup in onboarding progress
	if err := h.markOnboardingStep(tenantID, func(p *services.OnboardingProgress) {
		p.MpesaSetup = true
	}); err != nil {
		// non-fatal
	}

	return c.JSON(fiber.Map{"status": "saved"})
}

func (h *OnboardingHandler) markOnboardingStep(tenantID string, fn func(p *services.OnboardingProgress)) error {
	settings, err := h.settingsService.GetSettings(tenantID)
	if err != nil {
		settings = &services.TenantSettings{}
	}
	if settings.Onboarding == nil {
		settings.Onboarding = &services.OnboardingProgress{}
	}
	fn(settings.Onboarding)
	return h.settingsService.SaveSettings(tenantID, settings)
}

func (h *OnboardingHandler) HandleOnboardingProgress(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	// Check email verification from DB
	var count int64
	h.db.Model(&models.EmailVerificationToken{}).
		Where("user_id = ? AND used_at IS NOT NULL", userID).
		Count(&count)
	emailVerified := count > 0

	// Get onboarding progress from settings
	settings, err := h.settingsService.GetSettings(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load settings"})
	}

	var progress services.OnboardingProgress
	if settings.Onboarding != nil {
		progress = *settings.Onboarding
	}

	// Override email_verified from the authoritative DB check
	progress.EmailVerified = emailVerified

	return c.JSON(progress)
}

func (h *OnboardingHandler) HandleDismissOnboarding(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	if err := h.markOnboardingStep(tenantID, func(p *services.OnboardingProgress) {
		p.Dismissed = true
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to dismiss"})
	}

	return c.JSON(fiber.Map{"status": "dismissed"})
}

func (h *OnboardingHandler) HandleCheckEmailVerified(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	var count int64
	h.db.Model(&models.EmailVerificationToken{}).
		Where("user_id = ? AND used_at IS NOT NULL", userID).
		Count(&count)

	return c.JSON(fiber.Map{
		"email_verified": count > 0,
		"email":          user.Email,
		"name":           user.Name,
		"company_name":   user.CompanyName,
	})
}

func (h *OnboardingHandler) HandleOnboardingRegister(c *fiber.Ctx) error {
	var req services.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	resp, err := h.authService.Register(&req)
	if err != nil {
		if errors.Is(err, services.ErrEmailExists) || errors.Is(err, services.ErrInvalidEmail) || errors.Is(err, services.ErrWeakPassword) || errors.Is(err, services.ErrPasswordCompromised) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "registration failed"})
	}

	if err := h.authService.SendVerificationCode(resp.User.ID, resp.User.Email); err != nil {
		// Non-fatal — user can request resend
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"access_token":  resp.AccessToken,
		"refresh_token": resp.RefreshToken,
		"user": fiber.Map{
			"id":    resp.User.ID,
			"email": resp.User.Email,
			"name":  resp.User.Name,
		},
	})
}

func (h *OnboardingHandler) HandleOnboardingLogin(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	resp, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"access_token":  resp.AccessToken,
		"refresh_token": resp.RefreshToken,
		"user": fiber.Map{
			"id":      resp.User.ID,
			"name":    resp.User.Name,
			"email":   resp.User.Email,
			"company": resp.User.CompanyName,
		},
	})
}
