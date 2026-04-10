package models

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	encryptOnce    sync.Once
	encryptionKey  string
	cipherInstance cipher.AEAD
)

func InitEncryption(key string) error {
	var initErr error
	encryptOnce.Do(func() {
		if len(key) < 32 {
			initErr = errors.New("encryption key must be at least 32 characters")
			return
		}
		encryptionKey = key[:32]
		block, err := aes.NewCipher([]byte(encryptionKey))
		if err != nil {
			initErr = fmt.Errorf("failed to create cipher: %w", err)
			return
		}
		cipherInstance, err = cipher.NewGCM(block)
		if err != nil {
			initErr = fmt.Errorf("failed to create GCM: %w", err)
			return
		}
	})
	return initErr
}

func mustInitEncryption() {
	if cipherInstance == nil {
		panic("encryption not initialized - call InitEncryption first")
	}
}

func encrypt(plaintext string) string {
	if plaintext == "" {
		return ""
	}
	mustInitEncryption()

	nonce := make([]byte, cipherInstance.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plaintext
	}

	ciphertext := cipherInstance.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

func decrypt(ciphertext string) string {
	if ciphertext == "" {
		return ""
	}
	mustInitEncryption()

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return ciphertext
	}

	nonceSize := cipherInstance.NonceSize()
	if len(data) < nonceSize {
		return ciphertext
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := cipherInstance.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return ciphertext
	}

	return string(plaintext)
}

// EncryptValue provides public encryption using AES-256-GCM
func EncryptValue(plaintext string) string {
	return encrypt(plaintext)
}

// DecryptValue provides public decryption using AES-256-GCM
func DecryptValue(ciphertext string) string {
	return decrypt(ciphertext)
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	// Encrypt sensitive fields before saving
	if u.KRAPIN != "" {
		u.KRAPIN = encrypt(u.KRAPIN)
	}
	if u.Phone != "" {
		u.Phone = encrypt(u.Phone)
	}
	return nil
}

func (u *User) BeforeUpdate(tx *gorm.DB) error {
	// Get current values from database to compare
	var existing User
	if err := tx.First(&existing, "id = ?", u.ID).Error; err == nil {
		// Only encrypt if value changed
		if u.KRAPIN != "" && u.KRAPIN != existing.KRAPIN {
			// Check if already encrypted to avoid double encryption
			if _, err := base64.StdEncoding.DecodeString(existing.KRAPIN); err != nil {
				u.KRAPIN = encrypt(u.KRAPIN)
			}
		}
		if u.Phone != "" && u.Phone != existing.Phone {
			if _, err := base64.StdEncoding.DecodeString(existing.Phone); err != nil {
				u.Phone = encrypt(u.Phone)
			}
		}
	}
	return nil
}

func (u *User) AfterFind(tx *gorm.DB) error {
	// Decrypt sensitive fields after reading
	if u.KRAPIN != "" {
		u.KRAPIN = decrypt(u.KRAPIN)
	}
	if u.Phone != "" {
		u.Phone = decrypt(u.Phone)
	}
	return nil
}

func (c *Client) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	// Encrypt sensitive fields
	if c.KRAPIN != "" {
		c.KRAPIN = encrypt(c.KRAPIN)
	}
	if c.Phone != "" {
		c.Phone = encrypt(c.Phone)
	}
	if c.Email != "" {
		c.Email = encrypt(c.Email)
	}
	return nil
}

func (c *Client) BeforeUpdate(tx *gorm.DB) error {
	var existing Client
	if err := tx.First(&existing, "id = ?", c.ID).Error; err == nil {
		if c.KRAPIN != "" && c.KRAPIN != existing.KRAPIN {
			if _, err := base64.StdEncoding.DecodeString(existing.KRAPIN); err != nil {
				c.KRAPIN = encrypt(c.KRAPIN)
			}
		}
		if c.Phone != "" && c.Phone != existing.Phone {
			if _, err := base64.StdEncoding.DecodeString(existing.Phone); err != nil {
				c.Phone = encrypt(c.Phone)
			}
		}
		if c.Email != "" && c.Email != existing.Email {
			if _, err := base64.StdEncoding.DecodeString(existing.Email); err != nil {
				c.Email = encrypt(c.Email)
			}
		}
	}
	return nil
}

func (c *Client) AfterFind(tx *gorm.DB) error {
	// Decrypt sensitive fields
	if c.KRAPIN != "" {
		c.KRAPIN = decrypt(c.KRAPIN)
	}
	if c.Phone != "" {
		c.Phone = decrypt(c.Phone)
	}
	if c.Email != "" {
		c.Email = decrypt(c.Email)
	}
	return nil
}

func (i *Invoice) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	if i.InvoiceNumber == "" {
		i.InvoiceNumber = generateInvoiceNumber()
	}
	if i.MagicToken == "" {
		i.MagicToken = uuid.New().String()
	}
	// Store KES equivalent for dual display
	if i.Currency != "KES" && i.KESEquivalent == 0 && i.Total > 0 {
		// Will be calculated by service layer
	}
	return nil
}

func (i *InvoiceItem) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	return nil
}

func (p *Payment) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	// Encrypt phone number for payment records
	if p.PhoneNumber != "" {
		p.PhoneNumber = encrypt(p.PhoneNumber)
	}
	if p.CustomerEmail != "" {
		p.CustomerEmail = encrypt(p.CustomerEmail)
	}
	return nil
}

func (p *Payment) AfterFind(tx *gorm.DB) error {
	// Decrypt payment PII
	if p.PhoneNumber != "" {
		p.PhoneNumber = decrypt(p.PhoneNumber)
	}
	if p.CustomerEmail != "" {
		p.CustomerEmail = decrypt(p.CustomerEmail)
	}
	return nil
}

func generateInvoiceNumber() string {
	return "INV-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:4]
}
