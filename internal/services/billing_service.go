package services

import (
	"invoicefast/internal/database"
	"invoicefast/internal/models"
	"time"

	"github.com/google/uuid"
)

type BillingService struct {
	db *database.DB
}

func NewBillingService(db *database.DB) *BillingService {
	return &BillingService{db: db}
}

func (s *BillingService) InitiateMpesaSubscription(tenantID, phone string, amount int64) (string, error) {
	var sub models.Subscription
	if err := s.db.Where("tenant_id = ?", tenantID).First(&sub).Error; err != nil {
		return "", err
	}

	tx := models.SubscriptionTransaction{
		ID:             uuid.New().String(),
		SubscriptionID: sub.ID,
		TenantID:       tenantID,
		Amount:         amount,
		Currency:       "KES",
		PaymentMethod:  "mpesa",
		Status:         "pending",
		Type:           "initial",
		CreatedAt:      time.Now(),
	}
	if err := s.db.Create(&tx).Error; err != nil {
		return "", err
	}

	return tx.ID, nil
}

func (s *BillingService) ProcessMpesaCallback(checkoutRequestID string, status string) error {
	var tx models.SubscriptionTransaction
	if err := s.db.Where("provider_reference = ?", checkoutRequestID).First(&tx).Error; err != nil {
		return err
	}

	if status == "success" {
		tx.Status = "completed"
		now := time.Now()
		tx.PaidAt = &now

		var sub models.Subscription
		if err := s.db.Where("id = ?", tx.SubscriptionID).First(&sub).Error; err == nil {
			sub.Status = "active"
			sub.LastPaymentAt = &now
			sub.RetryCount = 0
			renewsAt := time.Now().AddDate(0, 1, 0)
			sub.RenewsAt = &renewsAt
			s.db.Save(&sub)
		}
	} else {
		tx.Status = "failed"
		tx.FailureReason = status
	}

	return s.db.Save(&tx).Error
}

func (s *BillingService) GetBillingHistory(tenantID string, page, limit int) ([]models.SubscriptionTransaction, int64) {
	var txs []models.SubscriptionTransaction
	var count int64

	s.db.Model(&models.SubscriptionTransaction{}).Where("tenant_id = ?", tenantID).Count(&count)
	s.db.Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&txs)

	return txs, count
}

func (s *BillingService) GetSavedPaymentMethods(tenantID string) []models.SavedPaymentMethod {
	var methods []models.SavedPaymentMethod
	s.db.Where("tenant_id = ?", tenantID).Order("is_default DESC").Find(&methods)
	return methods
}

func (s *BillingService) DeletePaymentMethod(tenantID, methodID string) error {
	return s.db.Where("id = ? AND tenant_id = ?", methodID, tenantID).Delete(&models.SavedPaymentMethod{}).Error
}

func (s *BillingService) SetDefaultPaymentMethod(tenantID, methodID string) error {
	s.db.Model(&models.SavedPaymentMethod{}).Where("tenant_id = ?", tenantID).Update("is_default", false)
	return s.db.Model(&models.SavedPaymentMethod{}).Where("id = ?", methodID).Update("is_default", true).Error
}
