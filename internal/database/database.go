package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	*gorm.DB
	sqlDB *sql.DB
}

func New(cfg *config.DatabaseConfig) (*DB, error) {
	// Configure GORM
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		PrepareStmt: false, // Disable prepared statements for SQLite
	}

	db, err := gorm.Open(sqlite.Open(cfg.DSN), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying sql.DB for connection pool settings
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// SQLite works better with a single connection to avoid locking issues
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Database connected: SQLite (single connection mode)")

	return &DB{DB: db, sqlDB: sqlDB}, nil
}

func (db *DB) Migrate() error {
	log.Println("Running database migrations...")

	// Enable WAL mode for SQLite
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		log.Printf("Warning: failed to set WAL mode: %v", err)
	}

	// Set busy timeout
	if err := db.Exec("PRAGMA busy_timeout = 5000").Error; err != nil {
		log.Printf("Warning: failed to set busy timeout: %v", err)
	}

	// Auto migrate
	err := db.AutoMigrate(
		&models.User{},
		&models.Client{},
		&models.Invoice{},
		&models.InvoiceItem{},
		&models.Payment{},
		&models.Reminder{},
		&models.Template{},
		&models.RefreshToken{},
		&models.AuditLog{},
		&models.APIKey{},
		&models.ExchangeRate{},
		&models.KRAQueueItem{},
	)
	if err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	log.Println("Migrations completed successfully")
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

// SeedDefaultTemplates placeholder
func (db *DB) SeedDefaultTemplates(userID string) error {
	return nil
}
