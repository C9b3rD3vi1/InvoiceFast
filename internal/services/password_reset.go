package services

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
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

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrTokenExpired     = errors.New("reset token has expired")
	ErrTokenUsed        = errors.New("reset token has already been used")
	ErrTokenInvalid     = errors.New("invalid reset token")
	ErrEmailNotVerified = errors.New("email address not verified")
	ErrNotFound         = errors.New("user not found")
	ErrRateLimited      = errors.New("too many reset requests - please wait")
)

// PasswordResetService handles password reset operations
type PasswordResetService struct {
	db           *database.DB
	cfg          *config.Config
	emailService *EmailService
}

// NewPasswordResetService creates a new password reset service
func NewPasswordResetService(db *database.DB, cfg *config.Config, email *EmailService) *PasswordResetService {
	return &PasswordResetService{
		db:           db,
		cfg:          cfg,
		emailService: email,
	}
}

// InitiatePasswordReset starts the password reset process
func (s *PasswordResetService) InitiatePasswordReset(email, ipAddress, userAgent string) (*models.PasswordResetToken, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	// 1. Find user by email
	var user models.User
	if err := s.db.First(&user, "email = ? AND is_active = ?", email, true).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Return success even if user doesn't exist (security: don't reveal)
			log.Printf("[SECURITY] Password reset attempted for non-existent email (masked: ***@%s)", extractDomain(email))
			return nil, nil
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// 2. Check rate limiting (max 3 requests per hour)
	var recentCount int64
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	if err := s.db.Model(&models.PasswordResetToken{}).
		Where("user_id = ? AND created_at > ?", user.ID, oneHourAgo).
		Count(&recentCount).Error; err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	if recentCount >= 3 {
		log.Printf("[SECURITY] Rate limited password reset for user: %s", user.ID)
		return nil, ErrRateLimited
	}

	// 3. Invalidate any existing tokens for this user
	if err := s.db.Where("user_id = ? AND used_at IS NULL", user.ID).
		Delete(&models.PasswordResetToken{}).Error; err != nil {
		log.Printf("Warning: failed to invalidate old tokens: %v", err)
	}

	// 4. Generate new secure token
	rawToken, hashedToken, err := s.generateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// 5. Create reset token record
	resetToken := &models.PasswordResetToken{
		ID:        generateUUID(),
		UserID:    user.ID,
		Token:     hashedToken,
		RawToken:  rawToken, // Will be cleared after sending
		Email:     email,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IPAddress: ipAddress,
		UserAgent: userAgent,
		CreatedAt: time.Now(),
	}

	if err := s.db.Create(resetToken).Error; err != nil {
		return nil, fmt.Errorf("failed to create reset token: %w", err)
	}

	// 6. Send reset email if email service is configured
	if s.emailService != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Panic in send reset email: %v", r)
				}
			}()

			resetLink := fmt.Sprintf("%s/reset-password?token=%s", s.cfg.Server.BaseURL, rawToken)

			emailData := &PasswordResetEmailData{
				UserEmail:   user.Email,
				UserName:    user.Name,
				ResetLink:   resetLink,
				ExpiresIn:   "1 hour",
				IPAddress:   ipAddress,
				RequestTime: time.Now().Format(time.RFC1123),
			}

			if err := s.emailService.SendPasswordResetEmail(emailData); err != nil {
				log.Printf("Failed to send password reset email: %v", err)
			}
		}()
	}

	// CRITICAL: Do NOT return the raw token - only return success indicator
	// The token is sent via email, never expose it in API response
	resetToken.RawToken = ""

	log.Printf("[AUDIT] Password reset initiated for user %s from IP %s", user.ID, ipAddress)

	return resetToken, nil
}

