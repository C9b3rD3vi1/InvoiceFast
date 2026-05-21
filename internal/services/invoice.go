package services

import (
	"errors"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"
	"invoicefast/internal/pdf"
	"invoicefast/internal/utils"

	"gorm.io/gorm"
)

var (
	ErrEmptyItems       = errors.New("invoice must have at least one item")
	ErrInvalidQuantity  = errors.New("item quantity cannot be negative")
	ErrInvoiceNotFound  = errors.New("invoice not found")
	ErrCannotEditPaid   = errors.New("cannot edit paid invoice")
	ErrCannotCancelPaid = errors.New("cannot cancel paid invoice")
	ErrCannotSendDraft  = errors.New("cannot send draft invoice")
	ErrInvalidBuyerType = errors.New("invalid buyer type for this client")
	ErrTenantRequired   = errors.New("tenant ID is required")
	ErrAlreadySent      = errors.New("invoice already sent")
)

var validCurrencies = utils.ValidCurrencies

type InvoiceService struct {
	db                *database.DB
	emailService      *EmailService
	whatsappService   *WhatsAppService
	exchangeService   *ExchangeRateService
	kraService        *KRAService
	cfg               *config.Config
	notificationSvc   *NotificationService
	pdfGenerator      *pdf.PDFGenerator
}

func NewInvoiceService(db *database.DB) *InvoiceService {
	return &InvoiceService{db: db}
}

func (s *InvoiceService) getTenantCurrency(tenantID string) string {
	if tenantID == "" {
		return utils.DefaultCurrency
	}
	var tenant models.Tenant
	if err := s.db.First(&tenant, "id = ?", tenantID).Error; err != nil {
		return utils.DefaultCurrency
	}
	if tenant.Currency == "" {
		return utils.DefaultCurrency
	}
	return tenant.Currency
}

type ServiceDependencies struct {
	DB            *database.DB
	Email         *EmailService
	WhatsApp      *WhatsAppService
	Notification  *NotificationService
	Exchange      *ExchangeRateService
	KRA           *KRAService
	SMS           *SMSService
	Config        *config.Config
	PDFGen        *pdf.PDFGenerator
}

func NewInvoiceServiceWithDeps(db *database.DB, deps *ServiceDependencies) *InvoiceService {
	svc := &InvoiceService{
		db:              db,
		emailService:    deps.Email,
		notificationSvc: deps.Notification,
		exchangeService: deps.Exchange,
		kraService:      deps.KRA,
		cfg:             deps.Config,
		pdfGenerator:    deps.PDFGen,
	}
	if deps.WhatsApp != nil {
		svc.whatsappService = deps.WhatsApp
	}
	return svc
}

func (s *InvoiceService) GetDB() *gorm.DB {
	return s.db.DB
}

// Request types
type CreateInvoiceRequest struct {
	ClientID      string               `json:"client_id" binding:"required"`
	Reference     string               `json:"reference"`
	Title         string               `json:"title"`
	Currency      string               `json:"currency"`
	TaxRate       float64              `json:"tax_rate"`
	Discount      float64              `json:"discount"`
	DueDate       time.Time            `json:"due_date"`
	Notes         string               `json:"notes"`
	Terms         string               `json:"terms"`
	BrandColor    string               `json:"brand_color"`
	LogoURL       string               `json:"logo_url"`
	BuyerType     string               `json:"buyer_type"`
	ExchangeRate  *float64             `json:"exchange_rate"`
	KESEquivalent *float64             `json:"kes_equivalent"`
	Items         []InvoiceItemRequest `json:"kraPayloadItems" binding:"required,min=1"`
}

// InvoiceItemRequest with extended fields for frontend compatibility
type InvoiceItemRequest struct {
	Description  string  `json:"description"`
	Name         string  `json:"name,omitempty"`
	Quantity     float64 `json:"quantity" binding:"required,min=-999999"`
	UnitPrice    float64 `json:"unit_price" binding:"required,min=0"`
	TaxRate      float64 `json:"tax_rate,omitempty"`
	DiscountRate float64 `json:"discount_rate,omitempty"`
	Unit         string  `json:"unit"`
}

type UpdateInvoiceRequest struct {
	Status     *string    `json:"status"`
	DueDate    *time.Time `json:"due_date"`
	Reference  *string    `json:"reference"`
	Currency   *string    `json:"currency"`
	Title      *string    `json:"title"`
	TaxRate    *float64   `json:"tax_rate"`
	Discount   *float64   `json:"discount"`
	Notes      *string    `json:"notes"`
	Terms      *string    `json:"terms"`
	BrandColor *string    `json:"brand_color"`
	BuyerType  *string    `json:"buyer_type"`
}

