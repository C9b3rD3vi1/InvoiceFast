package services

import (
	"fmt"
	"log"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

type OverdueService struct {
	db *database.DB
}

func NewOverdueService(db *database.DB) *OverdueService {
	return &OverdueService{db: db}
}

func (s *OverdueService) MarkOverdueInvoices() error {
	log.Println("Checking for overdue invoices...")

	now := time.Now()
	result := s.db.Model(&models.Invoice{}).
		Where("status NOT IN ? AND due_date < ? AND paid_amount < total",
			[]string{"paid", "cancelled", "draft"}, now).
		Updates(map[string]interface{}{
			"status": "overdue",
		})

	if result.Error != nil {
		return fmt.Errorf("failed to mark overdue invoices: %w", result.Error)
	}

	log.Printf("Marked %d invoices as overdue", result.RowsAffected)
	return nil
}

func (s *OverdueService) GetOverdueInvoices(tenantID string) ([]models.Invoice, error) {
	var invoices []models.Invoice
	err := s.db.Where("tenant_id = ? AND status = ? AND due_date < ?",
		tenantID, "overdue", time.Now()).
		Order("due_date ASC").
		Find(&invoices).Error
	return invoices, err
}

func (s *OverdueService) GetOverdueStats(tenantID string) (map[string]interface{}, error) {
	var count int64
	var total float64

	s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status = ?", tenantID, "overdue").
		Count(&count)

	s.db.Model(&models.Invoice{}).
		Select("COALESCE(SUM(total - paid_amount), 0)").
		Where("tenant_id = ? AND status = ?", tenantID, "overdue").
		Scan(&total)

	return map[string]interface{}{
		"count":             count,
		"total_outstanding": total,
	}, nil
}
