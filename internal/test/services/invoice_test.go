package services_test

import (
	"testing"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupInvoiceTestDB(t *testing.T) (*database.DB, string) {
	cfg := &config.DatabaseConfig{
		Driver:          "sqlite3",
		DSN:             ":memory:",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
		QueryTimeout:    10 * time.Second,
	}

	db, err := database.New(cfg)
	require.NoError(t, err)

	err = db.Migrate()
	require.NoError(t, err)

	// Create test user
	testConfig := setupTestConfig()
	auth := NewAuthService(db, testConfig)

	user, err := auth.Register(&RegisterRequest{
		Email:       "invoicetest@example.com",
		Password:    "TestPass123!",
		Name:        "Invoice Test",
		CompanyName: "Test Company Ltd",
	})
	require.NoError(t, err)

	// Create test client
	clientSvc := NewClientService(db)
	client, err := clientSvc.CreateClient(user.User.ID, &CreateClientRequest{
		Name:    "Test Client",
		Email:   "client@example.com",
		Phone:   "254712345678",
		Address: "Nairobi, Kenya",
	})
	require.NoError(t, err)

	return db, user.User.ID
}

func TestInvoiceService_CreateInvoice(t *testing.T) {
	db, userID := setupInvoiceTestDB(t)
	defer db.Close()

	cfg := setupTestConfig()
	svc := NewInvoiceService(db)

	clientID, err := getTestClientID(db, userID)
	require.NoError(t, err)

	tests := []struct {
		name    string
		req     *CreateInvoiceRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid invoice",
			req: &CreateInvoiceRequest{
				ClientID: clientID,
				Items: []InvoiceItemRequest{
					{Description: "Web Development", Quantity: 1, UnitPrice: 50000},
				},
				DueDate:  time.Now().AddDate(0, 0, 30),
				Currency: "KES",
			},
			wantErr: false,
		},
		{
			name: "multiple items",
			req: &CreateInvoiceRequest{
				ClientID: clientID,
				Items: []InvoiceItemRequest{
					{Description: "Design", Quantity: 10, UnitPrice: 5000},
					{Description: "Development", Quantity: 5, UnitPrice: 10000},
					{Description: "Testing", Quantity: 3, UnitPrice: 3000},
				},
				DueDate:  time.Now().AddDate(0, 0, 14),
				Currency: "USD",
			},
			wantErr: false,
		},
		{
			name: "missing client",
			req: &CreateInvoiceRequest{
				ClientID: "",
				Items: []InvoiceItemRequest{
					{Description: "Service", Quantity: 1, UnitPrice: 1000},
				},
				DueDate: time.Now().AddDate(0, 0, 30),
			},
			wantErr: true,
			errMsg:  "client ID is required",
		},
		{
			name: "empty items",
			req: &CreateInvoiceRequest{
				ClientID: clientID,
				Items:    []InvoiceItemRequest{},
				DueDate:  time.Now().AddDate(0, 0, 30),
			},
			wantErr: true,
			errMsg:  "at least one item",
		},
		{
			name: "past due date",
			req: &CreateInvoiceRequest{
				ClientID: clientID,
				Items: []InvoiceItemRequest{
					{Description: "Service", Quantity: 1, UnitPrice: 1000},
				},
				DueDate: time.Now().AddDate(0, 0, -1),
			},
			wantErr: true,
			errMsg:  "past",
		},
		{
			name: "with tax and discount",
			req: &CreateInvoiceRequest{
				ClientID: clientID,
				Items: []InvoiceItemRequest{
					{Description: "Consulting", Quantity: 8, UnitPrice: 10000},
				},
				DueDate:  time.Now().AddDate(0, 0, 30),
				TaxRate:  16,
				Discount: 5000,
				Currency: "KES",
			},
			wantErr: false,
		},
		{
			name: "negative quantity handled",
			req: &CreateInvoiceRequest{
				ClientID: clientID,
				Items: []InvoiceItemRequest{
					{Description: "Service", Quantity: -1, UnitPrice: 1000},
				},
				DueDate: time.Now().AddDate(0, 0, 30),
			},
			wantErr: true,
			errMsg:  "quantity",
		},
		{
			name: "invalid currency defaults to KES",
			req: &CreateInvoiceRequest{
				ClientID: clientID,
				Items: []InvoiceItemRequest{
					{Description: "Service", Quantity: 1, UnitPrice: 1000},
				},
				DueDate:  time.Now().AddDate(0, 0, 30),
				Currency: "INVALID",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoice, err := svc.CreateInvoice(userID, tt.req.ClientID, tt.req)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, invoice)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, invoice)
				assert.NotEmpty(t, invoice.ID)
				assert.NotEmpty(t, invoice.InvoiceNumber)
				assert.NotEmpty(t, invoice.MagicToken)
				assert.Equal(t, models.InvoiceStatusDraft, invoice.Status)

				// Verify totals calculation
				if len(tt.req.Items) > 0 {
					expectedSubtotal := 0.0
					for _, item := range tt.req.Items {
						if item.Quantity >= 0 {
							expectedSubtotal += item.Quantity * item.UnitPrice
						}
					}
					assert.Equal(t, expectedSubtotal, invoice.Subtotal)
				}

				// Check currency defaulting
				if tt.req.Currency == "INVALID" || tt.req.Currency == "" {
					assert.Equal(t, "KES", invoice.Currency)
				}
			}
		})
	}
}

