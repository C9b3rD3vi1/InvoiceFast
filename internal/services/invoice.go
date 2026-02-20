package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrEmptyItems       = errors.New("invoice must have at least one item")
	ErrInvalidQuantity  = errors.New("item quantity cannot be negative")
	ErrInvoiceNotFound  = errors.New("invoice not found")
	ErrCannotEditPaid   = errors.New("cannot edit paid invoice")
	ErrCannotCancelPaid = errors.New("cannot cancel paid invoice")
	ErrCannotSendDraft  = errors.New("cannot send draft invoice")
	ErrAlreadySent      = errors.New("invoice already sent")
	ErrOverdueAmount    = errors.New("payment exceeds invoice amount")
	ErrInvalidCurrency  = errors.New("invalid currency code")
)

var validCurrencies = map[string]bool{
	"KES": true, "USD": true, "EUR": true, "GBP": true,
	"TZS": true, "UGX": true, "NGN": true,
}

type InvoiceService struct {
	db *database.DB
}

func NewInvoiceService(db *database.DB) *InvoiceService {
	return &InvoiceService{db: db}
}

// CreateInvoice creates a new invoice with items
func (s *InvoiceService) CreateInvoice(userID, clientID string, req *CreateInvoiceRequest) (*models.Invoice, error) {
	// Validate inputs early (fail fast)
	if err := s.validateCreateRequest(userID, clientID, req); err != nil {
		return nil, err
	}

	client := &models.Client{}
	if err := s.db.First(client, "id = ? AND user_id = ?", clientID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("client not found: %w", ErrInvoiceNotFound)
		}
		return nil, fmt.Errorf("failed to find client: %w", err)
	}

	// Validate items
	if len(req.Items) == 0 {
		return nil, ErrEmptyItems
	}

	// Calculate totals
	var subtotal float64
	var items []models.InvoiceItem
	for i, item := range req.Items {
		// Validate individual item
		if item.Quantity < 0 {
			return nil, ErrInvalidQuantity
		}
		if item.UnitPrice < 0 {
			item.UnitPrice = 0 // Default negative to zero
		}
		if strings.TrimSpace(item.Description) == "" {
			item.Description = "Item" // Default empty description
		}

		lineTotal := item.Quantity * item.UnitPrice
		subtotal += lineTotal
		items = append(items, models.InvoiceItem{
			ID:          uuid.New().String(),
			Description: strings.TrimSpace(item.Description),
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Unit:        item.Unit,
			Total:       lineTotal,
			SortOrder:   i,
		})
	}

	// Validate currency - default to KES if invalid
	currency := strings.ToUpper(req.Currency)
	if currency == "" {
		currency = "KES"
	}
	if !validCurrencies[currency] {
		currency = "KES" // Default to KES
	}

	// Calculate tax and discount (ensure non-negative)
	taxRate := math.Max(0, math.Min(100, req.TaxRate)) // Clamp between 0-100
	taxAmount := subtotal * (taxRate / 100)
	discount := math.Max(0, req.Discount) // Ensure non-negative
	total := subtotal + taxAmount - discount

	// Handle edge case: total cannot be negative
	if total < 0 {
		total = 0
	}

	invoice := &models.Invoice{
		ID:            uuid.New().String(),
		UserID:        userID,
		ClientID:      clientID,
		InvoiceNumber: generateInvoiceNumber(userID),
		Reference:     strings.TrimSpace(req.Reference),
		Currency:      currency,
		Subtotal:      math.Round(subtotal*100) / 100,
		TaxRate:       taxRate,
		TaxAmount:     math.Round(taxAmount*100) / 100,
		Discount:      math.Round(discount*100) / 100,
		Total:         math.Round(total*100) / 100,
		Status:        models.InvoiceStatusDraft,
		DueDate:       req.DueDate,
		Notes:         strings.TrimSpace(req.Notes),
		Terms:         strings.TrimSpace(req.Terms),
		BrandColor:    req.BrandColor,
		LogoURL:       req.LogoURL,
		MagicToken:    uuid.New().String(),
	}

	// Use transaction for data integrity
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(invoice).Error; err != nil {
			return fmt.Errorf("failed to create invoice: %w", err)
		}

		// Add items
		for i := range items {
			items[i].InvoiceID = invoice.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			return fmt.Errorf("failed to create invoice items: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	invoice.Items = items
	invoice.Client = *client

	return invoice, nil
}

// validateCreateRequest validates the create invoice request
func (s *InvoiceService) validateCreateRequest(userID, clientID string, req *CreateInvoiceRequest) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("user ID is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return errors.New("client ID is required")
	}
	if req.DueDate.IsZero() {
		return errors.New("due date is required")
	}
	if req.DueDate.Before(time.Now().UTC().AddDate(0, 0, -1)) {
		return errors.New("due date cannot be in the past")
	}
	return nil
}

