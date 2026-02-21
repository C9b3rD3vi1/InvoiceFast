package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"
	"invoicefast/internal/utils"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize database
	db, err := database.New(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Log database stats periodically
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			stats := db.Stats()
			log.Printf("[DB] Open=%d Idle=%d InUse=%d WaitCount=%d",
				stats.OpenConnections, stats.Idle,
				stats.InUse, stats.WaitCount)
		}
	}()

	// Run migrations
	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize services
	authService := services.NewAuthService(db, cfg)
	invoiceService := services.NewInvoiceService(db)
	clientService := services.NewClientService(db)
	intasendService := services.NewIntasendService(&cfg.Intasend)

	// Initialize handlers
	handler := handlers.NewHandler(authService, invoiceService, clientService)
	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter()
	defer rateLimiter.Stop()
	// Setup Gin
	if cfg.Server.Mode == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Custom server with timeouts
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:      setupRouter(cfg, db, handler, rateLimiter, authService, invoiceService, intasendService),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting InvoiceFast server on :%s (mode: %s)", cfg.Server.Port, cfg.Server.Mode)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.Shutdown)
	defer cancel()
	defer func(){}()

	// Stop accepting new requests
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Close database connections
	if err := db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}

	log.Println("Server exited gracefully")
}

func setupRouter(cfg *config.Config, db *database.DB, handler *handlers.Handler,
	rateLimiter *middleware.RateLimiter, authService *services.AuthService,
	invoiceService *services.InvoiceService, intasendService *services.IntasendService) *gin.Engine {

	r := gin.New()

	// Recovery - prevents panics from crashing server
	r.Use(gin.Recovery())

	// Logger - structured logging
	r.Use(utils.LoggerMiddleware())

	// Request ID - for tracing
	r.Use(utils.RequestIDMiddleware())

	// CORS
	r.Use(utils.CORSHeaders("*"))

	// JSON headers
	r.Use(utils.JSONMiddleware())

	// Health check (no rate limiting, no auth)
	r.GET("/health", healthCheckHandler(db))

	// Public routes
	public := r.Group("/api/v1")
	{
		// Auth - rate limited
		public.POST("/auth/register", func(c *gin.Context) { rateLimiter.ServeHTTP(c) }, handler.Register)
		public.POST("/auth/login", func(c *gin.Context) { rateLimiter.ServeHTTP(c) }, handler.Login)
		public.POST("/auth/refresh", handler.RefreshToken)

		// Public invoice (magic link)
		public.GET("/invoice/:token", handler.GetInvoiceByToken)

		// Webhook (Intasend)
		public.POST("/webhook/intasend", func(c *gin.Context) {
			HandleIntasendWebhook(c, db, invoiceService, intasendService)
		})
	}

	// ===== STATIC FILES - Serve these BEFORE API routes =====
	// Landing page
	r.GET("/", func(c *gin.Context) {
		c.File("./frontend/public/landing.html")
	})
	// Login page
	r.GET("/login", func(c *gin.Context) {
		c.File("./frontend/public/login.html")
	})
	// Register page
	r.GET("/register", func(c *gin.Context) {
		c.File("./frontend/public/register.html")
	})
	// Forgot password
	r.GET("/forgot-password", func(c *gin.Context) {
		c.File("./frontend/public/forgot-password.html")
	})
	// Privacy
	r.GET("/privacy", func(c *gin.Context) {
		c.File("./frontend/public/privacy.html")
	})
	// Terms
	r.GET("/terms", func(c *gin.Context) {
		c.File("./frontend/public/terms.html")
	})
	// Invoice view
	r.GET("/invoice/:token", func(c *gin.Context) {
		c.File("./frontend/public/invoice.html")
	})
	// App (SPA)
	r.GET("/app", func(c *gin.Context) {
		c.File("./frontend/public/index.html")
	})
	// App fallback
	r.NoRoute(func(c *gin.Context) {
		c.File("./frontend/public/index.html")
	})

	// Protected routes
	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware(authService))
	protected.Use(func(c *gin.Context) { rateLimiter.ServeHTTP(c) }) // Apply rate limiting
	{
		// User
		protected.GET("/me", handler.GetMe)
		protected.PUT("/me", handler.UpdateUser)
		protected.POST("/change-password", handler.ChangePassword)
		protected.POST("/logout", handler.Logout)
		protected.POST("/api-keys", handler.GenerateAPIKey)

		// Clients
		protected.POST("/clients", handler.CreateClient)
		protected.GET("/clients", handler.GetClients)
		protected.GET("/clients/:id", handler.GetClient)
		protected.PUT("/clients/:id", handler.UpdateClient)
		protected.DELETE("/clients/:id", handler.DeleteClient)
		protected.GET("/clients/:id/stats", handler.GetClientStats)

		// Invoices
		protected.POST("/invoices", handler.CreateInvoice)
		protected.GET("/invoices", handler.GetInvoices)
		protected.GET("/invoices/:id", handler.GetInvoice)
		protected.PUT("/invoices/:id", handler.UpdateInvoice)
		protected.PUT("/invoices/:id/items", handler.UpdateInvoiceItems)
		protected.POST("/invoices/:id/send", handler.SendInvoice)
		protected.POST("/invoices/:id/cancel", handler.CancelInvoice)
		protected.POST("/invoices/:id/pay", func(c *gin.Context) {
			HandlePaymentRequest(c, db, invoiceService, intasendService)
		})

		// Dashboard
		protected.GET("/dashboard", handler.GetDashboard)
	}

	return r
}

// healthCheckHandler returns server health status
func healthCheckHandler(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check database
		if err := db.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  "database connection failed",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"time":    time.Now().UTC().Format(time.RFC3339),
			"version": "1.0.0",
		})
	}
}
