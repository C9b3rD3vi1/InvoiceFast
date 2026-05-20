package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	"invoicefast/internal/metrics"
	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/pdf"
	"invoicefast/internal/routes"
	"invoicefast/internal/services"
	"invoicefast/internal/worker"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberRecover "github.com/gofiber/fiber/v2/middleware/recover"
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
		redisCache, err = cache.NewRedisCache(cache.CacheConfig{
			RedisAddr:     cfg.RedisCache.RedisAddr,
			RedisPassword: cfg.RedisCache.RedisPassword,
			RedisDB:       cfg.RedisCache.RedisDB,
			Prefix:        "invoicefast",
			Environment:   cfg.Server.Mode,
			DefaultTTL:    cache.DurationMedium,
			MaxRetries:    3,
			PoolSize:      20,
			MinIdleConns:  5,
			DialTimeout:   5 * time.Second,
			ReadTimeout:   3 * time.Second,
			WriteTimeout:  3 * time.Second,
		})
		if err != nil {
			logSvc.Warn(context.Background(), "Redis connection failed (continuing without cache)", "error", err.Error())
		} else {
			defer redisCache.Close()
		}
	}
	defer db.Close()

	// Initialize backup service
	backupSvc := services.NewBackupService(&cfg.Backup, cfg.Database.DSN)
	backupSvc.Start()
	defer backupSvc.Stop()

	// Initialize template engine
	engine := html.New("./views", ".html")

	if err := db.Migrate(); err != nil {
		logSvc.Fatal(context.Background(), "Failed to run migrations", "error", err.Error())
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

	app.Use(fiberRecover.New())

	// SECURITY: Request ID middleware for tracing
	app.Use(middleware.RequestIDMiddleware())

	// SECURITY: Security headers middleware
	app.Use(middleware.SecurityHeadersMiddleware())

	// SECURITY: CSRF protection for state-changing requests
	app.Use(middleware.CSRF())

	// Prometheus metrics collection
	app.Use(metrics.PrometheusMiddleware())

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

	// Initialize exchange rate service (before services that depend on it)
	exchangeRateService := services.NewExchangeRateService(db)
	exchangeRateService.StartCronJob()

	authService := services.NewAuthService(db, cfg, emailService, auditService, exchangeRateService)

	// KRA service
	kraService := services.NewKRAServiceWithDB(cfg, db)

	// Initialize WhatsApp service (used by notification service)
	whatsappService := services.NewWhatsAppService()

	// Notification service (for all modules)
	notificationService := services.NewNotificationService(db, emailService, smsService, whatsappService, cfg)
	notificationService.Start()

	// Build invoice service with all dependencies
	invoiceService := services.NewInvoiceServiceWithDeps(db, &services.ServiceDependencies{
		DB:           db,
		Email:        emailService,
		WhatsApp:     whatsappService,
		Notification: notificationService,
		Exchange:     exchangeRateService,
		KRA:          kraService,
		Config:       cfg,
	})

	clientService := services.NewClientService(db)
	reportService := services.NewReportService(db)
	settingsService := services.NewSettingsService(db)

	// Automation services - Enterprise Edition
	jobQueue := services.NewJobQueueService(db)
	recurringInvoice := services.NewAutoRecurringInvoiceService(db, jobQueue)
	reminderService := services.NewAutoReminderService(db, jobQueue)
	workflowService := services.NewAutoWorkflowService(db, jobQueue)

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

	// Thank you message service - sends thank-you emails on payment
	thankYouService := services.NewThankYouMessageService(db, &services.ServiceDependencies{
		DB:           db,
		Email:        emailService,
		Notification: notificationService,
	})

	// Intasend service for STK Push
	var intasendService *services.IntasendService
	if cfg.Intasend.APIKey != "" && cfg.Intasend.APIURL != "" {
		intasendService = services.NewIntasendServiceWithDB(db, &cfg.Intasend, notificationService)
	}

	// Reminder service - uses notification service
	legacyReminderService := services.NewReminderService(db, &services.ServiceDependencies{
		DB:           db,
		Email:        emailService,
		Notification: notificationService,
	})

	// Start reminder cron job
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logSvc.Error(context.Background(), "panic recovered", "goroutine", "reminder_cron", "recover", r)
			}
		}()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		// Run immediately on startup
		if err := legacyReminderService.RunReminders(); err != nil {
			logSvc.Error(context.Background(), "Initial reminder error", "error", err.Error())
		}
		for {
			select {
			case <-stopCh:
				logSvc.Info(context.Background(), "Stopping reminder cron")
				return
			case <-ticker.C:
				if err := legacyReminderService.RunReminders(); err != nil {
					logSvc.Error(context.Background(), "Reminder error", "error", err.Error())
				}
			}
		}
	}()

	// KRA retry queue processor
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logSvc.Error(context.Background(), "panic recovered", "goroutine", "kra_retry_queue", "recover", r)
			}
		}()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				logSvc.Info(context.Background(), "Stopping KRA queue processor")
				return
			case <-ticker.C:
				if err := kraService.ProcessRetryQueue(); err != nil {
					logSvc.Error(context.Background(), "KRA queue error", "error", err.Error())
				}
			}
		}
	}()

	// Automation scheduler - runs every minute to check for triggers
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logSvc.Error(context.Background(), "panic recovered", "goroutine", "automation_scheduler", "recover", r)
			}
		}()
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		logSvc.Info(context.Background(), "Starting automation scheduler")
		for {
			select {
			case <-stopCh:
				logSvc.Info(context.Background(), "Stopping automation scheduler")
				return
			case <-ticker.C:
				// Process recurring invoices
				if err := recurringInvoiceService.ProcessRecurringInvoices(); err != nil {
					logSvc.Error(context.Background(), "Recurring invoice scheduler error", "error", err.Error())
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
	planService := services.NewPlanService(db, exchangeRateService)
	planService.SeedDefaultPlans()

	// Migrate users without subscription to trial plan
	if err := planService.MigrateUsersWithoutSubscription(); err != nil {
		logSvc.Warn(context.Background(), "Failed to migrate users without subscription", "error", err.Error())
	}

	stripeService := services.NewStripeService(db, cfg.Stripe.SecretKey, cfg.Stripe.PublicKey, cfg.Stripe.WebhookSecret)

	subscriptionService := services.NewSubscriptionService(db, planService, notificationService)
	billingService := services.NewBillingService(db, planService, subscriptionService, nil, notificationService, cfg)

	overdueService := services.NewOverdueService(db)

	// Initialize billing worker for cron jobs
	billingWorker := worker.NewBillingWorker(db, subscriptionService, billingService)

	// Start billing cron job (runs every hour)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logSvc.Error(context.Background(), "panic recovered", "goroutine", "billing_cron", "recover", r)
			}
		}()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		// Run immediately on startup
		billingWorker.RunAllJobs()
		for {
			select {
			case <-stopCh:
				logSvc.Info(context.Background(), "Stopping billing worker")
				return
			case <-ticker.C:
				billingWorker.RunAllJobs()
			}
		}
	}()

	// Overdue invoice cron job (runs every 30 minutes)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logSvc.Error(context.Background(), "panic recovered", "goroutine", "overdue_cron", "recover", r)
			}
		}()
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		logSvc.Info(context.Background(), "Starting overdue invoice cron")
		for {
			select {
			case <-stopCh:
				logSvc.Info(context.Background(), "Stopping overdue invoice cron")
				return
			case <-ticker.C:
				if err := overdueService.MarkOverdueInvoices(); err != nil {
					logSvc.Error(context.Background(), "Overdue invoice error", "error", err.Error())
				}
			}
		}
	}()

	// Initialize handlers
	// Initialize PDF service
	pdfService := &services.PDFService{}

	// Initialize PDF generator
	pdfGenerator := pdf.NewPDFGenerator("./templates", "./data/pdfs")

	// Reuse whatsappService created earlier

	invoiceHandler := handlers.NewInvoiceHandler(invoiceService, kraService, mpesaService, subscriptionService, attachmentService, pdfService, pdfGenerator, whatsappService, pdfWorker, settingsService)
	clientHandler := handlers.NewClientHandler(clientService, subscriptionService)
	settingsHandler := handlers.NewSettingsHandler(settingsService)
	paymentHandler := handlers.NewPaymentHandler(invoiceService, mpesaService, db, thankYouService)
	dashboardHandler := handlers.NewDashboardHandler(invoiceService, clientService, kraService)
	reportHandler := handlers.NewReportHandler(reportService)
	automationHandler := handlers.NewAutomationHandler(jobQueue, recurringInvoice, reminderService, workflowService)
	publicHandler := handlers.NewPublicHandlerWithTracking(invoiceService, authService, paymentService, mpesaService, intasendService, emailTrackingService, planService)
	passwordResetService := services.NewPasswordResetService(db, cfg, emailService)
	authHandler := handlers.NewAuthHandlerWithDeps(authService, auditService, invoiceService, clientService, passwordResetService, db)
	notificationHandler := handlers.NewNotificationHandler(db)
	notificationAdminHandler := services.NewNotificationHandler(db, notificationService, cfg)

	// Billing handler
	billingHandler := handlers.NewBillingHandler(subscriptionService, planService, billingService, stripeService, intasendService, exchangeRateService, db)

	// Late fee handler
	lateFeeHandler := handlers.NewLateFeeHandler(lateFeeService)

	// Expense handler
	expenseService := services.NewExpenseService(db)
	expenseHandler := handlers.NewExpenseHandler(expenseService)

	// Integration handler
	integrationService := services.NewIntegrationService(db)
	quickBooksService := services.NewQuickBooksService(cfg, db)
	integrationHandler := handlers.NewIntegrationHandler(integrationService, quickBooksService)

	// Onboarding handler
	onboardingHandler := handlers.NewOnboardingHandler(authService, invoiceService, clientService, settingsService, db)

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
		db, publicHandler, authHandler, webhookVerifier, emailService, rateLimiter, billingHandler,
		notificationAdminHandler)

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
	bulkActionHandler := handlers.NewBulkActionHandler(legacyReminderService)
	routes.BulkActionRoutes(app, bulkActionHandler, authService, db)

	// Activity feed routes
	activityService := services.NewActivityService(db)
	activityHandler := handlers.NewActivityHandler(activityService)
	routes.ActivityRoutes(app, activityHandler, authService, db)

	// Payment discrepancy alert service
	discrepancyService := services.NewPaymentDiscrepancyService(db, emailService)

	// Payment discrepancy check cron job (every 15 minutes)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logSvc.Error(context.Background(), "panic recovered", "goroutine", "discrepancy_check", "recover", r)
			}
		}()
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		if err := discrepancyService.CheckAndCreateAlerts(); err != nil {
			logSvc.Error(context.Background(), "Initial discrepancy check error", "error", err.Error())
		}
		for {
			select {
			case <-stopCh:
				logSvc.Info(context.Background(), "Stopping discrepancy check cron")
				return
			case <-ticker.C:
				if err := discrepancyService.CheckAndCreateAlerts(); err != nil {
					logSvc.Error(context.Background(), "Discrepancy check error", "error", err.Error())
				}
			}
		}
	}()

	// Payment matching routes
	paymentMatchingService := services.NewPaymentMatchingService(db, notificationService)
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

	// Onboarding routes with sensitive rate limiting
	sensitiveLimit := rateLimiter.SensitiveRateLimiter()
	routes.OnboardingRoutes(app, onboardingHandler, authService, db, sensitiveLimit)

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
					logSvc.Debug(context.Background(), "Subdomain detected", "host", hostname, "subdomain", subdomain)

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

	// Prometheus metrics endpoint
	metricsGroup := app.Group("")
	metricsGroup.Get("/metrics", metrics.Handler())
	metricsGroup.Get("/api/v1/metrics", metrics.Handler())

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
		defer func() {
			if r := recover(); r != nil {
				logSvc.Error(context.Background(), "panic recovered", "goroutine", "http_server", "recover", r)
			}
		}()
		addr := fmt.Sprintf(":%s", cfg.Server.Port)
		logSvc.Info(context.Background(), "Starting InvoiceFast", "addr", addr, "mode", cfg.Server.Mode)
		if err := app.Listen(addr); err != nil {
			logSvc.Fatal(context.Background(), "Server error", "error", err.Error())
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logSvc.Info(context.Background(), "Shutting down")
	if err := app.ShutdownWithTimeout(30 * time.Second); err != nil {
		logSvc.Warn(context.Background(), "Shutdown error", "error", err.Error())
	}
	logSvc.Info(context.Background(), "Server exited")
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	appLogger.Get().Error(context.Background(), "Unhandled error", "method", c.Method(), "path", c.Path(), "error", err.Error())

	// JSON error for API routes
	if strings.HasPrefix(c.Path(), "/api/") {
		return c.Status(code).JSON(fiber.Map{
			"error": err.Error(),
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
	rateLimiter *middleware.FiberRateLimiter, billingHandler *handlers.BillingHandler,
	notificationAdminHandler *services.NotificationHandler) {

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
	routes.TenantPaymentRoutes(app, paymentHandler, authService, db)
	routes.ReportRoutes(app, reportHandler, authService, db)
	routes.TeamRoutes(app, teamHandler, authService, db)
	routes.AutomationRoutes(app, db, authService)
	routes.NotificationRoutes(app, notificationHandler, authService, db)
	routes.NotificationAdminRoutes(app, notificationAdminHandler, authService, db)
	routes.BillingRoutes(app, billingHandler, authService, db, webhookVerifier, idempotencySvc)

	// Webhook endpoints (rate limited separately)
	webhook := app.Group("/api/v1/webhook")
	webhook.Use(rateLimiter.WebhookRateLimiter())
	// Add webhook routes here if needed

	// === Static Frontend Pages (SPA routes) ===
	routes.StaticRoutes(app, authHandler)
}