// GetInvoiceByID retrieves an invoice by ID
func (s *InvoiceService) GetInvoiceByID(invoiceID, userID string) (*models.Invoice, error) {
	if strings.TrimSpace(invoiceID) == "" || strings.TrimSpace(userID) == "" {
		return nil, ErrInvoiceNotFound
	}

	var invoice models.Invoice
	err := s.db.Preload("Client").Preload("Items").Preload("Payments").
		First(&invoice, "id = ? AND user_id = ?", invoiceID, userID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to fetch invoice: %w", err)
	}
	return &invoice, nil
}

// GetInvoiceByMagicToken retrieves an invoice by magic token (for client portal)
func (s *InvoiceService) GetInvoiceByMagicToken(token string) (*models.Invoice, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrInvoiceNotFound
	}

	var invoice models.Invoice
	err := s.db.Preload("Client").Preload("Items").Preload("Payments").Preload("User").
		First(&invoice, "magic_token = ?", token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to fetch invoice: %w", err)
	}
	return &invoice, nil
}

// GetInvoiceByNumber retrieves an invoice by invoice number
func (s *InvoiceService) GetInvoiceByNumber(invoiceNumber string) (*models.Invoice, error) {
	if strings.TrimSpace(invoiceNumber) == "" {
		return nil, ErrInvoiceNotFound
	}

	var invoice models.Invoice
	err := s.db.Preload("Client").Preload("Items").Preload("Payments").
		First(&invoice, "invoice_number = ?", invoiceNumber).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to fetch invoice: %w", err)
	}
	return &invoice, nil
}

// GetUserInvoices retrieves all invoices for a user with filtering
func (s *InvoiceService) GetUserInvoices(userID string, filter InvoiceFilter) ([]models.Invoice, int64, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, 0, errors.New("user ID is required")
	}

	var invoices []models.Invoice
	var total int64

	query := s.db.Model(&models.Invoice{}).Where("user_id = ?", userID)

	// Apply filters safely
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.ClientID != "" {
		query = query.Where("client_id = ?", filter.ClientID)
	}
	if filter.FromDate != nil && !filter.FromDate.IsZero() {
		query = query.Where("created_at >= ?", filter.FromDate)
	}
	if filter.ToDate != nil && !filter.ToDate.IsZero() {
		query = query.Where("created_at <= ?", filter.ToDate)
	}
	if filter.Search != "" {
		search := "%" + strings.TrimSpace(filter.Search) + "%"
		query = query.Where("invoice_number ILIKE ? OR reference ILIKE ?", search, search)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count invoices: %w", err)
	}

	// Apply pagination - ensure valid values
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20 // Default limit
	}

	// Apply ordering and pagination
	query = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit)

	if err := query.Preload("Client").Preload("Items").Find(&invoices).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch invoices: %w", err)
	}

	return invoices, total, nil
}

// UpdateInvoice updates an invoice
func (s *InvoiceService) UpdateInvoice(invoiceID, userID string, req *UpdateInvoiceRequest) (*models.Invoice, error) {
	invoice, err := s.GetInvoiceByID(invoiceID, userID)
	if err != nil {
		return nil, err
	}

	// Edge case: Can only edit draft invoices
	if invoice.Status != models.InvoiceStatusDraft {
		return nil, ErrCannotEditPaid
	}

	// Update fields safely
	if req.DueDate != nil {
		if req.DueDate.IsZero() {
			return nil, errors.New("due date cannot be empty")
		}
		invoice.DueDate = *req.DueDate
	}
	if req.Reference != nil {
		invoice.Reference = strings.TrimSpace(*req.Reference)
	}
	if req.Currency != nil {
		currency := strings.ToUpper(*req.Currency)
		if validCurrencies[currency] {
			invoice.Currency = currency
		}
	}
	if req.TaxRate != nil {
		invoice.TaxRate = math.Max(0, math.Min(100, *req.TaxRate))
	}
	if req.Discount != nil {
		invoice.Discount = math.Max(0, *req.Discount)
	}
	if req.Notes != nil {
		invoice.Notes = strings.TrimSpace(*req.Notes)
	}
	if req.Terms != nil {
		invoice.Terms = strings.TrimSpace(*req.Terms)
	}
	if req.BrandColor != nil {
		invoice.BrandColor = *req.BrandColor
	}

	// Recalculate totals
	s.recalculateInvoiceTotals(invoice)

	if err := s.db.Save(invoice).Error; err != nil {
		return nil, fmt.Errorf("failed to update invoice: %w", err)
	}

	return invoice, nil
}