func TestInvoiceService_UpdateInvoice(t *testing.T) {
	db, userID := setupInvoiceTestDB(t)
	defer db.Close()

	svc := NewInvoiceService(db)
	clientID, _ := getTestClientID(db, userID)

	// Create invoice
	invoice, err := svc.CreateInvoice(userID, clientID, &CreateInvoiceRequest{
		ClientID: clientID,
		Items: []InvoiceItemRequest{
			{Description: "Original", Quantity: 1, UnitPrice: 1000},
		},
		DueDate: time.Now().AddDate(0, 0, 30),
	})
	require.NoError(t, err)

	tests := []struct {
		name       string
		invoiceID  string
		updates    *UpdateInvoiceRequest
		wantErr    bool
		checkValue interface{}
	}{
		{
			name:      "update notes",
			invoiceID: invoice.ID,
			updates: &UpdateInvoiceRequest{
				Notes: strPtr("Updated notes"),
			},
			wantErr: false,
		},
		{
			name:      "update due date",
			invoiceID: invoice.ID,
			updates: &UpdateInvoiceRequest{
				DueDate: timePtr(time.Now().AddDate(0, 1, 0)),
			},
			wantErr: false,
		},
		{
			name:      "update discount",
			invoiceID: invoice.ID,
			updates: &UpdateInvoiceRequest{
				Discount: floatPtr(500),
			},
			wantErr: false,
		},
		{
			name:      "update tax rate",
			invoiceID: invoice.ID,
			updates: &UpdateInvoiceRequest{
				TaxRate: floatPtr(16),
			},
			wantErr: false,
		},
		{
			name:      "non-existent invoice",
			invoiceID: "non-existent-id",
			updates: &UpdateInvoiceRequest{
				Notes: strPtr("Test"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, err := svc.UpdateInvoice(tt.invoiceID, userID, tt.updates)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, updated)

				// Verify update was applied
				if tt.updates.Notes != nil {
					assert.Equal(t, *tt.updates.Notes, updated.Notes)
				}
				if tt.updates.Discount != nil {
					assert.Equal(t, *tt.updates.Discount, updated.Discount)
				}
			}
		})
	}
}

