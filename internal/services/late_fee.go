package services

import (
	"errors"
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LateFeeService struct {
	db *database.DB
}

func NewLateFeeService(db *database.DB) *LateFeeService {
	return &LateFeeService{db: db}
}

func (s *LateFeeService) GetConfig(tenantID string) (*models.LateFeeConfig, error) {
	var config models.LateFeeConfig
	err := s.db.Where("tenant_id = ?", tenantID).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.createDefaultConfig(tenantID)
		}
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	return &config, nil
}

func (s *LateFeeService) createDefaultConfig(tenantID string) (*models.LateFeeConfig, error) {
	config := &models.LateFeeConfig{
		ID:              uuid.New().String(),
		TenantID:        tenantID,
		IsEnabled:       false,
		FeeType:         "percentage",
		FeeAmount:       5.0,
		GracePeriodDays: 0,
		MaxLateFees:     0,
		ApplyOnTax:      true,
	}
	if err := s.db.Create(config).Error; err != nil {
		return nil, fmt.Errorf("failed to create default config: %w", err)
	}
	return config, nil
}

func (s *LateFeeService) UpdateConfig(tenantID string, req *UpdateLateFeeConfigRequest) (*models.LateFeeConfig, error) {
	config, err := s.GetConfig(tenantID)
	if err != nil {
		return nil, err
	}

	if req.IsEnabled != nil {
		config.IsEnabled = *req.IsEnabled
	}
	if req.FeeType != nil {
		config.FeeType = *req.FeeType
	}
	if req.FeeAmount != nil {
		config.FeeAmount = *req.FeeAmount
	}
	if req.GracePeriodDays != nil {
		config.GracePeriodDays = *req.GracePeriodDays
	}
	if req.MaxLateFees != nil {
		config.MaxLateFees = *req.MaxLateFees
	}
	if req.ApplyOnTax != nil {
		config.ApplyOnTax = *req.ApplyOnTax
	}

	if err := s.db.Save(config).Error; err != nil {
		return nil, fmt.Errorf("failed to update config: %w", err)
	}

	return config, nil
}

func (s *LateFeeService) CalculateLateFee(invoiceID string) (float64, error) {
	var invoice models.Invoice
	if err := s.db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return 0, fmt.Errorf("invoice not found: %w", err)
	}

	config, err := s.GetConfig(invoice.TenantID)
	if err != nil {
		return 0, err
	}

	if !config.IsEnabled {
		return 0, nil
	}

	now := time.Now()
	graceEnd := invoice.DueDate.AddDate(0, 0, config.GracePeriodDays)
	if now.Before(graceEnd) {
		return 0, nil // Within grace period
	}

	daysOverdue := int(now.Sub(invoice.DueDate).Hours() / 24)
	if daysOverdue <= 0 {
		return 0, nil
	}

	baseAmount := invoice.Total
	if !config.ApplyOnTax {
		baseAmount = invoice.Subtotal
	}

	var fee float64
	if config.FeeType == "percentage" {
		fee = baseAmount * (config.FeeAmount / 100)
	} else {
		fee = config.FeeAmount * float64(daysOverdue)
	}

	if config.MaxLateFees > 0 && fee > config.MaxLateFees {
		fee = config.MaxLateFees
	}

	existingFee := s.getExistingLateFee(invoiceID)
	return fee - existingFee, nil
}

func (s *LateFeeService) getExistingLateFee(invoiceID string) float64 {
	var total float64
	s.db.Model(&models.LateFeeInvoice{}).
		Where("invoice_id = ? AND waived = ?", invoiceID, false).
		Select("COALESCE(SUM(fee_amount), 0)").
		Scan(&total)
	return total
}

func (s *LateFeeService) ApplyLateFee(invoiceID, reason string) (*models.LateFeeInvoice, error) {
	feeAmount, err := s.CalculateLateFee(invoiceID)
	if err != nil {
		return nil, err
	}

	if feeAmount <= 0 {
		return nil, nil
	}

	var invoice models.Invoice
	if err := s.db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return nil, fmt.Errorf("invoice not found: %w", err)
	}

	lateFee := &models.LateFeeInvoice{
		ID:        uuid.New().String(),
		TenantID:  invoice.TenantID,
		InvoiceID: invoiceID,
		FeeAmount: feeAmount,
		FeeType:   "automatic",
		Reason:    reason,
		AppliedAt: time.Now(),
	}

	if err := s.db.Create(lateFee).Error; err != nil {
		return nil, fmt.Errorf("failed to apply late fee: %w", err)
	}

	s.db.Model(&invoice).Update("total", invoice.Total+feeAmount)

	return lateFee, nil
}

func (s *LateFeeService) WaiveLateFee(lateFeeID, userID string) error {
	result := s.db.Model(&models.LateFeeInvoice{}).
		Where("id = ?", lateFeeID).
		Updates(map[string]interface{}{
			"waived":    true,
			"waived_at": time.Now(),
			"waived_by": userID,
		})
	if result.Error != nil {
		return fmt.Errorf("failed to waive late fee: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *LateFeeService) GetLateFeesForInvoice(invoiceID string) ([]models.LateFeeInvoice, error) {
	var fees []models.LateFeeInvoice
	err := s.db.Where("invoice_id = ?", invoiceID).Order("applied_at DESC").Find(&fees).Error
	return fees, err
}

func (s *LateFeeService) ProcessAllOverdue() error {
	var invoices []models.Invoice
	now := time.Now()

	if err := s.db.Where("status = ? AND due_date < ?", "overdue", now).Find(&invoices).Error; err != nil {
		return fmt.Errorf("failed to find overdue invoices: %w", err)
	}

	for _, invoice := range invoices {
		s.ApplyLateFee(invoice.ID, "Automatic late fee applied")
	}

	return nil
}

type UpdateLateFeeConfigRequest struct {
	IsEnabled       *bool    `json:"is_enabled"`
	FeeType         *string  `json:"fee_type"` // percentage, fixed
	FeeAmount       *float64 `json:"fee_amount"`
	GracePeriodDays *int     `json:"grace_period_days"`
	MaxLateFees     *float64 `json:"max_late_fees"`
	ApplyOnTax      *bool    `json:"apply_on_tax"`
}
