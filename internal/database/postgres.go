package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"invoicefast/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// PostgreSQLDB wraps the GORM DB for PostgreSQL
type PostgreSQLDB struct {
	*DB
	sqlDB *sql.DB
}

// NewPostgreSQL creates a new PostgreSQL database connection
func NewPostgreSQL(cfg *config.DatabaseConfig) (*DB, error) {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		PrepareStmt: true,
	}

	// Open PostgreSQL connection
	db, err := gorm.Open(postgres.Open(cfg.DSN), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Get underlying sql.DB for connection pool settings
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	log.Printf("[PostgreSQL] Connected: max_open=%d, max_idle=%d, lifetime=%v",
		cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLifetime)

	return &DB{DB: db, sqlDB: sqlDB}, nil
}

// Migration handles database migrations between SQLite and PostgreSQL
type Migration struct {
	db *DB
}

// NewMigration creates a new migration handler
func NewMigration(db *DB) *Migration {
	return &Migration{db: db}
}

// MigrateFromSQLite migrates data from SQLite to PostgreSQL
func (m *Migration) MigrateFromSQLite(sqlitePath string) error {
	log.Println("[Migration] Starting migration from SQLite to PostgreSQL...")

	// Open SQLite
	sqliteDB, err := New(&config.DatabaseConfig{
		Driver:    "sqlite3",
		DSN:       sqlitePath,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		return fmt.Errorf("failed to open SQLite: %w", err)
	}
	defer sqliteDB.Close()

	// Migrate in order (respecting foreign keys)
	tables := []string{
		"users",
		"refresh_tokens",
		"api_keys",
		"clients",
		"templates",
		"invoices",
		"invoice_items",
		"payments",
		"reminders",
		"audit_logs",
	}

	totalMigrated := 0
	for _, table := range tables {
		count, err := m.migrateTable(sqliteDB, table)
		if err != nil {
			log.Printf("[Migration] Warning: error migrating %s: %v", table, err)
			continue
		}
		log.Printf("[Migration] Migrated %d records from %s", count, table)
		totalMigrated += count
	}

	log.Printf("[Migration] Completed! Total records migrated: %d", totalMigrated)
	return nil
}

// migrateTable migrates a single table
func (m *Migration) migrateTable(sqliteDB *DB, table string) (int, error) {
	switch table {
	case "users":
		return m.migrateUsers(sqliteDB)
	case "clients":
		return m.migrateClients(sqliteDB)
	case "invoices":
		return m.migrateInvoices(sqliteDB)
	case "invoice_items":
		return m.migrateInvoiceItems(sqliteDB)
	case "payments":
		return m.migratePayments(sqliteDB)
	case "refresh_tokens":
		return m.migrateRefreshTokens(sqliteDB)
	case "api_keys":
		return m.migrateAPIKeys(sqliteDB)
	case "templates":
		return m.migrateTemplates(sqliteDB)
	case "reminders":
		return m.migrateReminders(sqliteDB)
	case "audit_logs":
		return m.migrateAuditLogs(sqliteDB)
	default:
		return 0, fmt.Errorf("unknown table: %s", table)
	}
}

func (m *Migration) migrateUsers(sqliteDB *DB) (int, error) {
	type OldUser struct {
		ID           string    `gorm:"column:id"`
		Email        string    `gorm:"column:email"`
		PasswordHash string    `gorm:"column:password_hash"`
		Name         string    `gorm:"column:name"`
		Phone        string    `gorm:"column:phone"`
		CompanyName  string    `gorm:"column:company_name"`
		KRAPIN       string    `gorm:"column:kra_pin"`
		Plan         string    `gorm:"column:plan"`
		IsActive     bool      `gorm:"column:is_active"`
		CreatedAt    time.Time `gorm:"column:created_at"`
		UpdatedAt    time.Time `gorm:"column:updated_at"`
	}

	var users []OldUser
	if err := sqliteDB.Table("users").Find(&users).Error; err != nil {
		return 0, err
	}

	for _, u := range users {
		if err := m.db.Exec(`
			INSERT INTO users (id, email, password_hash, name, phone, company_name, kra_pin, plan, is_active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO NOTHING
		`, u.ID, u.Email, u.PasswordHash, u.Name, u.Phone, u.CompanyName, u.KRAPIN, u.Plan, u.IsActive, u.CreatedAt, u.UpdatedAt).Error; err != nil {
			log.Printf("[Migration] Error inserting user %s: %v", u.Email, err)
		}
	}

	return len(users), nil
}

func (m *Migration) migrateClients(sqliteDB *DB) (int, error) {
	type OldClient struct {
		ID           string    `gorm:"column:id"`
		UserID       string    `gorm:"column:user_id"`
		Name         string    `gorm:"column:name"`
		Email        string    `gorm:"column:email"`
		Phone        string    `gorm:"column:phone"`
		Address      string    `gorm:"column:address"`
		KRAPIN       string    `gorm:"column:kra_pin"`
		Currency     string    `gorm:"column:currency"`
		PaymentTerms int       `gorm:"column:payment_terms"`
		Notes        string    `gorm:"column:notes"`
		TotalBilled  float64   `gorm:"column:total_billed"`
		TotalPaid    float64   `gorm:"column:total_paid"`
		CreatedAt    time.Time `gorm:"column:created_at"`
		UpdatedAt    time.Time `gorm:"column:updated_at"`
	}

	var clients []OldClient
	if err := sqliteDB.Table("clients").Find(&clients).Error; err != nil {
		return 0, err
	}

	for _, c := range clients {
		if err := m.db.Exec(`
			INSERT INTO clients (id, user_id, name, email, phone, address, kra_pin, currency, payment_terms, notes, total_billed, total_paid, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO NOTHING
		`, c.ID, c.UserID, c.Name, c.Email, c.Phone, c.Address, c.KRAPIN, c.Currency, c.PaymentTerms, c.Notes, c.TotalBilled, c.TotalPaid, c.CreatedAt, c.UpdatedAt).Error; err != nil {
			log.Printf("[Migration] Error inserting client %s: %v", c.Name, err)
		}
	}

	return len(clients), nil
}

func (m *Migration) migrateInvoices(sqliteDB *DB) (int, error) {
	var invoices []map[string]interface{}
	if err := sqliteDB.Table("invoices").Find(&invoices).Error; err != nil {
		return 0, err
	}

	for _, inv := range invoices {
		columns := make([]string, 0)
		placeholders := make([]string, 0)
		values := make([]interface{}, 0)

		for k, v := range inv {
			columns = append(columns, k)
			placeholders = append(placeholders, "?")
			values = append(values, v)
		}

		query := fmt.Sprintf("INSERT INTO invoices (%s) VALUES (%s) ON CONFLICT (id) DO NOTHING",
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "))

		if err := m.db.Exec(query, values...).Error; err != nil {
			log.Printf("[Migration] Error inserting invoice %v: %v", inv["invoice_number"], err)
		}
	}

	return len(invoices), nil
}

func (m *Migration) migrateInvoiceItems(sqliteDB *DB) (int, error) {
	var items []map[string]interface{}
	if err := sqliteDB.Table("invoice_items").Find(&items).Error; err != nil {
		return 0, err
	}

	for _, item := range items {
		if err := m.db.Table("invoice_items").Create(item).Error; err != nil {
			// Ignore duplicate key errors
			if !strings.Contains(err.Error(), "duplicate key") {
				log.Printf("[Migration] Error inserting invoice item: %v", err)
			}
		}
	}

	return len(items), nil
}

func (m *Migration) migratePayments(sqliteDB *DB) (int, error) {
	var payments []map[string]interface{}
	if err := sqliteDB.Table("payments").Find(&payments).Error; err != nil {
		return 0, err
	}

	for _, p := range payments {
		if err := m.db.Table("payments").Create(p).Error; err != nil {
			if !strings.Contains(err.Error(), "duplicate key") {
				log.Printf("[Migration] Error inserting payment: %v", err)
			}
		}
	}

	return len(payments), nil
}

func (m *Migration) migrateRefreshTokens(sqliteDB *DB) (int, error) {
	var tokens []map[string]interface{}
	if err := sqliteDB.Table("refresh_tokens").Find(&tokens).Error; err != nil {
		return 0, err
	}

	for _, t := range tokens {
		if err := m.db.Table("refresh_tokens").Create(t).Error; err != nil {
			if !strings.Contains(err.Error(), "duplicate key") {
				log.Printf("[Migration] Error inserting refresh token: %v", err)
			}
		}
	}

	return len(tokens), nil
}

func (m *Migration) migrateAPIKeys(sqliteDB *DB) (int, error) {
	var keys []map[string]interface{}
	if err := sqliteDB.Table("api_keys").Find(&keys).Error; err != nil {
		return 0, err
	}

	for _, k := range keys {
		if err := m.db.Table("api_keys").Create(k).Error; err != nil {
			if !strings.Contains(err.Error(), "duplicate key") {
				log.Printf("[Migration] Error inserting API key: %v", err)
			}
		}
	}

	return len(keys), nil
}

func (m *Migration) migrateTemplates(sqliteDB *DB) (int, error) {
	var templates []map[string]interface{}
	if err := sqliteDB.Table("templates").Find(&templates).Error; err != nil {
		return 0, err
	}

	for _, t := range templates {
		if err := m.db.Table("templates").Create(t).Error; err != nil {
			if !strings.Contains(err.Error(), "duplicate key") {
				log.Printf("[Migration] Error inserting template: %v", err)
			}
		}
	}

	return len(templates), nil
}

func (m *Migration) migrateReminders(sqliteDB *DB) (int, error) {
	var reminders []map[string]interface{}
	if err := sqliteDB.Table("reminders").Find(&reminders).Error; err != nil {
		return 0, err
	}

	for _, r := range reminders {
		if err := m.db.Table("reminders").Create(r).Error; err != nil {
			if !strings.Contains(err.Error(), "duplicate key") {
				log.Printf("[Migration] Error inserting reminder: %v", err)
			}
		}
	}

	return len(reminders), nil
}

func (m *Migration) migrateAuditLogs(sqliteDB *DB) (int, error) {
	var logs []map[string]interface{}
	if err := sqliteDB.Table("audit_logs").Find(&logs).Error; err != nil {
		return 0, err
	}

	for _, l := range logs {
		if err := m.db.Table("audit_logs").Create(l).Error; err != nil {
			if !strings.Contains(err.Error(), "duplicate key") {
				log.Printf("[Migration] Error inserting audit log: %v", err)
			}
		}
	}

	return len(logs), nil
}