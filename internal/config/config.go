package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Intasend  IntasendConfig
	MPesa     MPesaConfig
	JWT       JWTConfig
	Mail      MailConfig
	RateLimit   RateLimitConfig
	CORS        CORSConfig
	Timeouts    TimeoutsConfig
	KRA         KRAConfig
	WhatsApp    WhatsAppConfig
	SMS         SMSConfig
	Stripe      StripeConfig
	QuickBooks  QuickBooksConfig
	Backup      BackupConfig
	RedisCache RedisCacheConfig
}

type ServerConfig struct {
	Port            string
	BaseURL         string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	Mode            string // "development", "production"
	DefaultCurrency string // Default currency (default: KES)
}

type CORSConfig struct {
	Enabled        bool
	AllowedOrigins string
	AllowedMethods string
	AllowedHeaders string
	ExposeHeaders  string
	MaxAge         int
}

type DatabaseConfig struct {
	Driver string
	DSN    string
	// Connection pool settings
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	QueryTimeout    time.Duration
}

type RedisCacheConfig struct {
	RedisAddr     string `env:"REDIS_ADDR"`
	RedisPassword string `env:"REDIS_PASSWORD"`
	RedisDB       int    `env:"REDIS_DB"`
	Environment   string `env:"ENVIRONMENT"`
}

type IntasendConfig struct {
	PublishableKey string
	SecretKey      string
	APIKey         string
	PublicKey      string
	APIURL         string
	WebhookSecret  string
	// Timeouts for external calls
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
}

type MPesaConfig struct {
	Enabled            bool
	ConsumerKey        string
	ConsumerSecret     string
	BusinessShortCode  string
	PassKey            string
	SecurityCredential string // SHA256 hash of online checkout password - for callback verification
	CallbackURL        string
	Environment        string // "sandbox" or "production"
	QueueTimeout       time.Duration
	ResultURL          string
}

type JWTConfig struct {
	Secret        string
	Expiry        time.Duration
	RefreshExpiry time.Duration
}

type SenderProfile struct {
	Name  string
	Email string
}

type MailConfig struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	FromEmail    string
	FromName     string
	Domain       string
	Senders      map[string]SenderProfile
}

type WhatsAppConfig struct {
	Enabled       bool
	APIKey        string
	APISecret     string
	PhoneNumber   string
	PhoneNumberID string
	BusinessID    string
	AccessToken   string
}

type SMSConfig struct {
	Enabled     bool   // Enable/disable SMS service
	Provider    string // Provider: "africastalking", "twilio", "bulk"
	APIKey      string
	APISecret   string
	SenderID    string // Sender ID (e.g., "INVOICEFAST")
	SMSEndpoint string // Custom endpoint for bulk SMS
}

type KRAConfig struct {
	Enabled    bool
	APIURL     string
	APIKey     string
	BranchCode string
	DeviceID   string
	BranchID   string
	PrivateKey string
	CertSerial string
}

type QuickBooksConfig struct {
	Enabled    bool
	ClientID    string
	ClientSecret string
	RealmID     string
	Environment string // sandbox, production
	RedirectURI string
}

type RateLimitConfig struct {
	Enabled         bool
	RequestsPer     int           // requests per window (default)
	Window          time.Duration // time window
	Burst           int           // burst allowance
	CleanupInterval time.Duration
	// Plan-based limits (requests per minute)
	FreeLimit       int `json:"free_limit" env:"RATE_FREE_LIMIT"`
	ProLimit        int `json:"pro_limit" env:"RATE_PRO_LIMIT"`
	AgencyLimit     int `json:"agency_limit" env:"RATE_AGENCY_LIMIT"`
	EnterpriseLimit int `json:"enterprise_limit" env:"RATE_ENTERPRISE_LIMIT"`
	// Environment overrides
	ProductionLimit  int `json:"production_limit" env:"RATE_PRODUCTION_LIMIT"`
	StagingLimit     int `json:"staging_limit" env:"RATE_STAGING_LIMIT"`
	DevelopmentLimit int `json:"development_limit" env:"RATE_DEVELOPMENT_LIMIT"`
}

