// DEPRECATED: This service manages the is_recurring flag on the Invoice model.
// The new AutoRecurringInvoiceService (in automation.go) uses a dedicated
// RecurringInvoice template model with job-queue scheduling and is the
// preferred system going forward.
//
// This service is kept for backward compatibility with existing invoices
// that have is_recurring=true. New development should use the automation
// sub-system at POST /api/v1/tenant/automations/recurring/.
package services

import (
	"context"
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RecurringInvoiceService struct {
	db             *database.DB
	invoiceService *InvoiceService
	emailService   *EmailService
}

func NewRecurringInvoiceService(db *database.DB, invoiceSvc *InvoiceService, emailSvc *EmailService) *RecurringInvoiceService {
	return &RecurringInvoiceService{
		db:             db,
		invoiceService: invoiceSvc,
		emailService:   emailSvc,
	}
}

// ProcessRecurringInvoices checks and processes due recurring invoices
func (s *RecurringInvoiceService) ProcessRecurringInvoices() error {
	logger.Get().Info(context.Background(), "Processing recurring invoices")

	var invoices []models.Invoice
	now := time.Now()

	if err := s.db.Where("is_recurring = ? AND recurring_next_date <= ? AND status != ?",
		true, now, models.InvoiceStatusCancelled).Find(&invoices).Error; err != nil {
		return fmt.Errorf("failed to find due recurring invoices: %w", err)
	}

	for _, parent := range invoices {
		if err := s.createRecurringInvoice(&parent); err != nil {
			logger.Get().Error(context.Background(), "Error creating recurring invoice from parent", "parent_id", parent.ID, "error", err)
			continue
		}
	}

	logger.Get().Info(context.Background(), "Processed recurring invoices", "count", len(invoices))
	return nil
}

func (s *RecurringInvoiceService) createRecurringInvoice(parent *models.Invoice) error {
	tenantID := parent.TenantID

	newInvoice := &models.Invoice{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		UserID:            parent.UserID,
		ClientID:          parent.ClientID,
		InvoiceNumber:     s.generateInvoiceNumber(tenantID),
		Currency:          parent.Currency,
		KESEquivalent:     parent.KESEquivalent,
		ExchangeRate:      parent.ExchangeRate,
		InvoiceType:       parent.InvoiceType,
		RecurringParentID: parent.ID,
		Subtotal:          parent.Subtotal,
		TaxRate:           parent.TaxRate,
		TaxAmount:         parent.TaxAmount,
		Discount:          parent.Discount,
		Total:             parent.Total,
		PaidAmount:        0,
		Status:            models.InvoiceStatusDraft,
		DueDate:           time.Now().AddDate(0, 1, 0),
		Notes:             parent.Notes,
		Terms:             parent.Terms,
		BrandColor:        parent.BrandColor,
		LogoURL:           parent.LogoURL,
	}

	items := []models.InvoiceItem{}
	s.db.Where("invoice_id = ?", parent.ID).Find(&items)
	newItems := make([]models.InvoiceItem, 0, len(items))
	for _, item := range items {
		newItem := models.InvoiceItem{
			ID:          uuid.New().String(),
			InvoiceID:   newInvoice.ID,
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Unit:        item.Unit,
			Total:       item.Total,
		}
		newItems = append(newItems, newItem)
	}
	newInvoice.Items = newItems

	if err := s.db.Create(newInvoice).Error; err != nil {
		return fmt.Errorf("failed to create recurring invoice: %w", err)
	}

	nextDate := s.calculateNextDate(parent.RecurringFrequency, parent.RecurringNextDate)
	if err := s.db.Model(parent).Updates(map[string]interface{}{
		"recurring_next_date": nextDate,
	}).Error; err != nil {
		logger.Get().Error(context.Background(), "Failed to update next date", "parent_id", parent.ID, "error", err)
	}

	logger.Get().Info(context.Background(), "Created recurring invoice from parent", "invoice_id", newInvoice.ID, "parent_id", parent.ID)
	return nil
}

func (s *RecurringInvoiceService) generateInvoiceNumber(tenantID string) string {
	year := time.Now().Year()
	month := time.Now().Month()

	var count int64
	s.db.Model(&models.Invoice{}).Where("tenant_id = ? AND created_at >= ?",
		tenantID, time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)).Count(&count)

	return fmt.Sprintf("INV-%d%02d-%04d", year, month, count+1)
}

func (s *RecurringInvoiceService) calculateNextDate(frequency string, lastDate time.Time) time.Time {
	switch frequency {
	case "daily":
		return lastDate.AddDate(0, 0, 1)
	case "weekly":
		return lastDate.AddDate(0, 0, 7)
	case "monthly":
		return lastDate.AddDate(0, 1, 0)
	case "quarterly":
		return lastDate.AddDate(0, 3, 0)
	case "yearly":
		return lastDate.AddDate(1, 0, 0)
	default:
		return lastDate.AddDate(0, 1, 0)
	}
}

// EnableRecurring enables recurring on an invoice
func (s *RecurringInvoiceService) EnableRecurring(tenantID, invoiceID, frequency string) error {
	var invoice models.Invoice
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return fmt.Errorf("invoice not found: %w", err)
	}

	nextDate := s.calculateNextDate(frequency, time.Now())

	updates := map[string]interface{}{
		"is_recurring":        true,
		"recurring_frequency": frequency,
		"recurring_next_date": nextDate,
	}

	if err := s.db.Model(&invoice).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to enable recurring: %w", err)
	}

	return nil
}

// DisableRecurring disables recurring on an invoice
func (s *RecurringInvoiceService) DisableRecurring(tenantID, invoiceID string) error {
	result := s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
		"is_recurring":        false,
		"recurring_frequency": nil,
		"recurring_next_date": nil,
	})

	if result.Error != nil {
		return fmt.Errorf("failed to disable recurring: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// GetRecurringInvoices returns all active recurring invoices for a tenant
func (s *RecurringInvoiceService) GetRecurringInvoices(tenantID string) ([]models.Invoice, error) {
	var invoices []models.Invoice
	err := s.db.Where("tenant_id = ? AND is_recurring = ?", tenantID, true).
		Order("recurring_next_date ASC").Find(&invoices).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get recurring invoices: %w", err)
	}
	return invoices, nil
}
