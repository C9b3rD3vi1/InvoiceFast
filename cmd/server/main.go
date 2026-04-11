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

	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORS.AllowedOrigins,
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Tenant-ID, Idempotency-Key",
		AllowCredentials: true,
	}))

	app.Use(middleware.LayoutMiddleware())

	invoiceService := services.NewInvoiceService(db)
	clientService := services.NewClientService(db)
	kraService := services.NewKRAServiceWithDB(cfg, db)
	settingsService := services.NewSettingsService(db)
	exchangeRateService := services.NewExchangeRateService(db)
	exchangeRateService.StartCronJob()

	var idempotencySvc *services.IdempotencyService
	if redisCache != nil {
		idempotencySvc = services.NewIdempotencyService(redisCache)
	}

	var emailService *services.EmailService
	if cfg.Mail.SMTPHost != "" {
		emailService = services.NewEmailService(cfg)
	}

	// Audit service for comprehensive logging
	auditService := services.NewAuditService(db)

	authService := services.NewAuthService(db, cfg, emailService, auditService)

	// WhatsApp service - now in internal/whatsapp package
	// var whatsappService *services.WhatsAppService
	// if cfg.WhatsApp.Enabled {
	// 	whatsappService = services.NewWhatsAppService(cfg, db)
	// }

	// Build invoice service with all features: exchange rates + notifications + KRA
	invoiceService = services.NewInvoiceServiceWithKRAService(db, exchangeRateService, emailService, nil, kraService, cfg)

	// Payment service for M-Pesa integration
	paymentService := services.NewPaymentService(db, cfg)

	// Intasend service for STK Push
	var intasendService *services.IntasendService
	if cfg.Intasend.SecretKey != "" && cfg.Intasend.APIURL != "" {
		intasendService = services.NewIntasendService(&cfg.Intasend)
	}

	reminderService := services.NewReminderService(db, emailService, nil)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
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

	handler := handlers.NewFiberHandler(authService, invoiceService, clientService, kraService, exchangeRateService, pdfWorker, mpesaService, auditService)

	// HTMX handler for SPA-like dashboard
	htmxHandler := handlers.NewHTMXHandler(invoiceService, clientService, kraService, settingsService, paymentService, pdfWorker, exchangeRateService)

	publicHandler := handlers.NewPublicHandler(invoiceService, authService, paymentService, mpesaService, intasendService)
	rateLimiter := middleware.NewFiberRateLimiter()

	// Serve static assets from /static directory
	app.Static("/static", "./static")
	app.Static("/css", "./static/css")
	app.Static("/js", "./static/js")
	app.Static("/images", "./static/images")

	// Serve static HTML pages (decoupled frontend)
	app.Get("/dashboard", func(c *fiber.Ctx) error {
		return c.SendFile("./static/dashboard.html")
	})
	app.Get("/invoices", func(c *fiber.Ctx) error {
		return c.SendFile("./static/invoices.html")
	})

	// Subdomain routing for branded client portal
	app.Use(func(c *fiber.Ctx) error {
		hostname := c.Hostname()

		// Check if this is a subdomain request (e.g., clientname.invoicefast.com)
		if strings.Contains(hostname, ".") {
			parts := strings.Split(hostname, ".")
			if len(parts) >= 3 {
				subdomain := parts[0]

				// Skip if it's just localhost or known domains
				if subdomain != "www" && subdomain != "api" && !strings.HasPrefix(hostname, "localhost") {
					log.Printf("[Subdomain] Detected: %s -> subdomain: %s", hostname, subdomain)

					// Look up tenant by subdomain
					var tenant models.Tenant
					if err := db.Where("subdomain = ? AND is_active = ?", subdomain, true).First(&tenant).Error; err == nil {
						// Store tenant info in context
						c.Locals("tenant_id", tenant.ID)
						c.Locals("tenant_subdomain", subdomain)
						c.Locals("tenant_brand_color", "#2563eb") // Default brand color

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

	// Public routes (landing, auth, portal)
	routes.PublicRoutes(app, publicHandler)
	routes.PublicAPIRoutes(app, publicHandler, rateLimiter)
	routes.PublicAuthRoutes(app, publicHandler, rateLimiter)

	// Setup all API routes via routes directory
	setupRoutes(app, cfg, handler, rateLimiter, authService, idempotencySvc, db, htmxHandler, publicHandler, webhookVerifier)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "healthy",
			"time":    time.Now().UTC().Format(time.RFC3339),
			"version": "1.0.0",
		})
	})

	go func() {
		addr := fmt.Sprintf(":%s", cfg.Server.Port)
		log.Printf("Starting InvoiceFast on %s (mode: %s)", addr, cfg.Server.Mode)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	// Signal cron jobs to stop
	close(stopCh)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	log.Println("Server exited")
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	return c.Status(code).JSON(fiber.Map{
		"error": err.Error(),
		"code":  code,
	})
}

func setupRoutes(app *fiber.App, cfg *config.Config,
	handler *handlers.FiberHandler, rateLimiter *middleware.FiberRateLimiter,
	authService *services.AuthService, idempotencySvc *services.IdempotencyService, db *database.DB, htmxHandler *handlers.HTMXHandler,
	publicHandler *handlers.PublicHandler, webhookVerifier *middleware.WebhookVerifierMiddleware) {

	// HTMX routes for dashboard SPA-like experience
	app.Get("/htmx/dashboard", htmxHandler.Dashboard)
	app.Get("/htmx/invoices", htmxHandler.InvoiceList)
	app.Get("/htmx/invoices/search", htmxHandler.InvoiceSearch)
	app.Get("/htmx/invoices/poll", htmxHandler.InvoiceStatusPoll)
	app.Get("/htmx/invoices/:id", htmxHandler.InvoiceRow)
	app.Get("/htmx/invoices/:id/kra-sync", htmxHandler.SyncToKRA)
	app.Get("/htmx/invoices/line-item", htmxHandler.AddLineItem)
	app.Post("/htmx/invoices", htmxHandler.CreateInvoicePOST)
	app.Get("/htmx/invoices/calc", htmxHandler.CalculateExchange)

	// Client routes
	app.Get("/htmx/clients", htmxHandler.GetClients)
	app.Get("/htmx/clients/search", htmxHandler.SearchClientsHTMX)
	app.Post("/htmx/clients", htmxHandler.CreateClientPOST)
	app.Get("/htmx/clients/:id/profile", htmxHandler.GetClientProfile)
	app.Get("/htmx/clients/:id/kra-pin", htmxHandler.RevealKRAPin)
	app.Get("/htmx/payments", htmxHandler.GetPayments)

	// Settings HTMX routes
	app.Post("/htmx/settings/mpesa", htmxHandler.SaveSettingsMpesa)
	app.Post("/htmx/settings/mpesa/test", htmxHandler.TestMpesaConnection)
	app.Post("/htmx/settings/kra", htmxHandler.SaveSettingsKRA)

	// Dashboard routes (with shell layout)
	app.Get("/dashboard", htmxHandler.Dashboard)
	app.Get("/dashboard/invoices", htmxHandler.RenderInvoices)
	app.Get("/dashboard/invoices/new", htmxHandler.RenderCreateInvoice)
	app.Get("/dashboard/clients", htmxHandler.RenderClients)
	app.Get("/dashboard/payments", htmxHandler.RenderPayments)
	app.Get("/dashboard/settings", htmxHandler.RenderSettings)

	// Routes via routes directory
	routes.DashboardRoutes(app, handler, authService, rateLimiter, db)
	routes.InvoiceRoutes(app, handler, authService, db)
	routes.ClientRoutes(app, handler, authService, db)
	routes.PaymentRoutes(app, handler, idempotencySvc, rateLimiter, webhookVerifier)
	routes.PaymentAPIRoutes(app, publicHandler, rateLimiter)
	routes.PublicInvoiceRoutes(app, handler, rateLimiter)
}
