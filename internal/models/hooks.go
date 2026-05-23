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
	encryptMu      sync.Mutex
	encryptionKey  string
	cipherInstance cipher.AEAD
)

func InitEncryption(key string) error {
	if len(key) < 32 {
		return errors.New("encryption key must be at least 32 characters")
	}
	encryptMu.Lock()
	defer encryptMu.Unlock()
	encryptionKey = key[:32]
	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}
	cipherInstance, err = cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}
	return nil
}

func mustInitEncryption() {
	if cipherInstance == nil {
		panic("encryption not initialized - call InitEncryption first")
	}
}

func encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	mustInitEncryption()

	nonce := make([]byte, cipherInstance.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := cipherInstance.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	mustInitEncryption()

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	nonceSize := cipherInstance.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := cipherInstance.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}

// EncryptValue provides public encryption using AES-256-GCM
func EncryptValue(plaintext string) (string, error) {
	return encrypt(plaintext)
}

// DecryptValue provides public decryption using AES-256-GCM
func DecryptValue(ciphertext string) (string, error) {
	return decrypt(ciphertext)
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	// Encrypt sensitive fields before saving
	if u.KRAPIN != "" {
		enc, err := encrypt(u.KRAPIN)
		if err != nil {
			return fmt.Errorf("failed to encrypt KRA PIN: %w", err)
		}
		u.KRAPIN = enc
	}
	if u.Phone != "" {
		enc, err := encrypt(u.Phone)
		if err != nil {
			return fmt.Errorf("failed to encrypt phone: %w", err)
		}
		u.Phone = enc
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
				enc, err := encrypt(u.KRAPIN)
				if err != nil {
					return fmt.Errorf("failed to encrypt KRA PIN: %w", err)
				}
				u.KRAPIN = enc
			}
		}
		if u.Phone != "" && u.Phone != existing.Phone {
			if _, err := base64.StdEncoding.DecodeString(existing.Phone); err != nil {
				enc, err := encrypt(u.Phone)
				if err != nil {
					return fmt.Errorf("failed to encrypt phone: %w", err)
				}
				u.Phone = enc
			}
		}
	}
	return nil
}

func (u *User) AfterFind(tx *gorm.DB) error {
	// Decrypt sensitive fields after reading
	if u.KRAPIN != "" {
		dec, err := decrypt(u.KRAPIN)
		if err != nil {
			return fmt.Errorf("failed to decrypt KRA PIN: %w", err)
		}
		u.KRAPIN = dec
	}
	if u.Phone != "" {
		dec, err := decrypt(u.Phone)
		if err != nil {
			return fmt.Errorf("failed to decrypt phone: %w", err)
		}
		u.Phone = dec
	}
	return nil
}

func (c *Client) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	// Encrypt sensitive fields
	if c.KRAPIN != "" {
		enc, err := encrypt(c.KRAPIN)
		if err != nil {
			return fmt.Errorf("failed to encrypt KRA PIN: %w", err)
		}
		c.KRAPIN = enc
	}
	if c.Phone != "" {
		enc, err := encrypt(c.Phone)
		if err != nil {
			return fmt.Errorf("failed to encrypt phone: %w", err)
		}
		c.Phone = enc
	}
	if c.Email != "" {
		enc, err := encrypt(c.Email)
		if err != nil {
			return fmt.Errorf("failed to encrypt email: %w", err)
		}
		c.Email = enc
	}
	return nil
}

func (c *Client) BeforeUpdate(tx *gorm.DB) error {
	var existing Client
	if err := tx.First(&existing, "id = ?", c.ID).Error; err == nil {
		if c.KRAPIN != "" && c.KRAPIN != existing.KRAPIN {
			if _, err := base64.StdEncoding.DecodeString(existing.KRAPIN); err != nil {
				enc, err := encrypt(c.KRAPIN)
				if err != nil {
					return fmt.Errorf("failed to encrypt KRA PIN: %w", err)
				}
				c.KRAPIN = enc
			}
		}
		if c.Phone != "" && c.Phone != existing.Phone {
			if _, err := base64.StdEncoding.DecodeString(existing.Phone); err != nil {
				enc, err := encrypt(c.Phone)
				if err != nil {
					return fmt.Errorf("failed to encrypt phone: %w", err)
				}
				c.Phone = enc
			}
		}
		if c.Email != "" && c.Email != existing.Email {
			if _, err := base64.StdEncoding.DecodeString(existing.Email); err != nil {
				enc, err := encrypt(c.Email)
				if err != nil {
					return fmt.Errorf("failed to encrypt email: %w", err)
				}
				c.Email = enc
			}
		}
	}
	return nil
}

func (c *Client) AfterFind(tx *gorm.DB) error {
	// Decrypt sensitive fields
	if c.KRAPIN != "" {
		dec, err := decrypt(c.KRAPIN)
		if err != nil {
			return fmt.Errorf("failed to decrypt KRA PIN: %w", err)
		}
		c.KRAPIN = dec
	}
	if c.Phone != "" {
		dec, err := decrypt(c.Phone)
		if err != nil {
			return fmt.Errorf("failed to decrypt phone: %w", err)
		}
		c.Phone = dec
	}
	if c.Email != "" {
		dec, err := decrypt(c.Email)
		if err != nil {
			return fmt.Errorf("failed to decrypt email: %w", err)
		}
		c.Email = dec
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
		enc, err := encrypt(p.PhoneNumber)
		if err != nil {
			return fmt.Errorf("failed to encrypt phone: %w", err)
		}
		p.PhoneNumber = enc
	}
	if p.CustomerEmail != "" {
		enc, err := encrypt(p.CustomerEmail)
		if err != nil {
			return fmt.Errorf("failed to encrypt email: %w", err)
		}
		p.CustomerEmail = enc
	}
	return nil
}

func (p *Payment) AfterFind(tx *gorm.DB) error {
	// Decrypt payment PII
	if p.PhoneNumber != "" {
		dec, err := decrypt(p.PhoneNumber)
		if err != nil {
			return fmt.Errorf("failed to decrypt phone: %w", err)
		}
		p.PhoneNumber = dec
	}
	if p.CustomerEmail != "" {
		dec, err := decrypt(p.CustomerEmail)
		if err != nil {
			return fmt.Errorf("failed to decrypt email: %w", err)
		}
		p.CustomerEmail = dec
	}
	return nil
}

func generateInvoiceNumber() string {
	return "INV-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:4]
}
