package services_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/models"
	"invoicefast/internal/routes"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOnboardingTest(t *testing.T) (*fiber.App, *handlers.OnboardingHandler, *database.DB, *services.AuthService) {
	if os.Getenv("ENCRYPTION_KEY") == "" {
		os.Setenv("ENCRYPTION_KEY", "test-encryption-key-32-bytes-long!!")
	}
	if err := models.InitEncryption(os.Getenv("ENCRYPTION_KEY")); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)

	db := &database.DB{DB: gdb}
	err = db.Migrate()
	require.NoError(t, err)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Mode: "development",
		},
		JWT: config.JWTConfig{
			Secret:        "test-jwt-secret-at-least-32-bytes-long!!",
			Expiry:        1 * time.Hour,
			RefreshExpiry: 72 * time.Hour,
		},
	}

	authService := services.NewAuthService(db, cfg, nil, nil, nil)
	invoiceService := services.NewInvoiceService(db)
	clientService := services.NewClientService(db)
	settingsService := services.NewSettingsService(db)

	handler := handlers.NewOnboardingHandler(authService, invoiceService, clientService, settingsService, db)

	app := fiber.New(fiber.Config{})
	routes.OnboardingRoutes(app, handler, authService, db)

	return app, handler, db, authService
}

func getBody(resp *http.Response) map[string]interface{} {
	var result map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	json.Unmarshal(body, &result)
	return result
}

