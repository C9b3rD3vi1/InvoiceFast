package services

import (
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
var authService *AuthService
var invoiceService *InvoiceService
var clientService *ClientService

func TestMain(m *testing.M) {
	// Setup
	gin.SetMode(gin.TestMode)

	testCfg = &config.Config{
		Database: config.DatabaseConfig{
			Driver: "sqlite3",
			DSN:    ":memory:",
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
	testDB, err = database.New(testCfg.Database.DSN)
	if err != nil {
		panic("Failed to connect to test database: " + err.Error())
	}

	// Run migrations
	if err := testDB.Migrate(); err != nil {
		panic("Failed to run migrations: " + err.Error())
	}

	// Initialize services
	authService = NewAuthService(testDB, testCfg)
	invoiceService = NewInvoiceService(testDB)
	clientService = NewClientService(testDB)

	// Run tests
	m.Run()
}

// ==================== AUTH TESTS ====================

func TestRegister(t *testing.T) {
	// Test valid registration directly through service
	req := RegisterRequest{
		Email:       "newuser@example.com",
		Password:    "password123",
		Name:        "New User",
		Phone:       "254712345678",
		CompanyName: "Test Co",
	}

	resp, err := authService.Register(&req)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.User.ID)
	assert.Equal(t, req.Email, resp.User.Email)

	// Test duplicate email
	_, err = authService.Register(&req)
	assert.Error(t, err)

	// Test invalid email
	invalidReq := RegisterRequest{
		Email:    "invalid-email",
		Password: "password123",
		Name:     "Test",
	}
	_, err = authService.Register(&invalidReq)
	assert.Error(t, err)

	// Test short password
	shortReq := RegisterRequest{
		Email:    "user2@example.com",
		Password: "123",
		Name:     "Test",
	}
	_, err = authService.Register(&shortReq)
	assert.Error(t, err)
}

func TestLogin(t *testing.T) {
	// First register a user
	registerReq := RegisterRequest{
		Email:    "login@example.com",
		Password: "password123",
		Name:     "Login Test",
	}
	authService.Register(&registerReq)

	// Test valid login
	resp, err := authService.Login("login@example.com", "password123")
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AccessToken)

	// Test wrong password
	_, err = authService.Login("login@example.com", "wrongpass")
	assert.Error(t, err)

	// Test non-existent user
	_, err = authService.Login("nobody@example.com", "password")
	assert.Error(t, err)
}

func TestJWTValidation(t *testing.T) {
	// Create a user and get token
	registerReq := RegisterRequest{
		Email:    "jwt@example.com",
		Password: "password123",
		Name:     "JWT Test",
	}
	resp, err := authService.Register(&registerReq)
	require.NoError(t, err)

	// Valid token
	claims, err := authService.ValidateToken(resp.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, registerReq.Email, claims.Email)

	// Invalid token
	_, err = authService.ValidateToken("invalid-token")
	assert.Error(t, err)
}

// ==================== CLIENT TESTS ====================

func TestClientCRUD(t *testing.T) {
	// Create user first
	user := createTestUser(t)

	// Test CreateClient
	clientReq := CreateClientRequest{
		Name:         "Test Client",
		Email:        "client@test.com",
		Phone:        "254712345678",
		Address:      "Nairobi, Kenya",
		Currency:     "KES",
		PaymentTerms: 30,
	}

	client, err := clientService.CreateClient(user.ID, &clientReq)
	require.NoError(t, err)
	assert.NotEmpty(t, client.ID)
	assert.Equal(t, "Test Client", client.Name)

	// Test GetClient
	fetchedClient, err := clientService.GetClient(client.ID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, client.ID, fetchedClient.ID)

	// Test UpdateClient
	updateReq := UpdateClientRequest{
		Name: ptr("Updated Client"),
	}
	updatedClient, err := clientService.UpdateClient(client.ID, user.ID, &updateReq)
	require.NoError(t, err)
	assert.Equal(t, "Updated Client", updatedClient.Name)

	// Test GetUserClients
	clients, total, err := clientService.GetUserClients(user.ID, ClientFilter{})
	require.NoError(t, err)
	assert.Greater(t, total, int64(0))
	assert.Greater(t, len(clients), 0)

	// Test DeleteClient
	err = clientService.DeleteClient(client.ID, user.ID)
	require.NoError(t, err)
}

func TestClientValidation(t *testing.T) {
	user := createTestUser(t)

	// Empty name should fail
	req := CreateClientRequest{
		Name:  "",
		Email: "valid@test.com",
	}
	_, err := clientService.CreateClient(user.ID, &req)
	assert.Error(t, err)
}

func TestClientSearch(t *testing.T) {
	user := createTestUser(t)

	// Create clients
	clientService.CreateClient(user.ID, &CreateClientRequest{
		Name:  "Searchable Client One",
		Email: "one@test.com",
	})
	clientService.CreateClient(user.ID, &CreateClientRequest{
		Name:  "Another Client Two",
		Email: "two@test.com",
	})

	// Search
	clients, _, err := clientService.GetUserClients(user.ID, ClientFilter{
		Search: "searchable",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, len(clients))
	assert.Contains(t, clients[0].Name, "Searchable")
}

// ==================== INVOICE TESTS ====================

func TestInvoiceCRUD(t *testing.T) {
	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create Invoice
	invoiceReq := CreateInvoiceRequest{
		ClientID:   client.ID,
		Currency:   "KES",
		TaxRate:    16,
		Discount:   0,
		DueDate:    time.Now().Add(30 * 24 * time.Hour),
		Notes:      "Test invoice",
		Terms:      "Net 30",
		BrandColor: "#2563eb",
		Items: []InvoiceItemRequest{
			{
				Description: "Web Development",
				Quantity:    10,
				UnitPrice:   5000,
				Unit:        "hours",
			},
		},
	}

	invoice, err := invoiceService.CreateInvoice(user.ID, client.ID, &invoiceReq)
	require.NoError(t, err)
	assert.NotEmpty(t, invoice.ID)
	assert.Equal(t, 50000.0, invoice.Subtotal) // 10 * 5000
	assert.Equal(t, 8000.0, invoice.TaxAmount) // 16% of 50000
	assert.Equal(t, 58000.0, invoice.Total)    // 50000 + 8000

	// Test GetInvoice
	fetched, err := invoiceService.GetInvoiceByID(invoice.ID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, invoice.InvoiceNumber, fetched.InvoiceNumber)

	// Test UpdateInvoice
	updateReq := UpdateInvoiceRequest{
		Notes: ptr("Updated notes"),
	}
	updated, err := invoiceService.UpdateInvoice(invoice.ID, user.ID, &updateReq)
	require.NoError(t, err)
	assert.Equal(t, "Updated notes", updated.Notes)

	// Test SendInvoice
	sent, err := invoiceService.SendInvoice(invoice.ID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, models.InvoiceStatusSent, sent.Status)
	assert.NotNil(t, sent.SentAt)

	// Test CancelInvoice
	err = invoiceService.CancelInvoice(invoice.ID, user.ID)
	require.NoError(t, err)
}

func TestInvoiceStatusFlow(t *testing.T) {
	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create and send invoice
	invoiceReq := CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now().Add(30 * 24 * time.Hour),
		Items: []InvoiceItemRequest{
			{
				Description: "Test Item",
				Quantity:    1,
				UnitPrice:   10000,
			},
		},
	}

	invoice, err := invoiceService.CreateInvoice(user.ID, client.ID, &invoiceReq)
	require.NoError(t, err)

	// Check initial status
	assert.Equal(t, models.InvoiceStatusDraft, invoice.Status)

	// Send invoice
	invoice, err = invoiceService.SendInvoice(invoice.ID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, models.InvoiceStatusSent, invoice.Status)

	// Record partial payment
	payment := &models.Payment{
		Amount:   5000,
		Currency: "KES",
		Method:   models.PaymentMethodMpesa,
		Status:   models.PaymentStatusCompleted,
		UserID:   user.ID,
	}

	err = invoiceService.RecordPayment(invoice.ID, payment)
	require.NoError(t, err)

	// Check partially paid status
	invoice, _ = invoiceService.GetInvoiceByID(invoice.ID, user.ID)
	assert.Equal(t, models.InvoiceStatusPartiallyPaid, invoice.Status)
	assert.Equal(t, 5000.0, invoice.PaidAmount)

	// Record full payment
	payment2 := &models.Payment{
		Amount:   5000, // Remaining amount
		Currency: "KES",
		Method:   models.PaymentMethodMpesa,
		Status:   models.PaymentStatusCompleted,
		UserID:   user.ID,
	}

	err = invoiceService.RecordPayment(invoice.ID, payment2)
	require.NoError(t, err)

	// Check paid status
	invoice, _ = invoiceService.GetInvoiceByID(invoice.ID, user.ID)
	assert.Equal(t, models.InvoiceStatusPaid, invoice.Status)
	assert.NotNil(t, invoice.PaidAt)
}

func TestInvoiceItems(t *testing.T) {
	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create invoice with items
	invoiceReq := CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now().Add(30 * 24 * time.Hour),
		Items: []InvoiceItemRequest{
			{Description: "Item 1", Quantity: 2, UnitPrice: 1000},
			{Description: "Item 2", Quantity: 3, UnitPrice: 500},
		},
	}

	invoice, err := invoiceService.CreateInvoice(user.ID, client.ID, &invoiceReq)
	require.NoError(t, err)
	assert.Equal(t, 2, len(invoice.Items))
	assert.Equal(t, 3500.0, invoice.Total) // (2*1000) + (3*500) = 3500
}

