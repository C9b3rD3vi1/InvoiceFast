package services

import (
	"database/sql"
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PaymentMatchingService struct {
	db *database.DB
}

func NewPaymentMatchingService(db *database.DB) *PaymentMatchingService {
	return &PaymentMatchingService{db: db}
}

func (s *PaymentMatchingService) CreateUnallocated(tenantID, reference, phone string, amount float64) (*models.UnallocatedPayment, error) {
	payment := &models.UnallocatedPayment{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		Amount:      amount,
		Currency:    "KES",
		Reference:   reference,
		PhoneNumber: phone,
		IsMatched:   false,
		CreatedAt:   time.Now(),
	}

	if err := s.db.Create(payment).Error; err != nil {
		return nil, fmt.Errorf("failed to create unallocated payment: %w", err)
	}

	return payment, nil
}

func (s *PaymentMatchingService) GetUnallocated(tenantID string) ([]models.UnallocatedPayment, error) {
	var payments []models.UnallocatedPayment
	err := s.db.Where("tenant_id = ? AND is_matched = ?", tenantID, false).
		Order("created_at DESC").
		Find(&payments).Error
	return payments, err
}

func (s *PaymentMatchingService) MatchPayment(paymentID, invoiceID, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var unallocated models.UnallocatedPayment
		if err := tx.First(&unallocated, "id = ? AND is_matched = ?", paymentID, false).Error; err != nil {
			return fmt.Errorf("unallocated payment not found: %w", err)
		}

		var invoice models.Invoice
		if err := tx.First(&invoice, "id = ? AND tenant_id = ?", invoiceID, unallocated.TenantID).Error; err != nil {
			return fmt.Errorf("invoice not found: %w", err)
		}

		payment := &models.Payment{
			ID:          uuid.New().String(),
			TenantID:    unallocated.TenantID,
			UserID:      userID,
			InvoiceID:   invoiceID,
			Amount:      unallocated.Amount,
			Currency:    unallocated.Currency,
			Method:      models.PaymentMethodMpesa,
			Status:      models.PaymentStatusCompleted,
			Reference:   unallocated.Reference,
			PhoneNumber: unallocated.PhoneNumber,
			CompletedAt: sql.NullTime{Time: time.Now(), Valid: true},
		}

		if err := tx.Create(payment).Error; err != nil {
			return fmt.Errorf("failed to create payment: %w", err)
		}

		invoice.PaidAmount += unallocated.Amount
		if invoice.PaidAmount >= invoice.Total {
			invoice.Status = models.InvoiceStatusPaid
			invoice.PaidAt = sql.NullTime{Time: time.Now(), Valid: true}
		} else {
			invoice.Status = models.InvoiceStatusPartiallyPaid
		}

		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		now := time.Now()
		if err := tx.Model(&unallocated).Updates(map[string]interface{}{
			"is_matched": true,
			"matched_at": now,
			"matched_by": userID,
		}).Error; err != nil {
			return fmt.Errorf("failed to update unallocated: %w", err)
		}

		return nil
	})
}

func (s *PaymentMatchingService) ManualMatch(tenantID, invoiceID, reference, phone string, amount float64, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var invoice models.Invoice
		if err := tx.First(&invoice, "id = ? AND tenant_id = ?", invoiceID, tenantID).Error; err != nil {
			return fmt.Errorf("invoice not found: %w", err)
		}

		payment := &models.Payment{
			ID:          uuid.New().String(),
			TenantID:    tenantID,
			UserID:      userID,
			InvoiceID:   invoiceID,
			Amount:      amount,
			Currency:    invoice.Currency,
			Method:      models.PaymentMethodMpesa,
			Status:      models.PaymentStatusCompleted,
			Reference:   reference,
			PhoneNumber: phone,
			CompletedAt: sql.NullTime{Time: time.Now(), Valid: true},
		}

		if err := tx.Create(payment).Error; err != nil {
			return fmt.Errorf("failed to create payment: %w", err)
		}

		invoice.PaidAmount += amount
		if invoice.PaidAmount >= invoice.Total {
			invoice.Status = models.InvoiceStatusPaid
			invoice.PaidAt = sql.NullTime{Time: time.Now(), Valid: true}
		} else {
			invoice.Status = models.InvoiceStatusPartiallyPaid
		}

		return tx.Save(&invoice).Error
	})
}

func (s *PaymentMatchingService) FindUnpaidByReference(tenantID, reference string) (*models.Invoice, error) {
	var payment models.Payment
	if err := s.db.Where("tenant_id = ? AND reference = ?", tenantID, reference).First(&payment).Error; err == nil {
		return nil, fmt.Errorf("payment already exists with reference %s", reference)
	}

	var invoices []models.Invoice
	s.db.Where("tenant_id = ? AND status NOT IN ? AND total > paid_amount",
		tenantID, []string{"paid", "cancelled"}).
		Find(&invoices)

	return nil, nil
}
