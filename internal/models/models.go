package models

import (
	"database/sql"
	"encoding/json"
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
	ID                   string     `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID             string     `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID               string     `json:"user_id" gorm:"type:uuid;index;not null"` // Legacy - for backward compat
	Name                 string     `json:"name" gorm:"not null"`
	Email                string     `json:"email"`
	Phone                string     `json:"phone"`
	Address              string     `json:"address"`
	KRAPIN               string     `json:"kra_pin"` // Encrypted - stored as ciphertext
	Currency             string     `json:"currency" gorm:"default:'KES'"`
	PaymentTerms         int        `json:"payment_terms" gorm:"default:30"`               // days (Net 15, Net 30, Net 60, etc.)
	DefaultPaymentMethod string     `json:"default_payment_method" gorm:"default:'mpesa'"` // mpesa, bank, card, cash
	InternalNotes        string     `json:"internal_notes"`                                // Private notes visible only to team
	Notes                string     `json:"notes"`                                         // Client-facing notes
	TotalBilled          float64    `json:"total_billed" gorm:"default:0"`
	TotalPaid            float64    `json:"total_paid" gorm:"default:0"`
	LastPaymentDate      *time.Time `json:"last_payment_date"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`

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

// Invoice represents an invoice
type Invoice struct {
	ID                string  `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID          string  `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID            string  `json:"user_id" gorm:"type:uuid;index;not null"`
	ClientID          string  `json:"client_id" gorm:"type:uuid;index;not null"`
	InvoiceNumber     string  `json:"invoice_number" gorm:"uniqueIndex"`
	Reference         string  `json:"reference"`
	Title             string  `json:"title"` // Invoice title/subject
	Currency          string  `json:"currency" gorm:"default:'KES'"`
	KESEquivalent     float64 `json:"kes_equivalent" gorm:"default:0"`
	ExchangeRate      float64 `json:"exchange_rate" gorm:"default:1"`
	InvoiceType       string  `json:"invoice_type" gorm:"default:'invoice'"`      // invoice, credit_note, debit_note
	OriginalInvoiceID string  `json:"original_invoice_id" gorm:"type:uuid;index"` // For credit/debit notes

	// Recurring Invoice
	IsRecurring         bool          `json:"is_recurring" gorm:"default:false"`
	RecurringFrequency  string        `json:"recurring_frequency"` // daily, weekly, monthly, quarterly, yearly
	RecurringNextDate   time.Time     `json:"recurring_next_date"`
	RecurringParentID   string        `json:"recurring_parent_id" gorm:"type:uuid;index"` // Child invoice from recurring
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
	ID           string    `json:"id" gorm:"type:uuid;primaryKey"`
	InvoiceID    string    `json:"invoice_id" gorm:"type:uuid;index;not null"`
	Description  string    `json:"description" gorm:"not null"`
	Quantity     float64   `json:"quantity" gorm:"default:1"`
	UnitPrice    float64   `json:"unit_price" gorm:"not null"`
	Unit         string    `json:"unit"` // e.g., "hours", "items", "pieces"
	TaxRate      float64   `json:"tax_rate" gorm:"default:0"`
	TaxAmount    float64   `json:"tax_amount" gorm:"default:0"`
	DiscountRate float64   `json:"discount_rate" gorm:"default:0"`
	DiscountAmt  float64   `json:"discount_amount" gorm:"default:0"`
	Total        float64   `json:"total" gorm:"not null"`
	SortOrder    int       `json:"sort_order" gorm:"default:0"`
	CreatedAt    time.Time `json:"created_at"`
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
