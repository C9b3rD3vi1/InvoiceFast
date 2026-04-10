package models

import (
	"database/sql"
	"time"
)

// Tenant represents an organization/company in the system
type Tenant struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey"`
	Name      string    `json:"name" gorm:"not null"`
	Subdomain string    `json:"subdomain" gorm:"uniqueIndex"` // For custom domains
	Plan      string    `json:"plan" gorm:"default:'free'"`   // free, pro, agency, enterprise
	Settings  string    `json:"settings" gorm:"type:text"`    // JSON settings
	IsActive  bool      `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User represents a user/tenant in the system
type User struct {
	ID           string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID     string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	Email        string    `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash string    `json:"-" gorm:"not null"`
	Name         string    `json:"name"`
	Phone        string    `json:"phone"`
	CompanyName  string    `json:"company_name"`
	KRAPIN       string    `json:"kra_pin"`                    // Encrypted - stored as ciphertext
	Plan         string    `json:"plan" gorm:"default:'free'"` // free, pro, agency, enterprise
	IsActive     bool      `json:"is_active" gorm:"default:true"`
	Role         string    `json:"role" gorm:"default:'user'"` // admin, manager, user
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	Tenant Tenant `json:"tenant,omitempty" gorm:"foreignKey:TenantID"`
}

// Client represents a customer/client of the user
type Client struct {
	ID           string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID     string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID       string    `json:"user_id" gorm:"type:uuid;index;not null"` // Legacy - for backward compat
	Name         string    `json:"name" gorm:"not null"`
	Email        string    `json:"email"`
	Phone        string    `json:"phone"`
	Address      string    `json:"address"`
	KRAPIN       string    `json:"kra_pin"` // Encrypted - stored as ciphertext
	Currency     string    `json:"currency" gorm:"default:'KES'"`
	PaymentTerms int       `json:"payment_terms" gorm:"default:30"` // days
	Notes        string    `json:"notes"`
	TotalBilled  float64   `json:"total_billed" gorm:"default:0"`
	TotalPaid    float64   `json:"total_paid" gorm:"default:0"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

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
)

// Invoice represents an invoice
type Invoice struct {
	ID                  string        `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID            string        `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID              string        `json:"user_id" gorm:"type:uuid;index;not null"`
	ClientID            string        `json:"client_id" gorm:"type:uuid;index;not null"`
	InvoiceNumber       string        `json:"invoice_number" gorm:"uniqueIndex"`
	Reference           string        `json:"reference"`
	Currency            string        `json:"currency" gorm:"default:'KES'"`
	KESEquivalent       float64       `json:"kes_equivalent" gorm:"default:0"` // Dual display: KES value
	ExchangeRate        float64       `json:"exchange_rate" gorm:"default:1"`  // Rate used for conversion
	Subtotal            float64       `json:"subtotal" gorm:"not null"`
	TaxRate             float64       `json:"tax_rate" gorm:"default:0"`
	TaxAmount           float64       `json:"tax_amount" gorm:"default:0"`
	Discount            float64       `json:"discount" gorm:"default:0"`
	Total               float64       `json:"total" gorm:"not null"`
	PaidAmount          float64       `json:"paid_amount" gorm:"default:0"`
	Status              InvoiceStatus `json:"status" gorm:"default:'draft'"`
	DueDate             time.Time     `json:"due_date"`
	SentAt              sql.NullTime  `json:"sent_at"`
	ViewedAt            sql.NullTime  `json:"viewed_at"`
	PaidAt              sql.NullTime  `json:"paid_at"`
	Notes               string        `json:"notes"`
	Terms               string        `json:"terms"`
	BrandColor          string        `json:"brand_color" gorm:"default:'#2563eb'"`
	LogoURL             string        `json:"logo_url"`
	PaymentLink         string        `json:"payment_link"`
	MagicToken          string        `json:"magic_token" gorm:"uniqueIndex"` // For client portal
	MagicTokenExpiresAt sql.NullTime  `json:"magic_token_expires_at"`         // Token expiration
	KRAICN              string        `json:"kra_icn"`                        // KRA Invoice Confirmation Number
	KRAQRCode           string        `json:"kra_qr_code"`                    // KRA QR Code
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`

	// Relations
	User     User          `json:"-" gorm:"foreignKey:UserID"`
	Client   Client        `json:"client,omitempty" gorm:"foreignKey:ClientID"`
	Items    []InvoiceItem `json:"items,omitempty" gorm:"foreignKey:InvoiceID"`
	Payments []Payment     `json:"payments,omitempty" gorm:"foreignKey:InvoiceID"`
}

// InvoiceItem represents a line item in an invoice
type InvoiceItem struct {
	ID          string    `json:"id" gorm:"type:uuid;primaryKey"`
	InvoiceID   string    `json:"invoice_id" gorm:"type:uuid;index;not null"`
	Description string    `json:"description" gorm:"not null"`
	Quantity    float64   `json:"quantity" gorm:"default:1"`
	UnitPrice   float64   `json:"unit_price" gorm:"not null"`
	Unit        string    `json:"unit"` // e.g., "hours", "items", "pieces"
	Total       float64   `json:"total" gorm:"not null"`
	SortOrder   int       `json:"sort_order" gorm:"default:0"`
	CreatedAt   time.Time `json:"created_at"`
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
	TenantID      string        `json:"tenant_id" gorm:"type:uuid;index;not null;uniqueIndex:idx_payment_tenant_ref"`
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
	CompletedAt   sql.NullTime  `json:"completed_at"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`

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
