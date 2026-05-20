package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"invoicefast/internal/logger"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"
	"invoicefast/internal/utils"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrEmailExists          = errors.New("email already registered")
	ErrInvalidEmail         = errors.New("invalid email format")
	ErrWeakPassword         = errors.New("password must be at least 8 characters with uppercase, lowercase, number and special character")
	ErrWrongPassword        = errors.New("incorrect password")
	ErrInvalidToken         = errors.New("invalid or expired token")
	ErrPasswordCompromised  = errors.New("this password has been found in a data breach. please choose a different password")
	ErrVerificationInvalid  = errors.New("invalid verification code")
	ErrVerificationExpired  = errors.New("verification code has expired")
	ErrEmailAlreadyVerified = errors.New("email already verified")
)

var commonPasswords = map[string]bool{
	"password": true, "12345678": true, "123456789": true, "password123": true,
	"123456": true, "qwerty": true, "12345": true, "1234567890": true,
	"password1": true, "1234567": true, "iloveyou": true, "adobe123": true,
	"123123": true, "sunshine": true, "12345678901": true, "princess": true,
	"admin": true, "welcome": true, "123654": true, "666666": true,
}

func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(password) > 128 {
		return errors.New("password must be at most 128 characters")
	}
	hasUpper, hasLower, hasNumber, hasSpecial := false, false, false, false
	specialChars := "!@#$%^&*()_+-=[]{}|;:,.<>?"
	for _, c := range password {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if c >= 'a' && c <= 'z' {
			hasLower = true
		}
		if c >= '0' && c <= '9' {
			hasNumber = true
		}
		if strings.Contains(specialChars, string(c)) {
			hasSpecial = true
		}
	}
	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if !hasNumber {
		return errors.New("password must contain at least one number")
	}
	if !hasSpecial {
		return errors.New("password must contain at least one special character")
	}
	if commonPasswords[strings.ToLower(password)] {
		return ErrPasswordCompromised
	}
	return nil
}

func (s *AuthService) InitiatePasswordReset(tenantID, email, ipAddress, userAgent string) (*models.PasswordResetToken, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	email = strings.ToLower(strings.TrimSpace(email))

	var user models.User
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// SECURITY: Return nil, nil to prevent email enumeration
			logger.Get().Info(context.Background(), "Password reset attempted for non-existent email", "email", email)
			return nil, nil
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	var recentCount int64
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	if err := s.db.Model(&models.PasswordResetToken{}).
		Where("tenant_id = ? AND user_id = ? AND created_at > ?", tenantID, user.ID, oneHourAgo).
		Count(&recentCount).Error; err != nil {
		return nil, fmt.Errorf("rate limit check failed: %w", err)
	}

	if recentCount >= 3 {
		logger.Get().Warn(context.Background(), "Rate limited password reset for user", "user_id", user.ID)
		return nil, errors.New("rate limit exceeded: too many reset requests")
	}

	if err := s.db.Where("tenant_id = ? AND user_id = ? AND expires_at > ?", tenantID, user.ID, time.Now()).
		Delete(&models.PasswordResetToken{}).Error; err != nil {
		return nil, fmt.Errorf("failed to invalidate tokens: %w", err)
	}

	rawToken := secureRandomToken()
	tokenHash := hashToken(rawToken, s.cfg.JWT.Secret)

	resetToken := &models.PasswordResetToken{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    user.ID,
		Token:     tokenHash,
		RawToken:  rawToken, // Will be cleared before returning
		Email:     email,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IPAddress: ipAddress,
		UserAgent: userAgent,
		CreatedAt: time.Now(),
	}

	if err := s.db.Create(resetToken).Error; err != nil {
		return nil, fmt.Errorf("failed to create reset token: %w", err)
	}

	// SECURITY: Clear raw token before returning - never expose in API response
	resetToken.RawToken = ""

	logger.Get().Info(context.Background(), "Password reset initiated", "user_id", user.ID, "ip_address", ipAddress)

	return resetToken, nil
}

