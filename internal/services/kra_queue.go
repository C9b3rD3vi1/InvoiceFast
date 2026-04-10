package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
)

// KRAQueueService handles KRA submission with retry logic
type KRAQueueService struct {
	db     *database.DB
	cfg    *config.Config
	kraSvc *KRAService
}

// NewKRAQueueService creates a new KRA queue service
func NewKRAQueueService(db *database.DB, kraSvc *KRAService, cfg *config.Config) *KRAQueueService {
	return &KRAQueueService{
		db:     db,
		cfg:    cfg,
		kraSvc: kraSvc,
	}
}

// QueueInvoiceForKRASubmission adds an invoice to the KRA submission queue
func (s *KRAQueueService) QueueInvoiceForKRASubmission(invoice *models.Invoice) error {
	if s.kraSvc == nil {
		return fmt.Errorf("KRA service not configured")
	}

	// Check if already queued
	var existing models.KRAQueueItem
	err := s.db.Where("invoice_id = ? AND status = ?", invoice.ID, "pending").First(&existing).Error
	if err == nil {
		return fmt.Errorf("invoice already queued for KRA submission")
	}

	// Get invoice items
	var items []models.InvoiceItem
	s.db.Where("invoice_id = ?", invoice.ID).Find(&items)

	// Convert to KRAItem type
	kraItems := make([]KRAItem, len(items))
	for i, item := range items {
		kraItems[i] = KRAItem{
			ItemCode:        item.ID,
			ItemDescription: item.Description,
			Quantity:        item.Quantity,
			UnitOfMeasure:   item.Unit,
			UnitPrice:       item.UnitPrice,
			Total:           item.Total,
		}
	}

	// Get client and user
	var client models.Client
	var user models.User
	s.db.First(&client, "id = ?", invoice.ClientID)
	s.db.First(&user, "id = ?", invoice.UserID)

	// Build KRA data
	kraData := &KRAInvoiceData{
		InvoiceNumber: invoice.InvoiceNumber,
		InvoiceDate:   invoice.CreatedAt.Format("2006-01-02"),
		InvoiceTime:   invoice.CreatedAt.Format("15:04:05"),
		Seller: KRASeller{
			RegistrationNumber: user.KRAPIN,
			BusinessName:       user.CompanyName,
			ContactMobile:      user.Phone,
			ContactEmail:       user.Email,
		},
		Buyer: KRABuyer{
			CustomerName:       client.Name,
			ContactMobile:      client.Phone,
			ContactEmail:       client.Email,
			RegistrationNumber: client.KRAPIN,
		},
		Items:             kraItems,
		SubTotal:          invoice.Subtotal,
		TotalExcludingVAT: invoice.Subtotal - invoice.Discount,
		VATRate:           invoice.TaxRate,
		VATAmount:         invoice.TaxAmount,
		TotalIncludingVAT: invoice.Total,
		Currency:          invoice.Currency,
	}

	// Serialize payload
	payloadJSON, err := json.Marshal(kraData)
	if err != nil {
		return fmt.Errorf("failed to serialize KRA payload: %w", err)
	}

	// Create queue item - use pointer for time fields
	nextRetry := time.Now()

	queueItem := &models.KRAQueueItem{
		ID:            uuid.New().String(),
		TenantID:      invoice.TenantID,
		InvoiceID:     invoice.ID,
		InvoiceNumber: invoice.InvoiceNumber,
		Payload:       string(payloadJSON),
		RetryCount:    0,
		MaxRetries:    3,
		Status:        "pending",
		NextRetryAt:   &nextRetry,
	}

	if err := s.db.Create(queueItem).Error; err != nil {
		return fmt.Errorf("failed to queue invoice for KRA: %w", err)
	}

	log.Printf("[KRA] Queued invoice %s for submission (queue_id: %s)", invoice.InvoiceNumber, queueItem.ID)
	return nil
}

// ProcessQueue processes pending KRA submissions with retry logic
func (s *KRAQueueService) ProcessQueue() error {
	if s.kraSvc == nil {
		return nil // KRA not configured
	}

	var pendingItems []models.KRAQueueItem
	err := s.db.Where("status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", "pending", time.Now()).
		Order("created_at ASC").
		Limit(10).
		Find(&pendingItems).Error
	if err != nil {
		return fmt.Errorf("failed to fetch pending KRA items: %w", err)
	}

	for _, item := range pendingItems {
		s.processQueueItem(item)
	}

	return nil
}

