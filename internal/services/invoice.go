package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
	ErrTenantRequired  = errors.New("tenant ID is required")
	ErrAlreadySent     = errors.New("invoice already sent")
)

var validCurrencies = map[string]bool{
	"KES": true, "USD": true, "EUR": true, "GBP": true,
	"TZS": true, "UGX": true, "NGN": true,
}

type InvoiceService struct {
	db              *database.DB
	emailService    *EmailService
	whatsappService *WhatsAppService
	exchangeService *ExchangeRateService
	kraService      *KRAService
	cfg             *config.Config
}

func NewInvoiceService(db *database.DB) *InvoiceService {
	return &InvoiceService{db: db}
}

func (s *InvoiceService) GetDB() *gorm.DB {
	return s.db.DB
}

func NewInvoiceServiceWithNotifications(db *database.DB, email *EmailService, whatsapp *WhatsAppService, cfg *config.Config) *InvoiceService {
	return &InvoiceService{
		db:              db,
		emailService:    email,
		whatsappService: whatsapp,
		cfg:             cfg,
	}
}

func NewInvoiceServiceWithExchange(db *database.DB, exchange *ExchangeRateService) *InvoiceService {
	return &InvoiceService{
		db:              db,
		exchangeService: exchange,
	}
}

func NewInvoiceServiceWithAll(db *database.DB, exchange *ExchangeRateService, email *EmailService, whatsapp *WhatsAppService, cfg *config.Config) *InvoiceService {
	return &InvoiceService{
		db:              db,
		exchangeService: exchange,
		emailService:    email,
		whatsappService: whatsapp,
		cfg:             cfg,
	}
}

// NewInvoiceServiceWithKRAService creates invoice service with KRA integration
func NewInvoiceServiceWithKRAService(db *database.DB, exchange *ExchangeRateService, email *EmailService, whatsapp *WhatsAppService, kra *KRAService, cfg *config.Config) *InvoiceService {
	return &InvoiceService{
		db:              db,
		exchangeService: exchange,
		emailService:    email,
		whatsappService: whatsapp,
		kraService:      kra,
		cfg:             cfg,
	}
}