// UpdateInvoiceItems updates invoice items
func (s *InvoiceService) UpdateInvoiceItems(invoiceID, userID string, items []InvoiceItemRequest) (*models.Invoice, error) {
	invoice, err := s.GetInvoiceByID(invoiceID, userID)
	if err != nil {
		return nil, err
	}

	// Can only edit draft invoices
	if invoice.Status != models.InvoiceStatusDraft {
		return nil, ErrCannotEditPaid
	}

	// Validate items
	if len(items) == 0 {
		return nil, ErrEmptyItems
	}

	// Use transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Delete existing items
		if err := tx.Where("invoice_id = ?", invoiceID).Delete(&models.InvoiceItem{}).Error; err != nil {
			return fmt.Errorf("failed to delete items: %w", err)
		}

		// Create new items
		var newItems []models.InvoiceItem
		var subtotal float64
		for i, item := range items {
			if item.Quantity < 0 {
				return ErrInvalidQuantity
			}
			lineTotal := item.Quantity * item.UnitPrice
			subtotal += lineTotal
			newItems = append(newItems, models.InvoiceItem{
				ID:          uuid.New().String(),
				InvoiceID:   invoiceID,
				Description: strings.TrimSpace(item.Description),
				Quantity:    item.Quantity,
				UnitPrice:   item.UnitPrice,
				Unit:        item.Unit,
				Total:       lineTotal,
				SortOrder:   i,
			})
		}

		if err := tx.Create(&newItems).Error; err != nil {
			return fmt.Errorf("failed to create items: %w", err)
		}

		// Update invoice totals
		invoice.Items = newItems
		s.recalculateInvoiceTotals(invoice)

		if err := tx.Save(invoice).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return invoice, nil
}

// recalculateInvoiceTotals recalculates invoice totals
func (s *InvoiceService) recalculateInvoiceTotals(invoice *models.Invoice) {
	var subtotal float64
	for _, item := range invoice.Items {
		subtotal += item.Total
	}
	invoice.Subtotal = math.Round(subtotal*100) / 100
	invoice.TaxAmount = math.Round(subtotal*(invoice.TaxRate/100)*100) / 100
	invoice.Total = math.Round((subtotal+invoice.TaxAmount-invoice.Discount)*100) / 100

	// Ensure total is not negative
	if invoice.Total < 0 {
		invoice.Total = 0
	}
}

// SendInvoice marks invoice as sent and triggers notifications
func (s *InvoiceService) SendInvoice(invoiceID, userID string) (*models.Invoice, error) {
	invoice, err := s.GetInvoiceByID(invoiceID, userID)
	if err != nil {
		return nil, err
	}

	// Edge case: Cannot send if already sent or paid
	if invoice.Status == models.InvoiceStatusSent || invoice.Status == models.InvoiceStatusPaid {
		return nil, ErrAlreadySent
	}

	// Edge case: Cannot send if cancelled
	if invoice.Status == models.InvoiceStatusCancelled {
		return nil, errors.New("cannot send cancelled invoice")
	}

	now := time.Now().UTC()
	invoice.Status = models.InvoiceStatusSent
	invoice.SentAt = gorm.NowFunc()

	if err := s.db.Save(invoice).Error; err != nil {
		return nil, fmt.Errorf("failed to send invoice: %w", err)
	}

	// Log the action
	s.db.Create(&models.AuditLog{
		ID:         uuid.New().String(),
		UserID:     userID,
		Action:     "invoice.sent",
		EntityType: "invoice",
		EntityID:   invoiceID,
		Details:    fmt.Sprintf(`{"invoice_number": "%s"}`, invoice.InvoiceNumber),
	})

	return invoice, nil
}

