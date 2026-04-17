package services

import (
	"fmt"
	"log"
	"time"

	"invoicefast/internal/database"
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
	log.Println("Processing recurring invoices...")

	var invoices []models.Invoice
	now := time.Now()

	if err := s.db.Where("is_recurring = ? AND recurring_next_date <= ? AND status != ?",
		true, now, "cancelled").Find(&invoices).Error; err != nil {
		return fmt.Errorf("failed to find due recurring invoices: %w", err)
	}

	for _, parent := range invoices {
		if err := s.createRecurringInvoice(&parent); err != nil {
			log.Printf("Error creating recurring invoice from %s: %v", parent.ID, err)
			continue
		}
	}

	log.Printf("Processed %d recurring invoices", len(invoices))
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
		Status:            "draft",
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
		log.Printf("Failed to update next date for %s: %v", parent.ID, err)
	}

	log.Printf("Created recurring invoice %s from parent %s", newInvoice.ID, parent.ID)
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
func (s *RecurringInvoiceService) EnableRecurring(invoiceID, frequency string) error {
	var invoice models.Invoice
	if err := s.db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
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
func (s *RecurringInvoiceService) DisableRecurring(invoiceID string) error {
	result := s.db.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
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
