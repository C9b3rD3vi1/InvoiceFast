package config

import (
	"os"
	"time"
)

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Intasend  IntasendConfig
	JWT       JWTConfig
	Mail      MailConfig
	RateLimit RateLimitConfig
	Timeouts  TimeoutsConfig
}

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	Mode         string // "development", "production"
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

type IntasendConfig struct {
	PublishableKey string
	SecretKey      string
	APIURL         string
	WebhookSecret  string
	// Timeouts for external calls
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
}

type JWTConfig struct {
	Secret        string
	Expiry        time.Duration
	RefreshExpiry time.Duration
}

type MailConfig struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	FromEmail    string
	FromName     string
}

type RateLimitConfig struct {
	Enabled         bool
	RequestsPer     int           // requests per window
	Window          time.Duration // time window
	Burst           int           // burst allowance
	CleanupInterval time.Duration
}

type TimeoutsConfig struct {
	DatabaseQuery time.Duration
	ExternalAPI   time.Duration
	Request       time.Duration
	Shutdown      time.Duration // graceful shutdown timeout
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("PORT", "8082"),
			ReadTimeout:  getDurationEnv("READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getDurationEnv("WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  getDurationEnv("IDLE_TIMEOUT", 120*time.Second),
			Mode:         getEnv("GIN_MODE", "debug"),
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
			APIURL:         getEnv("INTASEND_API_URL", "https://sandbox.intasend.com"),
			WebhookSecret:  getEnv("INTASEND_WEBHOOK_SECRET", ""),
			ConnectTimeout: getDurationEnv("INTASEND_CONNECT_TIMEOUT", 10*time.Second),
			ReadTimeout:    getDurationEnv("INTASEND_READ_TIMEOUT", 30*time.Second),
		},
		JWT: JWTConfig{
			Secret:        getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
			Expiry:        getDurationEnv("JWT_EXPIRY", 24*time.Hour),
			RefreshExpiry: getDurationEnv("JWT_REFRESH_EXPIRY", 7*24*time.Hour),
		},
		Mail: MailConfig{
			SMTPHost:     getEnv("SMTP_HOST", "smtp.gmail.com"),
			SMTPPort:     getEnv("SMTP_PORT", "587"),
			SMTPUsername: getEnv("SMTP_USERNAME", ""),
			SMTPPassword: getEnv("SMTP_PASSWORD", ""),
			FromEmail:    getEnv("FROM_EMAIL", "noreply@invoicefast.com"),
			FromName:     getEnv("FROM_NAME", "InvoiceFast"),
		},
		RateLimit: RateLimitConfig{
			Enabled:         getBoolEnv("RATE_LIMIT_ENABLED", true),
			RequestsPer:     getIntEnv("RATE_LIMIT_REQUESTS_PER", 100),
			Window:          getDurationEnv("RATE_LIMIT_WINDOW", 1*time.Minute),
			Burst:           getIntEnv("RATE_LIMIT_BURST", 20),
			CleanupInterval: getDurationEnv("RATE_LIMIT_CLEANUP", 5*time.Minute),
		},
		Timeouts: TimeoutsConfig{
			DatabaseQuery: getDurationEnv("TIMEOUT_DB_QUERY", 10*time.Second),
			ExternalAPI:   getDurationEnv("TIMEOUT_EXTERNAL_API", 30*time.Second),
			Request:       getDurationEnv("TIMEOUT_REQUEST", 60*time.Second),
			Shutdown:      getDurationEnv("TIMEOUT_SHUTDOWN", 30*time.Second),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
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