// RecordPayment records a payment for an invoice
func (s *InvoiceService) RecordPayment(invoiceID string, payment *models.Payment) error {
	invoice, err := s.GetInvoiceByID(invoiceID, payment.UserID)
	if err != nil {
		return err
	}

	// Save payment
	payment.InvoiceID = invoiceID
	if err := s.db.Create(payment).Error; err != nil {
		return fmt.Errorf("failed to record payment: %w", err)
	}

	// Update invoice
	invoice.PaidAmount += payment.Amount
	invoice.PaidAmount = math.Round(invoice.PaidAmount*100) / 100

	// Determine status based on paid amount
	if invoice.PaidAmount >= invoice.Total {
		// Full payment - cap at total (handle overpayment gracefully)
		invoice.PaidAmount = invoice.Total
		invoice.Status = models.InvoiceStatusPaid
		invoice.PaidAt = gorm.NowFunc()
	} else if invoice.PaidAmount > 0 {
		// Partial payment
		invoice.Status = models.InvoiceStatusPartiallyPaid
	}

	if err := s.db.Save(invoice).Error; err != nil {
		return fmt.Errorf("failed to update invoice: %w", err)
	}

	// Log the action
	s.db.Create(&models.AuditLog{
		ID:         uuid.New().String(),
		UserID:     payment.UserID,
		Action:     "payment.received",
		EntityType: "payment",
		EntityID:   payment.ID,
		Details:    fmt.Sprintf(`{"invoice_id": "%s", "amount": %f, "method": "%s"}`, invoiceID, payment.Amount, payment.Method),
	})

	return nil
}

// CancelInvoice cancels an invoice
func (s *InvoiceService) CancelInvoice(invoiceID, userID string) error {
	invoice, err := s.GetInvoiceByID(invoiceID, userID)
	if err != nil {
		return err
	}

	// Edge case: Cannot cancel paid invoice
	if invoice.Status == models.InvoiceStatusPaid {
		return ErrCannotCancelPaid
	}

	// Edge case: Cannot cancel already cancelled
	if invoice.Status == models.InvoiceStatusCancelled {
		return errors.New("invoice already cancelled")
	}

	invoice.Status = models.InvoiceStatusCancelled
	if err := s.db.Save(invoice).Error; err != nil {
		return fmt.Errorf("failed to cancel invoice: %w", err)
	}

	return nil
}

// GetDashboardStats returns dashboard statistics for a user
func (s *InvoiceService) GetDashboardStats(userID string, period string) (*DashboardStats, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}

	var stats DashboardStats

	// Determine date range
	var startDate time.Time
	now := time.Now().UTC()
	switch period {
	case "week":
		startDate = now.AddDate(0, 0, -7)
	case "month":
		startDate = now.AddDate(0, -1, 0)
	case "quarter":
		startDate = now.AddDate(0, -3, 0)
	case "year":
		startDate = now.AddDate(-1, 0, 0)
	default:
		startDate = now.AddDate(0, -1, 0)
	}

	// Total revenue (all time paid)
	s.db.Model(&models.Invoice{}).
		Where("user_id = ? AND status = ?", userID, models.InvoiceStatusPaid).
		Select("COALESCE(SUM(total), 0)").
		Scan(&stats.TotalRevenue)

	// Revenue this period
	s.db.Model(&models.Invoice{}).
		Where("user_id = ? AND status = ? AND paid_at >= ?", userID, models.InvoiceStatusPaid, startDate).
		Select("COALESCE(SUM(total), 0)").
		Scan(&stats.RevenueThisPeriod)

	// Outstanding (unpaid + partially paid)
	s.db.Model(&models.Invoice{}).
		Where("user_id = ? AND status IN ?", userID, []string{string(models.InvoiceStatusSent), string(models.InvoiceStatusViewed), string(models.InvoiceStatusPartiallyPaid), string(models.InvoiceStatusOverdue)}).
		Select("COALESCE(SUM(total - paid_amount), 0)").
		Scan(&stats.Outstanding)

	// Count by status
	s.db.Model(&models.Invoice{}).Where("user_id = ? AND status = ?", userID, models.InvoiceStatusDraft).Count(&stats.DraftCount)
	s.db.Model(&models.Invoice{}).Where("user_id = ? AND status = ?", userID, models.InvoiceStatusSent).Count(&stats.SentCount)
	s.db.Model(&models.Invoice{}).Where("user_id = ? AND status = ?", userID, models.InvoiceStatusPaid).Count(&stats.PaidCount)
	s.db.Model(&models.Invoice{}).Where("user_id = ? AND status = ?", userID, models.InvoiceStatusOverdue).Count(&stats.OverdueCount)

	// Total clients
	s.db.Model(&models.Client{}).Where("user_id = ?", userID).Count(&stats.TotalClients)

	// Total invoices
	s.db.Model(&models.Invoice{}).Where("user_id = ?", userID).Count(&stats.TotalInvoices)

	// Recent invoices
	s.db.Preload("Client").
		Order("created_at DESC").
		Limit(5).
		Find(&stats.RecentInvoices, "user_id = ?", userID)

	return &stats, nil
}

