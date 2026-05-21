package services_test

import (
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
	_, err := svc.EncryptSecretForTest("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot encrypt empty")
}

func TestDecryptEmptyString(t *testing.T) {
	svc := setupAuthServiceForEncryption(t)
	_, err := svc.DecryptSecretForTest("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot decrypt empty")
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

	tampered := encrypted[:len(encrypted)-1] + "X"
	_, err = svc.DecryptSecretForTest(tampered)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decryption failed")
}

func TestEncryptionDifferentKeys(t *testing.T) {
	if os.Getenv("ENCRYPTION_KEY") == "" {
		os.Setenv("ENCRYPTION_KEY", "test-encryption-key-for-testing-only-1234567890")
	}
	models.InitEncryption(os.Getenv("ENCRYPTION_KEY"))

	cfg1 := &config.Config{
		JWT: config.JWTConfig{Secret: "key-one-for-encrypt-decrypt-test-min-len!!"},
	}
	cfg2 := &config.Config{
		JWT: config.JWTConfig{Secret: "key-two-for-encrypt-decrypt-test-different!!"},
	}

	svc1 := services.NewAuthService(nil, cfg1, nil, nil, nil)
	svc2 := services.NewAuthService(nil, cfg2, nil, nil, nil)

	encrypted, err := svc1.EncryptSecretForTest("secret-data")
	require.NoError(t, err)

	_, err = svc2.DecryptSecretForTest(encrypted)
	assert.Error(t, err, "decrypting with different key should fail")
}
