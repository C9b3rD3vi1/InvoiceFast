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

func (s *PaymentMatchingService) GetPayments(tenantID string, filter PaymentFilter) ([]map[string]interface{}, int64, error) {
	var payments []models.Payment
	var total int64

	query := s.db.Where("tenant_id = ?", tenantID)

	if filter.Search != "" {
		search := "%" + filter.Search + "%"
		query = query.Where("reference ILIKE ? OR phone_number ILIKE ? OR customer_email ILIKE ?", search, search, search)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Method != "" {
		query = query.Where("method = ?", filter.Method)
	}
	if filter.ClientID != "" {
		query = query.Where("client_id = ?", filter.ClientID)
	}
	if filter.InvoiceID != "" {
		query = query.Where("invoice_id = ?", filter.InvoiceID)
	}
	if filter.DateFrom != "" {
		query = query.Where("created_at >= ?", filter.DateFrom)
	}
	if filter.DateTo != "" {
		query = query.Where("created_at <= ?", filter.DateTo)
	}

	// Count total
	query.Model(&models.Payment{}).Count(&total)

	// Pagination
	offset := (filter.Page - 1) * filter.Limit
	if offset < 0 {
		offset = 0
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}

	query = query.Order("created_at DESC").Offset(offset).Limit(filter.Limit)

	if err := query.Find(&payments).Error; err != nil {
		return nil, 0, err
	}

	// Get invoice and client info for each payment
	result := make([]map[string]interface{}, len(payments))
	for i, p := range payments {
		var invoice models.Invoice
		var client models.Client

		s.db.First(&invoice, p.InvoiceID)
		s.db.First(&client, invoice.ClientID)

		reconciled := p.InvoiceID != "" && p.Status == models.PaymentStatusCompleted

		result[i] = map[string]interface{}{
			"id":             p.ID,
			"tenant_id":      p.TenantID,
			"invoice_id":     p.InvoiceID,
			"client_id":      invoice.ClientID,
			"client_name":    client.Name,
			"invoice_number": invoice.InvoiceNumber,
			"amount":         p.Amount,
			"fee":            0.0,
			"currency":       p.Currency,
			"method":         p.Method,
			"status":         p.Status,
			"reference":      p.Reference,
			"phone_number":   p.PhoneNumber,
			"created_at":     p.CreatedAt,
			"completed_at":   p.CompletedAt,
			"reconciled":     reconciled,
		}
	}

	return result, total, nil
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

func (s *PaymentMatchingService) GetPaymentByID(tenantID, paymentID string) (*models.Payment, error) {
	var payment models.Payment
	err := s.db.Where("id = ? AND tenant_id = ?", paymentID, tenantID).First(&payment).Error
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}
	return &payment, nil
}

func (s *PaymentMatchingService) GetClientByID(tenantID, clientID string) (*models.Client, error) {
	var client models.Client
	err := s.db.Where("id = ? AND tenant_id = ?", clientID, tenantID).First(&client).Error
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}
	return &client, nil
}

func (s *PaymentMatchingService) ReconcilePayment(tenantID, paymentID, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var payment models.Payment
		if err := tx.Where("id = ? AND tenant_id = ?", paymentID, tenantID).First(&payment).Error; err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		// Get the invoice
		var invoice models.Invoice
		if err := tx.Where("id = ? AND tenant_id = ?", payment.InvoiceID, tenantID).First(&invoice).Error; err != nil {
			return fmt.Errorf("invoice not found: %w", err)
		}

		// Update invoice paid amount and status
		newPaidAmount := invoice.PaidAmount + payment.Amount
		invoice.PaidAmount = newPaidAmount

		if newPaidAmount >= invoice.Total {
			invoice.Status = models.InvoiceStatusPaid
			invoice.PaidAt = sql.NullTime{Time: time.Now(), Valid: true}
		} else if newPaidAmount > 0 {
			invoice.Status = models.InvoiceStatusPartiallyPaid
		}

		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		// Mark payment as reconciled if it was unallocated
		if payment.InvoiceID == "" {
			var unallocated models.UnallocatedPayment
			if err := tx.Where("reference = ? AND tenant_id = ?", payment.Reference, tenantID).First(&unallocated).Error; err == nil {
				unallocated.IsMatched = true
				unallocated.MatchedAt = new(time.Time)
				*unallocated.MatchedAt = time.Now()
				unallocated.MatchedBy = userID
				tx.Save(&unallocated)
			}
		}

		return nil
	})
}