func TestInvoiceService_SendInvoice(t *testing.T) {
	db, userID := setupInvoiceTestDB(t)
	defer db.Close()

	cfg := setupTestConfig()
	svc := NewInvoiceServiceWithNotifications(db, nil, nil, cfg)
	clientID, _ := getTestClientID(db, userID)

	// Create invoice
	invoice, err := svc.CreateInvoice(userID, clientID, &CreateInvoiceRequest{
		ClientID: clientID,
		Items: []InvoiceItemRequest{
			{Description: "Service", Quantity: 1, UnitPrice: 1000},
		},
		DueDate: time.Now().AddDate(0, 0, 30),
	})
	require.NoError(t, err)

	// Send invoice
	sentInvoice, err := svc.SendInvoice(invoice.ID, userID)
	require.NoError(t, err)
	assert.Equal(t, models.InvoiceStatusSent, sentInvoice.Status)
	assert.NotNil(t, sentInvoice.SentAt)

	// Cannot send again
	_, err = svc.SendInvoice(invoice.ID, userID)
	assert.Error(t, err)

	// Cannot edit sent invoice
	_, err = svc.UpdateInvoice(invoice.ID, userID, &UpdateInvoiceRequest{
		Notes: strPtr("New notes"),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot edit")
}

func TestInvoiceService_RecordPayment(t *testing.T) {
	db, userID := setupInvoiceTestDB(t)
	defer db.Close()

	svc := NewInvoiceService(db)
	clientID, _ := getTestClientID(db, userID)

	// Create and send invoice
	invoice, err := svc.CreateInvoice(userID, clientID, &CreateInvoiceRequest{
		ClientID: clientID,
		Items: []InvoiceItemRequest{
			{Description: "Service", Quantity: 1, UnitPrice: 10000},
		},
		DueDate: time.Now().AddDate(0, 0, 30),
	})
	require.NoError(t, err)
	_, err = svc.SendInvoice(invoice.ID, userID)
	require.NoError(t, err)

	tests := []struct {
		name       string
		amount     float64
		wantStatus models.InvoiceStatus
	}{
		{
			name:       "partial payment",
			amount:     5000,
			wantStatus: models.InvoiceStatusPartiallyPaid,
		},
		{
			name:       "full payment",
			amount:     5000, // Remaining
			wantStatus: models.InvoiceStatusPaid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payment := &models.Payment{
				ID:          "pay_" + tt.name,
				UserID:      userID,
				Amount:      tt.amount,
				Method:      models.PaymentMethodMpesa,
				Status:      models.PaymentStatusCompleted,
				CompletedAt: sqlNullTime(time.Now()),
			}

			err := svc.RecordPayment(invoice.ID, payment)
			require.NoError(t, err)

			// Verify invoice status
			updatedInvoice, err := svc.GetInvoiceByID(invoice.ID, userID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, updatedInvoice.Status)
		})
	}
}

func TestInvoiceService_CancelInvoice(t *testing.T) {
	db, userID := setupInvoiceTestDB(t)
	defer db.Close()

	svc := NewInvoiceService(db)
	clientID, _ := getTestClientID(db, userID)

	// Create draft invoice
	invoice, err := svc.CreateInvoice(userID, clientID, &CreateInvoiceRequest{
		ClientID: clientID,
		Items: []InvoiceItemRequest{
			{Description: "Service", Quantity: 1, UnitPrice: 1000},
		},
		DueDate: time.Now().AddDate(0, 0, 30),
	})
	require.NoError(t, err)

	// Cancel draft
	err = svc.CancelInvoice(invoice.ID, userID)
	require.NoError(t, err)

	cancelled, err := svc.GetInvoiceByID(invoice.ID, userID)
	require.NoError(t, err)
	assert.Equal(t, models.InvoiceStatusCancelled, cancelled.Status)

	// Cannot cancel already cancelled
	err = svc.CancelInvoice(invoice.ID, userID)
	assert.Error(t, err)
}

func TestInvoiceService_GetDashboardStats(t *testing.T) {
	db, userID := setupInvoiceTestDB(t)
	defer db.Close()

	svc := NewInvoiceService(db)
	clientID, _ := getTestClientID(db, userID)

	// Create various invoices
	for i := 0; i < 3; i++ {
		inv, err := svc.CreateInvoice(userID, clientID, &CreateInvoiceRequest{
			ClientID: clientID,
			Items: []InvoiceItemRequest{
				{Description: "Service", Quantity: 1, UnitPrice: float64((i + 1) * 10000)},
			},
			DueDate: time.Now().AddDate(0, 0, 30),
		})
		require.NoError(t, err)
	}

	// Send one
	inv, _ := svc.CreateInvoice(userID, clientID, &CreateInvoiceRequest{
		ClientID: clientID,
		Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 5000}},
		DueDate:  time.Now().AddDate(0, 0, 30),
	})
	svc.SendInvoice(inv.ID, userID)

	tests := []struct {
		period string
	}{
		{"week"},
		{"month"},
		{"quarter"},
		{"year"},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			stats, err := svc.GetDashboardStats(userID, tt.period)

			require.NoError(t, err)
			assert.NotNil(t, stats)
			assert.GreaterOrEqual(t, stats.DraftCount, int64(3))
			assert.GreaterOrEqual(t, stats.SentCount, int64(1))
		})
	}
}