func (s *AuthService) CompletePasswordReset(tenantID, token, newPassword, confirmPassword, ipAddress string) error {
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}
	if newPassword != confirmPassword {
		return errors.New("passwords do not match")
	}
	if err := ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	tokenHash := hashToken(token, s.cfg.JWT.Secret)
	var resetToken models.PasswordResetToken

	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&resetToken, "token = ?", tokenHash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidToken
		}
		return fmt.Errorf("database error: %w", err)
	}

	if resetToken.IsExpired() {
		return errors.New("token expired")
	}
	if resetToken.UsedAt != nil {
		return errors.New("token already used")
	}

	var user models.User
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", resetToken.UserID).Error; err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("password_hash", string(passwordHash)).Error; err != nil {
			return err
		}
		now := time.Now()
		resetToken.UsedAt = &now
		if err := tx.Save(&resetToken).Error; err != nil {
			return err
		}
		auditLog := &models.AuditLog{
			ID:         uuid.New().String(),
			UserID:     user.ID,
			Action:     "password_reset.completed",
			EntityType: "user",
			EntityID:   user.ID,
			Details:    fmt.Sprintf(`{"ip_address": "%s"}`, ipAddress),
			IPAddress:  ipAddress,
			CreatedAt:  time.Now(),
		}
		return tx.Create(auditLog).Error
	})

	if err != nil {
		return fmt.Errorf("failed to complete reset: %w", err)
	}

	if err := s.db.Where("user_id = ?", user.ID).Delete(&models.RefreshToken{}).Error; err != nil {
		logger.Get().Warn(context.Background(), "Failed to invalidate refresh tokens", "error", err)
	}

	return nil
}

func (s *AuthService) ValidateResetToken(tenantID, token string) (*models.User, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	tokenHash := hashToken(token, s.cfg.JWT.Secret)

	var resetToken models.PasswordResetToken
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&resetToken, "token = ?", tokenHash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	if resetToken.IsExpired() {
		return nil, errors.New("token expired")
	}
	if resetToken.UsedAt != nil {
		return nil, errors.New("token already used")
	}

	var user models.User
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", resetToken.UserID).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &user, nil
}

func secureRandomToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}

func hashToken(token, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

type AuthService struct {
	db               *database.DB
	cfg              *config.Config
	emailService     *EmailService
	auditService     *AuditService
	exchangeRateSvc  *ExchangeRateService
}

type Claims struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         *models.User `json:"user"`
}

func NewAuthService(db *database.DB, cfg *config.Config, emailSvc *EmailService, auditSvc *AuditService, exchangeRateSvc *ExchangeRateService) *AuthService {
	return &AuthService{
		db:              db,
		cfg:             cfg,
		emailService:   emailSvc,
		auditService:    auditSvc,
		exchangeRateSvc: exchangeRateSvc,
	}
}

