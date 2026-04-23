package models

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Tenant represents an organization/company in the system
type Tenant struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey"`
	Name      string    `json:"name" gorm:"not null"`
	Subdomain string    `json:"subdomain" gorm:"uniqueIndex"` // For custom domains
	Plan      string    `json:"plan" gorm:"default:'free'"`   // free, pro, agency, enterprise
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Website   string    `json:"website"`
	Country   string    `json:"country" gorm:"default:'KE'"`
	Timezone  string    `json:"timezone" gorm:"default:'Africa/Nairobi'"`
	Settings  string    `json:"settings" gorm:"type:text"`    // JSON settings
	IsActive  bool      `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User represents a user/tenant in the system
type User struct {
	ID                 string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID           string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	Email              string    `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash       string    `json:"-" gorm:"not null"`
	Name               string    `json:"name"`
	Phone              string    `json:"phone"`
	CompanyName        string    `json:"company_name"`
	KRAPIN             string    `json:"-"` // Encrypted - stored as ciphertext
	Plan               string    `json:"plan" gorm:"default:'free'"` // free, pro, agency, enterprise
	IsActive           bool      `json:"is_active" gorm:"default:true"`
	Role               string    `json:"role" gorm:"default:'user'"` // admin, manager, user
	TwoFactorEnabled   bool      `json:"two_factor_enabled" gorm:"default:false"`
	TwoFactorSecret    string    `json:"-"` // Encrypted TOTP secret
	TwoFactorVerifiedAt *time.Time `json:"two_factor_verified_at"`
	PasswordChangedAt  *time.Time `json:"password_changed_at"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	LoginAlertEnabled  bool      `json:"login_alert_enabled" gorm:"default:true"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	Tenant Tenant `json:"tenant,omitempty" gorm:"foreignKey:TenantID"`
}

