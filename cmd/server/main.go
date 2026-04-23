package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"invoicefast/internal/cache"
	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	appLogger "invoicefast/internal/logger"
	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/pdf"
	"invoicefast/internal/routes"
	"invoicefast/internal/services"
	"invoicefast/internal/worker"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"
)

func main() {
	// Load .env file if exists (won't fail if missing)
	_ = godotenv.Load()

	cfg := config.Load()

	logSvc := appLogger.LoadFromConfig(cfg)
	logSvc.Info(context.Background(), "InvoiceFast: Starting server",
		"mode", cfg.Server.Mode,
		"port", cfg.Server.Port,
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

	// Graceful shutdown channel
	stopCh := make(chan struct{})

	// Initialize Redis if configured
	var redisCache *cache.RedisCache
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		redisCache, err = cache.NewRedisCache(&cache.CacheConfig{URL: redisURL})
		if err != nil {
			log.Printf("Redis connection failed: %v (continuing without cache)", err)
		} else {
			defer redisCache.Close()
		}
	}
	defer db.Close()

	// Initialize template engine
	engine := html.New("./views", ".html")

	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	app := fiber.New(fiber.Config{
		AppName:      "InvoiceFast",
		ServerHeader: "InvoiceFast/1.0.0",
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
		ErrorHandler: customErrorHandler,
		Views:        engine,
	})

	app.Use(recover.New())

	// HTTPS redirect in production
	if cfg.Server.Mode == "production" {
		app.Use(func(c *fiber.Ctx) error {
			if c.Protocol() == "http" {
				// Only redirect if Host is not localhost
				host := c.Hostname()
				if !strings.HasPrefix(host, "localhost") && !strings.HasPrefix(host, "127.0.0.1") {
					return c.Status(fiber.StatusMovedPermanently).Redirect("https://" + c.Hostname() + c.OriginalURL())
				}
			}
			return c.Next()
		})
	}

	logSvc.Info(context.Background(), "InvoiceFast: Server initialized")

	// CORS configuration
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORS.AllowedOrigins,
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS,PATCH",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Tenant-ID, Idempotency-Key, X-Request-ID",
		AllowCredentials: true,
		MaxAge:           86400,
	}))

	// Layout middleware
	app.Use(middleware.LayoutMiddleware())

	// Initialize rate limiter (production-ready Fiber version)
	rateLimiter := middleware.NewFiberRateLimiter()
	defer rateLimiter.Stop()

	// Apply rate limiting globally
	app.Use(rateLimiter.HeadersMiddleware())

	// Initialize services
	var idempotencySvc *services.IdempotencyService
	if redisCache != nil {
		idempotencySvc = services.NewIdempotencyService(redisCache)
	}

	var emailService *services.EmailService
	if cfg.Mail.SMTPHost != "" {
		emailService = services.NewEmailService(cfg)
	}

	// SMS service for critical alerts (Africa's Talking)
	smsService := services.NewSMSService(&cfg.SMS)
	_ = smsService // Used by reminder service for SMS fallback

	// Audit service for comprehensive logging
	auditService := services.NewAuditService(db)

	authService := services.NewAuthService(db, cfg, emailService, auditService)

	// Initialize exchange rate service
	exchangeRateService := services.NewExchangeRateService(db)
	exchangeRateService.StartCronJob()

	// KRA service
	kraService := services.NewKRAServiceWithDB(cfg, db)

	// Build invoice service with all features
	invoiceService := services.NewInvoiceServiceWithKRAService(db, exchangeRateService, emailService, nil, kraService, cfg)

	clientService := services.NewClientService(db)
	reportService := services.NewReportService(db)
	settingsService := services.NewSettingsService(db)
	automationService := services.NewSimpleAutomationService(db)

	// Payment service for M-Pesa integration
	paymentService := services.NewPaymentService(db, cfg)

	// Item library service
	itemLibraryService := services.NewItemLibraryService(db)

	// Attachment service
	attachmentService := services.NewAttachmentService(db, "./uploads")

	// Recurring invoice service
	recurringInvoiceService := services.NewRecurringInvoiceService(db, invoiceService, emailService)

	// Email tracking service
	emailTrackingService := services.NewEmailTrackingService(db)

	// Late fee service
	lateFeeService := services.NewLateFeeService(db)

	// Thank you message service
	thankYouService := services.NewThankYouMessageService(db, emailService)
	_ = thankYouService // Used by payment handlers to send thank you on payment completion

	// Intasend service for STK Push
	var intasendService *services.IntasendService
	if cfg.Intasend.SecretKey != "" && cfg.Intasend.APIURL != "" {
		intasendService = services.NewIntasendService(&cfg.Intasend)
	}

	// Reminder service
	reminderService := services.NewReminderService(db, emailService, nil)

	// Start reminder cron job
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		// Run immediately on startup
		if err := reminderService.RunReminders(); err != nil {
			log.Printf("Initial reminder error: %v", err)
		}
		for {
			select {
			case <-stopCh:
				log.Println("Stopping reminder cron...")
				return
			case <-ticker.C:
				if err := reminderService.RunReminders(); err != nil {
					log.Printf("Reminder error: %v", err)
				}
			}
		}
	}()

	// KRA retry queue processor
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				log.Println("Stopping KRA queue processor...")
				return
			case <-ticker.C:
				if err := kraService.ProcessRetryQueue(); err != nil {
					log.Printf("KRA queue error: %v", err)
				}
			}
		}
	}()

	// Automation scheduler - runs every minute to check for triggers
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		log.Println("Starting automation scheduler...")
		for {
			select {
			case <-stopCh:
				log.Println("Stopping automation scheduler...")
				return
			case <-ticker.C:
				// Run active automations
				if err := runActiveAutomations(automationService); err != nil {
					log.Printf("Automation scheduler error: %v", err)
				}
				// Process recurring invoices
				if err := recurringInvoiceService.ProcessRecurringInvoices(); err != nil {
					log.Printf("Recurring invoice scheduler error: %v", err)
				}
			}
		}
	}()

	// PDF Worker (Redis-backed)
	var pdfWorker *worker.PDFWorker
	if redisCache != nil {
		pdfWorker = worker.NewPDFWorker(redisCache, db, cfg)
		pdfWorker.Start(context.Background())
		defer pdfWorker.Stop()
	}

	// Initialize M-Pesa service first (needed for handler)
	var mpesaService *services.MPesaService
	if cfg.MPesa.Enabled {
		mpesaService = services.NewMPesaService(cfg, db, redisCache)
	}

	// Create webhook verifier for middleware
	webhookVerifier := middleware.NewWebhookVerifierMiddleware(services.NewWebhookVerifier(cfg))

	// Billing services (must be before handlers that need them)
	planService := services.NewPlanService(db)
	planService.SeedDefaultPlans()
	subscriptionService := services.NewSubscriptionService(db, planService)
	billingService := services.NewBillingService(db)

	// Initialize billing worker for cron jobs
	billingWorker := worker.NewBillingWorker(db, subscriptionService, billingService)

	// Start billing cron job (runs every hour)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		// Run immediately on startup
		billingWorker.RunAllJobs()
		for {
			select {
			case <-stopCh:
				log.Println("Stopping billing worker...")
				return
			case <-ticker.C:
				billingWorker.RunAllJobs()
			}
		}
	}()

	// Initialize handlers
	// Initialize PDF service
	pdfService := &services.PDFService{}

	// Initialize PDF generator
	pdfGenerator := pdf.NewPDFGenerator("./templates", "./data/pdfs")

	// Initialize WhatsApp service
	whatsappService := services.NewWhatsAppService()

	invoiceHandler := handlers.NewInvoiceHandler(invoiceService, kraService, mpesaService, subscriptionService, attachmentService, pdfService, pdfGenerator, whatsappService)
	clientHandler := handlers.NewClientHandler(clientService, subscriptionService)
	settingsHandler := handlers.NewSettingsHandler(settingsService)
	paymentHandler := handlers.NewPaymentHandler(invoiceService, mpesaService)
	dashboardHandler := handlers.NewDashboardHandler(invoiceService, clientService)
	reportHandler := handlers.NewReportHandler(reportService)
	automationHandler := handlers.NewAutomationHandler(automationService)
	publicHandler := handlers.NewPublicHandlerWithTracking(invoiceService, authService, paymentService, mpesaService, intasendService, emailTrackingService)
	authHandler := handlers.NewAuthHandlerWithDeps(authService, auditService, invoiceService, clientService)
	notificationHandler := handlers.NewNotificationHandler(db)

	// Billing handler
	billingHandler := handlers.NewBillingHandler(subscriptionService, planService, billingService)

	// Late fee handler
	lateFeeHandler := handlers.NewLateFeeHandler(lateFeeService)

	// Expense handler
	expenseService := services.NewExpenseService(db)
	expenseHandler := handlers.NewExpenseHandler(expenseService)

	// Integration handler
	integrationService := services.NewIntegrationService(db)
	integrationHandler := handlers.NewIntegrationHandler(integrationService)

	// Static files
	app.Static("/static", "./static")
	app.Static("/css", "./static/css")
	app.Static("/js", "./static/js")
	app.Static("/images", "./static/images", fiber.Static{
		Browse: false,
	})

	// Setup all routes - ONLY ONCE
	setupRoutes(app, cfg, invoiceHandler, clientHandler, settingsHandler, paymentHandler,
		dashboardHandler, reportHandler, automationHandler, notificationHandler, authService, idempotencySvc,
		db, publicHandler, authHandler, webhookVerifier, emailService, rateLimiter, billingHandler)

	// Item library routes
	itemLibraryHandler := handlers.NewItemLibraryHandler(itemLibraryService)
	routes.ItemLibraryRoutes(app, itemLibraryHandler, authService, db)

	// Recurring invoice routes
	recurringInvoiceHandler := handlers.NewRecurringInvoiceHandler(recurringInvoiceService)
	routes.RecurringInvoiceRoutes(app, recurringInvoiceHandler, authService, db)

	// Late fee routes
	routes.LateFeeRoutes(app, lateFeeHandler, authService, db)

	// Expense routes
	routes.ExpenseRoutes(app, expenseHandler, authService, db)

	// Bulk action routes
	bulkActionHandler := handlers.NewBulkActionHandler(reminderService)
	routes.BulkActionRoutes(app, bulkActionHandler, authService, db)

	// Activity feed routes
	activityService := services.NewActivityService(db)
	activityHandler := handlers.NewActivityHandler(activityService)
	routes.ActivityRoutes(app, activityHandler, authService, db)

	// Payment matching routes
	paymentMatchingService := services.NewPaymentMatchingService(db)
	paymentMatchingHandler := handlers.NewPaymentMatchingHandler(paymentMatchingService, invoiceService)
	routes.PaymentMatchingRoutes(app, paymentMatchingHandler, authService, db)

	// Settlement report routes
	settlementService := services.NewMPaySettlementService(db)
	settlementHandler := handlers.NewSettlementHandler(settlementService)
	routes.SettlementRoutes(app, settlementHandler, authService, db)

	// Reminder sequence routes
	reminderSequenceService := services.NewReminderSequenceService(db, emailService)
	reminderSequenceHandler := handlers.NewReminderSequenceHandler(reminderSequenceService)
	routes.ReminderSequenceRoutes(app, reminderSequenceHandler, authService, db)

	// Integration routes
	routes.IntegrationRoutes(app, integrationHandler, authService, db)

	// Subdomain routing for branded client portal (AFTER main routes)
	app.Use(func(c *fiber.Ctx) error {
		hostname := c.Hostname()

		// Check if this is a subdomain request
		if strings.Contains(hostname, ".") {
			parts := strings.Split(hostname, ".")
			if len(parts) >= 3 {
				subdomain := parts[0]

				// Skip if it's just known domains
				if subdomain != "www" && subdomain != "api" && !strings.HasPrefix(hostname, "localhost") {
					log.Printf("[Subdomain] Detected: %s -> subdomain: %s", hostname, subdomain)

					// Look up tenant by subdomain
					var tenant models.Tenant
					if err := db.Where("subdomain = ? AND is_active = ?", subdomain, true).First(&tenant).Error; err == nil {
						// Store tenant info in context
						c.Locals("tenant_id", tenant.ID)
						c.Locals("tenant_subdomain", subdomain)
						c.Locals("tenant_brand_color", "#2563eb")

						// Check for custom brand settings
						if tenant.Settings != "" {
							var settings map[string]interface{}
							if json.Unmarshal([]byte(tenant.Settings), &settings) == nil {
								if color, ok := settings["brand_color"].(string); ok {
									c.Locals("tenant_brand_color", color)
								}
								if logo, ok := settings["logo_url"].(string); ok {
									c.Locals("tenant_logo_url", logo)
								}
							}
						}

						// Route to branded portal
						return c.Redirect("/portal/branded")
					}
				}
			}
		}

		return c.Next()
	})

	// Health endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		// Check database connection
		dbErr := db.Ping()

		status := "healthy"
		code := fiber.StatusOK

		if dbErr != nil {
			status = "degraded"
			code = fiber.StatusServiceUnavailable
		}

		return c.Status(code).JSON(fiber.Map{
			"status":   status,
			"time":     time.Now().UTC().Format(time.RFC3339),
			"version":  "1.0.0",
			"database": dbErr == nil,
			"redis":    redisCache != nil,
		})
	})

	// Start server
	go func() {
		addr := fmt.Sprintf(":%s", cfg.Server.Port)
		log.Printf("Starting InvoiceFast on %s (mode: %s)", addr, cfg.Server.Mode)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	// Signal all cron jobs to stop
	close(stopCh)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	log.Println("Server exited")
}