// CreateInvoice creates a new invoice with items (tenant-scoped)
func (s *InvoiceService) CreateInvoice(tenantID, userID, clientID string, req *CreateInvoiceRequest) (*models.Invoice, error) {
	// Validate tenant
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	// Validate inputs early (fail fast)
	if err := s.validateCreateRequest(userID, clientID, req); err != nil {
		return nil, err
	}

	// Query with tenant isolation
	client := &models.Client{}
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(client, "id = ?", clientID).Error; err != nil {
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

		// Get item-level tax and discount rates (default to invoice level if not specified)
		itemTaxRate := item.TaxRate
		if itemTaxRate < 0 {
			itemTaxRate = 0
		}
		if itemTaxRate > 100 {
			itemTaxRate = 100
		}
		itemDiscountRate := item.DiscountRate
		if itemDiscountRate < 0 {
			itemDiscountRate = 0
		}
		if itemDiscountRate > 100 {
			itemDiscountRate = 100
		}

		// Calculate line totals
		lineSubtotal := item.Quantity * item.UnitPrice
		itemDiscountAmt := lineSubtotal * (itemDiscountRate / 100)
		lineSubtotalAfterDiscount := lineSubtotal - itemDiscountAmt
		itemTaxAmt := lineSubtotalAfterDiscount * (itemTaxRate / 100)
		lineTotal := lineSubtotalAfterDiscount + itemTaxAmt

		subtotal += lineTotal
		items = append(items, models.InvoiceItem{
			ID:           uuid.New().String(),
			Description:  strings.TrimSpace(item.Description),
			Quantity:     item.Quantity,
			UnitPrice:    item.UnitPrice,
			Unit:         item.Unit,
			TaxRate:      itemTaxRate,
			TaxAmount:    math.Round(itemTaxAmt*100) / 100,
			DiscountRate: itemDiscountRate,
			DiscountAmt:  math.Round(itemDiscountAmt*100) / 100,
			Total:        math.Round(lineTotal*100) / 100,
			SortOrder:    i,
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

	// Calculate KES equivalent for dual display
	kesEquivalent := total
	exchangeRate := 1.0
	if req.ExchangeRate != nil && *req.ExchangeRate > 0 {
		exchangeRate = *req.ExchangeRate
		kesEquivalent = total * exchangeRate
		if req.KESEquivalent != nil && *req.KESEquivalent > 0 {
			kesEquivalent = *req.KESEquivalent
		}
	} else if currency != "KES" && s.exchangeService != nil {
		rate, err := s.exchangeService.GetRate(currency, "KES")
		if err == nil && rate > 0 {
			kesEquivalent = total * rate
			exchangeRate = rate
		}
	}

magicTokenExpires := time.Now().AddDate(0, 3, 0)
	
	// Determine buyer type from request or auto-detect
	buyerType := req.BuyerType
	if buyerType == "" {
		buyerType = DetectBuyerType(client)
	}
	
	invoice := &models.Invoice{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		UserID:            userID,
		ClientID:          clientID,
		Reference:         strings.TrimSpace(req.Reference),
		Currency:          currency,
		KESEquivalent:      math.Round(kesEquivalent*100) / 100,
		ExchangeRate:      exchangeRate,
		ExchangeRateAt:    time.Now(),
		Subtotal:          math.Round(subtotal*100) / 100,
		TaxRate:          taxRate,
		TotalTax:          math.Round(taxAmount*100) / 100,
		Discount:         math.Round(discount*100) / 100,
		Total:            math.Round(total*100) / 100,
		BalanceDue:       math.Round(total*100) / 100,
		TaxType:          models.TaxTypeStandard,
		Status:           models.InvoiceStatusDraft,
		DueDate:         req.DueDate,
		Notes:           strings.TrimSpace(req.Notes),
		Terms:           strings.TrimSpace(req.Terms),
		BrandColor:       req.BrandColor,
		LogoURL:          req.LogoURL,
		MagicToken:       uuid.New().String(),
		MagicTokenExpiresAt: &magicTokenExpires,
		KRAStatus:       models.KRAInvoiceStatusPending,
		Version:         1,
		BuyerClassification: buyerType,
	}

	// Set title if provided
	if req.Title != "" {
		invoice.Title = req.Title
	}

	// Use transaction for data integrity - including sequential numbering
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get next sequence number in a transaction-safe manner
		seq := models.InvoiceSequence{TenantID: tenantID, Prefix: "INV", Padding: 6}
		seqNum, err := seq.GetNextSequence(tx)
		if err != nil {
			return fmt.Errorf("failed to generate invoice number: %w", err)
		}
		invoice.InvoiceNumber = models.FormatInvoiceNumber("INV", seqNum, 6)
		invoice.SequenceNumber = seqNum

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

		// Validate totals before committing
		if err := models.ValidateInvoiceTotals(invoice, items); err != nil {
			return fmt.Errorf("invoice totals validation failed: %w", err)
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

	// Validate buyer type if provided
	if req.BuyerType != "" {
		client := &models.Client{}
		if err := s.db.First(client, "id = ?", clientID).Error; err == nil {
			// Client exists - validate buyer type
			if err := ValidateBuyerType(req.BuyerType, client); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetInvoiceByID retrieves an invoice by ID (tenant-scoped)
func (s *InvoiceService) GetInvoiceByID(tenantID, invoiceID string) (*models.Invoice, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}
	if strings.TrimSpace(invoiceID) == "" {
		return nil, ErrInvoiceNotFound
	}

	var invoice models.Invoice
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Preload("User").Preload("Client").Preload("Items").Preload("Payments").
		First(&invoice, "id = ?", invoiceID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to fetch invoice: %w", err)
	}
	return &invoice, nil
}

// validateID prevents using tenant ID as invoice ID
func (s *InvoiceService) validateID(invoiceID, tenantID string) error {
	if invoiceID == "" {
		return ErrInvoiceNotFound
	}
	// If the invoice ID matches the tenant ID, this is a mismatch
	if invoiceID == tenantID {
		return ErrInvoiceNotFound
	}
	return nil
}

// GetInvoiceByMagicToken retrieves an invoice by magic token (tenant-bound)
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

	// Check if token has expired
	if invoice.MagicTokenExpiresAt != nil && invoice.MagicTokenExpiresAt.Before(time.Now()) {
		return nil, errors.New("payment link has expired")
	}

	// Security: Verify invoice is accessible - must have valid tenant association
	// The token alone is not sufficient - we need to ensure the invoice belongs to an active tenant
	if invoice.TenantID == "" {
		return nil, errors.New("invalid payment link - no tenant association")
	}

	// Check that the user/tenant is still active
	var user models.User
	// Tenant-scoped user lookup - filter by invoice's tenant to prevent cross-tenant access
	if err := s.db.Scopes(database.TenantFilter(invoice.TenantID)).First(&user, "id = ?", invoice.UserID).Error; err != nil {
		return nil, errors.New("invalid payment link - user not found")
	}
	if !user.IsActive {
		return nil, errors.New("payment link inactive - contact administrator")
	}

	// Track viewed status if not already set
	if invoice.ViewedAt == nil {
		now := time.Now()
		s.db.Model(&invoice).Update("viewed_at", now)
	}

	return &invoice, nil
}

// RotateMagicToken generates a new magic token for an invoice
// SECURITY: Call this after successful payment to invalidate the old token
func (s *InvoiceService) RotateMagicToken(invoiceID string) error {
	newToken := uuid.New().String()
	// Token expires in 24 hours from rotation (can be extended if needed)
	expiresAt := time.Now().Add(24 * time.Hour)

	return s.db.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
		"magic_token":            newToken,
		"magic_token_expires_at": expiresAt,
	}).Error
}

// GetInvoiceByNumber retrieves an invoice by invoice number (tenant-scoped)
func (s *InvoiceService) GetInvoiceByNumber(tenantID, invoiceNumber string) (*models.Invoice, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}
	if strings.TrimSpace(invoiceNumber) == "" {
		return nil, ErrInvoiceNotFound
	}

	var invoice models.Invoice
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Preload("Client").Preload("Items").Preload("Payments").
		First(&invoice, "invoice_number = ?", invoiceNumber).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to fetch invoice: %w", err)
	}
	return &invoice, nil
}

// GetUserInvoices retrieves all invoices for a tenant with filtering (tenant-scoped)
func (s *InvoiceService) GetUserInvoices(tenantID string, filter InvoiceFilter) ([]models.Invoice, int64, error) {
	if tenantID == "" {
		return nil, 0, ErrTenantRequired
	}

	var invoices []models.Invoice
	var total int64

	query := s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{})

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
		if s.db.IsPostgres() {
			query = query.Where("invoice_number ILIKE ? OR reference ILIKE ?", search, search)
		} else {
			query = query.Where("LOWER(invoice_number) LIKE LOWER(?) OR LOWER(reference) LIKE LOWER(?)", search, search)
		}
	}
	if filter.KRAStatus != "" {
		if filter.KRAStatus == "not_submitted" {
			query = query.Where("kra_status = 'pending'")
		} else {
			query = query.Where("kra_status = ?", filter.KRAStatus)
		}
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

// UpdateInvoice updates an invoice (tenant-scoped)
func (s *InvoiceService) UpdateInvoice(tenantID, invoiceID string, req *UpdateInvoiceRequest) (*models.Invoice, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	invoice, err := s.GetInvoiceByID(tenantID, invoiceID)
	if err != nil {
		return nil, err
	}

	// Edge case: Can only edit draft invoices unless updating status
	if invoice.Status != models.InvoiceStatusDraft && req.Status == nil {
		return nil, ErrCannotEditPaid
	}

	// Update status if provided
	if req.Status != nil {
		newStatus := models.InvoiceStatus(*req.Status)
		// Validate status transition
		if invoice.Status == models.InvoiceStatusDraft && (newStatus == models.InvoiceStatusSent || newStatus == models.InvoiceStatusCancelled) {
			invoice.Status = newStatus
		} else if newStatus == models.InvoiceStatusCancelled && invoice.Status != models.InvoiceStatusPaid {
			invoice.Status = newStatus
		} else if newStatus == models.InvoiceStatusPaid {
			invoice.Status = newStatus
		}
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
	if req.Title != nil {
		invoice.Title = *req.Title
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

// UpdateInvoiceItems updates invoice items (tenant-scoped)
func (s *InvoiceService) UpdateInvoiceItems(tenantID, invoiceID string, items []InvoiceItemRequest) (*models.Invoice, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	invoice, err := s.GetInvoiceByID(tenantID, invoiceID)
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
			// Get item-level tax and discount rates
			itemTaxRate := item.TaxRate
			if itemTaxRate < 0 {
				itemTaxRate = 0
			}
			if itemTaxRate > 100 {
				itemTaxRate = 100
			}
			itemDiscountRate := item.DiscountRate
			if itemDiscountRate < 0 {
				itemDiscountRate = 0
			}
			if itemDiscountRate > 100 {
				itemDiscountRate = 100
			}

			// Calculate line totals
			lineSubtotal := item.Quantity * item.UnitPrice
			itemDiscountAmt := lineSubtotal * (itemDiscountRate / 100)
			lineSubtotalAfterDiscount := lineSubtotal - itemDiscountAmt
			itemTaxAmt := lineSubtotalAfterDiscount * (itemTaxRate / 100)
			lineTotal := lineSubtotalAfterDiscount + itemTaxAmt

			subtotal += lineTotal
			newItems = append(newItems, models.InvoiceItem{
				ID:           uuid.New().String(),
				InvoiceID:    invoiceID,
				Description:  strings.TrimSpace(item.Description),
				Quantity:     item.Quantity,
				UnitPrice:    item.UnitPrice,
				Unit:         item.Unit,
				TaxRate:      itemTaxRate,
				TaxAmount:    math.Round(itemTaxAmt*100) / 100,
				DiscountRate: itemDiscountRate,
				DiscountAmt:  math.Round(itemDiscountAmt*100) / 100,
				Total:        math.Round(lineTotal*100) / 100,
				SortOrder:    i,
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

// SendInvoice marks invoice as sent, triggers notifications, and submits to KRA e-TIMS (tenant-scoped)
func (s *InvoiceService) SendInvoice(tenantID, invoiceID, userID string) (*models.Invoice, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	invoice, err := s.GetInvoiceByID(tenantID, invoiceID)
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

	// Load client and user for notifications
	// SECURITY: Added TenantFilter to prevent IDOR
	var client models.Client
	var user models.User
	s.db.Scopes(database.TenantFilter(tenantID)).First(&client, "id = ?", invoice.ClientID)
	s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", userID)

	// Submit to KRA e-TIMS if configured (non-blocking, async)
	// KRA ICN will be updated after successful submission
	if s.kraService != nil && invoice.KRAICN == "" {
		tenantID := invoice.TenantID
		invoiceID := invoice.ID
		invoiceNum := invoice.InvoiceNumber
		createdAt := invoice.CreatedAt
		subtotal := invoice.Subtotal
		discount := invoice.Discount
		taxRate := invoice.TaxRate
		taxAmount := invoice.TaxAmount
		total := invoice.Total
		currency := invoice.Currency

		go func() {
			// Load fresh data for KRA submission
			// SECURITY: Added TenantFilter to prevent IDOR
			var cli models.Client
			var usr models.User
			s.db.Scopes(database.TenantFilter(tenantID)).First(&cli, "id = ?", invoice.ClientID)
			s.db.Scopes(database.TenantFilter(tenantID)).First(&usr, "id = ?", userID)

			// Build KRA data using the service's format
			items := make([]KRAItem, 0)
			s.db.Model(&models.InvoiceItem{}).Where("invoice_id = ?", invoiceID).Find(&items)

			kraData := &KRAInvoiceData{
				InvoiceNumber: invoiceNum,
				InvoiceDate:   createdAt.Format("2006-01-02"),
				InvoiceTime:   createdAt.Format("15:04:05"),
				Seller: KRASeller{
					RegistrationNumber: usr.KRAPIN,
					BusinessName:       usr.CompanyName,
					ContactMobile:      usr.Phone,
					ContactEmail:       usr.Email,
				},
				Buyer: KRABuyer{
					CustomerName:       cli.Name,
					ContactMobile:      cli.Phone,
					ContactEmail:       cli.Email,
					RegistrationNumber: cli.KRAPIN,
				},
				Items:             items,
				SubTotal:          subtotal,
				TotalExcludingVAT: subtotal - discount,
				VATRate:           taxRate,
				VATAmount:         taxAmount,
				TotalIncludingVAT: total,
				Currency:          currency,
			}

			kraResp, err := s.kraService.SubmitInvoice(kraData, invoice.TenantID, invoice.ID)
			if err != nil {
				log.Printf("[KRA] Failed to submit invoice %s: %v", invoiceNum, err)
				s.db.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
					"kra_status": "failed",
					"kra_error":  err.Error(),
				})
				return
			}

			s.db.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
				"kra_icn":          kraResp.ICN,
				"kra_qr_code":      kraResp.QRCode,
				"kra_status":       "submitted",
				"kra_submitted_at": time.Now(),
				"kra_error":        "",
			})
			log.Printf("[KRA] Invoice %s submitted - ICN: %s", invoiceNum, kraResp.ICN)
		}()
	}

	invoice.Status = models.InvoiceStatusSent
	now := time.Now()
	invoice.SentAt = &now

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

	// Send email notification (async, don't fail if email fails)
	go s.sendInvoiceNotifications(invoice, userID)

	return invoice, nil
}

// sendInvoiceNotifications sends email and WhatsApp notifications for an invoice
func (s *InvoiceService) sendInvoiceNotifications(invoice *models.Invoice, userID string) {
	if s.emailService == nil {
		return
	}

	// Load client and user for notifications (tenant-scoped)
	var client models.Client
	var user models.User
	tenantID := invoice.TenantID
	s.db.Scopes(database.TenantFilter(tenantID)).First(&client, "id = ?", invoice.ClientID)
	s.db.Scopes(database.TenantFilter(tenantID)).First(&user, "id = ?", userID)

	// Build invoice link
	invoiceLink := fmt.Sprintf("%s/invoice/%s", s.getBaseURL(), invoice.MagicToken)

	// Send email notification
	emailData := &InvoiceEmailData{
		CompanyName:   user.CompanyName,
		CompanyEmail:  user.Email,
		ClientName:    client.Name,
		ClientEmail:   client.Email,
		InvoiceNumber: invoice.InvoiceNumber,
		InvoiceLink:   invoiceLink,
		Amount:        invoice.Total,
		Currency:      invoice.Currency,
		DueDate:       invoice.DueDate.Format("02 Jan 2006"),
	}

	if err := s.emailService.SendInvoiceEmail(emailData); err != nil {
		log.Printf("Failed to send invoice email for %s: %v", invoice.InvoiceNumber, err)
	}

	// Send WhatsApp notification if configured
	if s.whatsappService != nil && client.Phone != "" {
		amount := fmt.Sprintf("%s %.2f", invoice.Currency, invoice.Total)
		data := map[string]string{
			"company": user.CompanyName,
			"invoice": invoice.InvoiceNumber,
			"amount":  amount,
			"link":    invoiceLink,
		}
		if err := s.whatsappService.SendInvoiceNotification(client.Phone, data); err != nil {
			log.Printf("Failed to send WhatsApp for %s: %v", invoice.InvoiceNumber, err)
		}
	}
}

// getBaseURL returns the base URL for the application
func (s *InvoiceService) getBaseURL() string {
	if s.cfg != nil {
		return s.cfg.Server.BaseURL
	}
	return "https://invoice.simuxtech.com"
}

// RecordPayment records a payment for an invoice (tenant-scoped)
// RecordPayment records a payment for an invoice with proper state machine enforcement
func (s *InvoiceService) RecordPayment(tenantID, invoiceID string, payment *models.Payment) error {
	if tenantID == "" {
		return ErrTenantRequired
	}

	// Use transaction for payment processing
	return s.db.Transaction(func(tx *gorm.DB) error {
		var invoice models.Invoice
		if err := tx.Scopes(database.TenantFilter(tenantID)).
			Preload("Items").
			First(&invoice, "id = ?", invoiceID).Error; err != nil {
			return ErrInvoiceNotFound
		}

		// State machine: Only allow payments on SENT, VIEWED, OVERDUE, or PARTIALLY_PAID
		allowedStatuses := []models.InvoiceStatus{
			models.InvoiceStatusSent,
			models.InvoiceStatusViewed,
			models.InvoiceStatusOverdue,
			models.InvoiceStatusPartiallyPaid,
		}
		allowed := false
		for _, status := range allowedStatuses {
			if invoice.Status == status {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("cannot record payment on invoice with status %s", invoice.Status)
		}

		// Validate payment amount
		if payment.Amount <= 0 {
			return errors.New("payment amount must be positive")
		}

		// Calculate new balance using decimal for precision
		oldPaidAmount := decimal.NewFromFloat(invoice.PaidAmount)
		paymentDecimal := decimal.NewFromFloat(payment.Amount)
		newPaidAmount := oldPaidAmount.Add(paymentDecimal)
		total := decimal.NewFromFloat(invoice.Total)

		// Handle overpayment gracefully
		if newPaidAmount.GreaterThan(total) {
			overpayment := newPaidAmount.Sub(total)
			newPaidAmount = total
			overpaymentFloat, _ := overpayment.Float64()
			payment.Amount = overpaymentFloat
		}

		// Set payment details
		payment.InvoiceID = invoiceID
		payment.Status = models.PaymentStatusCompleted
		if payment.ID == "" {
			payment.ID = uuid.New().String()
		}

		// Save payment
		if err := tx.Create(payment).Error; err != nil {
			return fmt.Errorf("failed to record payment: %w", err)
		}

		// Update invoice with optimistic locking
		invoice.PaidAmount, _ = newPaidAmount.Float64()
		invoice.PaidAmount = math.Round(invoice.PaidAmount*100) / 100
		balanceDue := total.Sub(newPaidAmount)
		invoice.BalanceDue, _ = balanceDue.Float64()
		invoice.BalanceDue = math.Round(invoice.BalanceDue*100) / 100

		// Determine new status based on paid amount
		newStatus := invoice.Status
		if newPaidAmount.Equal(total) || invoice.BalanceDue <= 0.01 {
			// Full payment
			newStatus = models.InvoiceStatusPaid
			now := time.Now()
			invoice.PaidAt = &now
		} else if newPaidAmount.GreaterThan(decimal.Zero) {
			// Partial payment
			newStatus = models.InvoiceStatusPartiallyPaid
		}

		// Validate state transition
		if err := models.ValidateTransition(invoice.Status, newStatus); err != nil {
			return err
		}
		invoice.Status = newStatus
		invoice.Version++

		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		// Log the action
		tx.Create(&models.AuditLog{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			UserID:     payment.UserID,
			Action:     "payment.received",
			EntityType: "payment",
			EntityID:   payment.ID,
			Details:    fmt.Sprintf(`{"invoice_id": "%s", "amount": %f, "method": "%s", "status": "%s"}`, invoiceID, payment.Amount, payment.Method, newStatus),
		})

		return nil
	})
}

// CancelInvoice cancels an invoice with proper state machine validation
func (s *InvoiceService) CancelInvoice(tenantID, invoiceID, userID string) error {
	if tenantID == "" {
		return ErrTenantRequired
	}

	// Use transaction for cancellation
	return s.db.Transaction(func(tx *gorm.DB) error {
		var invoice models.Invoice
		if err := tx.Scopes(database.TenantFilter(tenantID)).First(&invoice, "id = ?", invoiceID).Error; err != nil {
			return ErrInvoiceNotFound
		}

		// Validate state transition using state machine
		newStatus := models.InvoiceStatusCancelled
		if err := models.ValidateTransition(invoice.Status, newStatus); err != nil {
			return err
		}

		// Additional checks: cannot cancel if KRA accepted
		if invoice.KRAStatus == models.KRAInvoiceStatusAccepted {
			return errors.New("cannot cancel invoice: KRA accepted")
		}

		now := time.Now()
		invoice.Status = newStatus
		invoice.CancelledAt = &now
		invoice.Version++

		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to cancel invoice: %w", err)
		}

		// Log cancellation
		tx.Create(&models.AuditLog{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			UserID:     userID,
			Action:     "invoice_cancelled",
			EntityType: "invoice",
			EntityID:   invoiceID,
			Details:    fmt.Sprintf(`{"invoice_number": "%s", "previous_status": "%s"}`, invoice.InvoiceNumber, invoice.Status),
		})

		return nil
	})
}

// DeleteInvoice permanently deletes an invoice (tenant-scoped)
func (s *InvoiceService) DeleteInvoice(tenantID, invoiceID string) error {
	if tenantID == "" {
		return ErrTenantRequired
	}

	invoice, err := s.GetInvoiceByID(tenantID, invoiceID)
	if err != nil {
		return err
	}

	// Delete related records first
	s.db.Where("invoice_id = ?", invoiceID).Delete(&models.Payment{})
	s.db.Where("invoice_id = ?", invoiceID).Delete(&models.InvoiceItem{})

	// Delete invoice
	if err := s.db.Delete(invoice).Error; err != nil {
		return fmt.Errorf("failed to delete invoice: %w", err)
	}

	return nil
}

// GetDashboardStats returns dashboard statistics for a tenant (tenant-scoped)
func (s *InvoiceService) GetDashboardStats(tenantID, period string) (*DashboardStats, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	var stats DashboardStats

	// Determine date range
	now := time.Now()
	var startDate, prevStartDate time.Time
	var periodDays int
	switch period {
	case "week":
		startDate = now.AddDate(0, 0, -7)
		prevStartDate = now.AddDate(0, 0, -14)
		periodDays = 7
	case "month":
		startDate = now.AddDate(0, -1, 0)
		prevStartDate = now.AddDate(0, -2, 0)
		periodDays = 30
	case "quarter":
		startDate = now.AddDate(0, -3, 0)
		prevStartDate = now.AddDate(0, -6, 0)
		periodDays = 90
	case "year":
		startDate = now.AddDate(-1, 0, 0)
		prevStartDate = now.AddDate(-2, 0, 0)
		periodDays = 365
	default:
		startDate = now.AddDate(0, -1, 0)
		prevStartDate = now.AddDate(0, -2, 0)
		periodDays = 30
	}

	// Total revenue (all time paid)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ?", models.InvoiceStatusPaid).
		Select("COALESCE(SUM(total), 0)").
		Scan(&stats.TotalRevenue)

	// Revenue this period
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ? AND paid_at >= ?", models.InvoiceStatusPaid, startDate).
		Select("COALESCE(SUM(total), 0)").
		Scan(&stats.RevenueThisPeriod)

	// Revenue previous period (for comparison)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ? AND paid_at >= ? AND paid_at < ?", models.InvoiceStatusPaid, prevStartDate, startDate).
		Select("COALESCE(SUM(total), 0)").
		Scan(&stats.RevenuePreviousPeriod)

	// Calculate revenue growth
	if stats.RevenuePreviousPeriod > 0 {
		stats.RevenueGrowth = ((stats.RevenueThisPeriod - stats.RevenuePreviousPeriod) / stats.RevenuePreviousPeriod) * 100
	} else if stats.RevenueThisPeriod > 0 {
		stats.RevenueGrowth = 100
	}

	// Outstanding (unpaid + partially paid)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status IN ?", []string{string(models.InvoiceStatusSent), string(models.InvoiceStatusViewed), string(models.InvoiceStatusPartiallyPaid), string(models.InvoiceStatusOverdue)}).
		Select("COALESCE(SUM(total - paid_amount), 0)").
		Scan(&stats.Outstanding)

	// Count by status
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusDraft).Count(&stats.DraftCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusSent).Count(&stats.SentCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusPaid).Count(&stats.PaidCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusOverdue).Count(&stats.OverdueCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusPartiallyPaid).Count(&stats.PartialCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusCancelled).Count(&stats.CancelledCount)

	// Total clients
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Client{}).Count(&stats.TotalClients)

	// Total invoices
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Count(&stats.TotalInvoices)

	// Invoice growth (this period vs previous)
	var currentInvoices, prevInvoices int64
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("created_at >= ?", startDate).Count(&currentInvoices)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("created_at >= ? AND created_at < ?", prevStartDate, startDate).Count(&prevInvoices)
	if prevInvoices > 0 {
		stats.InvoiceGrowth = (float64(currentInvoices-prevInvoices) / float64(prevInvoices)) * 100
	}

	// Average invoice value
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ?", models.InvoiceStatusPaid).
		Select("COALESCE(AVG(total), 0)").
		Scan(&stats.AvgInvoiceValue)

	// Collection rate (paid / total billed)
	var totalBilled, totalPaid float64
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Select("COALESCE(SUM(total), 0)").Scan(&totalBilled)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ?", models.InvoiceStatusPaid).
		Select("COALESCE(SUM(total), 0)").Scan(&totalPaid)
	if totalBilled > 0 {
		stats.CollectionRate = (totalPaid / totalBilled) * 100
	}

	// Average payment days
	var avgDays float64
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ? AND paid_at IS NOT NULL", models.InvoiceStatusPaid).
		Select("COALESCE(AVG(EXTRACT(EPOCH FROM (paid_at - created_at)) / 86400), 0)").Scan(&avgDays)
	stats.AvgPaymentDays = avgDays

	// Get monthly revenue for last 12 months
	stats.MonthlyRevenue = s.getMonthlyRevenue(tenantID, 12)
	stats.MonthlyInvoices = s.getMonthlyInvoices(tenantID, 12)

	// Get daily revenue for current period
	stats.DailyRevenue = s.getDailyRevenue(tenantID, startDate, periodDays)
	stats.DailyInvoices = s.getDailyInvoices(tenantID, startDate, periodDays)

	// Get top paying clients
	stats.TopPayingClients = s.getTopPayingClients(tenantID, 5)

	// Recent invoices
	s.db.Scopes(database.TenantFilter(tenantID)).Preload("Client").
		Order("created_at DESC").
		Limit(5).
		Find(&stats.RecentInvoices)

	return &stats, nil
}

// GetInvoiceStats returns invoice-specific statistics for the invoices list page
func (s *InvoiceService) GetInvoiceStats(tenantID string) (*InvoiceStats, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	var stats InvoiceStats

	// Total invoices count
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Count(&stats.TotalInvoices)

	// Count by status
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusDraft).Count(&stats.DraftCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusSent).Count(&stats.SentCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusViewed).Count(&stats.ViewedCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusPaid).Count(&stats.PaidCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusPartiallyPaid).Count(&stats.PartiallyPaidCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusOverdue).Count(&stats.OverdueCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusCancelled).Count(&stats.CancelledCount)

	// Total value by status
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusPaid).Select("COALESCE(SUM(total), 0)").Scan(&stats.TotalPaid)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status IN ?", []string{string(models.InvoiceStatusSent), string(models.InvoiceStatusViewed), string(models.InvoiceStatusPartiallyPaid), string(models.InvoiceStatusOverdue)}).Select("COALESCE(SUM(total - paid_amount), 0)").Scan(&stats.TotalOutstanding)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusOverdue).Select("COALESCE(SUM(total - paid_amount), 0)").Scan(&stats.TotalOverdue)

	// Calculate totals
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Select("COALESCE(SUM(total), 0)").Scan(&stats.TotalValue)

	// Overdue invoices count
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusOverdue).Count(&stats.OverdueInvoicesCount)

	// Recently created invoices (last 7 days)
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("created_at >= ?", sevenDaysAgo).Count(&stats.Last7DaysInvoices)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("created_at >= ?", sevenDaysAgo).Select("COALESCE(SUM(total), 0)").Scan(&stats.Last7DaysValue)

	// Average invoice value
	if stats.TotalInvoices > 0 {
		stats.AverageInvoiceValue = stats.TotalValue / float64(stats.TotalInvoices)
	}

	// Collection rate
	if stats.TotalValue > 0 {
		stats.CollectionRate = (stats.TotalPaid / stats.TotalValue) * 100
	}

	return &stats, nil
}

