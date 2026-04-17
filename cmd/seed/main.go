package main

import (
	"context"
	"flag"
	"os"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// Load .env file if exists (won't fail if missing)
	_ = godotenv.Load()

	invoicesPtr := flag.Int("invoices", 0, "Number of test invoices to generate")
	flag.Parse()

	cfg := config.Load()

	logSvc := logger.LoadFromConfig(cfg)
	logSvc.Info(context.Background(), "InvoiceFast: Starting seed",
		"invoices", *invoicesPtr,
	)

	// Initialize encryption BEFORE any database operations
	// CRITICAL: Must be called before models are created/loaded
	encryptionKey := os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		logSvc.Fatal(context.Background(), "InvoiceFast: ENCRYPTION_KEY not set")
	}
	if err := models.InitEncryption(encryptionKey); err != nil {
		logSvc.Fatal(context.Background(), "InvoiceFast: Encryption initialization failed", "error", err.Error())
	}

	db, err := database.New(&cfg.Database)
	if err != nil {
		logSvc.Fatal(context.Background(), "InvoiceFast: Database error", "error", err.Error())
	}
	defer db.Close()

	// Create a test tenant if none exists
	var tenant models.Tenant
	if err := db.First(&tenant).Error; err != nil {
		tenant = models.Tenant{
			Name:      "Test Tenant",
			Subdomain: "test",
			Plan:      "pro",
			IsActive:  true,
		}
		if err := db.Create(&tenant).Error; err != nil {
			logSvc.Fatal(context.Background(), "InvoiceFast: Failed to create tenant", "error", err.Error())
		}
		logSvc.Info(context.Background(), "InvoiceFast: Created test tenant", "id", tenant.ID)
	}

	// Create a test user if none exists
	var user models.User
	if err := db.Where("email = ?", "test@example.com").First(&user).Error; err != nil {
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
		user = models.User{
			TenantID:     tenant.ID,
			Email:        "test@example.com",
			PasswordHash: string(hashedPassword),
			Name:         "Test User",
			IsActive:     true,
			Role:         "admin",
		}
		if err := db.Create(&user).Error; err != nil {
			logSvc.Fatal(context.Background(), "InvoiceFast: Failed to create user", "error", err.Error())
		}
		logSvc.Info(context.Background(), "InvoiceFast: Created test user", "id", user.ID)
	}

	// Seed some test clients if none exist
	var clientCount int64
	db.Model(&models.Client{}).Where("tenant_id = ?", tenant.ID).Count(&clientCount)
	if clientCount == 0 {
		clients := []models.Client{
			{
				TenantID: tenant.ID,
				UserID:   user.ID,
				Name:     "ABC Company Ltd",
				Email:    "accounts@abc.co.ke",
				Phone:    "254712345678",
				Currency: "KES",
			},
			{
				TenantID: tenant.ID,
				UserID:   user.ID,
				Name:     "XYZ Enterprises",
				Email:    "info@xyz.co.ke",
				Phone:    "254798765432",
				Currency: "USD",
			},
		}
		for _, client := range clients {
			if err := db.Create(&client).Error; err != nil {
				logSvc.Error(context.Background(), "InvoiceFast: Failed to create client", "error", err.Error())
			} else {
				logSvc.Info(context.Background(), "InvoiceFast: Created test client", "name", client.Name)
			}
		}
	}

	// Generate test invoices if requested
	if *invoicesPtr > 0 {
		invoiceSvc := services.NewInvoiceService(db)

		// Get a client to use for invoices
		var client models.Client
		if err := db.Where("tenant_id = ?", tenant.ID).First(&client).Error; err != nil {
			logSvc.Error(context.Background(), "InvoiceFast: No client found for invoice seeding", "error", err.Error())
			return
		}

		for i := 0; i < *invoicesPtr; i++ {
			dueDate := time.Now().AddDate(0, 0, 30) // 30 days from now
			req := services.CreateInvoiceRequest{
				ClientID: client.ID,
				Currency: "KES",
				DueDate:  dueDate,
				Items: []services.InvoiceItemRequest{
					{
						Description: "Web Development Services",
						Quantity:    1,
						UnitPrice:   50000,
					},
				},
				Notes: "Thank you for your business!",
				Terms: "Payment due within 30 days",
			}

			if _, err := invoiceSvc.CreateInvoice(tenant.ID, user.ID, client.ID, &req); err != nil {
				logSvc.Error(context.Background(), "InvoiceFast: Failed to create test invoice", "error", err.Error())
			} else {
				logSvc.Info(context.Background(), "InvoiceFast: Created test invoice", "number", i+1)
			}
		}
	}

	logSvc.Info(context.Background(), "InvoiceFast: Seeding completed successfully")
}