func (s *AuthService) Register(req *RegisterRequest) (*AuthResponse, error) {
	if err := validateEmail(req.Email); err != nil {
		return nil, ErrInvalidEmail
	}

	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	// Check if email exists with is_active = true
	lowerEmail := strings.ToLower(req.Email)
	var existing models.User
	err := s.db.Where("email = ? AND is_active = ?", lowerEmail, true).First(&existing).Error
	if err == nil {
		// Active user with this email exists - block registration
		return nil, ErrEmailExists
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	subdomain := generateSubdomain(req.CompanyName)
	if subdomain == "" {
		subdomain = uuid.New().String()[:8]
	}

	tenant := &models.Tenant{
		ID:        uuid.New().String(),
		Name:      req.CompanyName,
		Subdomain: subdomain,
		Plan:      "starter",
		IsActive:  true,
		Currency:  utils.DefaultCurrency,
	}
	if req.Currency != "" && utils.IsValidCurrency(req.Currency) {
		tenant.Currency = strings.ToUpper(req.Currency)
	}
	if err := s.db.Create(tenant).Error; err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	user := &models.User{
		ID:           uuid.New().String(),
		TenantID:     tenant.ID,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		Name:         req.Name,
		Phone:        normalizePhone(req.Phone),
		CompanyName:  req.CompanyName,
		KRAPIN:       strings.ToUpper(strings.TrimSpace(req.KRAPIN)),
		Plan:         "starter",
		IsActive:     true,
		Role:         "owner",
	}

	if err := s.db.Create(user).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	starterPlan := models.SubscriptionPlan{}
	if err := s.db.First(&starterPlan, "slug = ?", "starter").Error; err == nil && starterPlan.ID != "" {
		now := time.Now()
		trialEnd := now.AddDate(0, 0, 14)
		
		var amountKES int64
		if starterPlan.MonthlyPriceUSD > 0 && s.exchangeRateSvc != nil {
			converted, err := s.exchangeRateSvc.Convert(float64(starterPlan.MonthlyPriceUSD), "USD", "KES")
			if err == nil {
				amountKES = int64(converted)
			}
		} else if starterPlan.MonthlyPriceUSD > 0 {
			amountKES = int64(float64(starterPlan.MonthlyPriceUSD) * 150.0) // Fallback only if no exchange service
		}
		
		subscription := &models.Subscription{
			ID:                 uuid.New().String(),
			TenantID:           tenant.ID,
			PlanID:             starterPlan.ID,
			Status:             "trialing",
			BillingCycle:       "monthly",
			Amount:             amountKES,
			Currency:           "KES",
			TrialStart:         &now,
			TrialEnd:           &trialEnd,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   trialEnd,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := s.db.Create(subscription).Error; err != nil {
			logger.Get().Error(context.Background(), "Failed to create trial subscription", "error", err)
		} else {
			logger.Get().Info(context.Background(), "Trial subscription created", "tenant_id", tenant.ID, "plan", starterPlan.Name)
		}

		usage := &models.UsageTracking{
			ID:           uuid.New().String(),
			TenantID:     tenant.ID,
			InvoicesUsed: 0,
			ClientsUsed:  0,
			UsersUsed:    1,
			UpdatedAt:    time.Now(),
		}
		if err := s.db.Create(usage).Error; err != nil {
			logger.Get().Error(context.Background(), "Failed to create usage tracking", "error", err)
		}
	} else {
		logger.Get().Warn(context.Background(), "Starter plan not found, skipping trial creation", "error", err)
	}

	accessToken, err := s.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.GenerateRefreshTokenWithTenant(user.ID, user.TenantID)
	if err != nil {
		return nil, err
	}

	s.db.SeedDefaultTemplates(user.ID)

	if s.auditService != nil {
		_ = s.auditService.LogAction(context.Background(), user.TenantID, user.ID, AuditActionUserRegister, AuditEntityUser, user.ID, map[string]interface{}{
			"email":   user.Email,
			"company": user.CompanyName,
			"phone":   user.Phone,
		})
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}

func (s *AuthService) Login(email, password string) (*AuthResponse, error) {
	var user models.User
	if err := s.db.First(&user, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidEmail
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrWrongPassword
	}

	accessToken, err := s.generateAccessToken(&user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.GenerateRefreshTokenWithTenant(user.ID, user.TenantID)
	if err != nil {
		return nil, err
	}

	if s.auditService != nil {
		_ = s.auditService.LogLoginAttempt(context.Background(), user.TenantID, user.Email, "", true, "")
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         &user,
	}, nil
}

func (s *AuthService) RefreshToken(tenantID, refreshToken string) (*AuthResponse, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	var storedToken models.RefreshToken
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&storedToken, "token = ? AND expires_at > ?", refreshToken, time.Now()).Error; err != nil {
		return nil, ErrInvalidToken
	}

	var user models.User
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", storedToken.UserID).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	if err := s.db.Delete(&storedToken).Error; err != nil {
		return nil, fmt.Errorf("failed to delete refresh token: %w", err)
	}

	accessToken, err := s.generateAccessToken(&user)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := s.GenerateRefreshTokenWithTenant(user.ID, tenantID)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		User:         &user,
	}, nil
}

func (s *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	if strings.TrimSpace(tokenString) == "" {
		return nil, ErrInvalidToken
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.JWT.Secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

func (s *AuthService) GetUserByID(tenantID, userID string) (*models.User, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}

	var user models.User
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}
	return &user, nil
}

func (s *AuthService) UpdateUser(tenantID, userID string, req *UpdateUserRequest) (*models.User, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	user, err := s.GetUserByID(tenantID, userID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		user.Name = strings.TrimSpace(*req.Name)
	}
	if req.Phone != nil {
		user.Phone = normalizePhone(*req.Phone)
	}
	if req.CompanyName != nil {
		user.CompanyName = strings.TrimSpace(*req.CompanyName)
	}
	if req.KRAPIN != nil {
		user.KRAPIN = strings.ToUpper(strings.TrimSpace(*req.KRAPIN))
	}

	if err := s.db.Save(user).Error; err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return user, nil
}

func (s *AuthService) ChangePassword(tenantID, userID, oldPassword, newPassword string) error {
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	user, err := s.GetUserByID(tenantID, userID)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrWrongPassword
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now()
	user.PasswordHash = string(hashedPassword)
	user.PasswordChangedAt = &now
	if user.TwoFactorEnabled {
		user.TwoFactorVerifiedAt = nil
	}
	if err := s.db.Save(user).Error; err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	s.db.Scopes(database.TenantFilter(tenantID)).Where("user_id = ?", userID).Delete(&models.RefreshToken{})
	s.db.Scopes(database.TenantFilter(tenantID)).Where("user_id = ?", userID).Delete(&models.UserSession{})

	if s.auditService != nil {
		s.auditService.LogSecurityEvent(context.Background(), tenantID, userID, "password_changed", map[string]interface{}{})
	}

	return nil
}

func (s *AuthService) Logout(refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return nil
	}
	return s.db.Where("token = ?", refreshToken).Delete(&models.RefreshToken{}).Error
}