type KRADashboardStats struct {
	Total     int64 `json:"total"`
	Submitted int64 `json:"submitted"`
	Pending   int64 `json:"pending"`
	Failed    int64 `json:"failed"`
}

func (s *InvoiceService) GetKRADashboardStats(tenantID string) (*KRADashboardStats, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	var stats KRADashboardStats

	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Count(&stats.Total)

	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("kra_status = 'submitted'").
		Count(&stats.Submitted)

	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("kra_status = 'pending'").
		Count(&stats.Pending)

	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("kra_status = 'failed'").
		Count(&stats.Failed)

	return &stats, nil
}

// Helper functions for dashboard stats

func (s *InvoiceService) getMonthlyRevenue(tenantID string, months int) []MonthlyData {
	var data []MonthlyData
	now := time.Now()

	for i := months - 1; i >= 0; i-- {
		monthStart := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, now.Location())
		monthEnd := monthStart.AddDate(0, 1, 0)

		var total float64
		s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
			Where("status = ? AND paid_at >= ? AND paid_at < ?", models.InvoiceStatusPaid, monthStart, monthEnd).
			Select("COALESCE(SUM(total), 0)").
			Scan(&total)

		data = append(data, MonthlyData{
			Month:  monthStart.Format("2006-01"),
			Amount: total,
			Label:  monthStart.Format("Jan"),
		})
	}
	return data
}