type InvoiceFilter struct {
	Status    string
	ClientID  string
	FromDate  *time.Time
	ToDate    *time.Time
	Search    string
	Offset    int
	Limit     int
	KRAStatus string
}

type InvoiceStats struct {
	TotalValue           float64          `json:"total_value"`
	DraftCount           int64            `json:"draft_count"`
	SentCount            int64            `json:"sent_count"`
	PaidCount            int64            `json:"paid_count"`
	TotalPaid            float64          `json:"total_paid"`
	ViewedCount          int64            `json:"viewed_count"`
	CancelledCount       int64            `json:"cancelled_count"`
	Last7DaysInvoices    int64            `json:"last_seven_days_invoices"`
	OverdueCount         int64            `json:"overdue_count"`
	PartiallyPaidCount   int64            `json:"partially_paid_count"`
	OverdueInvoicesCount int64            `json:"overdue_invoice_count"`
	AverageInvoiceValue  float64          `json:"average_invoice_value"`
	CollectionRate       float64          `json:"collection_rate"`
	TotalOutstanding     float64          `json:"total_outstanding"`
	TotalOverdue         float64          `json:"total_overdue"`
	Last7DaysValue       float64          `json:"last_seven_days_value"`
	TotalClients         int64            `json:"total_clients"`
	TotalInvoices        int64            `json:"total_invoices"`
	RecentInvoices       []models.Invoice `json:"recent_invoices"`
}

type DashboardStats struct {
	TotalRevenue          float64          `json:"total_revenue"`
	RevenueThisPeriod     float64          `json:"revenue_this_period"`
	RevenuePreviousPeriod float64          `json:"revenue_previous_period"`
	Outstanding           float64          `json:"outstanding"`
	DraftCount            int64            `json:"draft_count"`
	SentCount             int64            `json:"sent_count"`
	PaidCount             int64            `json:"paid_count"`
	OverdueCount          int64            `json:"overdue_count"`
	PartialCount          int64            `json:"partial_count"`
	CancelledCount        int64            `json:"cancelled_count"`
	TotalClients          int64            `json:"total_clients"`
	TotalInvoices         int64            `json:"total_invoices"`
	RecentInvoices        []models.Invoice `json:"recent_invoices"`

	// Growth metrics
	RevenueGrowth float64 `json:"revenue_growth"`
	InvoiceGrowth float64 `json:"invoice_growth"`
	ClientGrowth  float64 `json:"client_growth"`

	// Payment analytics
	AvgInvoiceValue float64 `json:"avg_invoice_value"`
	CollectionRate  float64 `json:"collection_rate"`

	// Time analytics
	AvgPaymentDays   float64         `json:"avg_payment_days"`
	TopPayingClients []ClientRevenue `json:"top_paying_clients"`

	// Monthly comparison data (last 12 months)
	MonthlyRevenue  []MonthlyData `json:"monthly_revenue"`
	MonthlyInvoices []MonthlyData `json:"monthly_invoices"`

	// Daily data for current period
	DailyRevenue  []DailyData `json:"daily_revenue"`
	DailyInvoices []DailyData `json:"daily_invoices"`
}

type ClientRevenue struct {
	ClientID     string  `json:"client_id"`
	ClientName   string  `json:"client_name"`
	TotalPaid    float64 `json:"total_paid"`
	InvoiceCount int64   `json:"invoice_count"`
}

type MonthlyData struct {
	Month  string  `json:"month"`
	Amount float64 `json:"amount"`
	Label  string  `json:"label"`
}

type DailyData struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Label  string  `json:"label"`
}

// Internal types for KRA conversion
type kraInvoice struct {
	ID            string
	InvoiceNumber string
	Currency      string
	Subtotal      float64
	TaxRate       float64
	TaxAmount     float64
	Discount      float64
	Total         float64
	PaidAmount    float64
	CreatedAt     time.Time
	DueDate       time.Time
	Status        string
	Items         []kraInvoiceItem
}

type kraInvoiceItem struct {
	ID          string
	Description string
	Quantity    float64
	Unit        string
	UnitPrice   float64
	Total       float64
}

type kraUser struct {
	ID          string
	Email       string
	Phone       string
	CompanyName string
	KRAPIN      string
}

type kraClient struct {
	ID      string
	Name    string
	Email   string
	Phone   string
	Address string
	KRAPIN  string
}
