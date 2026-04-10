package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrEmailExists         = errors.New("email already registered")
	ErrInvalidEmail        = errors.New("invalid email format")
	ErrWeakPassword        = errors.New("password must be at least 8 characters with uppercase, lowercase, number and special character")
	ErrWrongPassword       = errors.New("incorrect password")
	ErrInvalidToken        = errors.New("invalid or expired token")
	ErrPasswordCompromised = errors.New("this password has been found in a data breach. please choose a different password")
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
		RawToken:  rawToken,
		Email:     email,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IPAddress: ipAddress,
		UserAgent: userAgent,
		CreatedAt: time.Now(),
	}

	if err := s.db.Create(resetToken).Error; err != nil {
		return nil, fmt.Errorf("failed to create reset token: %w", err)
	}

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
		log.Printf("[AUTH] Warning: Failed to invalidate refresh tokens: %v", err)
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
	db           *database.DB
	cfg          *config.Config
	emailService *EmailService
	auditService *AuditService
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

func NewAuthService(db *database.DB, cfg *config.Config, emailSvc *EmailService, auditSvc *AuditService) *AuthService {
	return &AuthService{db: db, cfg: cfg, emailService: emailSvc, auditService: auditSvc}
}

func (s *AuthService) Register(req *RegisterRequest) (*AuthResponse, error) {
	if err := validateEmail(req.Email); err != nil {
		return nil, ErrInvalidEmail
	}

	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	var existing models.User
	if err := s.db.First(&existing, "email = ?", req.Email).Error; err == nil {
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
		Plan:      "free",
		IsActive:  true,
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
		Plan:         "free",
		IsActive:     true,
		Role:         "owner",
	}

	if err := s.db.Create(user).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
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
	if len(newPassword) < 6 {
		return ErrWeakPassword
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

	user.PasswordHash = string(hashedPassword)
	if err := s.db.Save(user).Error; err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	s.db.Scopes(database.TenantFilter(tenantID)).Where("user_id = ?", userID).Delete(&models.RefreshToken{})

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