func (s *InvoiceService) getMonthlyInvoices(tenantID string, months int) []MonthlyData {
	var data []MonthlyData
	now := time.Now()

	for i := months - 1; i >= 0; i-- {
		monthStart := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, now.Location())
		monthEnd := monthStart.AddDate(0, 1, 0)

		var count int64
		s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
			Where("created_at >= ? AND created_at < ?", monthStart, monthEnd).
			Count(&count)

		data = append(data, MonthlyData{
			Month:  monthStart.Format("2006-01"),
			Amount: float64(count),
			Label:  monthStart.Format("Jan"),
		})
	}
	return data
}

func (s *InvoiceService) getDailyRevenue(tenantID string, startDate time.Time, days int) []DailyData {
	var data []DailyData

	for i := 0; i < days; i++ {
		day := startDate.AddDate(0, 0, i)
		nextDay := day.AddDate(0, 0, 1)

		var total float64
		s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
			Where("status = ? AND paid_at >= ? AND paid_at < ?", models.InvoiceStatusPaid, day, nextDay).
			Select("COALESCE(SUM(total), 0)").
			Scan(&total)

		data = append(data, DailyData{
			Date:   day.Format("2006-01-02"),
			Amount: total,
			Label:  day.Format("Jan 02"),
		})
	}
	return data
}