// CompletePasswordReset completes the password reset process
func (s *PasswordResetService) CompletePasswordReset(token, newPassword, confirmPassword string) error {
	// 1. Validate password match
	if newPassword != confirmPassword {
		return errors.New("passwords do not match")
	}

	// 2. Validate password strength
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	// 3. Hash and find the token
	tokenHash := s.hashToken(token)

	var resetToken models.PasswordResetToken
	if err := s.db.First(&resetToken, "token = ?", tokenHash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTokenInvalid
		}
		return fmt.Errorf("database error: %w", err)
	}

	// 4. Validate token state
	if err := resetToken.CanBeUsed(); err != nil {
		return err
	}

	// 5. Find user
	var user models.User
	if err := s.db.First(&user, "id = ?", resetToken.UserID).Error; err != nil {
		return ErrNotFound
	}

	// 6. Check new password is different from old
	if err := s.isSamePassword(user.PasswordHash, newPassword); err == nil {
		return errors.New("new password cannot be the same as your current password")
	}

	// 7. Hash new password
	newPasswordHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// 8. Update password in transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Update user password
		if err := tx.Model(&user).Update("password_hash", newPasswordHash).Error; err != nil {
			return err
		}

		// Mark token as used
		now := time.Now()
		resetToken.UsedAt = &now
		if err := tx.Save(&resetToken).Error; err != nil {
			return err
		}

		// Log the action
		auditLog := &models.AuditLog{
			ID:         generateUUID(),
			UserID:     user.ID,
			Action:     "password_reset.completed",
			EntityType: "user",
			EntityID:   user.ID,
			Details:    fmt.Sprintf(`{"method": "password_reset", "ip": "%s"}`, resetToken.IPAddress),
			CreatedAt:  time.Now(),
		}

		return tx.Create(auditLog).Error
	})

	if err != nil {
		return fmt.Errorf("failed to complete password reset: %w", err)
	}

	// 9. Invalidate all refresh tokens (log out all sessions)
	s.db.Where("user_id = ?", user.ID).Delete(&models.RefreshToken{})

	// 10. Send confirmation email
	if s.emailService != nil {
		go func() {
			emailData := &PasswordChangedEmailData{
				UserName:  user.Name,
				UserEmail: user.Email,
				ChangedAt: time.Now(),
				IPAddress: resetToken.IPAddress,
			}
			if err := s.emailService.SendPasswordChangedEmail(emailData); err != nil {
				log.Printf("Failed to send password changed email: %v", err)
			}
		}()
	}

	log.Printf("[AUDIT] Password reset completed for user %s", user.ID)

	return nil
}

// ValidateResetToken validates a reset token without consuming it
func (s *PasswordResetService) ValidateResetToken(token string) (*models.User, error) {
	tokenHash := s.hashToken(token)

	var resetToken models.PasswordResetToken
	if err := s.db.First(&resetToken, "token = ?", tokenHash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenInvalid
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	if err := resetToken.CanBeUsed(); err != nil {
		return nil, err
	}

	var user models.User
	if err := s.db.First(&user, "id = ?", resetToken.UserID).Error; err != nil {
		return nil, ErrNotFound
	}

	return &user, nil
}

// generateSecureToken generates a secure random token and its hash
func (s *PasswordResetService) generateSecureToken() (rawToken, hashedToken string, err error) {
	// Generate 32 bytes of random data
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Create URL-safe base64 encoded token
	rawToken = base64.URLEncoding.EncodeToString(bytes)

	// Create HMAC hash for storage (using config secret as key)
	hashedToken = s.hashToken(rawToken)

	return rawToken, hashedToken, nil
}

// hashToken creates a hash of the token for secure storage
func (s *PasswordResetService) hashToken(token string) string {
	// Use the JWT secret as the HMAC key for consistency
	h := hmac.New(sha256.New, []byte(s.cfg.JWT.Secret))
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

// hashPassword hashes a password using bcrypt
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// isSamePassword checks if the new password matches the old password
func (s *PasswordResetService) isSamePassword(hashedPassword, newPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(newPassword))
}

// generateUUID generates a new UUID string
func generateUUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	bytes[6] = (bytes[6] & 0x0f) | 0x40 // Version 4
	bytes[8] = (bytes[8] & 0x3f) | 0x80 // Variant 1
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
}

// CleanupExpiredTokens removes expired password reset tokens
func (s *PasswordResetService) CleanupExpiredTokens() error {
	result := s.db.Where("expires_at < ?", time.Now()).Delete(&models.PasswordResetToken{})
	if result.Error != nil {
		return result.Error
	}
	log.Printf("[CLEANUP] Removed %d expired password reset tokens", result.RowsAffected)
	return nil
}

// ==================== EMAIL TEMPLATES ====================

// PasswordResetEmailData for password reset email
type PasswordResetEmailData struct {
	UserName    string
	UserEmail   string
	ResetLink   string
	ExpiresIn   string
	IPAddress   string
	RequestTime string
}

// PasswordChangedEmailData for password changed confirmation
type PasswordChangedEmailData struct {
	UserName  string
	UserEmail string
	ChangedAt time.Time
	IPAddress string
}

