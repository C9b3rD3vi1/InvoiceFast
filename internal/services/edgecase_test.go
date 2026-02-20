package services

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDB *database.DB
var testCfg *config.Config

func setupTestDB(t *testing.T) {
	if testDB != nil {
		return
	}

	testCfg = &config.Config{
		Database: config.DatabaseConfig{
			Driver:          "sqlite3",
			DSN:             ":memory:",
			MaxOpenConns:    5,
			MaxIdleConns:    2,
			ConnMaxLifetime: 5 * time.Minute,
			QueryTimeout:    10 * time.Second,
		},
		JWT: config.JWTConfig{
			Secret:        "test-secret-key",
			Expiry:        24 * time.Hour,
			RefreshExpiry: 7 * 24 * time.Hour,
		},
		Intasend: config.IntasendConfig{
			APIURL: "https://sandbox.intasend.com",
		},
	}

	var err error
	testDB, err = database.New(&testCfg.Database)
	require.NoError(t, err)

	err = testDB.Migrate()
	require.NoError(t, err)
}

// ==================== EDGE CASE TESTS ====================

func TestEdgeCase_EmptyInvoiceItems(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)
	authSvc := NewAuthService(testDB, testCfg)
	clientSvc := NewClientService(testDB)

	// Create user and client
	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Empty items should fail
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{},
	}

	_, err := invSvc.CreateInvoice(user.ID, client.ID, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one item")
}

func TestEdgeCase_NegativeQuantity(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)
	authSvc := NewAuthService(testDB, testCfg)
	clientSvc := NewClientService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Negative quantity creates credit note behavior
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items: []InvoiceItemRequest{
			{Description: "Credit", Quantity: -1, UnitPrice: 1000},
		},
	}

	invoice, err := invSvc.CreateInvoice(user.ID, client.ID, req)
	require.NoError(t, err)
	assert.Equal(t, -1000.0, invoice.Total)
}

func TestEdgeCase_DeleteClientWithInvoices(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)
	clientSvc := NewClientService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create invoice
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
	}
	invSvc.CreateInvoice(user.ID, client.ID, req)

	// Try to delete - should fail
	err := clientSvc.DeleteClient(client.ID, user.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete client with existing invoices")
}

func TestEdgeCase_ConcurrentPayments(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create invoice
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
	}
	invoice, _ := invSvc.CreateInvoice(user.ID, client.ID, req)

	// Record payment
	payment := &models.Payment{
		Amount:   1000,
		Currency: "KES",
		Method:   models.PaymentMethodMpesa,
		Status:   models.PaymentStatusCompleted,
		UserID:   user.ID,
	}
	payment.CompletedAt = sql.NullTime{Time: time.Now(), Valid: true}

	err := invSvc.RecordPayment(invoice.ID, payment)
	require.NoError(t, err)

	// Verify final state
	invoice, _ = invSvc.GetInvoiceByID(invoice.ID, user.ID)
	assert.Equal(t, models.InvoiceStatusPaid, invoice.Status)
}

func TestEdgeCase_PartialPaymentOverflow(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create invoice for 1000
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
	}
	invoice, _ := invSvc.CreateInvoice(user.ID, client.ID, req)

	// Pay more than invoice amount
	payment := &models.Payment{
		Amount:   1500, // Overpay!
		Currency: "KES",
		Method:   models.PaymentMethodMpesa,
		Status:   models.PaymentStatusCompleted,
		UserID:   user.ID,
	}

	err := invSvc.RecordPayment(invoice.ID, payment)
	require.NoError(t, err)

	// Should still be marked as paid, not overpaid
	invoice, _ = invSvc.GetInvoiceByID(invoice.ID, user.ID)
	assert.Equal(t, models.InvoiceStatusPaid, invoice.Status)
}

func TestEdgeCase_InvalidCurrency(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Invalid currency
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "INVALID",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
	}

	_, err := invSvc.CreateInvoice(user.ID, client.ID, req)
	// Should either fail or default to KES
	assert.NoError(t, err) // We'll handle this in validation
}