func TestInvoiceCancelPaid(t *testing.T) {
	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create and pay invoice
	invoiceReq := CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now().Add(30 * 24 * time.Hour),
		Items: []InvoiceItemRequest{
			{Description: "Test", Quantity: 1, UnitPrice: 1000},
		},
	}

	invoice, err := invoiceService.CreateInvoice(user.ID, client.ID, &invoiceReq)
	require.NoError(t, err)

	// Pay it
	payment := &models.Payment{
		Amount: 1000,
		Method: models.PaymentMethodMpesa,
		Status: models.PaymentStatusCompleted,
		UserID: user.ID,
	}
	invoiceService.RecordPayment(invoice.ID, payment)

	// Try to cancel - should fail
	err = invoiceService.CancelInvoice(invoice.ID, user.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot cancel paid invoice")
}

// ==================== DASHBOARD TESTS ====================

func TestDashboardStats(t *testing.T) {
	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create some invoices
	for i := 0; i < 3; i++ {
		req := CreateInvoiceRequest{
			ClientID: client.ID,
			Currency: "KES",
			DueDate:  time.Now().Add(30 * 24 * time.Hour),
			Items: []InvoiceItemRequest{
				{Description: "Service", Quantity: 1, UnitPrice: 10000},
			},
		}
		inv, _ := invoiceService.CreateInvoice(user.ID, client.ID, &req)

		// Pay first two
		if i < 2 {
			invoiceService.SendInvoice(inv.ID, user.ID)
			payment := &models.Payment{
				Amount: 10000,
				Method: models.PaymentMethodMpesa,
				Status: models.PaymentStatusCompleted,
				UserID: user.ID,
			}
			invoiceService.RecordPayment(inv.ID, payment)
		}
	}

	stats, err := invoiceService.GetDashboardStats(user.ID, "month")
	require.NoError(t, err)

	assert.Equal(t, int64(3), stats.TotalInvoices)
	assert.Equal(t, int64(2), stats.PaidCount)
	assert.Equal(t, int64(1), stats.SentCount)
	assert.Equal(t, 20000.0, stats.TotalRevenue)
	assert.Equal(t, 10000.0, stats.Outstanding)
}

