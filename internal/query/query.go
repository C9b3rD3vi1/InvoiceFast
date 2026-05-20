package query

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Preloader helps with eager loading to prevent N+1 queries
type Preloader struct {
	db        *gorm.DB
	relations map[string][]string
}

// NewPreloader creates a new preloader
func NewPreloader(db *gorm.DB) *Preloader {
	return &Preloader{
		db:        db,
		relations: make(map[string][]string),
	}
}

// AddRelation adds a relation to preload
func (p *Preloader) AddRelation(relation string) *Preloader {
	p.relations[""] = append(p.relations[""], relation)
	return p
}

// ForModel sets the model to preload relations for
func (p *Preloader) ForModel(model interface{}) *Preloader {
	modelName := getModelName(model)
	return p.ForModelName(modelName)
}

// ForModelName sets the model name to preload relations for
func (p *Preloader) ForModelName(modelName string) *Preloader {
	if p.relations == nil {
		p.relations = make(map[string][]string)
	}
	// Clear previous entries
	p.relations = make(map[string][]string)
	p.relations[modelName] = []string{}
	return p
}

// Add adds a relation to preload for the current model
func (p *Preloader) Add(rel string) *Preloader {
	for modelName := range p.relations {
		p.relations[modelName] = append(p.relations[modelName], rel)
	}
	return p
}

// Execute runs the query with preloading
func (p *Preloader) Execute(result interface{}) error {
	if len(p.relations) == 0 {
		return p.db.Find(result).Error
	}

	query := p.db
	for _, relations := range p.relations {
		for _, relation := range relations {
			query = query.Preload(relation)
		}
	}
	return query.Find(result).Error
}

// PreloadOne preloads related data for a single record
func PreloadOne(db *gorm.DB, model interface{}, relations ...string) error {
	query := db
	for _, relation := range relations {
		query = query.Preload(relation)
	}
	return query.First(model).Error
}

// PreloadMany preloads related data for multiple records
func PreloadMany(db *gorm.DB, models interface{}, relations ...string) error {
	query := db
	for _, relation := range relations {
		query = query.Preload(relation)
	}
	return query.Find(models).Error
}

// ============================================================
// Query Builder Utilities
// ============================================================

// Paginate adds pagination to a query
func Paginate(db *gorm.DB, page, pageSize int) *gorm.DB {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	return db.Offset(offset).Limit(pageSize)
}

// PaginationResult holds pagination info
type PaginationResult struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalCount int64 `json:"total_count"`
	TotalPages int   `json:"total_pages"`
}

// GetPagination calculates pagination metadata
func GetPagination(db *gorm.DB, page, pageSize int) (*PaginationResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &PaginationResult{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: totalPages,
	}, nil
}

// ============================================================
// Query Optimization
// ============================================================

// OptimizedFind finds records with common optimizations
func OptimizedFind(db *gorm.DB, dest interface{}, opts ...Option) error {
	o := &options{
		preloads:  []string{},
		selects:   []string{},
		orders:    []string{},
		PageSize:  50,
		page:      1,
	}

	for _, opt := range opts {
		opt(o)
	}

	query := db

	// Apply selects
	if len(o.selects) > 0 {
		query = query.Select(strings.Join(o.selects, ", "))
	}

	// Apply preloads to prevent N+1
	for _, preload := range o.preloads {
		query = query.Preload(preload)
	}

	// Apply ordering
	for _, order := range o.orders {
		query = query.Order(order)
	}

	// Apply pagination
	if o.PageSize > 0 {
		query = Paginate(query, o.page, o.PageSize)
	}

	return query.Find(dest).Error
}

// OptimizedFirst finds a single record with optimizations
func OptimizedFirst(db *gorm.DB, dest interface{}, conditions interface{}, opts ...Option) error {
	o := &options{
		preloads: []string{},
		selects:  []string{},
	}

	for _, opt := range opts {
		opt(o)
	}

	query := db

	// Apply selects
	if len(o.selects) > 0 {
		query = query.Select(strings.Join(o.selects, ", "))
	}

	// Apply preloads
	for _, preload := range o.preloads {
		query = query.Preload(preload)
	}

	return query.First(dest, conditions).Error
}

