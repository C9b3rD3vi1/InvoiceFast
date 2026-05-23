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
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Tenant{}, &models.RefreshToken{}))

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-for-jwt-tokens-min-32-chars-long!!",
			Expiry:        15 * time.Minute,
			RefreshExpiry: 7 * 24 * time.Hour,
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

func newRequest(method, path, body string) *http.Request {
	req, _ := http.NewRequest(method, "http://example.com"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestAuthHandlerRegister(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)

	resp, err := app.Test(newRequest("POST", "/api/v1/auth/register", `{"email":"test@example.com","password":"Password123!","name":"Test User"}`), 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.NotEmpty(t, body["access_token"])
	assert.NotEmpty(t, body["user"])
}

func TestAuthHandlerRegisterDuplicate(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)

	payload := `{"email":"dupe@example.com","password":"Password123!","name":"Test User"}`
	resp, err := app.Test(newRequest("POST", "/api/v1/auth/register", payload), 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	resp2, err := app.Test(newRequest("POST", "/api/v1/auth/register", payload), 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusConflict, resp2.StatusCode)
}

func TestAuthHandlerLogin(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)
	app.Post("/api/v1/auth/login", authHandler.Login)

	payload := `{"email":"login@example.com","password":"Password123!","name":"Test User"}`
	resp, err := app.Test(newRequest("POST", "/api/v1/auth/register", payload), 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	loginResp, err := app.Test(newRequest("POST", "/api/v1/auth/login", `{"email":"login@example.com","password":"Password123!"}`), 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, loginResp.StatusCode)
}

func TestAuthHandlerLoginWrongPassword(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)
	app.Post("/api/v1/auth/login", authHandler.Login)

	app.Test(newRequest("POST", "/api/v1/auth/register", `{"email":"wrongpwd@example.com","password":"Password123!","name":"Test User"}`), 5000)

	loginResp, _ := app.Test(newRequest("POST", "/api/v1/auth/login", `{"email":"wrongpwd@example.com","password":"WrongPassword!"}`), 5000)
	assert.Equal(t, fiber.StatusUnauthorized, loginResp.StatusCode)
}

func TestAuthHandlerRegisterValidation(t *testing.T) {
	app := fiber.New()
	authHandler, _, _ := setupAuthHandler(t)
	app.Post("/api/v1/auth/register", authHandler.Register)

	tests := []struct {
		name    string
		body    string
		status  int
	}{
		{"missing email", `{"password":"Password123!","name":"Test"}`, fiber.StatusBadRequest},
		{"missing password", `{"email":"test@example.com","name":"Test"}`, fiber.StatusBadRequest},
		{"weak password", `{"email":"test@example.com","password":"short","name":"Test"}`, fiber.StatusBadRequest},
		{"invalid email", `{"email":"notanemail","password":"Password123!","name":"Test"}`, fiber.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := app.Test(newRequest("POST", "/api/v1/auth/register", tt.body), 5000)
			require.NoError(t, err)
			assert.Equal(t, tt.status, resp.StatusCode)
		})
	}
}