// SendPasswordResetEmail sends a password reset email
func (s *EmailService) SendPasswordResetEmail(data *PasswordResetEmailData) error {
	const templateHTML = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Reset Your Password</title>
    <style>
        body { font-family: 'Segoe UI', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .container { background: #f9f9f9; border-radius: 10px; padding: 30px; }
        h1 { color: #2563eb; margin-bottom: 20px; }
        .btn { display: inline-block; background: #2563eb; color: white; padding: 14px 28px; text-decoration: none; border-radius: 6px; margin: 20px 0; }
        .warning { background: #fef3cd; border-left: 4px solid #ffc107; padding: 10px 15px; margin: 20px 0; }
        .footer { font-size: 12px; color: #666; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔐 Reset Your Password</h1>
        
        <p>Hi {{.UserName}},</p>
        <p>We received a request to reset your password. Click the button below to create a new password:</p>
        
        <p style="text-align: center;">
            <a href="{{.ResetLink}}" class="btn">Reset Password</a>
        </p>
        
        <p>Or copy this link into your browser:</p>
        <p style="word-break: break-all; background: #eee; padding: 10px; border-radius: 4px;">{{.ResetLink}}</p>
        
        <p><strong>This link will expire in {{.ExpiresIn}}.</strong></p>
        
        <div class="warning">
            <p><strong>Security information:</strong></p>
            <ul>
                <li>This request was made from IP: {{.IPAddress}}</li>
                <li>Time: {{.RequestTime}}</li>
                <li>If you didn't request this, please ignore this email or contact support.</li>
            </ul>
        </div>
        
        <p>For your security, we recommend:</p>
        <ul>
            <li>Use a unique password (not used elsewhere)</li>
            <li>Minimum 8 characters with uppercase, lowercase, numbers, and symbols</li>
            <li>Never share your password with anyone</li>
        </ul>
        
        <div class="footer">
            <p>This email was sent to {{.UserEmail}}</p>
            <p>InvoiceFast - Professional Invoicing for African Businesses</p>
        </div>
    </div>
</body>
</html>
`

	// Simple template replacement (in production, use html/template)
	body := templateHTML
	body = strings.ReplaceAll(body, "{{.UserName}}", data.UserName)
	body = strings.ReplaceAll(body, "{{.ResetLink}}", data.ResetLink)
	body = strings.ReplaceAll(body, "{{.ExpiresIn}}", data.ExpiresIn)
	body = strings.ReplaceAll(body, "{{.IPAddress}}", data.IPAddress)
	body = strings.ReplaceAll(body, "{{.RequestTime}}", data.RequestTime)
	body = strings.ReplaceAll(body, "{{.UserEmail}}", data.UserEmail)

	req := EmailRequest{
		To:      []string{data.UserEmail},
		Subject: "Reset Your InvoiceFast Password",
		Body:    body,
		IsHTML:  true,
	}

	return s.Send(req)
}

// SendPasswordChangedEmail sends a confirmation email after password change
func (s *EmailService) SendPasswordChangedEmail(data *PasswordChangedEmailData) error {
	const templateHTML = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Password Changed Successfully</title>
    <style>
        body { font-family: 'Segoe UI', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .container { background: #f9f9f9; border-radius: 10px; padding: 30px; }
        h1 { color: #22c55e; margin-bottom: 20px; }
        .success { background: #dcfce7; border-left: 4px solid #22c55e; padding: 15px; margin: 20px 0; }
        .warning { background: #fff3cd; border-left: 4px solid #ffc107; padding: 10px 15px; margin: 20px 0; }
        .footer { font-size: 12px; color: #666; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>✅ Password Changed Successfully</h1>
        
        <p>Hi {{.UserName}},</p>
        <p>Your password has been successfully changed.</p>
        
        <div class="success">
            <p><strong>Details:</strong></p>
            <ul>
                <li>Changed at: {{.ChangedAt}}</li>
                <li>IP Address: {{.IPAddress}}</li>
            </ul>
        </div>
        
        <div class="warning">
            <p><strong>If you didn't make this change:</strong></p>
            <ol>
                <li>Reset your password immediately using the forgot password feature</li>
                <li>Contact our support team</li>
                <li>Review your account activity for any suspicious actions</li>
            </ol>
        </div>
        
        <p>For security reasons, you've been logged out of all devices.</p>
        
        <div class="footer">
            <p>InvoiceFast - Professional Invoicing for African Businesses</p>
        </div>
    </div>
</body>
</html>
`

	body := templateHTML
	body = strings.ReplaceAll(body, "{{.UserName}}", data.UserName)
	body = strings.ReplaceAll(body, "{{.ChangedAt}}", data.ChangedAt.Format("Jan 02, 2006 at 15:04 MST"))
	body = strings.ReplaceAll(body, "{{.IPAddress}}", data.IPAddress)

	req := EmailRequest{
		To:      []string{data.UserEmail},
		Subject: "Your InvoiceFast Password Has Been Changed",
		Body:    body,
		IsHTML:  true,
	}

	return s.Send(req)
}

func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "unknown"
	}
	return parts[1]
}