func TestOnboardingRegister(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	body := `{"email":"test@example.com","password":"SecurePass123!"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	data := getBody(resp)
	assert.Contains(t, data, "access_token")
	assert.Contains(t, data, "refresh_token")
	assert.NotEmpty(t, data["access_token"])
	assert.NotEmpty(t, data["refresh_token"])

	user, ok := data["user"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test@example.com", user["email"])
}

func TestOnboardingRegisterDuplicate(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	body := `{"email":"dup@example.com","password":"SecurePass123!"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	_, err := app.Test(req)
	require.NoError(t, err)

	req2 := httptest.NewRequest("POST", "/api/v1/onboarding/register", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp2.StatusCode)
}

func TestOnboardingRegisterInvalidEmail(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	body := `{"email":"not-an-email","password":"SecurePass123!"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestOnboardingRegisterWeakPassword(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	body := `{"email":"weak@example.com","password":"123"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestOnboardingLogin(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	body := `{"email":"login-test@example.com","password":"SecurePass123!"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	_, err := app.Test(req)
	require.NoError(t, err)

	loginBody := `{"email":"login-test@example.com","password":"SecurePass123!"}`
	req2 := httptest.NewRequest("POST", "/api/v1/onboarding/login", strings.NewReader(loginBody))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)

	data := getBody(resp2)
	assert.Contains(t, data, "access_token")
	assert.Contains(t, data, "user")
}

func TestOnboardingLoginWrongPassword(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	body := `{"email":"login-wrong@example.com","password":"SecurePass123!"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	_, err := app.Test(req)
	require.NoError(t, err)

	loginBody := `{"email":"login-wrong@example.com","password":"WrongPassword!"}`
	req2 := httptest.NewRequest("POST", "/api/v1/onboarding/login", strings.NewReader(loginBody))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp2.StatusCode)
}

func registerAndGetToken(t *testing.T, app *fiber.App, email, password string) (string, string, string) {
	body := `{"email":"` + email + `","password":"` + password + `"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, resp.StatusCode)

	data := getBody(resp)
	token := data["access_token"].(string)
	user := data["user"].(map[string]interface{})
	return token, user["id"].(string), data["refresh_token"].(string)
}

func TestOnboardingVerifyEmail(t *testing.T) {
	app, _, db, _ := setupOnboardingTest(t)

	token, userID, _ := registerAndGetToken(t, app, "verify@example.com", "SecurePass123!")

	var tok models.EmailVerificationToken
	err := db.Where("user_id = ? AND used_at IS NULL", userID).Order("created_at DESC").First(&tok).Error
	require.NoError(t, err, "verification token should exist")
	require.NotEmpty(t, tok.RawToken, "raw token should be stored for test")

	body := `{"code":"` + tok.RawToken + `"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/verify-email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	data := getBody(resp)
	assert.Equal(t, "verified", data["status"])
}

func TestOnboardingVerifyEmailWrongCode(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	token, _, _ := registerAndGetToken(t, app, "verify-wrong@example.com", "SecurePass123!")

	body := `{"code":"000000"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/verify-email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestOnboardingVerifyEmailNoAuth(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	body := `{"code":"123456"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/verify-email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestOnboardingResendCode(t *testing.T) {
	app, _, db, _ := setupOnboardingTest(t)

	token, userID, _ := registerAndGetToken(t, app, "resend@example.com", "SecurePass123!")

	var original models.EmailVerificationToken
	err := db.Where("user_id = ? AND used_at IS NULL", userID).Order("created_at DESC").First(&original).Error
	require.NoError(t, err)

	body := `{"email":"resend@example.com"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/resend-code", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var newToken models.EmailVerificationToken
	err = db.Where("user_id = ? AND used_at IS NULL", userID).Order("created_at DESC").First(&newToken).Error
	require.NoError(t, err)
	assert.NotEqual(t, original.ID, newToken.ID, "a new token should have been created")
}

func TestOnboardingEmailStatusUnauthenticated(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	req := httptest.NewRequest("GET", "/api/v1/onboarding/email-status", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestOnboardingEmailStatusBeforeVerification(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	token, _, _ := registerAndGetToken(t, app, "status-before@example.com", "SecurePass123!")

	req := httptest.NewRequest("GET", "/api/v1/onboarding/email-status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	data := getBody(resp)
	assert.Equal(t, false, data["email_verified"])
	assert.Equal(t, "status-before@example.com", data["email"])
}

func TestOnboardingEmailStatusAfterVerification(t *testing.T) {
	app, _, db, _ := setupOnboardingTest(t)

	token, userID, _ := registerAndGetToken(t, app, "status-after@example.com", "SecurePass123!")

	var tok models.EmailVerificationToken
	err := db.Where("user_id = ? AND used_at IS NULL", userID).Order("created_at DESC").First(&tok).Error
	require.NoError(t, err)

	verifyBody := `{"code":"` + tok.RawToken + `"}`
	verifyReq := httptest.NewRequest("POST", "/api/v1/onboarding/verify-email", strings.NewReader(verifyBody))
	verifyReq.Header.Set("Content-Type", "application/json")
	verifyReq.Header.Set("Authorization", "Bearer "+token)
	_, err = app.Test(verifyReq)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/api/v1/onboarding/email-status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	data := getBody(resp)
	assert.Equal(t, true, data["email_verified"])
	assert.Equal(t, "status-after@example.com", data["email"])
}

func TestOnboardingBusinessProfile(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	token, _, _ := registerAndGetToken(t, app, "biz@example.com", "SecurePass123!")

	body := `{"business_name":"Test Corp","business_type":"technology"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/business-profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	data := getBody(resp)
	assert.Equal(t, "saved", data["status"])
}

func TestOnboardingBusinessProfileEmptyName(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	token, _, _ := registerAndGetToken(t, app, "biz-empty@example.com", "SecurePass123!")

	body := `{"business_name":"","business_type":"technology"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/business-profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestOnboardingCreateInvoice(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	token, _, _ := registerAndGetToken(t, app, "invoice@example.com", "SecurePass123!")

	body := `{"client_name":"Acme Corp","client_email":"acme@example.com","description":"Web development","item_desc":"Website design","amount":50000,"currency":"KES"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/create-invoice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	data := getBody(resp)
	assert.Contains(t, data, "invoice_id")
	assert.Contains(t, data, "invoice_number")
	assert.Contains(t, data, "client_id")
	assert.Equal(t, "Acme Corp", data["client_name"])
	assert.Equal(t, float64(50000), data["amount"])
	assert.Equal(t, "KES", data["currency"])
}

func TestOnboardingCreateInvoiceNoClientName(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	token, _, _ := registerAndGetToken(t, app, "invoice-no-name@example.com", "SecurePass123!")

	body := `{"client_name":"","client_email":"","description":"test","item_desc":"test","amount":1000,"currency":"KES"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/create-invoice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestOnboardingCreateInvoiceZeroAmount(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	token, _, _ := registerAndGetToken(t, app, "invoice-zero@example.com", "SecurePass123!")

	body := `{"client_name":"Test Client","client_email":"","description":"test","item_desc":"test","amount":0,"currency":"KES"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/create-invoice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestOnboardingSavePayment(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	token, _, _ := registerAndGetToken(t, app, "payment@example.com", "SecurePass123!")

	body := `{"paybill_number":"247247","account_number":"INV123"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/save-payment", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	data := getBody(resp)
	assert.Equal(t, "saved", data["status"])
}

func TestOnboardingSavePaymentEmptyPaybill(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	token, _, _ := registerAndGetToken(t, app, "payment-empty@example.com", "SecurePass123!")

	body := `{"paybill_number":"","account_number":"INV123"}`
	req := httptest.NewRequest("POST", "/api/v1/onboarding/save-payment", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestOnboardingUnauthenticatedEndpoints(t *testing.T) {
	app, _, _, _ := setupOnboardingTest(t)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/api/v1/onboarding/verify-email", `{"code":"123456"}`},
		{"POST", "/api/v1/onboarding/resend-code", `{"email":"test@example.com"}`},
		{"POST", "/api/v1/onboarding/business-profile", `{"business_name":"Test"}`},
		{"POST", "/api/v1/onboarding/create-invoice", `{"client_name":"Test"}`},
		{"POST", "/api/v1/onboarding/save-payment", `{"paybill_number":"247247"}`},
		{"GET", "/api/v1/onboarding/email-status", ""},
	}

	for _, ep := range endpoints {
		var req *http.Request
		if ep.body != "" {
			req = httptest.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
		} else {
			req = httptest.NewRequest(ep.method, ep.path, nil)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		require.NoError(t, err, "request to %s %s failed", ep.method, ep.path)
		assert.Equal(t, fiber.StatusForbidden, resp.StatusCode,
			"%s %s should return 403 without auth", ep.method, ep.path)
	}
}

func TestOnboardingFullFlow(t *testing.T) {
	app, _, db, _ := setupOnboardingTest(t)

	token, userID, _ := registerAndGetToken(t, app, "full-flow@example.com", "SecurePass123!")

	var tok models.EmailVerificationToken
	err := db.Where("user_id = ? AND used_at IS NULL", userID).Order("created_at DESC").First(&tok).Error
	require.NoError(t, err)

	verifyBody := `{"code":"` + tok.RawToken + `"}`
	verifyReq := httptest.NewRequest("POST", "/api/v1/onboarding/verify-email", strings.NewReader(verifyBody))
	verifyReq.Header.Set("Content-Type", "application/json")
	verifyReq.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(verifyReq)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	emailStatusReq := httptest.NewRequest("GET", "/api/v1/onboarding/email-status", nil)
	emailStatusReq.Header.Set("Authorization", "Bearer "+token)
	resp, err = app.Test(emailStatusReq)
	require.NoError(t, err)
	data := getBody(resp)
	assert.Equal(t, true, data["email_verified"])

	bizBody := `{"business_name":"Full Flow Corp","business_type":"consulting"}`
	bizReq := httptest.NewRequest("POST", "/api/v1/onboarding/business-profile", strings.NewReader(bizBody))
	bizReq.Header.Set("Content-Type", "application/json")
	bizReq.Header.Set("Authorization", "Bearer "+token)
	resp, err = app.Test(bizReq)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	invBody := `{"client_name":"Flow Client","client_email":"client@flow.com","description":"Consulting","item_desc":"Strategy session","amount":75000,"currency":"KES"}`
	invReq := httptest.NewRequest("POST", "/api/v1/onboarding/create-invoice", strings.NewReader(invBody))
	invReq.Header.Set("Content-Type", "application/json")
	invReq.Header.Set("Authorization", "Bearer "+token)
	resp, err = app.Test(invReq)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
	invData := getBody(resp)
	assert.Contains(t, invData, "invoice_id")

	payBody := `{"paybill_number":"123456","account_number":"FLOW001"}`
	payReq := httptest.NewRequest("POST", "/api/v1/onboarding/save-payment", strings.NewReader(payBody))
	payReq.Header.Set("Content-Type", "application/json")
	payReq.Header.Set("Authorization", "Bearer "+token)
	resp, err = app.Test(payReq)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}
