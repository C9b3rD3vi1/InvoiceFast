package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	*gorm.DB
	sqlDB      *sql.DB
	isPostgres bool
}

// New creates a database connection based on the driver configuration
// Supports SQLite (development) and PostgreSQL (production)
func New(cfg *config.DatabaseConfig) (*DB, error) {
	var db *gorm.DB
	var err error

	// Determine driver and configure accordingly
	switch cfg.Driver {
	case "postgres", "postgresql":
		db, err = newPostgresDB(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		log.Println("Database connected: PostgreSQL")

	case "sqlite", "sqlite3":
		db, err = newSQLiteDB(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to SQLite: %w", err)
		}
		log.Println("Database connected: SQLite")

	default:
		// Default to SQLite for development
		log.Printf("Unknown driver '%s', defaulting to SQLite", cfg.Driver)
		db, err = newSQLiteDB(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to SQLite: %w", err)
		}
		log.Println("Database connected: SQLite (default)")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	return &DB{
		DB:         db,
		sqlDB:      sqlDB,
		isPostgres: cfg.Driver == "postgres" || cfg.Driver == "postgresql",
	}, nil
}

// newPostgresDB creates a new PostgreSQL connection with proper pooling
func newPostgresDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		PrepareStmt: true, // Enable prepared statements for better performance
	}

	// Build PostgreSQL DSN from config or use direct DSN
	dsn := cfg.DSN
	if dsn == "" {
		dsn = buildPostgresDSN(cfg)
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, err
	}

	// Configure connection pool for production
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// PostgreSQL supports proper connection pooling
	maxOpenConns := 25
	maxIdleConns := 10
	connMaxLifetime := 5 * time.Minute
	connMaxIdleTime := 1 * time.Minute

	if cfg.MaxOpenConns > 0 {
		maxOpenConns = cfg.MaxOpenConns
	}
	if cfg.MaxIdleConns > 0 {
		maxIdleConns = cfg.MaxIdleConns
	}
	if cfg.ConnMaxLifetime > 0 {
		connMaxLifetime = cfg.ConnMaxLifetime
	}
	if cfg.ConnMaxIdleTime > 0 {
		connMaxIdleTime = cfg.ConnMaxIdleTime
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	log.Printf("PostgreSQL connection pool: max_open=%d, max_idle=%d, lifetime=%v",
		maxOpenConns, maxIdleConns, connMaxLifetime)

	return db, nil
}

// newSQLiteDB creates a new SQLite connection (development)
func newSQLiteDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		PrepareStmt: false, // Disable prepared statements for SQLite
	}

	// Use in-memory if DSN is ":memory:"
	dsn := cfg.DSN
	if dsn == "" {
		dsn = "./data/invoicefast.db"
	}

	db, err := gorm.Open(sqlite.Open(dsn), gormConfig)
	if err != nil {
		return nil, err
	}

	// SQLite works better with limited connections to avoid locking
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// For SQLite, limit connections to avoid locking issues
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	return db, nil
}

// buildPostgresDSN builds PostgreSQL connection string from config
func buildPostgresDSN(cfg *config.DatabaseConfig) string {
	// If DSN is provided directly, use it
	if cfg.DSN != "" {
		return cfg.DSN
	}

	// Build from individual components (would need additional env vars)
	// This is a fallback - in production, provide full DSN via DB_DSN
	host := "localhost"
	port := "5432"
	user := "postgres"
	password := ""
	dbname := "invoicefast"
	sslmode := "disable"

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)
}

// Migrate runs database migrations based on the database type
func (db *DB) Migrate() error {
	log.Println("Running database migrations...")

	// PostgreSQL-specific settings
	if db.isPostgres {
		// PostgreSQL doesn't need WAL mode
		log.Println("Using PostgreSQL - migrations ready")
	} else {
		// SQLite-specific settings
		if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
			log.Printf("Warning: failed to set WAL mode: %v", err)
		}
		if err := db.Exec("PRAGMA busy_timeout = 5000").Error; err != nil {
			log.Printf("Warning: failed to set busy timeout: %v", err)
		}
	}

	// Auto migrate all models
	err := db.AutoMigrate(
		&models.User{},
		&models.Client{},
		&models.Invoice{},
		&models.InvoiceItem{},
		&models.InvoiceSequence{},
		&models.ItemLibrary{},
		&models.Attachment{},
		&models.Payment{},
		&models.Reminder{},
		&models.Template{},
		&models.RefreshToken{},
		&models.AuditLog{},
		&models.APIKey{},
		&models.ExchangeRate{},
		&models.KRAQueueItem{},
		&models.Tenant{},
		&models.PasswordResetToken{},
		&models.Notification{},
		&models.NotificationLog{},
		&models.EmailTracking{},
		&models.EmailTrackingLink{},
		&models.TeamInvite{},
		&models.Automation{},
		&models.AutomationLog{},
		&models.SubscriptionPlan{},
		&models.Subscription{},
		&models.SubscriptionTransaction{},
		&models.UsageTracking{},
		&models.SavedPaymentMethod{},
		&models.BillingInvoice{},
		&models.LateFeeConfig{},
		&models.LateFeeInvoice{},
		&models.UnallocatedPayment{},
		&models.ReminderSequence{},
		&models.ReminderSequenceLog{},
		// Expense models
		&models.Expense{},
		&models.ExpenseCategory{},
		&models.ExpenseAttachment{},
		// Integration models
		&models.Integration{},
		// Security models
		&models.UserSession{},
	)
	if err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	// Fix any incorrect unique constraints
	if err := db.fixPaymentConstraints(); err != nil {
		log.Printf("Warning: failed to fix payment constraints: %v", err)
	}

	log.Println("Migrations completed successfully")
	return nil
}

// fixPaymentConstraints removes incorrect unique constraints on payments table
func (db *DB) fixPaymentConstraints() error {
	// Drop unique index on tenant_id if it exists (tenant should have multiple payments)
	if db.isPostgres {
		// PostgreSQL
		db.Exec("DROP INDEX IF EXISTS idx_payments_tenant_id")
		// Create proper non-unique index
		db.Exec("CREATE INDEX IF NOT EXISTS idx_payments_tenant_id ON payments(tenant_id)")
	} else {
		// SQLite
		db.Exec("DROP INDEX IF EXISTS idx_payments_tenant_id")
		// Create proper non-unique index
		db.Exec("CREATE INDEX IF NOT EXISTS idx_payments_tenant_id ON payments(tenant_id)")
	}
	return nil
}

// Ping checks database connectivity
func (db *DB) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return db.sqlDB.PingContext(ctx)
}

// Stats returns database connection pool statistics
func (db *DB) Stats() sql.DBStats {
	return db.sqlDB.Stats()
}

// Close closes database connections gracefully
func (db *DB) Close() error {
	if db.sqlDB != nil {
		db.sqlDB.SetMaxOpenConns(0)
		db.sqlDB.SetMaxIdleConns(0)
		return db.sqlDB.Close()
	}
	return nil
}

// IsPostgres returns true if using PostgreSQL
func (db *DB) IsPostgres() bool {
	return db.isPostgres
}

// SeedDefaultTemplates placeholder
func (db *DB) SeedDefaultTemplates(userID string) error {
	return nil
}