// processQueueItem processes a single KRA queue item
func (s *KRAQueueService) processQueueItem(item models.KRAQueueItem) {
	log.Printf("[KRA] Processing queue item %s (attempt %d/%d)", item.ID, item.RetryCount+1, item.MaxRetries)

	// Parse payload
	var kraData KRAInvoiceData
	if err := json.Unmarshal([]byte(item.Payload), &kraData); err != nil {
		s.markFailed(&item, fmt.Sprintf("failed to parse payload: %v", err))
		return
	}

	// Attempt KRA submission
	kraResp, err := s.kraSvc.SubmitInvoice(&kraData)
	if err != nil {
		s.handleRetry(&item, err)
		return
	}

	// Success - update invoice with KRA response
	err = s.db.Model(&models.Invoice{}).Where("id = ?", item.InvoiceID).Updates(map[string]interface{}{
		"kra_icn":     kraResp.ICN,
		"kra_qr_code": kraResp.QRCode,
	}).Error
	if err != nil {
		log.Printf("[KRA] Warning: failed to update invoice %s with KRA ICN: %v", item.InvoiceNumber, err)
	}

	// Mark as completed
	s.markCompleted(&item, kraResp.ICN)
	log.Printf("[KRA] Invoice %s submitted successfully - ICN: %s", item.InvoiceNumber, kraResp.ICN)
}

// handleRetry handles retry logic with exponential backoff
func (s *KRAQueueService) handleRetry(item *models.KRAQueueItem, err error) {
	item.RetryCount++

	if item.RetryCount >= item.MaxRetries {
		s.markFailed(item, fmt.Sprintf("max retries exceeded: %v", err))
		return
	}

	// Exponential backoff: 5min, 15min, 45min
	backoff := []time.Duration{5, 15, 45}
	backoffIdx := item.RetryCount - 1
	if backoffIdx >= len(backoff) {
		backoffIdx = len(backoff) - 1
	}
	delay := backoff[backoffIdx] * time.Minute

	nextRetry := time.Now().Add(delay)
	item.NextRetryAt = &nextRetry
	item.LastError = fmt.Sprintf("attempt %d: %v", item.RetryCount, err)

	err = s.db.Save(item).Error
	if err != nil {
		log.Printf("[KRA] Failed to update retry count for %s: %v", item.ID, err)
	}

	log.Printf("[KRA] Item %s failed (attempt %d/%d), retry in %v: %v",
		item.ID, item.RetryCount, item.MaxRetries, delay, err)
}

// markCompleted marks a queue item as completed
func (s *KRAQueueService) markCompleted(item *models.KRAQueueItem, icn string) {
	now := time.Now()
	item.Status = "completed"
	item.CompletedAt = &now
	item.LastError = ""

	if err := s.db.Save(item).Error; err != nil {
		log.Printf("[KRA] Failed to mark item %s as completed: %v", item.ID, err)
	}
}

// markFailed marks a queue item as permanently failed
func (s *KRAQueueService) markFailed(item *models.KRAQueueItem, errMsg string) {
	item.Status = "failed"
	item.LastError = errMsg

	if err := s.db.Save(item).Error; err != nil {
		log.Printf("[KRA] Failed to mark item %s as failed: %v", item.ID, err)
	}

	log.Printf("[KRA] KRA submission permanently failed for invoice %s: %s", item.InvoiceNumber, errMsg)
}

// StartKRAWorker starts the background KRA queue processor
func (s *KRAQueueService) StartKRAWorker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	log.Println("[KRA] Starting KRA queue worker")

	for {
		select {
		case <-ctx.Done():
			log.Println("[KRA] Stopping KRA queue worker")
			return
		case <-ticker.C:
			if err := s.ProcessQueue(); err != nil {
				log.Printf("[KRA] Queue processing error: %v", err)
			}
		}
	}
}

// GetQueueStatus returns the status of the KRA queue
func (s *KRAQueueService) GetQueueStatus() (pending, failed, completed int64, err error) {
	s.db.Model(&models.KRAQueueItem{}).Where("status = ?", "pending").Count(&pending)
	s.db.Model(&models.KRAQueueItem{}).Where("status = ?", "failed").Count(&failed)
	s.db.Model(&models.KRAQueueItem{}).Where("status = ?", "completed").Count(&completed)
	return
}
