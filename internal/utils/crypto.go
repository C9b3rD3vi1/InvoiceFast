package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	instance *AESCrypt
	once     sync.Once
)

type AESCrypt struct {
	key []byte
	mu  sync.RWMutex
}

func InitAESCrypt(key string) error {
	var initErr error
	once.Do(func() {
		if len(key) < 32 {
			initErr = errors.New("encryption key must be at least 32 characters")
			return
		}
		instance = &AESCrypt{key: []byte(key[:32])}
	})
	return initErr
}

func GetAESCrypt() *AESCrypt {
	return instance
}

func MustInitAESCrypt() {
	err := InitAESCrypt(os.Getenv("ENCRYPTION_KEY"))
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize encryption: %v", err))
	}
}

func (e *AESCrypt) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (e *AESCrypt) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode: %w", err)
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

func EncryptKRAPIN(pin string) string {
	if pin == "" || len(pin) < 5 {
		return pin
	}
	crypt := GetAESCrypt()
	if crypt == nil {
		return pin
	}
	encrypted, _ := crypt.Encrypt(pin)
	return encrypted
}

func DecryptKRAPIN(encryptedPIN string) string {
	if encryptedPIN == "" {
		return ""
	}
	crypt := GetAESCrypt()
	if crypt == nil {
		return encryptedPIN
	}
	decrypted, _ := crypt.Decrypt(encryptedPIN)
	if decrypted == "" {
		return encryptedPIN
	}
	return decrypted
}