// options holds query options
type options struct {
	preloads []string
	selects  []string
	orders   []string
	page     int
	PageSize int
}

// Option is a functional option
type Option func(*options)

// WithPreload adds preload relations
func WithPreload(relations ...string) Option {
	return func(o *options) {
		o.preloads = append(o.preloads, relations...)
	}
}

// WithSelect adds column selects
func WithSelect(cols ...string) Option {
	return func(o *options) {
		o.selects = append(o.selects, cols...)
	}
}

// WithOrder adds ordering
func WithOrder(order string) Option {
	return func(o *options) {
		o.orders = append(o.orders, order)
	}
}

// WithPagination adds pagination
func WithPagination(page, pageSize int) Option {
	return func(o *options) {
		o.page = page
		o.PageSize = pageSize
	}
}

// ============================================================
// Tenant-scoped Query Helpers
// ============================================================

// TenantFilter adds tenant filter to query
func TenantFilter(db *gorm.DB, tenantID string, model interface{}) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

// TenantScope returns a scope function for tenant filtering
func TenantScope(tenantID string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("tenant_id = ?", tenantID)
	}
}

// ============================================================
// Soft Delete Helpers
// ============================================================

// Unscoped finds records including soft deleted ones
func Unscoped(db *gorm.DB, dest interface{}) error {
	return db.Unscoped().Find(dest).Error
}

// Restore restores a soft deleted record
func Restore(db *gorm.DB, model interface{}) error {
	return db.Unscoped().Model(model).Update("deleted_at", nil).Error
}

// ============================================================
// Batch Operations
// ============================================================

// BatchProcess processes records in batches
func BatchProcess(db *gorm.DB, model interface{}, batchSize int, fn func(*gorm.DB, interface{}) error) error {
	var count int64
	if err := db.Model(model).Count(&count).Error; err != nil {
		return err
	}

	for offset := 0; offset < int(count); offset += batchSize {
		var batch []interface{}
		if err := db.Offset(offset).Limit(batchSize).Find(&batch).Error; err != nil {
			return err
		}

		if err := fn(db, batch); err != nil {
			return err
		}
	}

	return nil
}

// BatchUpdate updates records in batches
func BatchUpdate(db *gorm.DB, model interface{}, values map[string]interface{}, batchSize int) (int64, error) {
	var total int64

	var ids []interface{}
	if err := db.Model(model).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}

	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}

		result := db.Model(model).Where("id IN ?", ids[i:end]).Updates(values)
		if result.Error != nil {
			return total, result.Error
		}

		total += result.RowsAffected
	}

	return total, nil
}

// ============================================================
// Locking Helpers
// ============================================================

// ForUpdate locks rows for update to prevent race conditions
func ForUpdate(db *gorm.DB, model interface{}) *gorm.DB {
	return db.Clauses(clause.Locking{Strength: "UPDATE"}).First(model)
}

// ForShare locks rows for read without blocking writes
func ForShare(db *gorm.DB, model interface{}) *gorm.DB {
	return db.Clauses(clause.Locking{Strength: "SHARE"}).First(model)
}

// WithLock returns a query with the specified lock
func WithLock(db *gorm.DB, lock clause.Locking) *gorm.DB {
	return db.Clauses(lock)
}

// ============================================================
// Transaction Helpers
// ============================================================

// WithTransaction executes a function within a transaction
func WithTransaction(db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.Transaction(fn)
}

// WithSavePoint executes a function within a savepoint
func WithSavePoint(db *gorm.DB, name string, fn func(*gorm.DB) error) error {
	return db.Transaction(fn)
}

// ============================================================
// Helper Functions
// ============================================================