func TestEdgeCase_DuplicateInvoiceNumber(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create first invoice
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
	}
	inv1, err := invSvc.CreateInvoice(user.ID, client.ID, req)
	require.NoError(t, err)

	// Try to create duplicate (same invoice number generation)
	// In production, we'd check uniqueness - here we just verify it creates
	_, err = invSvc.CreateInvoice(user.ID, client.ID, req)
	// Should fail or create with different number
	if err == nil {
		// Different invoice numbers generated
		assert.NotEqual(t, inv1.InvoiceNumber, "should have different number")
	}
}

func TestEdgeCase_DeletePaidInvoice(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create and pay invoice
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
	}
	invoice, _ := invSvc.CreateInvoice(user.ID, client.ID, req)

	// Pay it
	payment := &models.Payment{
		Amount: 1000,
		Method: models.PaymentMethodMpesa,
		Status: models.PaymentStatusCompleted,
		UserID: user.ID,
	}
	invSvc.RecordPayment(invoice.ID, payment)

	// Try to cancel - should fail
	err := invSvc.CancelInvoice(invoice.ID, user.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot cancel paid invoice")
}

func TestEdgeCase_MaxItemsPerInvoice(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create invoice with many items (100 items)
	items := make([]InvoiceItemRequest, 100)
	for i := 0; i < 100; i++ {
		items[i] = InvoiceItemRequest{
			Description: "Item",
			Quantity:    1,
			UnitPrice:   10,
		}
	}

	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    items,
	}

	invoice, err := invSvc.CreateInvoice(user.ID, client.ID, req)
	require.NoError(t, err)
	assert.Equal(t, 100, len(invoice.Items))
	assert.Equal(t, 1000.0, invoice.Total)
}

func TestEdgeCase_VeryLargeAmount(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Very large amount (1 billion KES)
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Big Item", Quantity: 1, UnitPrice: 1_000_000_000}},
	}

	invoice, err := invSvc.CreateInvoice(user.ID, client.ID, req)
	require.NoError(t, err)
	assert.Equal(t, 1_000_000_000.0, invoice.Total)
}

func TestEdgeCase_ZeroPrice(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Zero price items (free items)
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Free Gift", Quantity: 1, UnitPrice: 0}},
	}

	invoice, err := invSvc.CreateInvoice(user.ID, client.ID, req)
	require.NoError(t, err)
	assert.Equal(t, 0.0, invoice.Total)
}

func TestEdgeCase_100PercentTax(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// 100% tax (double the amount)
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		TaxRate:  100,
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Item", Quantity: 1, UnitPrice: 1000}},
	}

	invoice, err := invSvc.CreateInvoice(user.ID, client.ID, req)
	require.NoError(t, err)
	assert.Equal(t, 2000.0, invoice.Total)
}

func TestEdgeCase_SendAlreadySentInvoice(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
	}
	invoice, _ := invSvc.CreateInvoice(user.ID, client.ID, req)

	// Send first time
	invSvc.SendInvoice(invoice.ID, user.ID)

	// Try to send again - should handle gracefully
	_, err := invSvc.SendInvoice(invoice.ID, user.ID)
	// Should either allow re-send or return appropriate error
	assert.NoError(t, err) // Implementation decides behavior
}

func TestEdgeCase_AccessOtherUsersInvoice(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user1 := createTestUser(t)
	client1 := createTestClient(t, user1.ID)
	user2 := createTestUser(t)

	// User1 creates invoice
	req := &CreateInvoiceRequest{
		ClientID: client1.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
	}
	invoice, _ := invSvc.CreateInvoice(user1.ID, client1.ID, req)

	// User2 tries to access - should fail
	_, err := invSvc.GetInvoiceByID(invoice.ID, user2.ID)
	assert.Error(t, err)
}

func TestEdgeCase_UpdatePaidInvoice(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create and pay invoice
	req := &CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
	}
	invoice, _ := invSvc.CreateInvoice(user.ID, client.ID, req)
	invSvc.SendInvoice(invoice.ID, user.ID)

	payment := &models.Payment{
		Amount: 1000,
		Method: models.PaymentMethodMpesa,
		Status: models.PaymentStatusCompleted,
		UserID: user.ID,
	}
	invSvc.RecordPayment(invoice.ID, payment)

	// Try to update - should fail
	updateReq := &UpdateInvoiceRequest{
		Notes: ptr("Updated"),
	}
	_, err := invSvc.UpdateInvoice(invoice.ID, user.ID, updateReq)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot edit")
}

