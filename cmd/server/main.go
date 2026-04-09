package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"invoicefast/internal/cache"
	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/routes"
	"invoicefast/internal/services"
	"invoicefast/internal/worker"
)

func main() {
	cfg := config.Load()

	db, err := database.New(&cfg.Database)
	if err != nil {
		log.Fatalf("Database error: %v", err)
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
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format:     "[${time}] ${status} - ${method} ${path} ${latency}\n",
		TimeFormat: "2006-01-02 15:04:05",
	}))

	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORS.AllowedOrigins,
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Tenant-ID, Idempotency-Key",
		AllowCredentials: true,
	}))

	authService := services.NewAuthService(db, cfg)
	invoiceService := services.NewInvoiceService(db)
	clientService := services.NewClientService(db)
	kraService := services.NewKRAServiceWithDB(cfg, db)
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

	var whatsappService *services.WhatsAppService
	if cfg.WhatsApp.Enabled {
		whatsappService = services.NewWhatsAppService(cfg)
	}

	// Build invoice service with all features: exchange rates + notifications + KRA
	invoiceService = services.NewInvoiceServiceWithKRAService(db, exchangeRateService, emailService, whatsappService, kraService, cfg)

	// Payment service for M-Pesa integration
	paymentService := services.NewPaymentService(db, cfg)

	// Intasend service for STK Push
	var intasendService *services.IntasendService
	if cfg.Intasend.SecretKey != "" && cfg.Intasend.APIURL != "" {
		intasendService = services.NewIntasendService(&cfg.Intasend)
	}

	reminderService := services.NewReminderService(db, emailService, whatsappService)
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

	handler := handlers.NewFiberHandler(authService, invoiceService, clientService, kraService, exchangeRateService, pdfWorker)
	publicHandler := handlers.NewPublicHandler(invoiceService, authService, paymentService, intasendService)
	rateLimiter := middleware.NewFiberRateLimiter()

	// Serve static assets from /static directory
	app.Static("/static", "./static")
	app.Static("/css", "./static/css")
	app.Static("/js", "./static/js")
	app.Static("/images", "./static/images")

	// Public routes (landing, auth, portal)
	routes.PublicRoutes(app, publicHandler)
	routes.PublicAPIRoutes(app, publicHandler)
	routes.PublicAuthRoutes(app, publicHandler, rateLimiter)

	// Legacy page routes (for backward compatibility)
	app.Get("/pricing.html", func(c *fiber.Ctx) error {
		return c.Render("pricing.html", fiber.Map{
			"Title": "Pricing",
			"Page":  "pricing",
		}, "base")
	})
	app.Get("/contact.html", func(c *fiber.Ctx) error {
		return c.Render("contact.html", fiber.Map{
			"Title": "Contact",
			"Page":  "contact",
		}, "base")
	})
	app.Get("/forgot-password.html", func(c *fiber.Ctx) error {
		return c.Render("forgot-password.html", fiber.Map{
			"Title": "Reset Password",
			"Page":  "forgot",
		})
	})
	app.Get("/privacy.html", func(c *fiber.Ctx) error {
		return c.Render("privacy.html", fiber.Map{
			"Title": "Privacy Policy",
			"Page":  "privacy",
		})
	})
	app.Get("/terms.html", func(c *fiber.Ctx) error {
		return c.Render("terms.html", fiber.Map{
			"Title": "Terms of Service",
			"Page":  "terms",
		})
	})
	app.Get("/invoice.html", func(c *fiber.Ctx) error {
		return c.Render("invoice.html", fiber.Map{
			"Title": "Invoice",
			"Page":  "invoice",
		})
	})

	setupRoutes(app, cfg, handler, rateLimiter, authService, idempotencySvc)

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
	authService *services.AuthService, idempotencySvc *services.IdempotencyService) {

	// Swagger disabled - requires additional configuration
	// app.Get("/api/docs/*", swagger.HandlerDefault(app))

	authGroup := app.Group("/api/v1/auth")
	authGroup.Post("/register", rateLimiter.Limit(10, time.Minute), handler.Register)
	authGroup.Post("/login", rateLimiter.Limit(10, time.Minute), handler.Login)
	authGroup.Post("/refresh", handler.RefreshToken)
	authGroup.Post("/forgot-password", rateLimiter.Limit(5, time.Minute), handler.ForgotPassword)
	authGroup.Post("/reset-password", handler.ResetPassword)
	authGroup.Get("/validate-reset-token", handler.ValidateResetToken)

	publicGroup := app.Group("/api/v1")
	publicGroup.Get("/invoice/:token", handler.GetInvoiceByToken)
	publicGroup.Post("/webhook/intasend",
		middleware.IdempotencyMiddleware(idempotencySvc),
		handler.HandleIntasendWebhook)

	tenantGroup := app.Group("/api/v1/tenant")
	tenantGroup.Use(middleware.TenantMiddleware(authService))
	tenantGroup.Use(rateLimiter.Limit(100, time.Minute))
	{
		tenantGroup.Get("/me", handler.GetMe)
		tenantGroup.Put("/me", handler.UpdateUser)
		tenantGroup.Post("/change-password", handler.ChangePassword)
		tenantGroup.Post("/logout", handler.Logout)
		tenantGroup.Post("/api-keys", handler.GenerateAPIKey)

		tenantGroup.Post("/clients", handler.CreateClient)
		tenantGroup.Get("/clients", handler.GetClients)
		tenantGroup.Get("/clients/:id", handler.GetClient)
		tenantGroup.Put("/clients/:id", handler.UpdateClient)
		tenantGroup.Delete("/clients/:id", handler.DeleteClient)
		tenantGroup.Get("/clients/:id/stats", handler.GetClientStats)

		tenantGroup.Post("/invoices", handler.CreateInvoice)
		tenantGroup.Get("/invoices", handler.GetInvoices)
		tenantGroup.Get("/invoices/:id", handler.GetInvoice)
		tenantGroup.Put("/invoices/:id", handler.UpdateInvoice)
		tenantGroup.Put("/invoices/:id/items", handler.UpdateInvoiceItems)
		tenantGroup.Post("/invoices/:id/send", handler.SendInvoice)
		tenantGroup.Post("/invoices/:id/cancel", handler.CancelInvoice)
		tenantGroup.Post("/invoices/:id/pay", handler.RequestPayment)
		tenantGroup.Get("/invoices/:id/pdf", handler.GetInvoicePDF)
		tenantGroup.Get("/invoices/:id/status", handler.GetInvoiceStatus)

		tenantGroup.Get("/dashboard", handler.GetDashboard)
		tenantGroup.Get("/rates", handler.GetExchangeRates)
	}
}
