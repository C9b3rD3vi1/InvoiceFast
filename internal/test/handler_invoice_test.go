package services_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupInvoiceHandler(t *testing.T) (*handlers.InvoiceHandler, *database.DB, *services.InvoiceService, string) {
	if os.Getenv("ENCRYPTION_KEY") == "" {
		os.Setenv("ENCRYPTION_KEY", "test-encryption-key-for-testing-only-1234567890")
	}
	models.InitEncryption(os.Getenv("ENCRYPTION_KEY"))

	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)
	db := &database.DB{DB: gdb}
	require.NoError(t, db.AutoMigrate(
		&models.User{}, &models.Tenant{}, &models.Client{},
		&models.Invoice{}, &models.InvoiceItem{}, &models.InvoiceSequence{},
		&models.Payment{},
	))

	tenantID := uuid.New().String()
	tenant := &models.Tenant{ID: tenantID, Name: "Test Tenant", Plan: "free"}
	require.NoError(t, db.Create(tenant).Error)

	userID := uuid.New().String()
	user := &models.User{ID: userID, TenantID: tenantID, Email: "owner@test.com", Name: "Owner", Role: "owner"}
	require.NoError(t, db.Create(user).Error)

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret-key-for-jwt-tokens-min-32-chars-long!!"}}
	invoiceSvc := services.NewInvoiceServiceWithDeps(db, &services.ServiceDependencies{
		DB:     db,
		Config: cfg,
	})

	pdfSvc := &services.PDFService{}
	invoiceHandler := handlers.NewInvoiceHandler(invoiceSvc, nil, nil, nil, nil, pdfSvc, nil, nil, nil, nil, nil)

	return invoiceHandler, db, invoiceSvc, tenantID
}

func createTestClient(t *testing.T, db *database.DB, tenantID string) string {
	clientID := uuid.New().String()
	client := &models.Client{
		ID:       clientID,
		TenantID: tenantID,
		Name:     "Test Client",
		Email:    "client@test.com",
		Currency: "KES",
	}
	require.NoError(t, db.Create(client).Error)
	return clientID
}

func invoiceRequest(method, path, body string) *http.Request {
	req, _ := http.NewRequest(method, "http://example.com"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestInvoiceHandlerCreate(t *testing.T) {
	handler, db, _, tenantID := setupInvoiceHandler(t)
	clientID := createTestClient(t, db, tenantID)

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("tenant_id", tenantID)
		c.Locals("user_id", "test-user-id")
		c.Locals("user_role", "owner")
		return c.Next()
	})
	app.Post("/api/v1/tenant/invoices", handler.CreateInvoice)

	resp, err := app.Test(invoiceRequest("POST", "/api/v1/tenant/invoices",
		`{"client_id":"`+clientID+`","currency":"KES","kraPayloadItems":[{"description":"Test Item","quantity":1,"unit_price":100,"tax_rate":16}]}`), 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Equal(t, "KES", body["currency"])
	assert.NotEmpty(t, body["id"])
}

func TestInvoiceHandlerCreateValidation(t *testing.T) {
	handler, db, _, tenantID := setupInvoiceHandler(t)
	clientID := createTestClient(t, db, tenantID)

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("tenant_id", tenantID)
		c.Locals("user_id", "test-user-id")
		return c.Next()
	})
	app.Post("/api/v1/tenant/invoices", handler.CreateInvoice)

	tests := []struct {
		name   string
		body   string
		status int
	}{
		{"missing client", `{"currency":"KES","kraPayloadItems":[]}`, fiber.StatusBadRequest},
		{"invalid currency", `{"client_id":"` + clientID + `","currency":"INVALID","kraPayloadItems":[{"description":"Test","quantity":1,"unit_price":100,"tax_rate":0}]}`, fiber.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := app.Test(invoiceRequest("POST", "/api/v1/tenant/invoices", tt.body), 5000)
			require.NoError(t, err)
			assert.Equal(t, tt.status, resp.StatusCode)
		})
	}
}

func TestInvoiceServiceCreateAndGet(t *testing.T) {
	_, db, invoiceSvc, tenantID := setupInvoiceHandler(t)
	clientID := createTestClient(t, db, tenantID)

	req := &services.CreateInvoiceRequest{
		ClientID: clientID,
		Currency: "KES",
		Items: []services.InvoiceItemRequest{
			{Description: "Service Fee", Quantity: 2, UnitPrice: 150.00, TaxRate: 16},
		},
	}

	invoice, err := invoiceSvc.CreateInvoice(tenantID, "test-user-id", clientID, req)
	require.NoError(t, err)
	assert.NotEmpty(t, invoice.ID)
	assert.Equal(t, "KES", invoice.Currency)
	assert.Equal(t, models.InvoiceStatus("draft"), invoice.Status)
	assert.Equal(t, 2*150.00, invoice.Subtotal)
	assert.InDelta(t, 2*150.00*0.16, invoice.TaxAmount, 0.01)

	loaded, err := invoiceSvc.GetInvoiceByID(tenantID, invoice.ID)
	require.NoError(t, err)
	assert.Equal(t, invoice.ID, loaded.ID)
	assert.Equal(t, 1, len(loaded.Items))
}