// GenerateInvoicePDF generates PDF for an invoice
func (s *InvoiceService) GenerateInvoicePDF(invoice *models.Invoice) ([]byte, error) {
	// For MVP, return HTML that can be printed to PDF
	// In production, use a PDF library like unconv or chrome headless
	html, err := s.renderInvoiceHTML(invoice)
	if err != nil {
		return nil, err
	}
	return []byte(html), nil
}

func (s *InvoiceService) renderInvoiceHTML(invoice *models.Invoice) (string, error) {
	// Get user's template
	var template models.Template
	if err := s.db.First(&template, "user_id = ? AND is_default = ?", invoice.UserID, true).Error; err != nil {
		// Use default classic template
		template.HTML = getDefaultTemplate()
	}

	// Replace placeholders with actual data
	html := template.HTML
	html = strings.ReplaceAll(html, "{{.InvoiceNumber}}", invoice.InvoiceNumber)
	html = strings.ReplaceAll(html, "{{.CompanyName}}", invoice.User.CompanyName)
	html = strings.ReplaceAll(html, "{{.ClientName}}", invoice.Client.Name)
	html = strings.ReplaceAll(html, "{{.Total}}", fmt.Sprintf("%.2f", invoice.Total))
	html = strings.ReplaceAll(html, "{{.Status}}", string(invoice.Status))

	return html, nil
}

func getDefaultTemplate() string {
	return `<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Invoice {{.InvoiceNumber}}</title></head><body>
<h1>Invoice {{.InvoiceNumber}}</h1>
<p>From: {{.CompanyName}}</p>
<p>To: {{.ClientName}}</p>
<p>Total: {{.Total}}</p>
<p>Status: {{.Status}}</p>
</body></html>`
}

func generateInvoiceNumber(userID string) string {
	// Generate unique invoice number
	timestamp := time.Now().UTC().Format("20060102")
	randBytes := make([]byte, 2)
	rand.Read(randBytes)
	return fmt.Sprintf("INV-%s-%s", timestamp, hex.EncodeToString(randBytes))
}

// Request types
type CreateInvoiceRequest struct {
	ClientID   string               `json:"client_id" binding:"required"`
	Reference  string               `json:"reference"`
	Currency   string               `json:"currency"`
	TaxRate    float64              `json:"tax_rate"`
	Discount   float64              `json:"discount"`
	DueDate    time.Time            `json:"due_date" binding:"required"`
	Notes      string               `json:"notes"`
	Terms      string               `json:"terms"`
	BrandColor string               `json:"brand_color"`
	LogoURL    string               `json:"logo_url"`
	Items      []InvoiceItemRequest `json:"items" binding:"required,min=1"`
}

type InvoiceItemRequest struct {
	Description string  `json:"description" binding:"required"`
	Quantity    float64 `json:"quantity" binding:"required,min=-999999"`
	UnitPrice   float64 `json:"unit_price" binding:"required,min=0"`
	Unit        string  `json:"unit"`
}

type UpdateInvoiceRequest struct {
	DueDate    *time.Time `json:"due_date"`
	Reference  *string    `json:"reference"`
	Currency   *string    `json:"currency"`
	TaxRate    *float64   `json:"tax_rate"`
	Discount   *float64   `json:"discount"`
	Notes      *string    `json:"notes"`
	Terms      *string    `json:"terms"`
	BrandColor *string    `json:"brand_color"`
}

type InvoiceFilter struct {
	Status   string
	ClientID string
	FromDate *time.Time
	ToDate   *time.Time
	Search   string
	Offset   int
	Limit    int
}

type DashboardStats struct {
	TotalRevenue      float64          `json:"total_revenue"`
	RevenueThisPeriod float64          `json:"revenue_this_period"`
	Outstanding       float64          `json:"outstanding"`
	DraftCount        int64            `json:"draft_count"`
	SentCount         int64            `json:"sent_count"`
	PaidCount         int64            `json:"paid_count"`
	OverdueCount      int64            `json:"overdue_count"`
	TotalClients      int64            `json:"total_clients"`
	TotalInvoices     int64            `json:"total_invoices"`
	RecentInvoices    []models.Invoice `json:"recent_invoices"`
}
