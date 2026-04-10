package models

import (
	"errors"
	"time"
)

// Password reset errors
var (
	ErrTokenExpired        = errors.New("password reset token has expired")
	ErrTokenUsed           = errors.New("password reset token has already been used")
	ErrTokenInvalid        = errors.New("invalid password reset token")
	ErrPasswordWeak        = errors.New("password does not meet strength requirements")
	ErrPasswordCompromised = errors.New("password has been compromised in a data breach")
)

// PasswordResetToken represents a password reset token
type PasswordResetToken struct {
	ID        string     `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID  string     `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID    string     `json:"user_id" gorm:"type:uuid;index;not null"`
	Token     string     `json:"-" gorm:"uniqueIndex;not null"` // Hashed token (stored)
	RawToken  string     `json:"token,omitempty"`               // Raw token (only on creation)
	Email     string     `json:"email" gorm:"index;not null"`
	ExpiresAt time.Time  `json:"expires_at" gorm:"not null"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
	IPAddress string     `json:"ip_address"` // For audit trail
	UserAgent string     `json:"user_agent"`
}

// ResetTokenType is an enum for reset types
type ResetTokenType string

const (
	ResetTypePassword ResetTokenType = "password"
	ResetTypeEmail    ResetTokenType = "email"
)

// IsExpired checks if the token has expired
func (t *PasswordResetToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsUsed checks if the token has been used
func (t *PasswordResetToken) IsUsed() bool {
	return t.UsedAt != nil
}

// CanBeUsed checks if the token can be used
func (t *PasswordResetToken) CanBeUsed() error {
	if t.IsExpired() {
		return ErrTokenExpired
	}
	if t.IsUsed() {
		return ErrTokenUsed
	}
	return nil
}

// EmailVerificationToken represents an email verification token
type EmailVerificationToken struct {
	ID        string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID    string     `json:"user_id" gorm:"type:uuid;index;not null"`
	Token     string     `json:"-" gorm:"uniqueIndex;not null"`
	RawToken  string     `json:"token,omitempty"`
	Email     string     `json:"email" gorm:"index;not null"`
	ExpiresAt time.Time  `json:"expires_at" gorm:"not null"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type MagicLinkToken struct {
	ID        string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID    string     `json:"user_id" gorm:"type:uuid;index;not null"`
	Token     string     `json:"-" gorm:"uniqueIndex;not null"`
	RawToken  string     `json:"token,omitempty"`
	ExpiresAt time.Time  `json:"expires_at" gorm:"not null"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
	Purpose   string     `json:"purpose"` // "login", "password_reset", "email_verify"
}