func (s *AuthService) GenerateAPIKey(userID, keyName string) (string, error) {
	if strings.TrimSpace(keyName) == "" {
		keyName = "Default"
	}

	bytes := make([]byte, 32)
	rand.Read(bytes)
	apiKey := "if_sk_" + base64.URLEncoding.EncodeToString(bytes)

	hash := sha256.Sum256([]byte(apiKey))
	keyHash := fmt.Sprintf("%x", hash[:])

	apiKeyModel := &models.APIKey{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      strings.TrimSpace(keyName),
		Key:       apiKey,
		KeyHash:   keyHash,
		IsActive:  true,
		ExpiresAt: time.Now().AddDate(1, 0, 0),
	}

	if err := s.db.Create(apiKeyModel).Error; err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	return apiKey, nil
}

func (s *AuthService) ValidateAPIKey(tenantID, apiKey string) (*models.User, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("API key is required")
	}

	hash := sha256.Sum256([]byte(apiKey))
	keyHash := fmt.Sprintf("%x", hash[:])

	var key models.APIKey
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&key, "key_hash = ? AND is_active = ? AND (expires_at IS NULL OR expires_at > ?)", keyHash, true, time.Now()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid API key")
		}
		return nil, fmt.Errorf("failed to validate API key: %w", err)
	}

	key.LastUsedAt = sql.NullTime{Time: time.Now(), Valid: true}
	s.db.Save(&key)

	return s.GetUserByID(tenantID, key.UserID)
}

type RegisterRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Phone       string `json:"phone"`
	CompanyName string `json:"company_name"`
	KRAPIN      string `json:"kra_pin"`
	Currency    string `json:"currency"`
}

type UpdateUserRequest struct {
	Name        *string `json:"name"`
	Phone       *string `json:"phone"`
	CompanyName *string `json:"company_name"`
	KRAPIN      *string `json:"kra_pin"`
}

func (s *AuthService) generateAccessToken(user *models.User) (string, error) {
	tenantID := user.TenantID
	if tenantID == "" {
		tenantID = user.ID
	}
	claims := &Claims{
		UserID:   user.ID,
		TenantID: tenantID,
		Email:    user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.cfg.JWT.Expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "invoicefast",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWT.Secret))
}

func (s *AuthService) generateRefreshToken(userID string) (string, error) {
	return s.GenerateRefreshTokenWithTenant(userID, "")
}

