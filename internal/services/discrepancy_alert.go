package services

import (
	"fmt"
	"log"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

type DiscrepancyAlert struct {
	ID             string     `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID       string     `json:"tenant_id" gorm:"type:uuid;index"`
	PaymentID      string     `json:"payment_id" gorm:"type:uuid;index"`
	InvoiceID      string     `json:"invoice_id" gorm:"type:uuid;index"`
	ExpectedAmount float64    `json:"expected_amount"`
	ActualAmount   float64    `json:"actual_amount"`
	Discrepancy    float64    `json:"discrepancy"`
	Status         string     `json:"status"` // detected, resolved, ignored
	Resolution     string     `json:"resolution"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	ResolvedBy     string     `json:"resolved_by"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (DiscrepancyAlert) TableName() string {
	return "discrepancy_alerts"
}

type PaymentDiscrepancyService struct {
	db           *database.DB
	emailService *EmailService
}

func NewPaymentDiscrepancyService(db *database.DB, email *EmailService) *PaymentDiscrepancyService {
	return &PaymentDiscrepancyService{
		db:           db,
		emailService: email,
	}
}

func (s *PaymentDiscrepancyService) CheckAndCreateAlerts() error {
	var payments []models.Payment
	now := time.Now()
	s.db.Where("status = ? AND created_at > ?", "completed", now.AddDate(0, 0, -7)).
		Find(&payments)

	for i := range payments {
		payment := &payments[i]
		var invoice models.Invoice
		if err := s.db.First(&invoice, "id = ?", payment.InvoiceID).Error; err != nil {
			continue
		}

		expectedAmount := invoice.Total - invoice.PaidAmount + payment.Amount
		discrepancy := payment.Amount - expectedAmount
		if discrepancy < 0 {
			discrepancy = -discrepancy
		}

		threshold := invoice.Total * 0.01
		if discrepancy > threshold && discrepancy > 10 {
			s.createAlert(payment, &invoice, expectedAmount, discrepancy)
		}
	}

	return nil
}

func (s *PaymentDiscrepancyService) createAlert(payment *models.Payment, invoice *models.Invoice, expected, discrepancy float64) {
	var existing DiscrepancyAlert
	err := s.db.Where("payment_id = ? AND status = ?", payment.ID, "detected").First(&existing).Error
	if err == nil {
		return
	}

	alert := DiscrepancyAlert{
		ID:             fmt.Sprintf("alert-%s", payment.ID[:8]),
		TenantID:       payment.TenantID,
		PaymentID:      payment.ID,
		InvoiceID:      invoice.ID,
		ExpectedAmount: expected,
		ActualAmount:   payment.Amount,
		Discrepancy:    discrepancy,
		Status:         "detected",
		CreatedAt:      time.Now(),
	}

	s.db.Create(&alert)
	log.Printf("Payment discrepancy detected: Payment %s has %.2f difference from expected %.2f",
		payment.Reference, discrepancy, expected)
}

func (s *PaymentDiscrepancyService) GetAlerts(tenantID string) ([]DiscrepancyAlert, error) {
	var alerts []DiscrepancyAlert
	err := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&alerts).Error
	return alerts, err
}

func (s *PaymentDiscrepancyService) ResolveAlert(alertID, resolution, userID string) error {
	return s.db.Model(&DiscrepancyAlert{}).
		Where("id = ?", alertID).
		Updates(map[string]interface{}{
			"status":      "resolved",
			"resolution":  resolution,
			"resolved_at": time.Now(),
			"resolved_by": userID,
		}).Error
}

func (s *PaymentDiscrepancyService) IgnoreAlert(alertID, userID string) error {
	return s.db.Model(&DiscrepancyAlert{}).
		Where("id = ?", alertID).
		Updates(map[string]interface{}{
			"status":      "ignored",
			"resolved_at": time.Now(),
			"resolved_by": userID,
		}).Error
}