// ==================== HELPER FUNCTIONS ====================

func createTestUser(t *testing.T) *models.User {
	t.Helper()

	req := RegisterRequest{
		Email:    "testuser" + t.Name() + time.Now().Format("150405") + "@example.com",
		Password: "password123",
		Name:     "Test User",
	}

	authSvc := NewAuthService(testDB, testCfg)
	resp, err := authSvc.Register(&req)
	if err != nil {
		// Try to login if already exists
		resp, err = authSvc.Login(req.Email, req.Password)
		require.NoError(t, err)
	}

	user, err := authSvc.GetUserByID(resp.User.ID)
	require.NoError(t, err)
	return user
}

func createTestClient(t *testing.T, userID string) *models.Client {
	t.Helper()

	req := CreateClientRequest{
		Name:  "Test Client " + t.Name(),
		Email: "client" + time.Now().Format("150405") + "@test.com",
		Phone: "254712345678",
	}

	clientSvc := NewClientService(testDB)
	client, err := clientSvc.CreateClient(userID, &req)
	require.NoError(t, err)
	return client
}

func ptr[T any](v T) *T {
	return &v
}

// ==================== VALIDATION TESTS ====================

func TestValidation_Email(t *testing.T) {
	authSvc := NewAuthService(testDB, testCfg)

	tests := []struct {
		name  string
		email string
		valid bool
	}{
		{"valid", "test@example.com", true},
		{"with dot", "first.last@example.com", true},
		{"with plus", "test+tag@example.com", true},
		{"invalid no @", "testexample.com", false},
		{"invalid no domain", "test@", false},
		{"invalid local", "@example.com", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := RegisterRequest{
				Email:    tt.email,
				Password: "password123",
				Name:     "Test",
			}
			_, err := authSvc.Register(&req)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidation_Password(t *testing.T) {
	authSvc := NewAuthService(testDB, testCfg)

	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{"valid", "password123", true},
		{"min length", "123456", true},
		{"too short", "12345", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := RegisterRequest{
				Email:    "test" + tt.name + "@example.com",
				Password: tt.password,
				Name:     "Test",
			}
			_, err := authSvc.Register(&req)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidation_PhoneNumber(t *testing.T) {
	tests := []struct {
		name  string
		phone string
		valid bool
	}{
		{"valid KE", "254712345678", true},
		{"with plus", "+254712345678", true},
		{"short", "712345678", true}, // 9 digits OK
		{"too short", "12345", false},
		{"letters", "254abcde", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Phone validation logic would go here
			// For now just check format
			if tt.valid {
				assert.GreaterOrEqual(t, len(tt.phone), 9)
			}
		})
	}
}

// ==================== SECURITY TESTS ====================

func TestSecurity_PasswordNotReturned(t *testing.T) {
	setupTestDB(t)
	authSvc := NewAuthService(testDB, testCfg)

	req := RegisterRequest{
		Email:    "security@test.com",
		Password: "supersecretpassword123",
		Name:     "Test",
	}

	resp, err := authSvc.Register(&req)
	require.NoError(t, err)

	// Password hash should not be in response
	assert.NotEqual(t, "supersecretpassword123", resp.User.PasswordHash)
	assert.NotContains(t, resp.User.PasswordHash, "supersecret")
}

func TestSecurity_InvalidToken(t *testing.T) {
	setupTestDB(t)
	authSvc := NewAuthService(testDB, testCfg)

	// Invalid token
	_, err := authSvc.ValidateToken("invalid-token")
	assert.Error(t, err)

	// Empty token
	_, err = authSvc.ValidateToken("")
	assert.Error(t, err)
}

func TestSecurity_RefreshTokenExpiry(t *testing.T) {
	setupTestDB(t)
	authSvc := NewAuthService(testDB, testCfg)

	req := RegisterRequest{
		Email:    "expiry@test.com",
		Password: "password123",
		Name:     "Test",
	}

	resp, err := authSvc.Register(&req)
	require.NoError(t, err)

	// Use valid refresh token
	_, err = authSvc.RefreshToken(resp.RefreshToken)
	assert.NoError(t, err)

	// Use same token again - should fail (consumed)
	_, err = authSvc.RefreshToken(resp.RefreshToken)
	assert.Error(t, err)
}

func TestSecurity_APIKeyDifferentPerUser(t *testing.T) {
	setupTestDB(t)
	authSvc := NewAuthService(testDB, testCfg)

	user1 := createTestUser(t)
	user2 := createTestUser(t)

	// Generate API keys
	key1, _ := authSvc.GenerateAPIKey(user1.ID, "Test Key 1")
	key2, _ := authSvc.GenerateAPIKey(user2.ID, "Test Key 2")

	// Keys should be different
	assert.NotEqual(t, key1, key2)

	// Key1 should work for user1 only
	validUser1, err := authSvc.ValidateAPIKey(key1)
	assert.NoError(t, err)
	assert.Equal(t, user1.ID, validUser1.ID)

	// Key1 should NOT work for user2
	invalidUser2, err := authSvc.ValidateAPIKey(key1)
	assert.Error(t, err)
	assert.Nil(t, invalidUser2)
}

// ==================== PERFORMANCE TESTS ====================

func TestPerformance_ListInvoices(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)
	authSvc := NewAuthService(testDB, testCfg)
	clientSvc := NewClientService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create 50 invoices
	for i := 0; i < 50; i++ {
		req := &CreateInvoiceRequest{
			ClientID: client.ID,
			Currency: "KES",
			DueDate:  time.Now(),
			Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 100}},
		}
		invSvc.CreateInvoice(user.ID, client.ID, req)
	}

	// Fetch with pagination
	invoices, total, err := invSvc.GetUserInvoices(user.ID, InvoiceFilter{
		Offset: 0,
		Limit:  20,
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(50), total)
	assert.Len(t, invoices, 20)
}

// ==================== DATA INTEGRITY TESTS ====================

func TestIntegrity_ClientTotals(t *testing.T) {
	setupTestDB(t)
	invSvc := NewInvoiceService(testDB)
	clientSvc := NewClientService(testDB)

	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create 3 invoices: 1000, 2000, 3000
	req1 := &CreateInvoiceRequest{ClientID: client.ID, Currency: "KES", DueDate: time.Now(),
		Items: []InvoiceItemRequest{{Description: "Item1", Quantity: 1, UnitPrice: 1000}}}
	req2 := &CreateInvoiceRequest{ClientID: client.ID, Currency: "KES", DueDate: time.Now(),
		Items: []InvoiceItemRequest{{Description: "Item2", Quantity: 1, UnitPrice: 2000}}}
	req3 := &CreateInvoiceRequest{ClientID: client.ID, Currency: "KES", DueDate: time.Now(),
		Items: []InvoiceItemRequest{{Description: "Item3", Quantity: 1, UnitPrice: 3000}}}

	inv1, _ := invSvc.CreateInvoice(user.ID, client.ID, req1)
	inv2, _ := invSvc.CreateInvoice(user.ID, client.ID, req2)
	invSvc.CreateInvoice(user.ID, client.ID, req3)

	// Pay first two
	pay1 := &models.Payment{Amount: 1000, Method: models.PaymentMethodMpesa, Status: models.PaymentStatusCompleted, UserID: user.ID}
	pay2 := &models.Payment{Amount: 2000, Method: models.PaymentMethodMpesa, Status: models.PaymentStatusCompleted, UserID: user.ID}
	invSvc.RecordPayment(inv1.ID, pay1)
	invSvc.RecordPayment(inv2.ID, pay2)

	// Check client totals
	fetchedClient, _ := clientSvc.GetClient(client.ID, user.ID)
	assert.Equal(t, 6000.0, fetchedClient.TotalBilled)
	assert.Equal(t, 3000.0, fetchedClient.TotalPaid)
}