type TimeoutsConfig struct {
	DatabaseQuery time.Duration
	ExternalAPI   time.Duration
	Request       time.Duration
	Shutdown      time.Duration // graceful shutdown timeout
}

type BackupConfig struct {
	Enabled       bool
	Schedule      string // cron expression (default: "0 3 * * *" = 3am daily)
	LocalDir      string // local backup directory
	S3Bucket      string // S3-compatible bucket name
	S3Region      string
	S3Endpoint    string // optional custom endpoint (e.g., MinIO)
	S3AccessKey   string
	S3SecretKey   string
	RetentionDays int    // days to keep backups
}

type StripeConfig struct {
	SecretKey      string
	PublicKey     string
	WebhookSecret string
}

func Load() *Config {
	ginMode := getEnv("GIN_MODE", "debug")
	isProduction := ginMode == "production"

	// CRITICAL: Validate JWT secret in ALL environments (not just production)
	// Weak secrets in development can be exploited if code is deployed elsewhere
	jwtSecret := os.Getenv("JWT_SECRET")
	if err := validateJWTSecret(jwtSecret, isProduction); err != nil {
		log.Fatalf("FATAL: JWT secret validation failed: %v", err)
	}

	domain := getEnv("MAIL_SENDER_DOMAIN", "invoicefast.app")

	return &Config{
		Server: ServerConfig{
			Port:            getEnv("PORT", "8082"),
			BaseURL:         getEnv("BASE_URL", "https://invoice.simuxtech.com"),
			ReadTimeout:     getDurationEnv("READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getDurationEnv("WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:     getDurationEnv("IDLE_TIMEOUT", 120*time.Second),
			Mode:            ginMode,
			DefaultCurrency: getEnv("DEFAULT_CURRENCY", "KES"),
		},
		Database: DatabaseConfig{
			Driver: getEnv("DB_DRIVER", "sqlite3"),
			DSN:    getEnv("DB_DSN", "./data/invoicefast.db"),
			// Connection pool - prevent exhaustion
			MaxOpenConns:    getIntEnv("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getIntEnv("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getDurationEnv("DB_CONN_MAX_LIFETIME", 5*time.Minute),
			ConnMaxIdleTime: getDurationEnv("DB_CONN_MAX_IDLE_TIME", 1*time.Minute),
			QueryTimeout:    getDurationEnv("DB_QUERY_TIMEOUT", 10*time.Second),
		},
		Intasend: IntasendConfig{
			PublishableKey: getEnv("INTASEND_PUBLISHABLE_KEY", ""),
			SecretKey:      getEnv("INTASEND_SECRET_KEY", ""),
			APIKey:         getEnv("INTASEND_API_KEY", ""),
			PublicKey:      getEnv("INTASEND_PUBLIC_KEY", ""),
			APIURL:         getEnv("INTASEND_API_URL", "https://sandbox.intasend.com"),
			WebhookSecret:  getEnv("INTASEND_WEBHOOK_SECRET", ""),
			ConnectTimeout: getDurationEnv("INTASEND_CONNECT_TIMEOUT", 10*time.Second),
			ReadTimeout:    getDurationEnv("INTASEND_READ_TIMEOUT", 30*time.Second),
		},
		MPesa: MPesaConfig{
			Enabled:            getBoolEnv("MPESA_ENABLED", false),
			ConsumerKey:        getEnv("MPESA_CONSUMER_KEY", ""),
			ConsumerSecret:     getEnv("MPESA_CONSUMER_SECRET", ""),
			BusinessShortCode:  getEnv("MPESA_BUSINESS_SHORT_CODE", ""),
			PassKey:            getEnv("MPESA_PASS_KEY", ""),
			SecurityCredential: getEnv("MPESA_SECURITY_CREDENTIAL", ""), // SHA256 hash of online checkout password
			CallbackURL:        getEnv("MPESA_CALLBACK_URL", ""),
			Environment:        getEnv("MPESA_ENVIRONMENT", "sandbox"),
			QueueTimeout:       getDurationEnv("MPESA_QUEUE_TIMEOUT", 30*time.Second),
			ResultURL:          getEnv("MPESA_RESULT_URL", ""),
		},
		JWT: JWTConfig{
			Secret:        getEnv("JWT_SECRET", "dev-secret-change-in-production-min-32-chars!"),
			Expiry:        getDurationEnv("JWT_EXPIRY", 24*time.Hour),
			RefreshExpiry: getDurationEnv("JWT_REFRESH_EXPIRY", 7*24*time.Hour),
		},
		Mail: MailConfig{
			SMTPHost:     getEnv("SMTP_HOST", "smtp.gmail.com"),
			SMTPPort:     getEnv("SMTP_PORT", "587"),
			SMTPUsername: getEnv("SMTP_USERNAME", ""),
			SMTPPassword: getEnv("SMTP_PASSWORD", ""),
			FromEmail:    getEnv("FROM_EMAIL", "noreply@"+domain),
			FromName:     getEnv("FROM_NAME", "InvoiceFast"),
			Domain:       domain,
			Senders: map[string]SenderProfile{
				"billing": {
					Name:  getEnv("MAIL_SENDER_BILLING_NAME", "InvoiceFast Billing"),
					Email: getEnv("MAIL_SENDER_BILLING_EMAIL", "billing@"+domain),
				},
				"info": {
					Name:  getEnv("MAIL_SENDER_INFO_NAME", "InvoiceFast"),
					Email: getEnv("MAIL_SENDER_INFO_EMAIL", "info@"+domain),
				},
				"noreply": {
					Name:  getEnv("MAIL_SENDER_NOREPLY_NAME", "InvoiceFast"),
					Email: getEnv("MAIL_SENDER_NOREPLY_EMAIL", "noreply@"+domain),
				},
			},
		},
		RateLimit: RateLimitConfig{
			Enabled:         getBoolEnv("RATE_LIMIT_ENABLED", true),
			RequestsPer:     getIntEnv("RATE_LIMIT_REQUESTS", 100),
			Window:          getDurationEnv("RATE_LIMIT_WINDOW", 1*time.Minute),
			Burst:           getIntEnv("RATE_LIMIT_BURST", 20),
			CleanupInterval: getDurationEnv("RATE_LIMIT_CLEANUP", 5*time.Minute),
		},
		CORS: CORSConfig{
			Enabled:        getBoolEnv("CORS_ENABLED", true),
			AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:8082"),
			AllowedMethods: getEnv("ALLOWED_METHODS", "GET,POST,PUT,DELETE,OPTIONS,PATCH"),
			AllowedHeaders: getEnv("ALLOWED_HEADERS", "Content-Type,Authorization,X-API-Key,Accept,Origin"),
			ExposeHeaders:  getEnv("EXPOSE_HEADERS", "Content-Length,X-Request-ID"),
			MaxAge:         getIntEnv("CORS_MAX_AGE", 86400),
		},
		Timeouts: TimeoutsConfig{
			DatabaseQuery: getDurationEnv("TIMEOUT_DB_QUERY", 10*time.Second),
			ExternalAPI:   getDurationEnv("TIMEOUT_EXTERNAL_API", 30*time.Second),
			Request:       getDurationEnv("TIMEOUT_REQUEST", 60*time.Second),
			Shutdown:      getDurationEnv("TIMEOUT_SHUTDOWN", 30*time.Second),
		},
		KRA: KRAConfig{
			Enabled:    getBoolEnv("KRA_ENABLED", false),
			APIURL:     getEnv("KRA_API_URL", "https://api.kra.go.ke"),
			APIKey:     getEnv("KRA_API_KEY", ""),
			BranchCode: getEnv("KRA_BRANCH_CODE", ""),
			DeviceID:   getEnv("KRA_DEVICE_ID", ""),
			BranchID:   getEnv("KRA_BRANCH_ID", ""),
			PrivateKey: getEnv("KRA_PRIVATE_KEY", ""),
			CertSerial: getEnv("KRA_CERT_SERIAL", ""),
		},
		WhatsApp: WhatsAppConfig{
			Enabled:       getBoolEnv("WHATSAPP_ENABLED", false),
			APIKey:        getEnv("WHATSAPP_API_KEY", ""),
			APISecret:     getEnv("WHATSAPP_API_SECRET", ""),
			PhoneNumber:   getEnv("WHATSAPP_PHONE_NUMBER", ""),
			PhoneNumberID: getEnv("WHATSAPP_PHONE_NUMBER_ID", ""),
			BusinessID:    getEnvWithFallback("WHATSAPP_BUSINESS_ACCOUNT_ID", "WHATSAPP_BUSINESS_ID", ""),
			AccessToken:   getEnv("WHATSAPP_ACCESS_TOKEN", ""),
		},
		SMS: SMSConfig{
			Enabled:     getBoolEnv("SMS_ENABLED", false),
			Provider:    getEnv("SMS_PROVIDER", "africastalking"),
			APIKey:      getEnv("SMS_API_KEY", ""),
			APISecret:   getEnv("SMS_API_SECRET", ""),
			SenderID:    getEnv("SMS_SENDER_ID", "INVOICEFAST"),
			SMSEndpoint: getEnv("SMS_ENDPOINT", ""),
		},
		Stripe: StripeConfig{
			SecretKey:      getEnv("STRIPE_SECRET_KEY", ""),
			PublicKey:     getEnv("STRIPE_PUBLIC_KEY", ""),
			WebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		},
		QuickBooks: QuickBooksConfig{
			Enabled:    getBoolEnv("QUICKBOOKS_ENABLED", false),
			ClientID:    getEnv("QUICKBOOKS_CLIENT_ID", ""),
			ClientSecret: getEnv("QUICKBOOKS_CLIENT_SECRET", ""),
			RealmID:     getEnv("QUICKBOOKS_REALM_ID", ""),
			Environment: getEnv("QUICKBOOKS_ENVIRONMENT", "sandbox"), // sandbox, production
			RedirectURI: getEnv("QUICKBOOKS_REDIRECT_URI", ""),
		},
		
		RedisCache: RedisCacheConfig{
			RedisAddr:     getEnv("REDIS_ADDR", "localhost"),
			RedisPassword: getEnv("REDIS_PASSWORD", ""),
			RedisDB:       getIntEnv("REDIS_DB", 0),
			Environment:   getEnv("REDIS_ENVIRONMENT", "development"),
			
		},
		Backup: BackupConfig{
			Enabled:       getBoolEnv("BACKUP_ENABLED", true),
			Schedule:      getEnv("BACKUP_SCHEDULE", "0 3 * * *"),
			LocalDir:      getEnv("BACKUP_LOCAL_DIR", "./data/backups"),
			S3Bucket:      getEnv("BACKUP_S3_BUCKET", ""),
			S3Region:      getEnv("BACKUP_S3_REGION", "us-east-1"),
			S3Endpoint:    getEnv("BACKUP_S3_ENDPOINT", ""),
			S3AccessKey:   getEnv("BACKUP_S3_ACCESS_KEY", ""),
			S3SecretKey:   getEnv("BACKUP_S3_SECRET_KEY", ""),
			RetentionDays: getIntEnv("BACKUP_RETENTION_DAYS", 30),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvWithFallback(primary, fallback, defaultValue string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	if value := os.Getenv(fallback); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal := parseInt(value); intVal > 0 {
			return intVal
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if dur, err := time.ParseDuration(value); err == nil {
			return dur
		}
	}
	return defaultValue
}

func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

// validateJWTSecret validates JWT secret strength
func validateJWTSecret(secret string, isProduction bool) error {
	developmentDefault := "dev-secret-change-in-production-min-32-chars!"

	// Check for default/empty secrets
	if secret == "" || secret == developmentDefault {
		if isProduction {
			return fmt.Errorf("JWT_SECRET must be set in production (got empty/default value)")
		}
		// In development, log a warning but allow it
		log.Printf("WARNING: Using default JWT secret. Set JWT_SECRET environment variable for security.")
		return nil
	}

	// Check minimum length
	minLength := 32
	if isProduction {
		minLength = 64 // Stronger for production
	}
	if len(secret) < minLength {
		return fmt.Errorf("JWT_SECRET must be at least %d characters (got %d)", minLength, len(secret))
	}

	// Check for entropy/complexity
	if isProduction {
		if hasLowEntropy(secret) {
			return fmt.Errorf("JWT_SECRET has low entropy - use a more complex secret")
		}
	}

	// Check for known bad secrets
	badSecrets := []string{
		"secret", "password", "123456", "jwt-secret", "change-me",
		"your-secret-key", "super-secret", "invoice-secret",
	}
	secretLower := strings.ToLower(secret)
	for _, bad := range badSecrets {
		if strings.Contains(secretLower, bad) {
			return fmt.Errorf("JWT_SECRET contains weak pattern: %s", bad)
		}
	}

	return nil
}

func validateEncryptionKey(key string, isProduction bool) error {
	developmentDefault := "dev-key-32-chars-minimum!!"

	if key == "" || key == developmentDefault {
		if isProduction {
			return fmt.Errorf("ENCRYPTION_KEY must be set in production (got empty/default value)")
		}
		log.Printf("WARNING: Using default encryption key. Set ENCRYPTION_KEY environment variable for security.")
		return nil
	}

	minLength := 32
	if len(key) < minLength {
		return fmt.Errorf("ENCRYPTION_KEY must be at least %d characters (got %d)", minLength, len(key))
	}

	return nil
}

// hasLowEntropy checks if a string has predictable patterns
func hasLowEntropy(s string) bool {
	// Check for repeated characters
	repeated := 0
	for i := 1; i < len(s); i++ {
		if s[i] == s[i-1] {
			repeated++
		}
	}
	if repeated > len(s)/3 {
		return true
	}

	// Check for common patterns
	patterns := []string{"123456", "qwerty", "abcdef", "password", "secret"}
	for _, p := range patterns {
		if strings.Contains(strings.ToLower(s), p) {
			return true
		}
	}

	return false
}

// validateProductionConfig validates critical production settings
func validateProductionConfig() {
	// Check for critical production configurations
	warnings := []string{}

	// Validate CORS origins
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" || allowedOrigins == "http://localhost:3000,http://localhost:8082" {
		log.Println("WARNING: ALLOWED_ORIGINS not properly configured for production")
	}

	// Validate webhook secret
	webhookSecret := os.Getenv("INTASEND_WEBHOOK_SECRET")
	if webhookSecret == "" {
		log.Println("WARNING: INTASEND_WEBHOOK_SECRET not set - webhook verification will fail")
	}

	// Validate database connection
	dbDriver := os.Getenv("DB_DRIVER")
	if dbDriver == "sqlite3" || dbDriver == "" {
		log.Println("WARNING: SQLite is not recommended for production. Consider PostgreSQL.")
	}

	// Check for SSL/HTTPS
	port := os.Getenv("PORT")
	if port == "8082" || port == "" {
		baseURL := os.Getenv("BASE_URL")
		if baseURL != "" && strings.HasPrefix(baseURL, "http://") {
			log.Println("WARNING: BASE_URL uses HTTP. Use HTTPS in production.")
		}
	}

	// Email configuration
	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		warnings = append(warnings, "SMTP_HOST not configured - emails will not be sent")
	}

	if len(warnings) > 0 {
		log.Println("=== Production Configuration Warnings ===")
		for _, w := range warnings {
			log.Printf("  - %s\n", w)
		}
	}
}
