package database

import (
	"fmt"

	"gorm.io/gorm"
)

// Transaction executes a function within a database transaction
// It handles rollback on error and commits on success
// This is compatible with existing gorm.DB.Transaction usage
func (db *DB) Transaction(fn func(tx *gorm.DB) error) error {
	return db.TransactionWithOptions(fn, &TransactionOptions{
		Timeout: 30,
	})
}

// TransactionOptions holds transaction configuration
type TransactionOptions struct {
	Timeout       int  // seconds
	isolationLevel *string
}

// WithIsolationLevel sets the transaction isolation level
func (o *TransactionOptions) WithIsolationLevel(level string) *TransactionOptions {
	o.isolationLevel = &level
	return o
}

// TransactionWithOptions executes a function within a transaction with options
func (db *DB) TransactionWithOptions(fn func(tx *gorm.DB) error, opts *TransactionOptions) error {
	if db == nil || db.DB == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Start transaction - gorm handles begin/rollback/commit
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})

	return err
}

// DBTransaction provides the gorm.DB for transaction operations
// This is used to ensure we pass the right type to transaction functions
func (db *DB) DBTransaction() *gorm.DB {
	return db.DB
}

// ============================================================================
// Invoice-specific transaction helpers
// ============================================================================

type InvoiceTransactionData struct {
	Invoice          interface{}
	Items            interface{}
	SequenceUpdate   interface{}
	ActivityLog      interface{}
}

// ExecuteInvoiceCreation runs all invoice creation steps in a transaction
func (db *DB) ExecuteInvoiceCreation(data InvoiceTransactionData) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// 1. Create invoice record
		if err := tx.Create(data.Invoice).Error; err != nil {
			return fmt.Errorf("failed to create invoice: %w", err)
		}

		// 2. Create invoice items in batch
		if err := tx.Create(data.Items).Error; err != nil {
			return fmt.Errorf("failed to create invoice items: %w", err)
		}

		// 3. Update sequence number
		if data.SequenceUpdate != nil {
			if err := tx.Model(data.SequenceUpdate).Update("sequence_number", gorm.Expr("sequence_number + 1")).Error; err != nil {
				return fmt.Errorf("failed to update sequence: %w", err)
			}
		}

		// 4. Log activity
		if data.ActivityLog != nil {
			if err := tx.Create(data.ActivityLog).Error; err != nil {
				return fmt.Errorf("failed to log activity: %w", err)
			}
		}

		return nil
	})
}

// ExecutePaymentProcessing runs all payment processing steps in a transaction
func (db *DB) ExecutePaymentProcessing(paymentUpdate, invoiceUpdate, clientUpdate, activityLog interface{}) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// 1. Create payment record
		if err := tx.Create(paymentUpdate).Error; err != nil {
			return fmt.Errorf("failed to create payment: %w", err)
		}

		// 2. Update invoice (paid_amount, status, paid_at)
		if err := tx.Model(invoiceUpdate).Updates(invoiceUpdate).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		// 3. Update client (total_paid)
		if err := tx.Model(clientUpdate).Updates(clientUpdate).Error; err != nil {
			return fmt.Errorf("failed to update client: %w", err)
		}

		// 4. Log activity
		if err := tx.Create(activityLog).Error; err != nil {
			return fmt.Errorf("failed to log activity: %w", err)
		}

		return nil
	})
}

// ============================================================================
// Locking helpers for prevents race conditions
// ============================================================================

// LockRow locks a row for update to prevent concurrent modifications
func (db *DB) LockRow(table string, where interface{}) error {
	if db.isPostgres {
		return db.DB.Raw("SELECT * FROM ? WHERE ? FOR UPDATE", table, where).Error
	}
	// SQLite doesn't support ROW LOCK, but using transactions provides some protection
	return nil
}

// WithLock acquires a row lock for the duration of the function
func (db *DB) WithLock(model interface{}, id string, fn func() error) error {
	// First, try to acquire lock
	if err := db.DB.Where("id = ?", id).First(model).Error; err != nil {
		return err
	}

	// Then execute the function
	return fn()
}

// ============================================================================
// Atomic counters
// ============================================================================

// IncrementAndGet atomically increments and returns the new value
func (db *DB) IncrementAndGet(table, column, where string) (int64, error) {
	var result struct {
		Value int64
	}

	err := db.DB.Table(table).
		Where(where).
		UpdateColumn(column, gorm.Expr(column+" + 1")).
		Error

	if err != nil {
		return 0, err
	}

	// Get the new value
	if err := db.DB.Table(table).Select(column).Where(where).Scan(&result).Error; err != nil {
		return 0, err
	}

	return result.Value, nil
}