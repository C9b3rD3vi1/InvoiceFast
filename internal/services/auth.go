package services

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
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
	ErrEmailExists   = errors.New("email already registered")
	ErrInvalidEmail  = errors.New("invalid email format")
	ErrWeakPassword  = errors.New("password must be at least 6 characters")
	ErrWrongPassword = errors.New("incorrect password")
	ErrInvalidToken  = errors.New("invalid or expired token")
)

type AuthService struct {
	db  *database.DB
	cfg *config.Config
}

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         *models.User `json:"user"`
}

func NewAuthService(db *database.DB, cfg *config.Config) *AuthService {
	return &AuthService{db: db, cfg: cfg}
}

// Register creates a new user account
func (s *AuthService) Register(req *RegisterRequest) (*AuthResponse, error) {
	// Validate email format
	if err := validateEmail(req.Email); err != nil {
		return nil, ErrInvalidEmail
	}

	// Validate password
	if len(req.Password) < 6 {
		return nil, ErrWeakPassword
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

	user := &models.User{
		ID:           uuid.New().String(),
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		Name:         req.Name,
		Phone:        normalizePhone(req.Phone),
		CompanyName:  req.CompanyName,
		KRAPIN:       strings.ToUpper(strings.TrimSpace(req.KRAPIN)),
		Plan:         "free",
		IsActive:     true,
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
func (s *AuthService) RefreshToken(refreshToken string) (*AuthResponse, error) {
	// Find the refresh token in DB
	var storedToken models.RefreshToken
	if err := s.db.First(&storedToken, "token = ? AND expires_at > ?", refreshToken, time.Now()).Error; err != nil {
		return nil, ErrInvalidToken
	}

	// Get user
	var user models.User
	if err := s.db.First(&user, "id = ?", storedToken.UserID).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Delete old refresh token
	s.db.Delete(&storedToken)

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
func (s *AuthService) GetUserByID(userID string) (*models.User, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}

	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}
	return &user, nil
}

// UpdateUser updates user profile
func (s *AuthService) UpdateUser(userID string, req *UpdateUserRequest) (*models.User, error) {
	user, err := s.GetUserByID(userID)
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
func (s *AuthService) ChangePassword(userID, oldPassword, newPassword string) error {
	if len(newPassword) < 6 {
		return ErrWeakPassword
	}

	user, err := s.GetUserByID(userID)
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

	// Invalidate all refresh tokens
	s.db.Where("user_id = ?", userID).Delete(&models.RefreshToken{})

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
func (s *AuthService) ValidateAPIKey(apiKey string) (*models.User, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("API key is required")
	}

	hash := sha256.Sum256([]byte(apiKey))
	keyHash := fmt.Sprintf("%x", hash[:])

	var key models.APIKey
	if err := s.db.First(&key, "key_hash = ? AND is_active = ? AND (expires_at IS NULL OR expires_at > ?)", keyHash, true, time.Now()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid API key")
		}
		return nil, fmt.Errorf("failed to validate API key: %w", err)
	}

	// Update last used
	key.LastUsedAt = sql.NullTime{Time: time.Now(), Valid: true}
	s.db.Save(&key)

	return s.GetUserByID(key.UserID)
}

// Request types
type RegisterRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
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
	claims := &Claims{
		UserID: user.ID,
		Email:  user.Email,
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