func (s *InvoiceService) getDailyInvoices(tenantID string, startDate time.Time, days int) []DailyData {
	var data []DailyData

	for i := 0; i < days; i++ {
		day := startDate.AddDate(0, 0, i)
		nextDay := day.AddDate(0, 0, 1)

		var count int64
		s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
			Where("created_at >= ? AND created_at < ?", day, nextDay).
			Count(&count)

		data = append(data, DailyData{
			Date:   day.Format("2006-01-02"),
			Amount: float64(count),
			Label:  day.Format("Jan 02"),
		})
	}
	return data
}

func (s *InvoiceService) getTopPayingClients(tenantID string, limit int) []ClientRevenue {
	var results []ClientRevenue

	type clientTotals struct {
		ClientID     string
		ClientName   string
		TotalPaid    float64
		InvoiceCount int64
	}

	rows, err := s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Invoice{}).
		Select("client_id, client.name as client_name, COALESCE(SUM(total), 0) as total_paid, COUNT(*) as invoice_count").
		Joins("LEFT JOIN clients client ON client.id = invoices.client_id").
		Where("invoices.status = ?", models.InvoiceStatusPaid).
		Group("client_id").
		Order("total_paid DESC").
		Limit(limit).
		Rows()

	if err != nil {
		return results
	}
	defer rows.Close()

	for rows.Next() {
		var ct clientTotals
		if err := rows.Scan(&ct.ClientID, &ct.ClientName, &ct.TotalPaid, &ct.InvoiceCount); err == nil {
			results = append(results, ClientRevenue{
				ClientID:     ct.ClientID,
				ClientName:   ct.ClientName,
				TotalPaid:    ct.TotalPaid,
				InvoiceCount: ct.InvoiceCount,
			})
		}
	}
	return results
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
	// Get user's template (tenant-scoped)
	var template models.Template
	if err := s.db.Scopes(database.TenantFilter(invoice.TenantID)).
		First(&template, "is_default = ?", true).Error; err != nil {
		// Use default classic template
		template.HTML = getDefaultTemplate()
	}

	// Replace placeholders with actual data
	html := template.HTML
	html = strings.ReplaceAll(html, "{{.InvoiceNumber}}", invoice.InvoiceNumber)
	html = strings.ReplaceAll(html, "{{.Title}}", invoice.Title)
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