// UserSession represents an active user session
type UserSession struct {
	ID           string    `json:"id" gorm:"type:uuid;primaryKey"`
	UserID       string    `json:"user_id" gorm:"type:uuid;index;not null"`
	TenantID     string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	TokenHash    string    `json:"-" gorm:"not null"`
	DeviceInfo   string    `json:"device_info"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	Location     string    `json:"location"`
	IsCurrent    bool      `json:"is_current" gorm:"default:false"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// ClientStatus represents the status of a client
type ClientStatus string

const (
	ClientStatusActive   ClientStatus = "active"
	ClientStatusInactive ClientStatus = "inactive"
	ClientStatusArchived ClientStatus = "archived"
)

// Client represents a customer/client of the user
type Client struct {
	ID                   string       `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID             string       `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID               string       `json:"user_id" gorm:"type:uuid;index;not null"` // Legacy - for backward compat
	Name                 string       `json:"name" gorm:"not null"`
	Email                string       `json:"email"`
	Phone                string       `json:"phone"`
	Address              string       `json:"address"`
	KRAPIN               string       `json:"kra_pin"` // Encrypted - stored as ciphertext
	Currency             string       `json:"currency" gorm:"default:'KES'"`
	PaymentTerms         int          `json:"payment_terms" gorm:"default:30"`               // days (Net 15, Net 30, Net 60, etc.)
	DefaultPaymentMethod string       `json:"default_payment_method" gorm:"default:'mpesa'"` // mpesa, bank, card, cash
	Status               ClientStatus `json:"status" gorm:"default:'active'"`
	Tags                 string       `json:"-" gorm:"tags"`  // Stored as JSON string in DB
	InternalNotes        string       `json:"internal_notes"` // Private notes visible only to team
	Notes                string       `json:"notes"`          // Client-facing notes
	TotalBilled          float64      `json:"total_billed" gorm:"default:0"`
	TotalPaid            float64      `json:"total_paid" gorm:"default:0"`
	InvoiceCount         int64        `json:"invoice_count" gorm:"-"`
	TagsList             []string     `json:"tags" gorm:"-"` // For API response only
	LastPaymentDate      *time.Time   `json:"last_payment_date"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`

	Invoices []Invoice `json:"-" gorm:"foreignKey:ClientID"`
}

// InvoiceStatus represents the status of an invoice
type InvoiceStatus string

const (
	InvoiceStatusDraft         InvoiceStatus = "draft"
	InvoiceStatusSent          InvoiceStatus = "sent"
	InvoiceStatusViewed        InvoiceStatus = "viewed"
	InvoiceStatusPartiallyPaid InvoiceStatus = "partially_paid"
	InvoiceStatusPaid          InvoiceStatus = "paid"
	InvoiceStatusOverdue       InvoiceStatus = "overdue"
	InvoiceStatusCancelled     InvoiceStatus = "cancelled"
	InvoiceStatusVoid          InvoiceStatus = "void"
	InvoiceStatusCreditNote    InvoiceStatus = "credit_note"
	InvoiceStatusDebitNote     InvoiceStatus = "debit_note"
)

// KRAInvoiceStatus represents KRA submission status
type KRAInvoiceStatus string

const (
	KRAInvoiceStatusPending   KRAInvoiceStatus = "pending"
	KRAInvoiceStatusSubmitted KRAInvoiceStatus = "submitted"
	KRAInvoiceStatusFailed    KRAInvoiceStatus = "failed"
	KRAInvoiceStatusAccepted  KRAInvoiceStatus = "accepted"
	KRAInvoiceStatusRejected  KRAInvoiceStatus = "rejected"
)

// TaxType represents the type of tax applied
type TaxType string

const (
	TaxTypeStandard   TaxType = "standard"   // Standard rate (e.g., 16% VAT)
	TaxTypeZeroRated  TaxType = "zero_rated" // 0% - exports, international services
	TaxTypeExempt     TaxType = "exempt"     // Exempt from tax
	TaxTypeNone       TaxType = "none"       // No tax
)

// Invoice represents an invoice - IMMUTABLE after PAID or KRA SUBMITTED
type Invoice struct {
	ID                string           `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID          string           `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID            string           `json:"user_id" gorm:"type:uuid;index;not null"`
	ClientID          string           `json:"client_id" gorm:"type:uuid;index;not null"`
	InvoiceNumber     string           `json:"invoice_number" gorm:"uniqueIndex"`
	SequenceNumber    int64            `json:"sequence_number"` // For sequential numbering
	Reference         string           `json:"reference"`
	Title             string           `json:"title"` // Invoice title/subject
	Currency          string           `json:"currency" gorm:"default:'KES'"`
	KESEquivalent     float64          `json:"kes_equivalent" gorm:"default:0"`     // KES value for forex
	ExchangeRate      float64          `json:"exchange_rate" gorm:"default:1"`      // Rate at invoice creation
	ExchangeRateAt    time.Time        `json:"exchange_rate_at"`                   // When rate was captured
	InvoiceType       string           `json:"invoice_type" gorm:"default:'invoice'"` // invoice, credit_note, debit_note
	OriginalInvoiceID string           `json:"original_invoice_id" gorm:"type:uuid;index"` // For credit/debit notes

	// Recurring Invoice
	IsRecurring        bool          `json:"is_recurring" gorm:"default:false"`
	RecurringFrequency string        `json:"recurring_frequency"` // daily, weekly, monthly, quarterly, yearly
	RecurringNextDate  time.Time     `json:"recurring_next_date"`
	RecurringParentID  string        `json:"recurring_parent_id" gorm:"type:uuid;index"` // Child invoice from recurring

	// Monetary fields - Using float64 for DB compatibility, but MUST use decimal in code
	Subtotal     float64 `json:"subtotal"`
	Discount     float64 `json:"discount" gorm:"default:0"`
	TotalTax     float64 `json:"total_tax" gorm:"default:0"`
	TaxAmount    float64 `json:"tax_amount" gorm:"-"` // Alias for TotalTax (backward compatibility)
	Total        float64 `json:"total" gorm:"not null"`
	PaidAmount   float64 `json:"paid_amount" gorm:"default:0"`
	BalanceDue   float64 `json:"balance_due" gorm:"default:0"` // Calculated: Total - PaidAmount

	// Tax breakdown for KRA compliance
	TaxType           TaxType  `json:"tax_type" gorm:"default:'standard'"`
	TaxRate           float64 `json:"tax_rate" gorm:"default:16"` // 16% default for VAT
	TaxableAmount     float64 `json:"taxable_amount"`              // Amount before tax
	ExemptAmount      float64 `json:"exempt_amount"`               // Exempt portion
	ZeroRatedAmount   float64 `json:"zero_rated_amount"`          // Zero-rated portion

	Status InvoiceStatus `json:"status" gorm:"default:'draft'"`

	DueDate     time.Time  `json:"due_date"`
	SentAt      *time.Time `json:"sent_at"`
	ViewedAt    *time.Time `json:"viewed_at"`
	PaidAt      *time.Time `json:"paid_at"`
	CancelledAt *time.Time `json:"cancelled_at"`

	Notes      string `json:"notes"`
	Terms      string `json:"terms"`
	BrandColor string `json:"brand_color" gorm:"default:'#2563eb'"`
	LogoURL    string `json:"logo_url"`

	PaymentLink         string     `json:"payment_link"`
	MagicToken          string     `json:"magic_token" gorm:"uniqueIndex"` // For client portal
	MagicTokenExpiresAt *time.Time `json:"magic_token_expires_at"`         // Token expiration

	// KRA eTIMS Fields
	KRAICN           string           `json:"kra_icn"`            // KRA Invoice Confirmation Number
	KRAQRCode        string            `json:"kra_qr_code"`        // KRA QR Code
	KRAStatus        KRAInvoiceStatus  `json:"kra_status"`         // pending, submitted, failed, accepted, rejected
	KRASubmittedAt   *time.Time        `json:"kra_submitted_at"`  // When submitted to KRA
	KRAError         string            `json:"kra_error"`          // Error message if failed
	KRARetryCount    int               `json:"kra_retry_count"`   // Number of retry attempts
	KRAIdempotencyKey string           `json:"kra_idempotency_key"` // Prevent duplicate submissions

	// Concurrency control
	Version int `json:"version" gorm:"default:1"` // Optimistic locking

	// Soft delete
	DeletedAt *time.Time `json:"-" gorm:"index"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relations
	User     User          `json:"-" gorm:"foreignKey:UserID"`
	Client   Client        `json:"client,omitempty" gorm:"foreignKey:ClientID"`
	Items    []InvoiceItem `json:"items,omitempty" gorm:"foreignKey:InvoiceID"`
	Payments []Payment     `json:"payments,omitempty" gorm:"foreignKey:InvoiceID"`
}

// BeforeUpdate - Prevent modifications to immutable invoices
func (i *Invoice) BeforeUpdate(tx *gorm.DB) error {
	// Check if invoice is paid - immutable
	if i.Status == InvoiceStatusPaid || i.Status == InvoiceStatusPartiallyPaid {
		return errors.New("invoice cannot be modified: status is paid or partially paid")
	}

	// Check if KRA submitted - immutable
	if i.KRAStatus == KRAInvoiceStatusSubmitted || i.KRAStatus == KRAInvoiceStatusAccepted {
		return errors.New("invoice cannot be modified: KRA submission in progress or accepted")
	}

	// Check version for optimistic locking
	var current Invoice
	if err := tx.First(&current, "id = ?", i.ID).Error; err != nil {
		return err
	}
	if current.Version != i.Version {
		return errors.New("concurrent modification detected: invoice was modified by another process")
	}

	// Increment version
	i.Version = current.Version + 1
	return nil
}

// BeforeDelete - Prevent hard deletes
func (i *Invoice) BeforeDelete(tx *gorm.DB) error {
	// Soft delete only - never hard delete financial records
	if i.Status == InvoiceStatusPaid || i.Status == InvoiceStatusPartiallyPaid {
		return errors.New("invoice cannot be deleted: financial records must be preserved")
	}
	if i.KRAStatus == KRAInvoiceStatusSubmitted || i.KRAStatus == KRAInvoiceStatusAccepted {
		return errors.New("invoice cannot be deleted: KRA submission exists")
	}
	return nil
}

// GetTaxAmount returns the tax amount (alias for TotalTax for backward compatibility)
func (i *Invoice) GetTaxAmount() float64 {
	return i.TaxAmount
}

// SetTaxAmount sets the tax amount (alias for TotalTax for backward compatibility)
func (i *Invoice) SetTaxAmount(amount float64) {
	i.TaxAmount = amount
	i.TotalTax = amount
}

// ============================================
// STATE MACHINE - Invoice Status Transitions
// ============================================

// ValidTransitions defines allowed status transitions
var ValidTransitions = map[InvoiceStatus][]InvoiceStatus{
	InvoiceStatusDraft:         {InvoiceStatusSent, InvoiceStatusCancelled, InvoiceStatusVoid},
	InvoiceStatusSent:          {InvoiceStatusPaid, InvoiceStatusPartiallyPaid, InvoiceStatusOverdue, InvoiceStatusCancelled, InvoiceStatusVoid},
	InvoiceStatusViewed:        {InvoiceStatusPaid, InvoiceStatusPartiallyPaid, InvoiceStatusOverdue, InvoiceStatusCancelled, InvoiceStatusVoid},
	InvoiceStatusPartiallyPaid: {InvoiceStatusPaid, InvoiceStatusOverdue, InvoiceStatusCancelled},
	InvoiceStatusPaid:          {InvoiceStatusVoid}, // Only void allowed after paid
	InvoiceStatusOverdue:       {InvoiceStatusPaid, InvoiceStatusPartiallyPaid, InvoiceStatusCancelled},
	InvoiceStatusCancelled:     {},
	InvoiceStatusVoid:         {},
	InvoiceStatusCreditNote:    {},
	InvoiceStatusDebitNote:    {},
}

// CanTransition checks if transition from one status to another is valid
func CanTransition(from, to InvoiceStatus) bool {
	allowed, exists := ValidTransitions[from]
	if !exists {
		return false
	}
	for _, status := range allowed {
		if status == to {
			return true
		}
	}
	return false
}

// TransitionError represents an invalid state transition
type TransitionError struct {
	From InvoiceStatus
	To   InvoiceStatus
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("invalid status transition from %s to %s", e.From, e.To)
}

// ValidateTransition returns error if transition is invalid
func ValidateTransition(from, to InvoiceStatus) error {
	if !CanTransition(from, to) {
		return &TransitionError{From: from, To: to}
	}
	return nil
}

// ============================================
// INVOICE SEQUENCE - Sequential Numbering
// ============================================

// InvoiceSequence tracks invoice numbers per tenant
type InvoiceSequence struct {
	ID              string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID        string    `json:"tenant_id" gorm:"type:uuid;uniqueIndex;not null"`
	LastSequenceNum int64     `json:"last_sequence_num" gorm:"default:0"`
	Prefix         string    `json:"prefix" gorm:"default:'INV'"`
	Padding        int       `json:"padding" gorm:"default:6"` // Number of digits (INV-000001)
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// GetNextSequence returns the next sequence number in a transaction-safe manner
func (s *InvoiceSequence) GetNextSequence(tx *gorm.DB) (int64, error) {
	var seq InvoiceSequence
	
	// Try with locking first (for PostgreSQL)
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		FirstOrCreate(&seq, InvoiceSequence{TenantID: s.TenantID}).Error
	
	// If locking not supported (SQLite), fallback to regular FirstOrCreate
	if err != nil || seq.TenantID == "" {
		err = tx.FirstOrCreate(&seq, InvoiceSequence{TenantID: s.TenantID}).Error
	}
	
	if err != nil {
		return 0, fmt.Errorf("failed to get sequence: %w", err)
	}

	newNum := seq.LastSequenceNum + 1
	
	// Use Update with explicit WHERE clause for SQLite compatibility
	err = tx.Model(&seq).Where("tenant_id = ?", s.TenantID).Update("last_sequence_num", newNum).Error
	if err != nil {
		return 0, fmt.Errorf("failed to update sequence: %w", err)
	}

	return newNum, nil
}

// FormatInvoiceNumber formats sequence number with prefix and padding
func FormatInvoiceNumber(prefix string, sequence int64, padding int) string {
	return fmt.Sprintf("%s-%0*d", prefix, padding, sequence)
}

// ============================================
// FINANCIAL VALIDATION
// ============================================

// ValidateInvoiceTotals ensures financial integrity
func ValidateInvoiceTotals(invoice *Invoice, items []InvoiceItem) error {
	var calculatedSubtotal float64
	var calculatedTax float64
	var calculatedTotal float64

	for _, item := range items {
		// Subtotal = quantity * unit_price
		itemSubtotal := item.Quantity * item.UnitPrice

		// Discount
		var discount float64
		if item.DiscountRate > 0 {
			discount = itemSubtotal * (item.DiscountRate / 100)
		} else if item.DiscountAmt > 0 {
			discount = item.DiscountAmt
		}

		// Taxable amount after discount
		taxable := itemSubtotal - discount

		// Tax amount
		var tax float64
		if item.TaxRate > 0 && item.TaxType != TaxTypeNone && item.TaxType != TaxTypeExempt {
			tax = taxable * (item.TaxRate / 100)
		}

		// Item total = taxable + tax (matches service calculation)
		itemTotal := taxable + tax

		calculatedSubtotal += itemSubtotal
		calculatedTax += tax
		calculatedTotal += itemTotal
	}

	// Apply global discount
	calculatedTotal = calculatedSubtotal + calculatedTax - invoice.Discount

	// Allow 0.01 rounding tolerance for KES
	const tolerance = 0.01

	if !withinTolerance(invoice.Total, calculatedTotal, tolerance) {
		return fmt.Errorf("total mismatch: stored=%v, calculated=%v", invoice.Total, calculatedTotal)
	}

	// Validate non-negative
	if invoice.Total < 0 {
		return errors.New("invoice total cannot be negative")
	}
	if invoice.BalanceDue < 0 {
		return errors.New("balance due cannot be negative")
	}

	return nil
}


func withinTolerance(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

// ============================================
// TAX CALCULATION ENGINE
// ============================================

// CalculateLineItemTax calculates tax for a single line item
func CalculateLineItemTax(quantity, unitPrice, discountRate, discountAmt, taxRate float64, taxType TaxType) (subtotal, taxAmount, total float64) {
	subtotal = quantity * unitPrice

	// Calculate discount
	var discount float64
	if discountRate > 0 {
		discount = subtotal * (discountRate / 100)
	} else {
		discount = discountAmt
	}

	afterDiscount := subtotal - discount

	// Calculate tax based on type
	if taxType == TaxTypeStandard && taxRate > 0 {
		taxAmount = afterDiscount * (taxRate / 100)
	} else {
		taxAmount = 0
	}

	total = afterDiscount + taxAmount
	return
}

// InvoiceItem represents a line item in an invoice
type InvoiceItem struct {
	ID           string   `json:"id" gorm:"type:uuid;primaryKey"`
	InvoiceID    string   `json:"invoice_id" gorm:"type:uuid;index;not null"`
	Description  string   `json:"description" gorm:"not null"`
	Quantity     float64  `json:"quantity" gorm:"default:1"`
	UnitPrice    float64  `json:"unit_price" gorm:"not null"` // Unit price before tax/discount
	Unit         string   `json:"unit"`                         // e.g., "hours", "items", "pieces"

	// Tax per line item
	TaxType   TaxType  `json:"tax_type" gorm:"default:'standard'"`
	TaxRate   float64  `json:"tax_rate" gorm:"default:0"`  // e.g., 16 for 16%
	TaxAmount float64  `json:"tax_amount" gorm:"default:0"` // Calculated tax

	// Discount per line item
	DiscountRate float64 `json:"discount_rate" gorm:"default:0"` // Percentage discount
	DiscountAmt  float64 `json:"discount_amount" gorm:"default:0"` // Fixed discount amount

	// Calculated fields (MUST match: quantity * unit_price + tax - discount)
	Subtotal    float64 `json:"subtotal"`    // quantity * unit_price
	Total       float64 `json:"total" gorm:"not null"`       // Subtotal + TaxAmount - DiscountAmt

	SortOrder int       `json:"sort_order" gorm:"default:0"`
	DeletedAt  *time.Time `json:"-" gorm:"index"`
	CreatedAt time.Time  `json:"created_at"`
}

// PaymentMethod represents the payment method
type PaymentMethod string

const (
	PaymentMethodMpesa    PaymentMethod = "mpesa"
	PaymentMethodCard     PaymentMethod = "card"
	PaymentMethodBank     PaymentMethod = "bank"
	PaymentMethodCash     PaymentMethod = "cash"
	PaymentMethodIntasend PaymentMethod = "intasend"
)

// PaymentStatus represents the status of a payment
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusCompleted PaymentStatus = "completed"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusRefunded  PaymentStatus = "refunded"
)

// Payment represents a payment for an invoice
type Payment struct {
	ID            string        `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID      string        `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID        string        `json:"user_id" gorm:"type:uuid;index"`
	InvoiceID     string        `json:"invoice_id" gorm:"type:uuid;index;not null"`
	Amount        float64       `json:"amount" gorm:"not null"`
	Currency      string        `json:"currency" gorm:"default:'KES'"`
	Method        PaymentMethod `json:"method" gorm:"not null"`
	Status        PaymentStatus `json:"status" gorm:"default:'pending'"`
	Reference     string        `json:"reference" gorm:"index"` // M-Pesa receipt number
	IntasendID    string        `json:"intasend_id"`
	PhoneNumber   string        `json:"phone_number"`
	CustomerEmail string        `json:"customer_email"`
	FailureReason string        `json:"failure_reason"`
	CompletedAt   *time.Time    `json:"completed_at"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`

	Invoice Invoice `json:"-" gorm:"foreignKey:InvoiceID"`
}

// Reminder represents an automated reminder
type Reminder struct {
	ID          string       `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID    string       `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID      string       `json:"user_id" gorm:"type:uuid;index;not null"`
	InvoiceID   string       `json:"invoice_id" gorm:"type:uuid;index;not null"`
	Type        string       `json:"type"`   // email, whatsapp, sms
	Status      string       `json:"status"` // pending, sent, failed
	ScheduledAt time.Time    `json:"scheduled_at"`
	SentAt      sql.NullTime `json:"sent_at"`
	Error       string       `json:"error"`
	CreatedAt   time.Time    `json:"created_at"`
}

// UnallocatedPayment represents a payment that couldn't be matched to an invoice
type UnallocatedPayment struct {
	ID          string     `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID    string     `json:"tenant_id" gorm:"type:uuid;index;not null"`
	Amount      float64    `json:"amount" gorm:"not null"`
	Currency    string     `json:"currency" gorm:"default:'KES'"`
	Reference   string     `json:"reference" gorm:"index"` // Payment reference (e.g., M-Pesa receipt)
	PhoneNumber string     `json:"phone_number"`
	Notes       string     `json:"notes"` // Admin notes
	IsMatched   bool       `json:"is_matched" gorm:"default:false"`
	MatchedAt   *time.Time `json:"matched_at"`
	MatchedBy   string     `json:"matched_by"` // User ID who matched
	CreatedAt   time.Time  `json:"created_at"`
}

// Template represents an invoice template
type Template struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID  string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID    string    `json:"user_id" gorm:"type:uuid;index;not null"`
	Name      string    `json:"name" gorm:"not null"`
	HTML      string    `json:"html" gorm:"type:text"`
	IsDefault bool      `json:"is_default" gorm:"default:false"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RefreshToken for JWT refresh
type RefreshToken struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID  string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID    string    `json:"user_id" gorm:"type:uuid;index;not null"`
	Token     string    `json:"token" gorm:"uniqueIndex;not null"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditLog for tracking changes
type AuditLog struct {
	ID         string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID   string    `json:"tenant_id" gorm:"type:uuid;index"`
	UserID     string    `json:"user_id" gorm:"type:uuid;index"`
	Action     string    `json:"action" gorm:"not null"`
	EntityType string    `json:"entity_type"` // invoice, client, payment
	EntityID   string    `json:"entity_id"`
	Details    string    `json:"details"` // JSON blob
	IPAddress  string    `json:"ip_address"`
	CreatedAt  time.Time `json:"created_at"`
}

// APIKey for programmatic access
type APIKey struct {
	ID         string       `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID   string       `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID     string       `json:"user_id" gorm:"type:uuid;index;not null"`
	Name       string       `json:"name"`
	Key        string       `json:"key" gorm:"uniqueIndex;not null"`
	KeyHash    string       `json:"-" gorm:"not null"`
	LastUsedAt sql.NullTime `json:"last_used_at"`
	ExpiresAt  time.Time    `json:"expires_at"`
	IsActive   bool         `json:"is_active" gorm:"default:true"`
	CreatedAt  time.Time    `json:"created_at"`
}

// ExchangeRate for storing currency rates
type ExchangeRate struct {
	ID           string    `json:"id" gorm:"type:uuid;primaryKey"`
	Currency     string    `json:"currency" gorm:"not null;index"`
	BaseCurrency string    `json:"base_currency" gorm:"default:'KES'"`
	Rate         float64   `json:"rate" gorm:"not null"`
	ValidFrom    time.Time `json:"valid_from"`
	CreatedAt    time.Time `json:"created_at"`
}

// KRAQueueStatus represents KRA submission status
type KRAQueueStatus string

const (
	KRAQueuePending   KRAQueueStatus = "pending"
	KRAQueueFailed    KRAQueueStatus = "failed"
	KRAQueueCompleted KRAQueueStatus = "completed"
)

// KRAQueueItem for failed KRA submissions
type KRAQueueItem struct {
	ID            string         `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID      string         `json:"tenant_id" gorm:"type:uuid;index;not null"`
	InvoiceID     string         `json:"invoice_id" gorm:"type:uuid;index;not null"`
	InvoiceNumber string         `json:"invoice_number"`
	Payload       string         `json:"payload" gorm:"type:text"` // JSON payload
	RetryCount    int            `json:"retry_count" gorm:"default:0"`
	MaxRetries    int            `json:"max_retries" gorm:"default:3"`
	Status        KRAQueueStatus `json:"status" gorm:"default:'pending'"`
	LastError     string         `json:"last_error"`
	NextRetryAt   *time.Time     `json:"next_retry_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// Notification represents a user notification
type Notification struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID  string    `json:"tenant_id" gorm:"type:uuid;index"`
	UserID    string    `json:"user_id" gorm:"type:uuid;index"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Category  string    `json:"category"` // Invoices, Payments, Clients, System
	Icon      string    `json:"icon"`     // bell, file-text, credit-card, users, settings
	Actor     string    `json:"actor"`
	Read      bool      `json:"read" gorm:"default:false"`
	Link      string    `json:"link"`
	Data      string    `json:"data"` // JSON additional data
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NotificationLog for tracking notification delivery
type NotificationLog struct {
	ID          string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID    string    `json:"tenant_id" gorm:"type:uuid;index"`
	UserID      string    `json:"user_id" gorm:"type:uuid;index"`
	InvoiceID   string    `json:"invoice_id" gorm:"type:uuid;index"`
	ClientID    string    `json:"client_id" gorm:"type:uuid;index"`
	Type        string    `json:"type"`     // email, whatsapp, sms
	Provider    string    `json:"provider"` // twilio, metas
	To          string    `json:"to"`       // phone or email
	Status      string    `json:"status"`   // pending, sent, delivered, failed
	ErrorCode   string    `json:"error_code"`
	ErrorMsg    string    `json:"error_msg"`
	ExternalID  string    `json:"external_id"` // provider message ID
	RetryCount  int       `json:"retry_count" gorm:"default:0"`
	SentAt      time.Time `json:"sent_at"`
	DeliveredAt time.Time `json:"delivered_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TeamInvite represents a team member invitation
type TeamInvite struct {
	ID         string     `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID   string     `json:"tenant_id" gorm:"type:uuid;index;not null"`
	InvitedBy  string     `json:"invited_by" gorm:"type:uuid;index;not null"`
	Email      string     `json:"email" gorm:"index;not null"`
	Name       string     `json:"name"`
	Role       string     `json:"role" gorm:"default:'staff'"`
	Token      string     `json:"token" gorm:"uniqueIndex;not null"`
	Status     string     `json:"status" gorm:"default:'pending'"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Automation represents an automation workflow
type Automation struct {
	ID          string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID    string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID      string    `json:"user_id" gorm:"type:uuid;index;not null"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	TriggerType string    `json:"trigger_type"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AutomationLog represents automation execution logs
type AutomationLog struct {
	ID           string     `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID     string     `json:"tenant_id" gorm:"type:uuid;index;not null"`
	AutomationID string     `json:"automation_id" gorm:"type:uuid;index;not null"`
	Status       string     `json:"status"` // running, completed, failed
	ErrorMessage string     `json:"error_message"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// SubscriptionPlan represents a subscription plan
type SubscriptionPlan struct {
	ID              string    `json:"id" gorm:"type:uuid;primaryKey"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug" gorm:"uniqueIndex"`
	Description     string    `json:"description"`
	MonthlyPriceUSD int64     `json:"monthly_price_usd"` // base price in USD cents
	YearlyPriceUSD  int64     `json:"yearly_price_usd"`  // base price in USD cents
	FeaturesJSON    string    `json:"features_json"`     // JSON array of features
	LimitsJSON      string    `json:"limits_json"`       // JSON object with limits
	IsActive        bool      `json:"is_active" gorm:"default:true"`
	SortOrder       int       `json:"sort_order" gorm:"default:0"`
	TrialDays       int       `json:"trial_days" gorm:"default:14"` // default 14-day trial
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Subscription represents a tenant's subscription
type Subscription struct {
	ID                     string     `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID               string     `json:"tenant_id" gorm:"type:uuid;uniqueIndex"`
	PlanID                 string     `json:"plan_id" gorm:"type:uuid;index"`
	Status                 string     `json:"status" gorm:"default:'active'"`         // active, trialing, past_due, canceled, suspended, expired
	BillingCycle           string     `json:"billing_cycle" gorm:"default:'monthly'"` // monthly, yearly
	Provider               string     `json:"provider"`                               // mpesa, intasend, manual
	ProviderCustomerID     string     `json:"provider_customer_id"`
	ProviderSubscriptionID string     `json:"provider_subscription_id"`
	PaymentMethod          string     `json:"payment_method"` // mpesa, card
	Amount                 int64      `json:"amount"`         // amount in cents
	Currency               string     `json:"currency" gorm:"default:'KES'"`
	StartsAt               time.Time  `json:"starts_at"`
	RenewsAt               *time.Time `json:"renews_at"`
	ExpiresAt              *time.Time `json:"expires_at"`
	TrialEndsAt            *time.Time `json:"trial_ends_at"`
	CancelledAt            *time.Time `json:"cancelled_at"`
	SuspendedAt            *time.Time `json:"suspended_at"`
	RetryCount             int        `json:"retry_count" gorm:"default:0"`
	LastPaymentAt          *time.Time `json:"last_payment_at"`
	LastPaymentError       string     `json:"last_payment_error"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// SubscriptionTransaction represents payment transactions
type SubscriptionTransaction struct {
	ID                string     `json:"id" gorm:"type:uuid;primaryKey"`
	SubscriptionID    string     `json:"subscription_id" gorm:"type:uuid;index"`
	TenantID          string     `json:"tenant_id" gorm:"type:uuid;index"`
	Amount            int64      `json:"amount"` // in cents
	Currency          string     `json:"currency" gorm:"default:'KES'"`
	ProviderReference string     `json:"provider_reference"`
	PaymentMethod     string     `json:"payment_method"`                  // mpesa, card
	Status            string     `json:"status" gorm:"default:'pending'"` // pending, completed, failed, refunded
	Type              string     `json:"type"`                            // initial, renewal, upgrade, downgrade, refund
	PaidAt            *time.Time `json:"paid_at"`
	FailureReason     string     `json:"failure_reason"`
	MetadataJSON      string     `json:"metadata_json"`
	CreatedAt         time.Time  `json:"created_at"`
}

// UsageTracking represents tenant usage for plan limits
type UsageTracking struct {
	ID              string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID        string    `json:"tenant_id" gorm:"type:uuid;uniqueIndex"`
	InvoicesUsed    int       `json:"invoices_used" gorm:"default:0"`
	ClientsUsed     int       `json:"clients_used" gorm:"default:0"`
	UsersUsed       int       `json:"users_used" gorm:"default:0"`
	StorageUsed     int64     `json:"storage_used" gorm:"default:0"` // in bytes
	APIRequestsUsed int64     `json:"api_requests_used" gorm:"default:0"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SavedPaymentMethod represents stored payment methods
type SavedPaymentMethod struct {
	ID                 string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID           string    `json:"tenant_id" gorm:"type:uuid;index"`
	ProviderToken      string    `json:"provider_token"` // token from payment provider
	ProviderCustomerID string    `json:"provider_customer_id"`
	PaymentType        string    `json:"payment_type"` // mpesa, card
	Brand              string    `json:"brand"`        // Visa, Mastercard, M-Pesa
	Last4              string    `json:"last4"`        // last 4 digits
	ExpiryMonth        int       `json:"expiry_month"`
	ExpiryYear         int       `json:"expiry_year"`
	IsDefault          bool      `json:"is_default" gorm:"default:false"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// BillingInvoice represents invoices sent to tenants for billing
type BillingInvoice struct {
	ID             string     `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID       string     `json:"tenant_id" gorm:"type:uuid;index"`
	SubscriptionID string     `json:"subscription_id" gorm:"type:uuid;index"`
	InvoiceNumber  string     `json:"invoice_number" gorm:"uniqueIndex"`
	Amount         int64      `json:"amount"`
	Currency       string     `json:"currency" gorm:"default:'KES'"`
	Status         string     `json:"status" gorm:"default:'draft'"` // draft, sent, paid, failed, void
	DueDate        time.Time  `json:"due_date"`
	PaidAt         *time.Time `json:"paid_at"`
	PDFURL         string     `json:"pdf_url"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (p *SubscriptionPlan) GetFeatures() []string {
	var features []string
	if p.FeaturesJSON != "" {
		json.Unmarshal([]byte(p.FeaturesJSON), &features)
	}
	return features
}

func (p *SubscriptionPlan) GetLimits() map[string]int {
	limits := make(map[string]int)
	if p.LimitsJSON != "" {
		json.Unmarshal([]byte(p.LimitsJSON), &limits)
	}
	return limits
}

func (p *SubscriptionPlan) HasFeature(feature string) bool {
	features := p.GetFeatures()
	for _, f := range features {
		if f == feature {
			return true
		}
	}
	return false
}

func (p *SubscriptionPlan) GetLimit(resource string) int {
	limits := p.GetLimits()
	if limit, ok := limits[resource]; ok {
		return limit
	}
	return -1
}

func (s *Subscription) IsActive() bool {
	return s.Status == "active" || s.Status == "trialing"
}

func (s *Subscription) IsCanceled() bool {
	return s.Status == "canceled" || s.Status == "expired"
}

func (s *Subscription) IsExpired() bool {
	if s.ExpiresAt != nil && s.ExpiresAt.Before(time.Now()) {
		return true
	}
	if s.Status == "expired" {
		return true
	}
	return false
}

func (s *Subscription) DaysUntilRenewal() int {
	if s.RenewsAt == nil {
		return 0
	}
	return int(s.RenewsAt.Sub(time.Now()).Hours() / 24)
}

func (s *Subscription) HasTrial() bool {
	return s.TrialEndsAt != nil && time.Now().Before(*s.TrialEndsAt)
}

// Integration represents an external service integration
type Integration struct {
	ID            string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID      string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	Provider      string    `json:"provider" gorm:"not null"` // whatsapp, sms, email, smtp, slack
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Config        string    `json:"-" gorm:"type:text"`        // Encrypted JSON config
	IsActive      bool      `json:"is_active" gorm:"default:true"`
	IsConfigured  bool      `json:"is_configured"`             // True if credentials are configured
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
