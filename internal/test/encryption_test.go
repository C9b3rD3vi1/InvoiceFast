package services_test

import (
	"encoding/base64"
	"os"
	"testing"

	"invoicefast/internal/config"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAuthServiceForEncryption(t *testing.T) *services.AuthService {
	if os.Getenv("ENCRYPTION_KEY") == "" {
		os.Setenv("ENCRYPTION_KEY", "test-encryption-key-for-testing-only-1234567890")
	}
	require.NoError(t, models.InitEncryption(os.Getenv("ENCRYPTION_KEY")))

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-jwt-tokens-min-32-chars-long!!",
		},
	}
	return services.NewAuthService(nil, cfg, nil, nil, nil)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	svc := setupAuthServiceForEncryption(t)
	original := "my-super-secret-data-12345"

	encrypted, err := svc.EncryptSecretForTest(original)
	require.NoError(t, err)
	assert.NotEmpty(t, encrypted)
	assert.NotEqual(t, original, encrypted)

	decrypted, err := svc.DecryptSecretForTest(encrypted)
	require.NoError(t, err)
	assert.Equal(t, original, decrypted)
}

func TestEncryptEmptyString(t *testing.T) {
	svc := setupAuthServiceForEncryption(t)
	result, err := svc.EncryptSecretForTest("")
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestDecryptEmptyString(t *testing.T) {
	svc := setupAuthServiceForEncryption(t)
	result, err := svc.DecryptSecretForTest("")
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestDecryptInvalidBase64(t *testing.T) {
	svc := setupAuthServiceForEncryption(t)
	_, err := svc.DecryptSecretForTest("!!!invalid-base64!!!")
	assert.Error(t, err)
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	svc := setupAuthServiceForEncryption(t)
	original := "sensitive-value"

	encrypted, err := svc.EncryptSecretForTest(original)
	require.NoError(t, err)

	// Decode to bytes, flip a bit in the ciphertext portion, re-encode
	raw, _ := base64.StdEncoding.DecodeString(encrypted)
	nonceSize := 12
	if len(raw) > nonceSize+1 {
		raw[nonceSize] ^= 0x01
	}
	tampered := base64.StdEncoding.EncodeToString(raw)
	_, err = svc.DecryptSecretForTest(tampered)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decryption failed",
		"tampering ciphertext after nonce should trigger GCM auth failure")
}

func TestEncryptionDifferentKeys(t *testing.T) {
	key1 := "key-one-for-encrypt-decrypt-test-min-len-123456789012"
	key2 := "key-two-for-encrypt-decrypt-test-different-1234567890"

	models.InitEncryption(key1)
	encrypted, err := models.EncryptValue("secret-data")
	require.NoError(t, err)

	models.InitEncryption(key2)
	_, err = models.DecryptValue(encrypted)
	assert.Error(t, err, "decrypting with different ENCRYPTION_KEY should fail")
}