// ==================== EDGE CASE TESTS ====================

func TestEdgeCase_EmptyInvoiceItems(t *testing.T) {
	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	req := CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{},
	}

	_, err := invoiceService.CreateInvoice(user.ID, client.ID, &req)
	assert.Error(t, err)
}

func TestEdgeCase_DeleteClientWithInvoices(t *testing.T) {
	user := createTestUser(t)
	client := createTestClient(t, user.ID)

	// Create invoice
	req := CreateInvoiceRequest{
		ClientID: client.ID,
		Currency: "KES",
		DueDate:  time.Now(),
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
	}
	invoiceService.CreateInvoice(user.ID, client.ID, &req)

	// Try to delete - should fail
	err := clientService.DeleteClient(client.ID, user.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete client with existing invoices")
}

// ==================== HELPER FUNCTIONS ====================

func createTestUser(t *testing.T) *models.User {
	t.Helper()

	req := RegisterRequest{
		Email:    "testuser" + t.Name() + time.Now().Format("150405") + "@example.com",
		Password: "password123",
		Name:     "Test User",
	}

	resp, err := authService.Register(&req)
	if err != nil {
		// Try to login if already exists
		resp, err = authService.Login(req.Email, req.Password)
		require.NoError(t, err)
	}

	user, err := authService.GetUserByID(resp.User.ID)
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

	client, err := clientService.CreateClient(userID, &req)
	require.NoError(t, err)
	return client
}

func ptr[T any](v T) *T {
	return &v
}