func (s *AuthService) GenerateRefreshTokenWithTenant(userID, tenantID string) (string, error) {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	token := base64.URLEncoding.EncodeToString(bytes)

	if tenantID == "" {
		tenantID = userID
	}

	refreshToken := &models.RefreshToken{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		Token:     token,
		ExpiresAt: time.Now().Add(s.cfg.JWT.RefreshExpiry),
	}

	if err := s.db.Create(refreshToken).Error; err != nil {
		return "", fmt.Errorf("failed to create refresh token: %w", err)
	}

	return token, nil
}

func validateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return errors.New("email is required")
	}
	if !strings.Contains(email, "@") {
		return errors.New("invalid email format")
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return errors.New("invalid email format")
	}

	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return errors.New("invalid email format")
	}

	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}
	if !containsUppercase(password) {
		return ErrWeakPassword
	}
	if !containsLowercase(password) {
		return ErrWeakPassword
	}
	if !containsNumber(password) {
		return ErrWeakPassword
	}
	if !containsSpecial(password) {
		return ErrWeakPassword
	}
	if commonPasswords[strings.ToLower(password)] {
		return ErrPasswordCompromised
	}
	if containsCommonPatterns(password) {
		return ErrWeakPassword
	}

	return nil
}

func containsUppercase(s string) bool {
	for _, c := range s {
		if c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
}

func containsLowercase(s string) bool {
	for _, c := range s {
		if c >= 'a' && c <= 'z' {
			return true
		}
	}
	return false
}

func containsNumber(s string) bool {
	for _, c := range s {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

func containsSpecial(s string) bool {
	specialChars := "!@#$%^&*()_+-=[]{}|;:,.<>?"
	for _, c := range s {
		if strings.Contains(specialChars, string(c)) {
			return true
		}
	}
	return false
}

func containsCommonPatterns(password string) bool {
	lower := strings.ToLower(password)
	sequences := []string{"123", "abc", "qwerty", "asdf", "zxcv"}
	for _, seq := range sequences {
		if lower == seq {
			return true
		}
	}
	for i := 0; i < len(password)-3; i++ {
		if password[i] == password[i+1] && password[i+1] == password[i+2] {
			return true
		}
	}
	return false
}

func normalizePhone(phone string) string {
	if strings.TrimSpace(phone) == "" {
		return ""
	}

	var digits string
	for _, c := range phone {
		if c >= '0' && c <= '9' {
			digits += string(c)
		}
	}

	if len(digits) == 10 && digits[0] == '0' {
		return "254" + digits[1:]
	}
	if len(digits) == 9 {
		return "254" + digits
	}
	if len(digits) == 12 && digits[:3] == "254" {
		return digits
	}

	return phone
}

func generateSubdomain(companyName string) string {
	if strings.TrimSpace(companyName) == "" {
		return ""
	}

	var result string
	for i, c := range strings.ToLower(companyName) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			result += string(c)
		} else if c == ' ' || c == '-' || c == '_' {
			if i > 0 && len(result) > 0 && result[len(result)-1] != '-' {
				result += "-"
			}
		}
		if len(result) >= 50 {
			break
		}
	}

	result = strings.Trim(result, "-")

	if len(result) < 3 {
		return ""
	}

	return result
}

type TwoFactorSetup struct {
	Secret         string   `json:"secret"`
	QRCodeURL      string   `json:"qr_code_url"`
	QRCodeImageURL string   `json:"qr_code_image_url"`
	BackupCodes    []string `json:"backup_codes"`
}

func (s *AuthService) SetupTwoFactor(tenantID, userID string) (*TwoFactorSetup, error) {
	user, err := s.GetUserByID(tenantID, userID)
	if err != nil {
		return nil, err
	}

	if user.TwoFactorEnabled {
		return nil, errors.New("two-factor authentication is already enabled")
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "InvoiceFast",
		AccountName: user.Email,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate 2FA key: %w", err)
	}

	backupCodes := generateBackupCodes(8)

	user.TwoFactorSecret = s.encryptSecret(key.Secret())
	if err := s.db.Save(user).Error; err != nil {
		return nil, fmt.Errorf("failed to save 2FA secret: %w", err)
	}

	return &TwoFactorSetup{
		Secret:         key.Secret(),
		QRCodeURL:      key.URL(),
		QRCodeImageURL: key.URL(),
		BackupCodes:    backupCodes,
	}, nil
}

func (s *AuthService) VerifyAndEnableTwoFactor(tenantID, userID, code string) error {
	user, err := s.GetUserByID(tenantID, userID)
	if err != nil {
		return err
	}

	if user.TwoFactorEnabled {
		return errors.New("two-factor authentication is already enabled")
	}

	if user.TwoFactorSecret == "" {
		return errors.New("please set up 2FA first")
	}

	secret := s.decryptSecret(user.TwoFactorSecret)
	if !validateTOTP(secret, code) {
		return errors.New("invalid verification code")
	}

	now := time.Now()
	user.TwoFactorEnabled = true
	user.TwoFactorVerifiedAt = &now

	backupCodes := generateBackupCodes(10)
	user.TwoFactorSecret = s.encryptSecret(secret + "|" + strings.Join(backupCodes, ","))

	if err := s.db.Save(user).Error; err != nil {
		return fmt.Errorf("failed to enable 2FA: %w", err)
	}

	if s.auditService != nil {
		s.auditService.LogSecurityEvent(context.Background(), tenantID, userID, "two_factor_enabled", map[string]interface{}{})
	}

	return nil
}

func (s *AuthService) DisableTwoFactor(tenantID, userID, password, code string) error {
	user, err := s.GetUserByID(tenantID, userID)
	if err != nil {
		return err
	}

	if !user.TwoFactorEnabled {
		return nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return ErrWrongPassword
	}

	secret := s.decryptSecret(user.TwoFactorSecret)
	parts := strings.Split(secret, "|")
	if len(parts) > 1 {
		backupCodes := strings.Split(parts[1], ",")
		for _, bc := range backupCodes {
			if bc == code {
				goto verified
			}
		}
	}

	if !validateTOTP(secret, code) {
		return errors.New("invalid verification code")
	}

verified:
	user.TwoFactorEnabled = false
	user.TwoFactorSecret = ""
	user.TwoFactorVerifiedAt = nil

	if err := s.db.Save(user).Error; err != nil {
		return fmt.Errorf("failed to disable 2FA: %w", err)
	}

	if s.auditService != nil {
		s.auditService.LogSecurityEvent(context.Background(), tenantID, userID, "two_factor_disabled", map[string]interface{}{})
	}

	return nil
}

func (s *AuthService) GetSessions(tenantID, userID string) ([]models.UserSession, error) {
	var sessions []models.UserSession
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Where("user_id = ? AND expires_at > ?", userID, time.Now()).
		Order("last_active_at DESC").
		Find(&sessions).Error
	return sessions, err
}

func (s *AuthService) RevokeSession(tenantID, userID, sessionID string) error {
	session := models.UserSession{}
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Where("id = ? AND user_id = ?", sessionID, userID).
		First(&session).Error
	if err != nil {
		return errors.New("session not found")
	}

	if session.IsCurrent {
		return errors.New("cannot revoke current session")
	}

	return s.db.Delete(&session).Error
}

func (s *AuthService) RevokeAllSessions(tenantID, userID, exceptCurrent string) error {
	query := s.db.Scopes(database.TenantFilter(tenantID)).
		Where("user_id = ? AND expires_at > ?", userID, time.Now())

	if exceptCurrent != "" {
		query = query.Where("id != ?", exceptCurrent)
	}

	return query.Delete(&models.UserSession{}).Error
}

func (s *AuthService) GetLoginHistory(tenantID, userID string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var logs []models.AuditLog
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Where("user_id = ? AND action = 'login'", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, len(logs))
	for i, log := range logs {
		result[i] = map[string]interface{}{
			"id":         log.ID,
			"ip_address": log.IPAddress,
			"action":     log.Action,
			"details":    log.Details,
			"created_at": log.CreatedAt,
		}
	}

	return result, nil
}

func (s *AuthService) UpdateLoginAlerts(tenantID, userID string, enabled bool) error {
	user, err := s.GetUserByID(tenantID, userID)
	if err != nil {
		return err
	}
	user.LoginAlertEnabled = enabled
	return s.db.Save(user).Error
}

func generateBackupCodes(count int) []string {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		b := make([]byte, 4)
		rand.Read(b)
		codes[i] = fmt.Sprintf("%08x", b)
	}
	return codes
}

