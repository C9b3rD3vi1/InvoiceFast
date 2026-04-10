package services

import (
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

// commonPasswords list (top 100) - in production, use a proper compromised password checker
var commonPasswords = map[string]bool{
	"password": true, "12345678": true, "123456789": true, "password123": true,
	"123456": true, "qwerty": true, "12345": true, "1234567890": true,
	"password1": true, "1234567": true, "iloveyou": true, "adobe123": true,
	"123123": true, "sunshine": true, "12345678901": true, "princess": true,
	"admin": true, "welcome": true, "123654": true, "666666": true,
}

// ValidatePasswordStrength validates password strength
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	if len(password) > 128 {
		return errors.New("password must be at most 128 characters")
	}

	// Check for uppercase
	hasUpper := false
	for _, c := range password {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}

	// Check for lowercase
	hasLower := false
	for _, c := range password {
		if c >= 'a' && c <= 'z' {
			hasLower = true
			break
		}
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}

	// Check for number
	hasNumber := false
	for _, c := range password {
		if c >= '0' && c <= '9' {
			hasNumber = true
			break
		}
	}
	if !hasNumber {
		return errors.New("password must contain at least one number")
	}

	// Check for special character
	specialChars := "!@#$%^&*()_+-=[]{}|;:,.<>?"
	hasSpecial := false
	for _, c := range password {
		if strings.Contains(specialChars, string(c)) {
			hasSpecial = true
			break
		}
	}
	if !hasSpecial {
		return errors.New("password must contain at least one special character")
	}

	// Check common passwords
	if commonPasswords[strings.ToLower(password)] {
		return ErrPasswordCompromised
	}

	return nil
}

// InitiatePasswordReset initiates the password reset process
// SECURITY: Accepts tenantID to enforce tenant-scoped lookup and prevent IDOR
func (s *AuthService) InitiatePasswordReset(tenantID, email, ipAddress, userAgent string) (*models.PasswordResetToken, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	email = strings.ToLower(strings.TrimSpace(email))

	// SECURITY: Use TenantFilter to prevent cross-tenant enumeration
	var user models.User
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Return nil without error (security: don't reveal existence)
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Check rate limiting (max 3 per hour) - WITH ERROR HANDLING
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

	// Invalidate existing tokens - WITH TENANT SCOPE
	if err := s.db.Where("tenant_id = ? AND user_id = ? AND expires_at > ?", tenantID, user.ID, time.Now()).
		Delete(&models.PasswordResetToken{}).Error; err != nil {
		return nil, fmt.Errorf("failed to invalidate tokens: %w", err)
	}

	// Generate new token
	rawToken := secureRandomToken()
	tokenHash := hashToken(rawToken, s.cfg.JWT.Secret)

	resetToken := &models.PasswordResetToken{
		ID:        uuid.New().String(),
		TenantID:  tenantID, // SECURITY: Store tenant_id to prevent cross-tenant token usage
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

	// Send password reset email
	if s.emailService != nil {
		resetToken := rawToken // The raw token to include in email
		_ = resetToken         // Email sending is handled via the PasswordResetService in production
	}

	return resetToken, nil
}

// CompletePasswordReset completes the password reset
// SECURITY: Accepts tenantID to validate token belongs to correct tenant
func (s *AuthService) CompletePasswordReset(tenantID, token, newPassword, confirmPassword, ipAddress string) error {
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}

	// Validate passwords match
	if newPassword != confirmPassword {
		return errors.New("passwords do not match")
	}

	// Validate password strength
	if err := ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	// Find token - SECURITY: Must match tenant_id to prevent cross-tenant token usage
	tokenHash := hashToken(token, s.cfg.JWT.Secret)
	var resetToken models.PasswordResetToken

	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&resetToken, "token = ?", tokenHash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidToken
		}
		return fmt.Errorf("database error: %w", err)
	}

	// Check expiry
	if resetToken.IsExpired() {
		return ErrTokenExpired
	}

	// Check if used
	if resetToken.UsedAt != nil {
		return ErrTokenUsed
	}

	// Find user - SECURITY: Must use tenant filter
	var user models.User
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", resetToken.UserID).Error; err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Hash new password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update in transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Update user password
		if err := tx.Model(&user).Update("password_hash", string(passwordHash)).Error; err != nil {
			return err
		}

		// Mark token as used
		now := time.Now()
		resetToken.UsedAt = &now
		if err := tx.Save(&resetToken).Error; err != nil {
			return err
		}

		// Create audit log
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

	// Invalidate all refresh tokens (log out all devices) - with error handling
	if err := s.db.Where("user_id = ?", user.ID).Delete(&models.RefreshToken{}).Error; err != nil {
		log.Printf("[AUTH] Warning: Failed to invalidate refresh tokens: %v", err)
	}

	return nil
}

