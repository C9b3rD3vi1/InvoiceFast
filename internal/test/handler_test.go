package services_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAuthHandler(t *testing.T) (*handlers.AuthHandler, *database.DB, *services.AuthService) {
	if os.Getenv("ENCRYPTION_KEY") == "" {
		os.Setenv("ENCRYPTION_KEY", "test-encryption-key-for-testing-only-1234567890")
	}
	if err := models.InitEncryption(os.Getenv("ENCRYPTION_KEY")); err != nil {
		t.Fatalf("Failed to init encryption: %v", err)
	}

	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)

	db := &database.DB{DB: gdb}
	require.NoError(t, db.AutoMigrate(&models.User{}))

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-for-jwt-tokens-min-32-chars-long!!",
			AccessTTL:     15 * time.Minute,
			RefreshTTL:    7 * 24 * time.Hour,
			Issuer:        "invoicefast-test",
		},
		Server: config.ServerConfig{Mode: "test"},
		Mail:   config.MailConfig{SMTPHost: ""},
	}

	authSvc := services.NewAuthService(db, cfg, nil, nil, nil)
	auditSvc := services.NewAuditService(db)
	passwordResetSvc := services.NewPasswordResetService(db, cfg, nil)
	authHandler := handlers.NewAuthHandlerWithDeps(authSvc, auditSvc, nil, nil, passwordResetSvc, db)
	return authHandler, db, authSvc
}

func TestAuthHandlerRegister(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)

	payload := `{"email":"test@example.com","password":"Password123!","name":"Test User"}`
	req, _ := http.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.NotEmpty(t, body["token"])
	assert.NotEmpty(t, body["user"])
}

func TestAuthHandlerRegisterDuplicate(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)

	payload := `{"email":"dupe@example.com","password":"Password123!","name":"Test User"}`
	req, _ := http.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	req2, _ := http.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusConflict, resp2.StatusCode)
}

func TestAuthHandlerLogin(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)
	app.Post("/api/v1/auth/login", authHandler.Login)

	payload := `{"email":"login@example.com","password":"Password123!","name":"Test User"}`
	req, _ := http.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	loginPayload := `{"email":"login@example.com","password":"Password123!"}`
	loginReq, _ := http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := app.Test(loginReq, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, loginResp.StatusCode)
}

func TestAuthHandlerLoginWrongPassword(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)
	app.Post("/api/v1/auth/login", authHandler.Login)

	payload := `{"email":"wrongpwd@example.com","password":"Password123!","name":"Test User"}`
	req, _ := http.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	app.Test(req, 5000)

	loginPayload := `{"email":"wrongpwd@example.com","password":"WrongPassword!"}`
	loginReq, _ := http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, _ := app.Test(loginReq, 5000)
	assert.Equal(t, fiber.StatusUnauthorized, loginResp.StatusCode)
}

func TestAuthHandlerRegisterValidation(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)

	tests := []struct {
		name    string
		payload string
		status  int
	}{
		{"missing email", `{"password":"Password123!","name":"Test"}`, fiber.StatusBadRequest},
		{"missing password", `{"email":"test@example.com","name":"Test"}`, fiber.StatusBadRequest},
		{"weak password", `{"email":"test@example.com","password":"short","name":"Test"}`, fiber.StatusBadRequest},
		{"invalid email", `{"email":"notanemail","password":"Password123!","name":"Test"}`, fiber.StatusBadRequest},
		{"empty body", `{}`, fiber.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req, 5000)
			require.NoError(t, err)
			assert.Equal(t, tt.status, resp.StatusCode)
		})
	}
}

func TestAuthHandlerRefreshToken(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)
	app.Post("/api/v1/auth/refresh", authHandler.RefreshToken)

	payload := `{"email":"refresh@example.com","password":"Password123!","name":"Test User"}`
	req, _ := http.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)

	var registerResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&registerResp)
	refreshToken, _ := registerResp["refresh_token"].(string)

	refreshPayload := `{"refresh_token":"` + refreshToken + `"}`
	refreshReq, _ := http.NewRequest("POST", "/api/v1/auth/refresh", strings.NewReader(refreshPayload))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshResp, err := app.Test(refreshReq, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, refreshResp.StatusCode)
}

func TestAuthHandlerInvalidRefreshToken(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/refresh", authHandler.RefreshToken)

	payload := `{"refresh_token":"invalid-token-value"}`
	req, _ := http.NewRequest("POST", "/api/v1/auth/refresh", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}