func validateTOTP(secret, code string) bool {
	plainSecret := strings.Split(secret, "|")[0]
	return totp.Validate(code, plainSecret)
}

func (s *AuthService) encryptSecret(plaintext string) string {
	if s.cfg == nil {
		return plaintext
	}
	key := []byte(s.cfg.JWT.Secret)
	if len(key) < 32 {
		key = append(key, make([]byte, 32-len(key))...)
	}
	block, _ := aes.NewCipher(key[:32])
	iv := make([]byte, block.BlockSize())
	rand.Read(iv)
	cfb := cipher.NewCFBEncrypter(block, iv)
	ciphertext := make([]byte, len(plaintext))
	cfb.XORKeyStream(ciphertext, []byte(plaintext))
	result := append(iv, ciphertext...)
	return base64.StdEncoding.EncodeToString(result)
}

func (s *AuthService) decryptSecret(encoded string) string {
	if s.cfg == nil {
		return encoded
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	key := []byte(s.cfg.JWT.Secret)
	if len(key) < 32 {
		key = append(key, make([]byte, 32-len(key))...)
	}
	block, _ := aes.NewCipher(key[:32])
	iv := data[:block.BlockSize()]
	ciphertext := data[block.BlockSize():]
	block2, _ := aes.NewCipher(key[:32])
	plaintext := make([]byte, len(ciphertext))
	cfb := cipher.NewCFBDecrypter(block2, iv)
	cfb.XORKeyStream(plaintext, ciphertext)
	return string(plaintext)
}

// SendVerificationCode generates a 6-digit code, stores it, and emails it.
func (s *AuthService) SendVerificationCode(userID, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))

	var user models.User
	if err := s.db.First(&user, "id = ? AND email = ?", userID, email).Error; err != nil {
		return errors.New("user not found")
	}

	// Invalidate any previous unused codes
	_ = s.db.Where("user_id = ? AND used_at IS NULL", userID).
		Delete(&models.EmailVerificationToken{}).Error

	var b [3]byte
	rand.Read(b[:])
	code := fmt.Sprintf("%06d", int(b[0])<<12|int(b[1])<<6|int(b[2])%1000000)
	hashed := fmt.Sprintf("%x", sha256.Sum256([]byte(code)))

	token := &models.EmailVerificationToken{
		ID:        uuid.New().String(),
		UserID:    userID,
		Token:     hashed,
		RawToken:  code,
		Email:     email,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if err := s.db.Create(token).Error; err != nil {
		return fmt.Errorf("failed to create verification token: %w", err)
	}

	if s.emailService != nil {
		go s.emailService.Send(EmailRequest{
			To:      []string{email},
			Subject: "Your InvoiceFast verification code",
			Body: fmt.Sprintf(`
				<div style="font-family: Arial, sans-serif; max-width: 480px; margin: 0 auto;">
					<h2 style="color: #1a1a2e;">Verify your email</h2>
					<p style="color: #666; font-size: 14px;">Enter this code to verify your email address:</p>
					<div style="background: #f5f5f5; border-radius: 8px; padding: 20px; text-align: center; margin: 16px 0;">
						<span style="font-size: 32px; font-weight: 700; letter-spacing: 8px; color: #1a1a2e; font-family: monospace;">%s</span>
					</div>
					<p style="color: #999; font-size: 12px;">This code expires in 10 minutes.</p>
				</div>`, code),
			IsHTML: true,
		})
	}

	return nil
}