// runActiveAutomations executes all active automations with triggers
func runActiveAutomations(automationService *services.AutomationService) error {
	// Query for active automations using the service method
	automations, err := automationService.GetActiveAutomations()
	if err != nil {
		return fmt.Errorf("failed to query automations: %w", err)
	}

	// For each active automation, process triggers
	for _, automation := range automations {
		// Trigger checking is handled by ProcessTrigger when events occur
		// Time-based triggers are checked by the automation scheduler
		_ = automation
	}

	return nil
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	// Log the error
	log.Printf("[ERROR] %s %s: %v", c.Method(), c.Path(), err)

	// JSON error for API endpoints
	if strings.HasPrefix(c.Path(), "/api/") {
		return c.Status(code).JSON(fiber.Map{
			"error": err.Error(),
			"code":  code,
		})
	}

	// HTML error for web routes
	return c.Status(code).Render("error", fiber.Map{
		"Status": code,
		"Error":  err.Error(),
	})
}

func setupRoutes(app *fiber.App, cfg *config.Config,
	invoiceHandler *handlers.InvoiceHandler, clientHandler *handlers.ClientHandler,
	settingsHandler *handlers.SettingsHandler, paymentHandler *handlers.PaymentHandler,
	dashboardHandler *handlers.DashboardHandler, reportHandler *handlers.ReportHandler,
	automationHandler *handlers.AutomationHandler, notificationHandler *handlers.NotificationHandler,
	authService *services.AuthService, idempotencySvc *services.IdempotencyService,
	db *database.DB, publicHandler *handlers.PublicHandler, authHandler *handlers.AuthHandler,
	webhookVerifier *middleware.WebhookVerifierMiddleware, emailService *services.EmailService,
	rateLimiter *middleware.FiberRateLimiter, billingHandler *handlers.BillingHandler) {

	teamHandler := handlers.NewTeamHandler(db, authService, emailService)

	// === Apply rate limiting to auth endpoints ===
	authRateLimit := rateLimiter.AuthRateLimiter()

	// === API v1 Routes (organized by domain) ===

	// Public routes (no auth required)
	routes.PublicRoutes(app, publicHandler)
	routes.PublicAPIRoutes(app, publicHandler)

	// Auth routes with stricter rate limiting
	auth := app.Group("/api/v1/auth")
	auth.Use(authRateLimit)
	routes.AuthRoutes(app, authHandler)

	// Public contact endpoint
	routes.PublicAuthRoutes(app, publicHandler)

	// Protected tenant routes (require authentication)
	routes.TenantRoutes(app, authHandler, authService, db)
	routes.InvoiceRoutes(app, invoiceHandler, authService, db)
	routes.ClientRoutes(app, clientHandler, authService, db)
	routes.SettingsRoutes(app, settingsHandler, authService, db)
	routes.DashboardRoutes(app, dashboardHandler, authService, db)
	routes.PaymentRoutes(app, paymentHandler, idempotencySvc, webhookVerifier)
	routes.PaymentAPIRoutes(app, paymentHandler)
	routes.ReportRoutes(app, reportHandler, authService, db)
	routes.TeamRoutes(app, teamHandler, authService, db)
	routes.AutomationRoutes(app, automationHandler, authService, db)
	routes.NotificationRoutes(app, notificationHandler, authService, db)
	routes.BillingRoutes(app, billingHandler, authService, db)

	// Webhook endpoints (rate limited separately)
	webhook := app.Group("/api/v1/webhook")
	webhook.Use(rateLimiter.WebhookRateLimiter())
	// Add webhook routes here if needed

	// === Static Frontend Pages (SPA routes) ===
	routes.StaticRoutes(app)
}
