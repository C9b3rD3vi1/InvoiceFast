package handlers

import (
	"context"

	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	authService    *services.AuthService
	auditService   *services.AuditService
	invoiceService *services.InvoiceService
	clientService  *services.ClientService
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(authSvc *services.AuthService, auditSvc *services.AuditService) *AuthHandler {
	return &AuthHandler{
		authService:  authSvc,
		auditService: auditSvc,
	}
}

// NewAuthHandlerWithDeps creates a new AuthHandler with dependencies
func NewAuthHandlerWithDeps(authSvc *services.AuthService, auditSvc *services.AuditService, invSvc *services.InvoiceService, clientSvc *services.ClientService) *AuthHandler {
	return &AuthHandler{
		authService:    authSvc,
		auditService:   auditSvc,
		invoiceService: invSvc,
		clientService:  clientSvc,
	}
}

// Login - authenticate user
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	ip := c.IP()
	resp, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		if h.auditService != nil {
			_ = h.auditService.LogLoginAttempt(context.Background(), "", req.Email, ip, false, err.Error())
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	if h.auditService != nil {
		_ = h.auditService.LogLoginAttempt(context.Background(), resp.User.TenantID, req.Email, ip, true, "")
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

// Register - create new user
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req services.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	resp, err := h.authService.Register(&req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
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

// RefreshToken - refresh access token
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	resp, err := h.authService.RefreshToken(tenantID, req.RefreshToken)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"access_token":  resp.AccessToken,
		"refresh_token": resp.RefreshToken,
	})
}

// GetMe - get current user
func (h *AuthHandler) GetMe(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	user, err := h.authService.GetUserByID(tenantID, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	return c.JSON(user)
}

// UpdateUser - update user profile
func (h *AuthHandler) UpdateUser(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req services.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	user, err := h.authService.UpdateUser(tenantID, userID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(user)
}

// ChangePassword - change user password
func (h *AuthHandler) ChangePassword(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	err := h.authService.ChangePassword(tenantID, userID, req.OldPassword, req.NewPassword)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Password changed successfully"})
}

// Logout - invalidate refresh token
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if err := h.authService.Logout(req.RefreshToken); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Logged out successfully"})
}

// Search - global search across invoices and clients
func (h *AuthHandler) Search(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	query := c.Query("q")
	if query == "" {
		return c.JSON(fiber.Map{"invoices": []interface{}{}, "clients": []interface{}{}})
	}

	var invoiceResults []interface{}
	var clientResults []interface{}

	// Search invoices
	if h.invoiceService != nil {
		invoices, _, err := h.invoiceService.GetUserInvoices(tenantID, services.InvoiceFilter{
			Search: query,
			Limit:  5,
		})
		if err == nil {
			for _, inv := range invoices {
				invoiceResults = append(invoiceResults, fiber.Map{
					"id":          inv.ID,
					"number":      inv.InvoiceNumber,
					"client_name": inv.ClientID,
					"amount":      inv.Total,
					"status":      inv.Status,
				})
			}
		}
	}

	// Search clients
	if h.clientService != nil {
		clients, _, err := h.clientService.GetUserClients(tenantID, services.ClientFilter{
			Search: query,
			Limit:  5,
		})
		if err == nil {
			for _, cl := range clients {
				clientResults = append(clientResults, fiber.Map{
					"id":    cl.ID,
					"name":  cl.Name,
					"email": cl.Email,
				})
			}
		}
	}

	return c.JSON(fiber.Map{
		"invoices": invoiceResults,
		"clients":  clientResults,
	})
}

func (h *AuthHandler) SetupTwoFactor(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	setup, err := h.authService.SetupTwoFactor(tenantID, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"secret":          setup.Secret,
		"qr_code_url":     setup.QRCodeURL,
		"qr_code_image":   setup.QRCodeImageURL,
		"backup_codes":    setup.BackupCodes,
	})
}

func (h *AuthHandler) VerifyTwoFactor(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if err := h.authService.VerifyAndEnableTwoFactor(tenantID, userID, req.Code); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Two-factor authentication enabled"})
}

func (h *AuthHandler) DisableTwoFactor(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if err := h.authService.DisableTwoFactor(tenantID, userID, req.Password, req.Code); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Two-factor authentication disabled"})
}

func (h *AuthHandler) GetSessions(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	sessions, err := h.authService.GetSessions(tenantID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	result := make([]map[string]interface{}, len(sessions))
	for i, s := range sessions {
		result[i] = map[string]interface{}{
			"id":           s.ID,
			"device_info":  s.DeviceInfo,
			"ip_address":   s.IPAddress,
			"location":     s.Location,
			"is_current":   s.IsCurrent,
			"last_active": s.LastActiveAt,
		}
	}

	return c.JSON(result)
}

func (h *AuthHandler) RevokeSession(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	sessionID := c.Params("id")
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	if err := h.authService.RevokeSession(tenantID, userID, sessionID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Session revoked"})
}

func (h *AuthHandler) RevokeAllSessions(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	currentSessionID := c.Query("except")
	if err := h.authService.RevokeAllSessions(tenantID, userID, currentSessionID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "All other sessions revoked"})
}

func (h *AuthHandler) GetLoginHistory(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	limit := c.QueryInt("limit", 20)
	history, err := h.authService.GetLoginHistory(tenantID, userID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(history)
}

func (h *AuthHandler) UpdateLoginAlerts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if err := h.authService.UpdateLoginAlerts(tenantID, userID, req.Enabled); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Login alerts updated"})
}

func (h *AuthHandler) GetSecurityStatus(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == "" || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	user, err := h.authService.GetUserByID(tenantID, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	sessions, _ := h.authService.GetSessions(tenantID, userID)
	loginHistory, _ := h.authService.GetLoginHistory(tenantID, userID, 10)

	return c.JSON(fiber.Map{
		"two_factor_enabled":  user.TwoFactorEnabled,
		"password_changed_at": user.PasswordChangedAt,
		"last_login_at":      user.LastLoginAt,
		"login_alert_enabled": user.LoginAlertEnabled,
		"active_sessions":   len(sessions),
		"recent_logins":     loginHistory,
	})
}