// SendVerificationLink generates a token and sends an email with a verification link.
func (s *AuthService) SendVerificationLink(userID string) error {
	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		return errors.New("user not found")
	}

	// Invalidate any previous unused tokens
	_ = s.db.Where("user_id = ? AND used_at IS NULL", userID).
		Delete(&models.EmailVerificationToken{}).Error

	var b [16]byte
	rand.Read(b[:])
	rawToken := hex.EncodeToString(b[:])
	hashed := fmt.Sprintf("%x", sha256.Sum256([]byte(rawToken)))

	token := &models.EmailVerificationToken{
		ID:        uuid.New().String(),
		UserID:    userID,
		Token:     hashed,
		RawToken:  rawToken,
		Email:     user.Email,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := s.db.Create(token).Error; err != nil {
		return fmt.Errorf("failed to create verification token: %w", err)
	}

	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", s.cfg.Server.BaseURL, rawToken)

	if s.emailService != nil {
		go s.emailService.Send(EmailRequest{
			To:      []string{user.Email},
			Subject: "Verify your InvoiceFast account",
			Body: fmt.Sprintf(`
				<div style="font-family: Arial, sans-serif; max-width: 480px; margin: 0 auto;">
					<div style="text-align: center; padding: 24px 0;">
						<img src="https://invoicefast.app/images/logo/logo.png" alt="InvoiceFast" style="height: 40px;">
					</div>
					<h2 style="color: #1a1a2e; text-align: center;">Verify your email address</h2>
					<p style="color: #666; font-size: 14px; text-align: center;">Click the button below to verify your email and unlock all InvoiceFast features:</p>
					<div style="text-align: center; margin: 24px 0;">
						<a href="%s" style="display: inline-block; background: #0277BD; color: #fff; padding: 14px 32px; border-radius: 8px; text-decoration: none; font-weight: 600; font-size: 15px;">Verify email</a>
					</div>
					<p style="color: #999; font-size: 12px; text-align: center;">This link expires in 24 hours.</p>
					<p style="color: #999; font-size: 12px; text-align: center;">If you didn't create an InvoiceFast account, ignore this email.</p>
				</div>`, verifyURL),
			IsHTML: true,
		})
	}

	return nil
}

