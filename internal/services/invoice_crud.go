package services

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/metrics"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreateInvoice creates a new invoice with kraPayloadItems (tenant-scoped)
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

	// Validate kraPayloadItems
	if len(req.Items) == 0 {
		return nil, ErrEmptyItems
	}

	// Calculate totals
	var totalPreTax float64
	var totalItemTax float64
	var invoiceTaxAmount float64
	var kraPayloadItems []models.InvoiceItem
	for i, item := range req.Items {
		if item.Quantity < 0 {
			return nil, ErrInvalidQuantity
		}
		if item.UnitPrice < 0 {
			item.UnitPrice = 0
		}
		if strings.TrimSpace(item.Description) == "" {
			item.Description = "Item"
		}

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

		lineSubtotal := item.Quantity * item.UnitPrice
		itemDiscountAmt := lineSubtotal * (itemDiscountRate / 100)
		lineSubtotalAfterDiscount := lineSubtotal - itemDiscountAmt
		itemTaxAmt := lineSubtotalAfterDiscount * (itemTaxRate / 100)
		lineTotal := lineSubtotalAfterDiscount + itemTaxAmt

		totalPreTax += lineSubtotal
		totalItemTax += itemTaxAmt
		kraPayloadItems = append(kraPayloadItems, models.InvoiceItem{
			ID:           uuid.New().String(),
			Description:  strings.TrimSpace(item.Description),
			Quantity:     item.Quantity,
			UnitPrice:    models.ToCents(item.UnitPrice),
			Subtotal:     models.ToCents(lineSubtotal),
			Unit:         item.Unit,
			TaxRate:      itemTaxRate,
			TaxAmount:    models.ToCents(itemTaxAmt),
			DiscountRate: itemDiscountRate,
			DiscountAmt:  models.ToCents(itemDiscountAmt),
			Total:        models.ToCents(lineTotal),
			SortOrder:    i,
		})
	}

	currency := strings.ToUpper(req.Currency)
	if currency == "" {
		currency = s.getTenantCurrency(tenantID)
	}
	if !validCurrencies[currency] {
		return nil, fmt.Errorf("unsupported currency: %s", currency)
	}

	taxRate := math.Max(0, math.Min(100, req.TaxRate))
	discount := math.Max(0, req.Discount)
	subtotal := totalPreTax

	// Apply invoice-level tax rate to items without their own tax rate
	if taxRate > 0 {
		invoiceTaxAmount = (totalPreTax - totalItemTax) * (taxRate / 100)
	}
	totalTax := totalItemTax + invoiceTaxAmount
	total := subtotal + totalTax - discount

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
		KESEquivalent:     models.ToCents(kesEquivalent),
		ExchangeRate:      exchangeRate,
		ExchangeRateAt:    time.Now(),
		Subtotal:          models.ToCents(subtotal),
		TaxRate:           taxRate,
		TotalTax:          models.ToCents(totalTax),
		TaxAmount:         models.ToCents(totalTax),
		Discount:          models.ToCents(discount),
		Total:             models.ToCents(total),
		BalanceDue:        models.ToCents(total),
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

		// Add kraPayloadItems
		for i := range kraPayloadItems {
			kraPayloadItems[i].InvoiceID = invoice.ID
		}
		if err := tx.Create(&kraPayloadItems).Error; err != nil {
			return fmt.Errorf("failed to create invoice kraPayloadItems: %w", err)
		}

		// Validate totals before committing
		if err := models.ValidateInvoiceTotals(invoice, kraPayloadItems); err != nil {
			return fmt.Errorf("invoice totals validation failed: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	invoice.Items = kraPayloadItems
	invoice.Client = *client

	metrics.RecordInvoiceCreated()

	return invoice, nil
}

// validateCreateRequest validates the create invoice request
func (s *InvoiceService) validateCreateRequest(userID, clientID string, req *CreateInvoiceRequest) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("user ID is required")
	}

	// Validate due date is not in the past
	if !req.DueDate.IsZero() && req.DueDate.Before(time.Now().Truncate(24*time.Hour)) {
		return errors.New("due date cannot be in the past")
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
		invoice.Discount = models.ToCents(math.Max(0, *req.Discount))
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

// UpdateInvoiceItems updates invoice kraPayloadItems (tenant-scoped)
func (s *InvoiceService) UpdateInvoiceItems(tenantID, invoiceID string, kraPayloadItems []InvoiceItemRequest) (*models.Invoice, error) {
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

	// Validate kraPayloadItems
	if len(kraPayloadItems) == 0 {
		return nil, ErrEmptyItems
	}

	// Use transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Delete existing kraPayloadItems
		if err := tx.Where("invoice_id = ?", invoiceID).Delete(&models.InvoiceItem{}).Error; err != nil {
			return fmt.Errorf("failed to delete kraPayloadItems: %w", err)
		}

		// Create new kraPayloadItems
		var newItems []models.InvoiceItem
		var subtotal float64
		for i, item := range kraPayloadItems {
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
				UnitPrice:    models.ToCents(item.UnitPrice),
				Unit:         item.Unit,
				TaxRate:      itemTaxRate,
				TaxAmount:    models.ToCents(itemTaxAmt),
				DiscountRate: itemDiscountRate,
				DiscountAmt:  models.ToCents(itemDiscountAmt),
				Total:        models.ToCents(lineTotal),
				SortOrder:    i,
			})
		}

		if err := tx.Create(&newItems).Error; err != nil {
			return fmt.Errorf("failed to create kraPayloadItems: %w", err)
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

// recalculateInvoiceTotals recalculates invoice totals from line items
func (s *InvoiceService) recalculateInvoiceTotals(invoice *models.Invoice) {
	var subtotal, totalTax models.Money
	for _, item := range invoice.Items {
		subtotal += item.Total
		totalTax += item.TaxAmount
	}
	invoice.Subtotal = subtotal
	invoice.TotalTax = totalTax
	invoice.TaxAmount = invoice.TotalTax
	invoice.Total = subtotal.Subtract(invoice.Discount)

	if invoice.Total.LessThan(0) {
		invoice.Total = 0
	}
}