func TestInvoiceService_EdgeCases(t *testing.T) {
	db, userID := setupInvoiceTestDB(t)
	defer db.Close()

	svc := NewInvoiceService(db)
	clientID, _ := getTestClientID(db, userID)

	t.Run("invoice number generation", func(t *testing.T) {
		// Create multiple invoices and verify unique numbers
		numbers := make(map[string]bool)
		for i := 0; i < 10; i++ {
			inv, err := svc.CreateInvoice(userID, clientID, &CreateInvoiceRequest{
				ClientID: clientID,
				Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 100}},
				DueDate:  time.Now().AddDate(0, 0, 30),
			})
			require.NoError(t, err)

			_, exists := numbers[inv.InvoiceNumber]
			assert.False(t, exists, "Invoice number should be unique")
			numbers[inv.InvoiceNumber] = true

			// Verify format: INV-YYYYMMDD-XXXX
			assert.Contains(t, inv.InvoiceNumber, "INV-")
		}
	})

	t.Run("magic token expiration", func(t *testing.T) {
		inv, err := svc.CreateInvoice(userID, clientID, &CreateInvoiceRequest{
			ClientID: clientID,
			Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 100}},
			DueDate:  time.Now().AddDate(0, 0, 30),
		})
		require.NoError(t, err)

		// Magic token should be set
		assert.NotEmpty(t, inv.MagicToken)

		// Token should expire in future
		assert.True(t, inv.MagicTokenExpiresAt.Valid)
		assert.True(t, inv.MagicTokenExpiresAt.Time.After(time.Now()))
	})

	t.Run("large invoice totals", func(t *testing.T) {
		inv, err := svc.CreateInvoice(userID, clientID, &CreateInvoiceRequest{
			ClientID: clientID,
			Items: []InvoiceItemRequest{
				{Description: "Big Project", Quantity: 1000, UnitPrice: 100000},
			},
			DueDate: time.Now().AddDate(0, 0, 30),
		})
		require.NoError(t, err)
		assert.Equal(t, 100000000.0, inv.Total)
	})

	t.Run("zero discount and negative handled", func(t *testing.T) {
		inv, err := svc.CreateInvoice(userID, clientID, &CreateInvoiceRequest{
			ClientID: clientID,
			Items:    []InvoiceItemRequest{{Description: "Test", Quantity: 1, UnitPrice: 1000}},
			DueDate:  time.Now().AddDate(0, 0, 30),
			Discount: -500, // Should be set to 0
		})
		require.NoError(t, err)
		assert.Equal(t, 0.0, inv.Discount)
	})
}

// Helper functions
func getTestClientID(db *database.DB, userID string) (string, error) {
	var client models.Client
	if err := db.First(&client, "user_id = ?", userID).Error; err != nil {
		return "", err
	}
	return client.ID, nil
}

func strPtr(s string) *string        { return &s }
func timePtr(t time.Time) *time.Time { return &t }
func floatPtr(f float64) *float64    { return &f }
func sqlNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: true}
}