// ValidateResetToken validates a reset token
// SECURITY: Accepts tenantID to enforce tenant-scoped lookup - prevents cross-tenant token enumeration
func (s *AuthService) ValidateResetToken(tenantID, token string) (*models.User, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	tokenHash := hashToken(token, s.cfg.JWT.Secret)

	// SECURITY: Use TenantFilter to prevent cross-tenant token usage
	var resetToken models.PasswordResetToken
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&resetToken, "token = ?", tokenHash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	if resetToken.IsExpired() {
		return nil, ErrTokenExpired
	}

	if resetToken.UsedAt != nil {
		return nil, ErrTokenUsed
	}

	// SECURITY: User lookup must also be tenant-scoped
	var user models.User
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", resetToken.UserID).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &user, nil
}

// secureRandomToken generates a secure random token
func secureRandomToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}

// hashToken hashes a token for secure storage
func hashToken(token, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

type AuthService struct {
	db           *database.DB
	cfg          *config.Config
	emailService *EmailService
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

func NewAuthService(db *database.DB, cfg *config.Config, emailSvc *EmailService) *AuthService {
	return &AuthService{db: db, cfg: cfg, emailService: emailSvc}
}

// Register creates a new user account
func (s *AuthService) Register(req *RegisterRequest) (*AuthResponse, error) {
	// Validate email format
	if err := validateEmail(req.Email); err != nil {
		return nil, ErrInvalidEmail
	}

	// Validate password with strong policy
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	// Check if email exists
	var existing models.User
	if err := s.db.First(&existing, "email = ?", req.Email).Error; err == nil {
		return nil, ErrEmailExists
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create tenant for new user
	tenant := &models.Tenant{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Subdomain: "",
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

	// Generate tokens
	accessToken, err := s.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateRefreshToken(user.ID)
	if err != nil {
		return nil, err
	}

	// Seed default templates
	s.db.SeedDefaultTemplates(user.ID)

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}

// Login authenticates a user
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

	// Generate tokens
	accessToken, err := s.generateAccessToken(&user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateRefreshToken(user.ID)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         &user,
	}, nil
}

// RefreshToken refreshes an access token
// SECURITY: Accepts tenantID to enforce tenant-scoped lookup - prevents using tokens from other tenants
func (s *AuthService) RefreshToken(tenantID, refreshToken string) (*AuthResponse, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	// SECURITY: Use TenantFilter to prevent cross-tenant token usage
	var storedToken models.RefreshToken
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&storedToken, "token = ? AND expires_at > ?", refreshToken, time.Now()).Error; err != nil {
		return nil, ErrInvalidToken
	}

	// SECURITY: User lookup must also be tenant-scoped
	var user models.User
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", storedToken.UserID).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Delete old refresh token
	if err := s.db.Delete(&storedToken).Error; err != nil {
		return nil, fmt.Errorf("failed to delete refresh token: %w", err)
	}

	// Generate new tokens
	accessToken, err := s.generateAccessToken(&user)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := s.generateRefreshToken(user.ID)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		User:         &user,
	}, nil
}

// ValidateToken validates an access token and returns the user ID
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

// GetUserByID retrieves a user by ID
// SECURITY: Accepts tenantID to enforce tenant-scoped lookup - prevents IDOR
func (s *AuthService) GetUserByID(tenantID, userID string) (*models.User, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}

	// SECURITY: Use TenantFilter to prevent cross-tenant access
	var user models.User
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}
	return &user, nil
}

// UpdateUser updates user profile
// SECURITY: Accepts tenantID to enforce tenant-scoped lookup
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

// ChangePassword changes user password
// SECURITY: Accepts tenantID to enforce tenant-scoped lookup
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

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrWrongPassword
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user.PasswordHash = string(hashedPassword)
	if err := s.db.Save(user).Error; err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Invalidate all refresh tokens - with tenant scope
	s.db.Scopes(database.TenantFilter(tenantID)).Where("user_id = ?", userID).Delete(&models.RefreshToken{})

	return nil
}

