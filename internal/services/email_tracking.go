package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EmailTrackingService struct {
	db *database.DB
}

func NewEmailTrackingService(db *database.DB) *EmailTrackingService {
	return &EmailTrackingService{db: db}
}

func (s *EmailTrackingService) CreateTracking(tenantID, invoiceID, recipient, subject, emailType string) string {
	trackingID := uuid.New().String()

	tracking := &models.EmailTracking{
		ID:        trackingID,
		TenantID:  tenantID,
		InvoiceID: invoiceID,
		Recipient: recipient,
		Subject:   subject,
		EmailType: emailType,
		SentAt:    time.Now(),
	}

	s.db.Create(tracking)
	return trackingID
}

func (s *EmailTrackingService) GetTrackingPixelURL(trackingID string) string {
	return fmt.Sprintf("/api/track/open/%s.png", trackingID)
}

func (s *EmailTrackingService) GetClickURL(trackingID, originalURL string) string {
	hash := sha256.Sum256([]byte(trackingID + originalURL))
	linkID := hex.EncodeToString(hash[:8])
	return fmt.Sprintf("/api/track/click/%s/%s", linkID, trackingID)
}

func (s *EmailTrackingService) TrackOpen(trackingID string, userAgent string, ipAddress string) error {
	trackingID = trackingID + ".png" // Handle .png suffix

	var tracking models.EmailTracking
	if err := s.db.First(&tracking, "id = ?", trackingID).Error; err != nil {
		return err
	}

	now := time.Now()
	updates := map[string]interface{}{
		"open_count": gorm.Expr("open_count + 1"),
	}

	if !tracking.OpenedAt.Valid {
		updates["opened_at"] = now
	}

	if userAgent != "" {
		updates["user_agent"] = userAgent
	}
	if ipAddress != "" {
		updates["ip_address"] = ipAddress
	}

	return s.db.Model(&tracking).Updates(updates).Error
}

func (s *EmailTrackingService) TrackClick(linkID, trackingID string) (string, error) {
	var link models.EmailTrackingLink
	if err := s.db.First(&link, "id = ?", linkID).Error; err != nil {
		return "", err
	}

	s.db.Model(&link).Update("redirect_count", gorm.Expr("redirect_count + 1"))
	s.db.Model(&models.EmailTracking{}).Where("id = ?", link.TrackingID).
		Update("click_count", gorm.Expr("click_count + 1"))

	return link.OriginalURL, nil
}

func (s *EmailTrackingService) GetStats(tenantID string) (map[string]interface{}, error) {
	var totalSent, totalOpened, totalClicked int64

	s.db.Model(&models.EmailTracking{}).Where("tenant_id = ?", tenantID).Count(&totalSent)
	s.db.Model(&models.EmailTracking{}).Where("tenant_id = ? AND opened_at IS NOT NULL", tenantID).Count(&totalOpened)
	s.db.Model(&models.EmailTracking{}).Where("tenant_id = ? AND click_count > 0", tenantID).Count(&totalClicked)

	openRate := 0.0
	if totalSent > 0 {
		openRate = float64(totalOpened) / float64(totalSent) * 100
	}
	clickRate := 0.0
	if totalOpened > 0 {
		clickRate = float64(totalClicked) / float64(totalOpened) * 100
	}

	return map[string]interface{}{
		"total_sent":    totalSent,
		"total_opened":  totalOpened,
		"total_clicked": totalClicked,
		"open_rate":     openRate,
		"click_rate":    clickRate,
	}, nil
}

func (s *EmailTrackingService) GetEmailStatsByInvoice(invoiceID string) ([]models.EmailTracking, error) {
	var tracking []models.EmailTracking
	err := s.db.Where("invoice_id = ?", invoiceID).Order("sent_at DESC").Find(&tracking).Error
	return tracking, err
}