// VerifyEmailLink validates a link-based verification token and marks the user verified.
func (s *AuthService) VerifyEmailLink(rawToken string) (*models.User, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, errors.New("invalid token")
	}

	hashed := fmt.Sprintf("%x", sha256.Sum256([]byte(rawToken)))

	var token models.EmailVerificationToken
	if err := s.db.Where("token = ? AND used_at IS NULL", hashed).
		Order("created_at DESC").Limit(1).First(&token).Error; err != nil {
		return nil, ErrVerificationInvalid
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, ErrVerificationExpired
	}

	now := time.Now()
	token.UsedAt = &now
	if err := s.db.Save(&token).Error; err != nil {
		return nil, fmt.Errorf("failed to mark token used: %w", err)
	}

	var user models.User
	if err := s.db.First(&user, "id = ?", token.UserID).Error; err != nil {
		return nil, errors.New("user not found")
	}

	return &user, nil
}

// VerifyEmailCode validates a 6-digit verification code for the given user.
func (s *AuthService) VerifyEmailCode(userID, code string) error {
	code = strings.TrimSpace(code)

	var tokens []models.EmailVerificationToken
	if err := s.db.Where("user_id = ? AND used_at IS NULL", userID).
		Order("created_at DESC").Limit(1).Find(&tokens).Error; err != nil {
		return fmt.Errorf("failed to query tokens: %w", err)
	}
	if len(tokens) == 0 {
		return ErrVerificationInvalid
	}

	tok := tokens[0]
	if time.Now().After(tok.ExpiresAt) {
		return ErrVerificationExpired
	}

	hashed := fmt.Sprintf("%x", sha256.Sum256([]byte(code)))
	if hashed != tok.Token {
		return ErrVerificationInvalid
	}

	now := time.Now()
	tok.UsedAt = &now
	if err := s.db.Save(&tok).Error; err != nil {
		return fmt.Errorf("failed to mark token used: %w", err)
	}

	return nil
}