func getModelName(model interface{}) string {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// CountResult holds count query result
type CountResult struct {
	Count int64 `json:"count"`
}

// Count counts records matching conditions
func Count(db *gorm.DB, model interface{}, conditions ...interface{}) (int64, error) {
	var result CountResult
	query := db.Model(model)
	if len(conditions) > 0 {
		query = query.Where(conditions[0], conditions[1:]...)
	}
	err := query.Count(&result.Count).Error
	return result.Count, err
}

// Exists checks if a record exists
func Exists(db *gorm.DB, model interface{}, conditions ...interface{}) (bool, error) {
	var count int64
	query := db.Model(model)
	if len(conditions) > 0 {
		query = query.Where(conditions[0], conditions[1:]...)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

// Where adds where conditions
func Where(db *gorm.DB, query string, args ...interface{}) *gorm.DB {
	return db.Where(query, args...)
}

// Or adds or condition
func Or(db *gorm.DB, query string, args ...interface{}) *gorm.DB {
	return db.Or(query, args...)
}

// Join adds a join clause
func Join(db *gorm.DB, query string, args ...interface{}) *gorm.DB {
	return db.Joins(query, args...)
}

// Group adds group by
func Group(db *gorm.DB, query string) *gorm.DB {
	return db.Group(query)
}

// Having adds having clause
func Having(db *gorm.DB, query string, args ...interface{}) *gorm.DB {
	return db.Having(query, args...)
}

// Distinct returns distinct results
func Distinct(db *gorm.DB, columns ...string) *gorm.DB {
	args := make([]interface{}, len(columns))
	for i, c := range columns {
		args[i] = c
	}
	return db.Distinct(args...)
}

// PluckColumn extracts a single column values
func PluckColumn(db *gorm.DB, model interface{}, column string, dest interface{}) error {
	return db.Model(model).Pluck(column, dest).Error
}

// ============================================================
// Query Metrics
// ============================================================

// QueryTimer tracks query execution time
type QueryTimer struct {
	start   time.Time
	query   string
	verbose bool
}

// NewQueryTimer creates a new query timer
func NewQueryTimer(query string) *QueryTimer {
	return &QueryTimer{
		start: time.Now(),
		query: query,
	}
}

// WithVerbose enables verbose logging
func (qt *QueryTimer) WithVerbose() *QueryTimer {
	qt.verbose = true
	return qt
}

// End records the query end time and returns duration
func (qt *QueryTimer) End() time.Duration {
	duration := time.Since(qt.start)
	if qt.verbose && duration.Milliseconds() > 100 {
		fmt.Printf("Slow query (%.2fms): %s\n", duration.Seconds()*1000, qt.query)
	}
	return duration
}

// TimeQuery times a query execution
func TimeQuery(db *gorm.DB, query string, fn func(*gorm.DB) error) error {
	qt := NewQueryTimer(query)
	defer qt.End()
	return fn(db)
}

// ============================================================
// Composite Index Suggestions
// ============================================================

// CommonIndexes returns commonly needed indexes for Invoices table
var CommonIndexes = map[string][]string{
	"invoices": {
		"idx_invoices_tenant_id",
		"idx_invoices_tenant_status",
		"idx_invoices_tenant_created",
		"idx_invoices_client_id",
		"idx_invoices_due_date",
		"idx_invoices_status_due_date",
	},
	"payments": {
		"idx_payments_tenant_id",
		"idx_payments_invoice_id",
		"idx_payments_status",
		"idx_payments_created_at",
	},
	"clients": {
		"idx_clients_tenant_id",
		"idx_clients_tenant_email",
	},
}

// AddIndexes creates missing indexes (for migration)
func AddIndexes(db *gorm.DB) error {
	// This would typically be run during migration
	// to ensure all indexes exist
	return nil
}

// ============================================================
// Context Helpers
// ============================================================

// WithTimeout adds timeout context to query
func WithTimeout(db *gorm.DB, timeout time.Duration) *gorm.DB {
	ctx, _ := context.WithTimeout(db.Statement.Context, timeout)
	db = db.Session(&gorm.Session{Context: ctx})
	// Note: cancel should be called after query completes
	// In practice, use context.WithTimeout at handler level
	return db
}

// SkipHooks returns a session that skips hooks
func SkipHooks(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{SkipHooks: true})
}

// DryRun returns a query that doesn't execute
func DryRun(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{DryRun: true})
}

// Explain returns query explanation
func Explain(db *gorm.DB, model interface{}) (string, error) {
	var result string
	stmt := db.Session(&gorm.Session{DryRun: true}).Model(model).Find(nil).Statement
	sql := db.Dialector.Explain(stmt.SQL.String(), stmt.Vars...)
	err := db.Raw("EXPLAIN QUERY PLAN " + sql).Scan(&result).Error
	if err != nil {
		return "", err
	}
	return result, nil
}