func generateCreditNoteNumber(userID string) string {
	timestamp := time.Now().UTC().Format("20060102")
	randBytes := make([]byte, 2)
	rand.Read(randBytes)
	return fmt.Sprintf("CN-%s-%s", timestamp, hex.EncodeToString(randBytes))
}

func generateDebitNoteNumber(userID string) string {
	timestamp := time.Now().UTC().Format("20060102")
	randBytes := make([]byte, 2)
	rand.Read(randBytes)
	return fmt.Sprintf("DN-%s-%s", timestamp, hex.EncodeToString(randBytes))
}

// CreateCreditNote creates a credit note from an original invoice
func (s *InvoiceService) CreateCreditNote(tenantID, userID, originalInvoiceID string, items []CreateCreditNoteItem) (*models.Invoice, error) {
	original, err := s.GetInvoiceByID(tenantID, originalInvoiceID)
	if err != nil {
		return nil, fmt.Errorf("original invoice not found: %w", err)
	}

	var creditItems []models.InvoiceItem
	var subtotal float64
	for i, item := range items {
		lineTotal := item.Quantity * item.UnitPrice
		subtotal += lineTotal
		creditItems = append(creditItems, models.InvoiceItem{
			ID:          uuid.New().String(),
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Unit:        item.Unit,
			Total:       lineTotal,
			SortOrder:   i,
		})
	}

	taxRate := original.TaxRate
	taxAmount := subtotal * (taxRate / 100)
	discount := math.Max(0, original.Discount)
	total := subtotal + taxAmount - discount

	// Validate buyer type inheritance - must match original for KRA compliance
	buyerType := original.BuyerClassification
	if buyerType == "" {
		buyerType = "B2C"
	}

	// Validate original was submitted to KRA
	if original.KRAICN == "" {
		return nil, fmt.Errorf("cannot create credit note: original invoice not submitted to KRA")
	}

	creditNote := &models.Invoice{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		UserID:            userID,
		ClientID:          original.ClientID,
		InvoiceNumber:     generateCreditNoteNumber(userID),
		Reference:         "Credit for " + original.InvoiceNumber,
		Currency:          original.Currency,
		InvoiceType:       "credit_note",
		OriginalInvoiceID: originalInvoiceID,
		OriginalICN:       original.KRAICN, // Link to original KRA ICN
		BuyerClassification: buyerType, // Inherit buyer type
		Subtotal:          math.Round(subtotal*100) / 100,
		TaxRate:           taxRate,
		TaxAmount:         math.Round(taxAmount*100) / 100,
		Discount:          math.Round(discount*100) / 100,
		Total:             math.Round((-total)*100) / 100,
		Status:            models.InvoiceStatusCreditNote,
		DueDate:           time.Now().AddDate(0, 0, 30),
		Notes:             "Credit note for: " + original.InvoiceNumber,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(creditNote).Error; err != nil {
			return fmt.Errorf("failed to create credit note: %w", err)
		}
		for i := range creditItems {
			creditItems[i].InvoiceID = creditNote.ID
		}
		if err := tx.Create(&creditItems).Error; err != nil {
			return fmt.Errorf("failed to create credit note items: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	creditNote.Items = creditItems
	return creditNote, nil
}

type CreateCreditNoteItem struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Unit        string  `json:"unit"`
}

type CreateDebitNoteItem struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Unit        string  `json:"unit"`
}

// CreateDebitNote creates a debit note from an original invoice
// Debit notes are used when additional charges need to be billed (e.g., extra services)
func (s *InvoiceService) CreateDebitNote(tenantID, userID, originalInvoiceID string, items []CreateDebitNoteItem) (*models.Invoice, error) {
	original, err := s.GetInvoiceByID(tenantID, originalInvoiceID)
	if err != nil {
		return nil, fmt.Errorf("original invoice not found: %w", err)
	}

	var debitItems []models.InvoiceItem
	var subtotal float64
	for i, item := range items {
		lineTotal := item.Quantity * item.UnitPrice
		subtotal += lineTotal
		debitItems = append(debitItems, models.InvoiceItem{
			ID:          uuid.New().String(),
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Unit:        item.Unit,
			Total:       lineTotal,
			SortOrder:   i,
		})
	}

	taxRate := original.TaxRate
	taxAmount := subtotal * (taxRate / 100)
	discount := math.Max(0, original.Discount)
	total := subtotal + taxAmount - discount

	// Validate buyer type inheritance - must match original for KRA compliance
	buyerType := original.BuyerClassification
	if buyerType == "" {
		buyerType = "B2C"
	}

	// Validate original was submitted to KRA
	if original.KRAICN == "" {
		return nil, fmt.Errorf("cannot create debit note: original invoice not submitted to KRA")
	}

	debitNote := &models.Invoice{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		UserID:            userID,
		ClientID:          original.ClientID,
		InvoiceNumber:     generateDebitNoteNumber(userID),
		Reference:         "Debit for " + original.InvoiceNumber,
		Currency:          original.Currency,
		InvoiceType:       "debit_note",
		OriginalInvoiceID: originalInvoiceID,
		OriginalICN:       original.KRAICN,
		BuyerClassification: buyerType,
		Subtotal:          math.Round(subtotal*100) / 100,
		TaxRate:           taxRate,
		TaxAmount:         math.Round(taxAmount*100) / 100,
		Discount:          math.Round(discount*100) / 100,
		Total:             math.Round(total*100) / 100,
		Status:            models.InvoiceStatusSent,
		DueDate:           time.Now().AddDate(0, 0, 30),
		Notes:             "Debit note for: " + original.InvoiceNumber,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(debitNote).Error; err != nil {
			return fmt.Errorf("failed to create debit note: %w", err)
		}
		for i := range debitItems {
			debitItems[i].InvoiceID = debitNote.ID
		}
		if err := tx.Create(&debitItems).Error; err != nil {
			return fmt.Errorf("failed to create debit note items: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	debitNote.Items = debitItems
	return debitNote, nil
}

// Request types
type CreateInvoiceRequest struct {
	ClientID          string               `json:"client_id" binding:"required"`
	Reference         string               `json:"reference"`
	Title             string               `json:"title"`
	Currency          string               `json:"currency"`
	TaxRate           float64              `json:"tax_rate"`
	Discount          float64              `json:"discount"`
	DueDate           time.Time            `json:"due_date"`
	Notes             string               `json:"notes"`
	Terms             string               `json:"terms"`
	BrandColor        string               `json:"brand_color"`
	LogoURL           string               `json:"logo_url"`
	BuyerType         string               `json:"buyer_type"` // B2B, B2C, B2E, EXPORT
	ExchangeRate      *float64             `json:"exchange_rate"`  // Manual override for exchange rate
	KESEquivalent     *float64             `json:"kes_equivalent"` // Manual override for KES equivalent
	Items            []InvoiceItemRequest `json:"items" binding:"required,min=1"`
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
	Status      *string    `json:"status"`
	DueDate     *time.Time `json:"due_date"`
	Reference   *string    `json:"reference"`
	Currency    *string    `json:"currency"`
	Title       *string    `json:"title"`
	TaxRate     *float64   `json:"tax_rate"`
	Discount   *float64   `json:"discount"`
	Notes       *string    `json:"notes"`
	Terms       *string    `json:"terms"`
	BrandColor  *string    `json:"brand_color"`
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

func internalInvoiceToKRA(invoice *models.Invoice) *kraInvoice {
	items := make([]kraInvoiceItem, len(invoice.Items))
	for i, item := range invoice.Items {
		items[i] = kraInvoiceItem{
			ID:          item.ID,
			Description: item.Description,
			Quantity:    item.Quantity,
			Unit:        item.Unit,
			UnitPrice:   item.UnitPrice,
			Total:       item.Total,
		}
	}
	return &kraInvoice{
		ID:            invoice.ID,
		InvoiceNumber: invoice.InvoiceNumber,
		Currency:      invoice.Currency,
		Subtotal:      invoice.Subtotal,
		TaxRate:       invoice.TaxRate,
		TaxAmount:     invoice.TaxAmount,
		Discount:      invoice.Discount,
		Total:         invoice.Total,
		PaidAmount:    invoice.PaidAmount,
		CreatedAt:     invoice.CreatedAt,
		DueDate:       invoice.DueDate,
		Status:        string(invoice.Status),
		Items:         items,
	}
}

func internalUserToKRA(user *models.User) *kraUser {
	return &kraUser{
		ID:          user.ID,
		Email:       user.Email,
		Phone:       user.Phone,
		CompanyName: user.CompanyName,
		KRAPIN:      user.KRAPIN,
	}
}

func internalClientToKRA(client *models.Client) *kraClient {
	return &kraClient{
		ID:      client.ID,
		Name:    client.Name,
		Email:   client.Email,
		Phone:   client.Phone,
		Address: client.Address,
		KRAPIN:  client.KRAPIN,
	}
}

// SubmitInvoiceToKRA manually submits an invoice to KRA eTIMS
// SubmitInvoiceToKRA submits an invoice to KRA with idempotency
func (s *InvoiceService) SubmitInvoiceToKRA(tenantID, invoiceID string) (*KRAResponse, error) {
	if s.kraService == nil {
		return nil, errors.New("KRA service not configured")
	}

	var result *KRAResponse

	// Use transaction for idempotent submission
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var invoice models.Invoice
		if err := tx.Scopes(database.TenantFilter(tenantID)).
			Preload("Items").
			First(&invoice, "id = ?", invoiceID).Error; err != nil {
			return ErrInvoiceNotFound
		}

		// PRE-KRA VALIDATION: Validate invoice before submission
		if err := s.validateInvoiceForKRA(&invoice); err != nil {
			return fmt.Errorf("invoice validation failed: %w", err)
		}

		// Idempotency check: if already submitted, return existing result
		if invoice.KRAStatus == models.KRAInvoiceStatusSubmitted || invoice.KRAStatus == models.KRAInvoiceStatusAccepted {
			if invoice.KRAICN != "" {
				return nil // Already submitted successfully
			}
		}

		// Check for pending idempotency key to prevent duplicate submissions
		if invoice.KRAIdempotencyKey != "" {
			// Check if we have a recent submission attempt
			if invoice.KRASubmittedAt != nil {
				timeSince := time.Since(*invoice.KRASubmittedAt)
				if timeSince < 5*time.Minute {
					// Still processing or failed, don't resubmit
					if invoice.KRAStatus == models.KRAInvoiceStatusFailed {
						return fmt.Errorf("KRA submission in progress or failed, please wait or retry later")
					}
					return nil // Already processing
				}
			}
		}

		// Generate idempotency key
		idempotencyKey := uuid.New().String()
		tx.Model(&models.Invoice{}).Where("id = ?", invoiceID).Update("kra_idempotency_key", idempotencyKey)

		// Get related data with error handling
		var cli models.Client
		if err := tx.Scopes(database.TenantFilter(tenantID)).First(&cli, "id = ?", invoice.ClientID).Error; err != nil {
			return fmt.Errorf("failed to fetch client: %w", err)
		}
		
		var usr models.User
		if err := tx.Scopes(database.TenantFilter(tenantID)).First(&usr, "id = ?", invoice.UserID).Error; err != nil {
			return fmt.Errorf("failed to fetch user: %w", err)
		}

		items := make([]KRAItem, 0)
		if err := tx.Model(&models.InvoiceItem{}).Where("invoice_id = ?", invoiceID).Find(&items).Error; err != nil {
			return fmt.Errorf("failed to fetch invoice items: %w", err)
		}

		// Determine buyer classification based on client
		buyerClassification := determineBuyerClassification(&cli)

		// Get seller address from user - use CompanyName as fallback
		sellerAddress := usr.CompanyName
		// Note: PhysicalAddress should be added to User model for full KRA compliance
		// For now using CompanyName as it typically contains business location

		// Determine invoice classification
		invoiceClassification := invoice.InvoiceClassification
		if invoiceClassification == "" {
			invoiceClassification = "normal"
		}

		// Determine payment mode from payments (simplified)
		paymentMode := "CASH"
		
		kraData := &KRAInvoiceData{
			InvoiceNumber: invoice.InvoiceNumber,
			InvoiceDate:   invoice.CreatedAt.Format("2006-01-02"),
			InvoiceTime:   invoice.CreatedAt.Format("15:04:05"),
			Seller: KRASeller{
				RegistrationNumber: usr.KRAPIN,
				BusinessName:       usr.CompanyName,
				Address:            sellerAddress,
				ContactMobile:      usr.Phone,
				ContactEmail:       usr.Email,
			},
			Buyer: KRABuyer{
				BuyerType:          buyerClassification,
				CustomerName:       cli.Name,
				Address:            cli.Address,
				ContactMobile:      cli.Phone,
				ContactEmail:       cli.Email,
				RegistrationNumber: cli.KRAPIN,
			},
			Items:              items,
			SubTotal:          invoice.Subtotal,
			TotalExcludingVAT: invoice.Subtotal - invoice.Discount,
			VATRate:           invoice.TaxRate,
			VATAmount:         invoice.TotalTax,
			TotalIncludingVAT: invoice.Total,
			Currency:          invoice.Currency,
			PaymentMode:       paymentMode,
		}

		kraResp, err := s.kraService.SubmitInvoice(kraData, invoice.TenantID, invoice.ID)
		if err != nil {
			tx.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
				"kra_status":      models.KRAInvoiceStatusFailed,
				"kra_error":      err.Error(),
				"kra_retry_count": gorm.Expr("kra_retry_count + 1"),
			})
			tx.Create(&models.AuditLog{
				ID:         uuid.New().String(),
				TenantID:   tenantID,
				UserID:     invoice.UserID,
				Action:     "kra_failed",
				EntityType: "invoice",
				EntityID:   invoiceID,
				Details:    fmt.Sprintf(`{"invoice_number": "%s", "error": "%s"}`, invoice.InvoiceNumber, err.Error()),
			})
			return err
		}

		now := time.Now()
		tx.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
			"kra_icn":              kraResp.ICN,
			"kra_qr_code":          kraResp.QRCode,
			"kra_status":           models.KRAInvoiceStatusSubmitted,
			"kra_submitted_at":     now,
			"kra_error":            "",
			"kra_idempotency_key":  "",
		})

		tx.Create(&models.AuditLog{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			UserID:     invoice.UserID,
			Action:     "kra_success",
			EntityType: "invoice",
			EntityID:   invoiceID,
			Details:    fmt.Sprintf(`{"invoice_number": "%s", "icn": "%s"}`, invoice.InvoiceNumber, kraResp.ICN),
		})

		log.Printf("[KRA] Invoice %s submitted - ICN: %s", invoice.InvoiceNumber, kraResp.ICN)

		result = kraResp
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// ClearKRAData clears KRA submission data for retry
func (s *InvoiceService) ClearKRAData(tenantID, invoiceID string) error {
	if tenantID == "" {
		return ErrTenantRequired
	}
	return s.db.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
		"kra_icn":          "",
		"kra_qr_code":      "",
		"kra_status":       "",
		"kra_submitted_at": nil,
		"kra_error":        "",
	}).Error
}