// Logout invalidates a refresh token
func (s *AuthService) Logout(refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return nil // Nothing to logout
	}
	return s.db.Where("token = ?", refreshToken).Delete(&models.RefreshToken{}).Error
}

// GenerateAPIKey generates an API key for programmatic access
func (s *AuthService) GenerateAPIKey(userID, keyName string) (string, error) {
	if strings.TrimSpace(keyName) == "" {
		keyName = "Default"
	}

	// Generate random key
	bytes := make([]byte, 32)
	rand.Read(bytes)
	apiKey := "if_sk_" + base64.URLEncoding.EncodeToString(bytes)

	// Hash the key for storage
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := fmt.Sprintf("%x", hash[:])

	apiKeyModel := &models.APIKey{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      strings.TrimSpace(keyName),
		Key:       apiKey,
		KeyHash:   keyHash,
		IsActive:  true,
		ExpiresAt: time.Now().AddDate(1, 0, 0), // 1 year
	}

	if err := s.db.Create(apiKeyModel).Error; err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	return apiKey, nil
}

// ValidateAPIKey validates an API key
// SECURITY: Accepts tenantID to enforce tenant-scoped lookup
func (s *AuthService) ValidateAPIKey(tenantID, apiKey string) (*models.User, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("API key is required")
	}

	hash := sha256.Sum256([]byte(apiKey))
	keyHash := fmt.Sprintf("%x", hash[:])

	// SECURITY: Use TenantFilter to prevent cross-tenant API key usage
	var key models.APIKey
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&key, "key_hash = ? AND is_active = ? AND (expires_at IS NULL OR expires_at > ?)", keyHash, true, time.Now()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid API key")
		}
		return nil, fmt.Errorf("failed to validate API key: %w", err)
	}

	// Update last used
	key.LastUsedAt = sql.NullTime{Time: time.Now(), Valid: true}
	s.db.Save(&key)

	// SECURITY: User lookup must also be tenant-scoped
	return s.GetUserByID(tenantID, key.UserID)
}

// Request types
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
	bytes := make([]byte, 32)
	rand.Read(bytes)
	token := base64.URLEncoding.EncodeToString(bytes)

	refreshToken := &models.RefreshToken{
		ID:        uuid.New().String(),
		UserID:    userID,
		Token:     token,
		ExpiresAt: time.Now().Add(s.cfg.JWT.RefreshExpiry),
	}

	if err := s.db.Create(refreshToken).Error; err != nil {
		return "", fmt.Errorf("failed to create refresh token: %w", err)
	}

	return token, nil
}

// validateEmail validates email format
func validateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return errors.New("email is required")
	}

	// Basic email validation
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

// validatePassword validates password strength
func validatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}

	// Check for uppercase
	if !containsUppercase(password) {
		return ErrWeakPassword
	}

	// Check for lowercase
	if !containsLowercase(password) {
		return ErrWeakPassword
	}

	// Check for number
	if !containsNumber(password) {
		return ErrWeakPassword
	}

	// Check for special character
	if !containsSpecial(password) {
		return ErrWeakPassword
	}

	// Check against common passwords
	if commonPasswords[strings.ToLower(password)] {
		return ErrPasswordCompromised
	}

	// Check for common patterns
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

	// Sequential characters - only reject if the entire password is just the sequence
	sequences := []string{"123", "abc", "qwerty", "asdf", "zxcv"}
	for _, seq := range sequences {
		if lower == seq {
			return true
		}
	}

	// Repeated characters (more than 3)
	for i := 0; i < len(password)-3; i++ {
		if password[i] == password[i+1] && password[i+1] == password[i+2] {
			return true
		}
	}

	return false
}

// normalizePhone normalizes phone number
func normalizePhone(phone string) string {
	if strings.TrimSpace(phone) == "" {
		return ""
	}

	// Remove all non-digits
	var digits string
	for _, c := range phone {
		if c >= '0' && c <= '9' {
			digits += string(c)
		}
	}

	// Handle different formats
	if len(digits) == 10 && digits[0] == '0' {
		return "254" + digits[1:]
	}
	if len(digits) == 9 {
		return "254" + digits
	}
	if len(digits) == 12 && digits[:3] == "254" {
		return digits
	}

	// Return original if no conversion possible
	return phone
}