// KRAActivityEvent represents a KRA activity event
type KRAActivityEvent struct {
	ID            string    `json:"id"`
	InvoiceID     string    `json:"invoice_id"`
	InvoiceNumber string    `json:"invoice_number"`
	Action        string    `json:"action"` // submitted, failed, retried
	Status        string    `json:"status"`
	ICN           string    `json:"icn,omitempty"`
	Error         string    `json:"error,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// GetKRAActivityFeed returns recent KRA activity for the tenant
func (s *InvoiceService) GetKRAActivityFeed(tenantID string, limit int) ([]KRAActivityEvent, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var events []KRAActivityEvent

	// Get invoices with KRA activity in the last 24 hours
	s.db.Scopes(database.TenantFilter(tenantID)).
		Where("kra_status IN ('submitted', 'failed') OR kra_icn IS NOT NULL").
		Order("updated_at DESC").
		Limit(limit).
		Find(&events)

	// Build activity events from audit logs for more detail
	var auditLogs []models.AuditLog
	s.db.Scopes(database.TenantFilter(tenantID)).
		Where("action IN ('kra_success', 'kra_failed')").
		Order("created_at DESC").
		Limit(limit).
		Find(&auditLogs)

	// Merge audit logs with invoice data
	activityMap := make(map[string]*KRAActivityEvent)
	for _, inv := range events {
		ts := inv.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		activityMap[inv.InvoiceID] = &inv
	}

	// Add more recent events from audit logs
	for _, log := range auditLogs {
		if existing, ok := activityMap[log.EntityID]; ok {
			if existing.Error == "" {
				var details map[string]interface{}
				json.Unmarshal([]byte(log.Details), &details)
				if err, ok := details["error"].(string); ok && err != "" {
					existing.Error = err
				}
			}
			continue
		}
		// Create event from audit log
		var inv models.Invoice
		if err := s.db.First(&inv, "id = ?", log.EntityID).Error; err != nil {
			continue
		}
		action := "submitted"
		if log.Action == "kra_failed" {
			action = "failed"
		}
		var details map[string]interface{}
		json.Unmarshal([]byte(log.Details), &details)
		errMsg := ""
		if err, ok := details["error"].(string); ok {
			errMsg = err
		}
		events = append(events, KRAActivityEvent{
			ID:            log.ID,
			InvoiceID:     log.EntityID,
			InvoiceNumber: inv.InvoiceNumber,
			Action:        action,
			Status:        string(inv.KRAStatus),
			ICN:           inv.KRAICN,
			Error:         errMsg,
			Timestamp:     log.CreatedAt,
		})
	}

	// Sort by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	if len(events) > limit {
		events = events[:limit]
	}

	return events, nil
}

// SubmitAllPendingToKRA submits all pending invoices to KRA
func (s *InvoiceService) SubmitAllPendingToKRA(tenantID string) (submitted, failed int, err error) {
	if tenantID == "" {
		return 0, 0, ErrTenantRequired
	}

	var pendingInvoices []models.Invoice
	s.db.Scopes(database.TenantFilter(tenantID)).
		Where("(kra_status IS NULL OR kra_status = '') AND (kra_icn IS NULL OR kra_icn = '')").
		Find(&pendingInvoices)

	for _, inv := range pendingInvoices {
		_, err := s.SubmitInvoiceToKRA(tenantID, inv.ID)
		if err != nil {
			failed++
			log.Printf("[KRA] Failed to submit invoice %s: %v", inv.InvoiceNumber, err)
		} else {
			submitted++
		}
	}

	return submitted, failed, nil
}

// validateInvoiceForKRA validates invoice before KRA submission
func (s *InvoiceService) validateInvoiceForKRA(invoice *models.Invoice) error {
	// Check required fields
	if invoice.InvoiceNumber == "" {
		return errors.New("invoice number is required")
	}
	
	if invoice.Total <= 0 {
		return errors.New("invoice total must be greater than zero")
	}
	
	// Validate seller KRA PIN
	if invoice.UserID == "" {
		return errors.New("user ID is required")
	}
	
	// Validate client
	if invoice.ClientID == "" {
		return errors.New("client is required")
	}
	
	// Tax validation
	if invoice.TaxRate < 0 || invoice.TaxRate > 100 {
		return errors.New("invalid tax rate (must be 0-100)")
	}
	
	// For credit/debit notes, require original ICN
	if invoice.InvoiceType == "credit_note" || invoice.InvoiceType == "debit_note" {
		if invoice.OriginalICN == "" {
			return errors.New("credit/debit notes must reference original invoice ICN")
		}
	}
	
	return nil
}

// determineBuyerClassification determines KRA buyer classification based on client
func determineBuyerClassification(client *models.Client) string {
	if client == nil {
		return "B2C" // Default
	}
	
	// B2B if client has KRA PIN with valid format
	if client.KRAPIN != "" {
		// Validate PIN format: starts with A, ends with B
		if strings.HasPrefix(client.KRAPIN, "A") && strings.HasSuffix(client.KRAPIN, "B") {
			return "B2B"
		}
		// Invalid PIN format, treat as B2C
		return "B2C"
	}
	
	// Check for export indicators in email/address
	email := strings.ToLower(client.Email)
	address := strings.ToLower(client.Address)
	
	if strings.Contains(email, ".export") || 
	   strings.Contains(email, "abroad") ||
	   strings.Contains(address, "export") ||
	   strings.Contains(address, "duty free") {
		return "EXPORT"
	}
	
	// Default to B2C for consumers
	return "B2C"
}
