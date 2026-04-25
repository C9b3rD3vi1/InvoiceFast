package services

import (
	"encoding/json"
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
)

// ============================================================================
// JOB QUEUE SERVICE - Persistent, DB-backed job processing
// ============================================================================

// JobQueueService handles persistent job queue operations
type JobQueueService struct {
	db *database.DB
}

// NewJobQueueService creates a new job queue service
func NewJobQueueService(db *database.DB) *JobQueueService {
	return &JobQueueService{db: db}
}

// EnqueueJob adds a job to the queue (DB-backed, survives restarts)
func (s *JobQueueService) EnqueueJob(job *models.AutomationJob) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	if job.Status == "" {
		job.Status = models.JobStatusPending
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = time.Now()
	}
	return s.db.Create(job).Error
}

// EnqueueJobWithIdempotency ensures no duplicate jobs (idempotent enqueue)
func (s *JobQueueService) EnqueueJobWithIdempotency(key string, job *models.AutomationJob) error {
	if key == "" {
		return fmt.Errorf("idempotency key required")
	}
	
	// Check for existing job with same idempotency key
	var existing models.AutomationJob
	err := s.db.Where("idempotency_key = ? AND status NOT IN ?", key, []string{models.JobStatusFailed, models.JobStatusDeadLetter}).First(&existing).Error
	if err == nil {
		return nil // Job already exists, skip
	}
	
	// Create new job
	return s.EnqueueJob(job)
}

// GetPendingJobs retrieves jobs ready to be processed
func (s *JobQueueService) GetPendingJobs(limit int) ([]models.AutomationJob, error) {
	var jobs []models.AutomationJob
	now := time.Now()
	err := s.db.Where("status = ? AND run_at <= ?", models.JobStatusPending, now).
		Order("priority DESC, run_at ASC").
		Limit(limit).
		Find(&jobs).Error
	return jobs, err
}

// GetJobsByTenant retrieves jobs for a specific tenant
func (s *JobQueueService) GetJobsByTenant(tenantID string, status string, limit, offset int) ([]models.AutomationJob, int64, error) {
	var jobs []models.AutomationJob
	var total int64
	
	query := s.db.Model(&models.AutomationJob{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	
	query.Count(&total)
	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&jobs).Error
	return jobs, total, err
}

// ClaimJob marks a job as processing (prevents double execution)
func (s *JobQueueService) ClaimJob(jobID string) error {
	now := time.Now()
	result := s.db.Model(&models.AutomationJob{}).
		Where("id = ? AND status = ?", jobID, models.JobStatusPending).
		Updates(map[string]interface{}{
			"status":      models.JobStatusProcessing,
			"started_at":  now,
			"updated_at":  now,
		})
	if result.RowsAffected == 0 {
		return fmt.Errorf("job already claimed or not found")
	}
	return result.Error
}

// CompleteJob marks a job as completed
func (s *JobQueueService) CompleteJob(jobID string, result string) error {
	now := time.Now()
	metadata, _ := json.Marshal(map[string]interface{}{"result": result})
	return s.db.Model(&models.AutomationJob{}).
		Where("id = ?", jobID).
		Updates(map[string]interface{}{
			"status":       models.JobStatusCompleted,
			"completed_at": now,
			"metadata":    string(metadata),
			"updated_at":  now,
		}).Error
}

// FailJob handles job failure with retry logic
func (s *JobQueueService) FailJob(jobID string, errMsg string) error {
	var job models.AutomationJob
	if err := s.db.Where("id = ?", jobID).First(&job).Error; err != nil {
		return err
	}
	
	job.RetryCount++
	job.LastError = errMsg
	job.UpdatedAt = time.Now()
	
	// Check if can retry
	if job.RetryCount < job.MaxRetries {
		// Exponential backoff: 1min, 5min, 15min, 1hr
		backoffMinutes := []int{1, 5, 15, 60}
		idx := job.RetryCount - 1
		if idx >= len(backoffMinutes) {
			idx = len(backoffMinutes) - 1
		}
		nextRetry := time.Now().Add(time.Duration(backoffMinutes[idx]) * time.Minute)
		job.NextRetryAt = &nextRetry
		job.Status = models.JobStatusPending
		job.RunAt = nextRetry
		
		if err := s.db.Save(&job).Error; err != nil {
			return err
		}
		
		return fmt.Errorf("job failed, scheduled for retry")
	}
	
	// Move to dead letter queue
	job.Status = models.JobStatusDeadLetter
	return s.db.Save(&job).Error
}

// MoveToDeadLetter marks a job as permanently failed
func (s *JobQueueService) MoveToDeadLetter(jobID string, reason string) error {
	return s.db.Model(&models.AutomationJob{}).
		Where("id = ?", jobID).
		Updates(map[string]interface{}{
			"status":     models.JobStatusDeadLetter,
			"last_error": reason,
			"updated_at": time.Now(),
		}).Error
}

// RetryDeadLetter retries a dead letter job
func (s *JobQueueService) RetryDeadLetter(jobID string) error {
	job, err := s.GetJob(jobID)
	if err != nil {
		return err
	}
	
	job.Status = models.JobStatusPending
	job.RetryCount = 0
	job.MaxRetries = 3
	job.RunAt = time.Now().Add(1 * time.Minute)
	return s.db.Save(job).Error
}

// GetJob retrieves a single job
func (s *JobQueueService) GetJob(id string) (*models.AutomationJob, error) {
	var job models.AutomationJob
	if err := s.db.Where("id = ?", id).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// GetJobStats returns job statistics
func (s *JobQueueService) GetJobStats(tenantID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	type StatusCount struct {
		Status string
		Count  int64
	}
	var counts []StatusCount
	s.db.Model(&models.AutomationJob{}).Where("tenant_id = ?", tenantID).
		Select("status, COUNT(*) as count").
		Group("status").Scan(&counts)
	
	statusMap := make(map[string]int64)
	for _, c := range counts {
		statusMap[c.Status] = c.Count
	}
	
	stats["pending"] = statusMap["pending"]
	stats["processing"] = statusMap["processing"]
	stats["completed"] = statusMap["completed"]
	stats["failed"] = statusMap["failed"]
	stats["dead_letter"] = statusMap["dead_letter"]
	stats["total"] = statusMap["pending"] + statusMap["processing"] + statusMap["completed"] + statusMap["failed"] + statusMap["dead_letter"]
	
	return stats, nil
}

// ============================================================================
// RECURRING INVOICE SERVICE - Handles recurring invoice automation
// ============================================================================

var autoInvoiceCounter int64

func autoGenerateInvoiceNumber() string {
	autoInvoiceCounter++
	timestamp := time.Now().UnixNano() / 1000000
	return fmt.Sprintf("INV-%d-%d", timestamp, autoInvoiceCounter)
}

func getFloat(m map[string]interface{}, key string, def float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	if v, ok := m[key].(int); ok {
		return float64(v)
	}
	return def
}

func getString(m map[string]interface{}, key string, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

// AutoRecurringInvoiceService handles recurring invoice automation
type AutoRecurringInvoiceService struct {
	db        *database.DB
	jobQueue *JobQueueService
}

// NewAutoRecurringInvoiceService creates a new recurring invoice service
func NewAutoRecurringInvoiceService(db *database.DB, jobQueue *JobQueueService) *AutoRecurringInvoiceService {
	return &AutoRecurringInvoiceService{
		db:        db,
		jobQueue: jobQueue,
	}
}

// CreateRecurringInvoiceRequest defines request for creating recurring invoice
type CreateRecurringInvoiceRequest struct {
	Name           string                  `json:"name"`
	Description   string                  `json:"description"`
	ClientID      string                  `json:"client_id"`
	Frequency     string                  `json:"frequency"` // daily, weekly, monthly, custom
	IntervalDays  int                     `json:"interval_days"`
	StartDate     time.Time                `json:"start_date"`
	EndDate       *time.Time              `json:"end_date"`
	MaxCycles     *int                   `json:"max_cycles"`
	InvoiceTemplate map[string]interface{}    `json:"invoice_template"`
	AutoSend      bool                    `json:"auto_send"`
	AutoSubmitKRA bool                    `json:"auto_submit_kra"`
}

// GetRecurringInvoices returns all recurring invoices for a tenant
func (s *AutoRecurringInvoiceService) GetRecurringInvoices(tenantID, status string) ([]models.RecurringInvoice, error) {
	var recurring []models.RecurringInvoice
	query := s.db.Where("tenant_id = ?", tenantID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Order("created_at DESC").Find(&recurring).Error
	return recurring, err
}

// GetRecurringInvoice returns a single recurring invoice
func (s *AutoRecurringInvoiceService) GetRecurringInvoice(tenantID, id string) (*models.RecurringInvoice, error) {
	var recurring models.RecurringInvoice
	err := s.db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&recurring).Error
	return &recurring, err
}

// CreateRecurringInvoice creates a new recurring invoice template
func (s *AutoRecurringInvoiceService) CreateRecurringInvoice(tenantID, userID string, req *CreateRecurringInvoiceRequest) (*models.RecurringInvoice, error) {
	if req.ClientID == "" {
		return nil, fmt.Errorf("client is required")
	}
	
	// Validate client exists
	var client models.Client
	if err := s.db.Where("id = ? AND tenant_id = ?", req.ClientID, tenantID).First(&client).Error; err != nil {
		return nil, fmt.Errorf("client not found")
	}
	
	// Calculate next run date
	nextRun := calculateNextRunDate(req.StartDate, req.Frequency, req.IntervalDays)
	
	templateJSON, _ := json.Marshal(req.InvoiceTemplate)
	
	recurring := &models.RecurringInvoice{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		UserID:           userID,
		ClientID:         req.ClientID,
		Name:            req.Name,
		Description:     req.Description,
		Frequency:       req.Frequency,
		IntervalDays:    req.IntervalDays,
		StartDate:       req.StartDate,
		EndDate:        req.EndDate,
		NextRunDate:     nextRun,
		MaxCycles:       req.MaxCycles,
		CurrentCycle:    0,
		InvoiceTemplate: string(templateJSON),
		AutoSend:        req.AutoSend,
		AutoSubmitKRA:   req.AutoSubmitKRA,
		IsActive:        true,
		Status:         "active",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	
	if err := s.db.Create(recurring).Error; err != nil {
		return nil, err
	}
	
	// Schedule first job
	s.ScheduleJob(recurring)
	
	return recurring, nil
}

// PauseRecurringInvoice pauses a recurring invoice
func (s *AutoRecurringInvoiceService) PauseRecurringInvoice(tenantID, id string) error {
	recurring, err := s.GetRecurringInvoice(tenantID, id)
	if err != nil {
		return err
	}
	
	now := time.Now()
	recurring.Status = "paused"
	recurring.IsActive = false
	recurring.PausedAt = &now
	recurring.UpdatedAt = now
	
	return s.db.Save(recurring).Error
}

// ResumeRecurringInvoice resumes a paused recurring invoice
func (s *AutoRecurringInvoiceService) ResumeRecurringInvoice(tenantID, id string) error {
	recurring, err := s.GetRecurringInvoice(tenantID, id)
	if err != nil {
		return err
	}
	
	recurring.Status = "active"
	recurring.IsActive = true
	recurring.PausedAt = nil
	recurring.NextRunDate = calculateNextRunDate(recurring.StartDate, recurring.Frequency, recurring.IntervalDays)
	recurring.UpdatedAt = time.Now()
	
	if err := s.db.Save(recurring).Error; err != nil {
		return err
	}
	
	return s.ScheduleJob(recurring)
}

// DeleteRecurringInvoice deletes a recurring invoice
func (s *AutoRecurringInvoiceService) DeleteRecurringInvoice(tenantID, id string) error {
	recurring, err := s.GetRecurringInvoice(tenantID, id)
	if err != nil {
		return err
	}
	
	s.db.Where("automation_id = ? AND status = ?", id, models.JobStatusPending).
		Delete(&models.AutomationJob{})
	
	return s.db.Delete(recurring).Error
}

// ScheduleJob schedules a job for recurring invoice execution
func (s *AutoRecurringInvoiceService) ScheduleJob(recurring *models.RecurringInvoice) error {
	if s.jobQueue == nil {
		return nil
	}
	
	payload, _ := json.Marshal(map[string]interface{}{
		"recurring_id":  recurring.ID,
		"tenant_id":   recurring.TenantID,
		"client_id":   recurring.ClientID,
		"template":    recurring.InvoiceTemplate,
		"auto_send":   recurring.AutoSend,
		"auto_submit_kra": recurring.AutoSubmitKRA,
	})
	
	job := &models.AutomationJob{
		ID:            uuid.New().String(),
		TenantID:       recurring.TenantID,
		JobType:       models.JobTypeRecurringInvoice,
		Priority:     1,
		Payload:      string(payload),
		Status:       models.JobStatusPending,
		RunAt:        recurring.NextRunDate,
		MaxRetries:    3,
		AutomationID:  &recurring.ID,
		ClientID:     &recurring.ClientID,
		IdempotencyKey: fmt.Sprintf("recurring_%s_%d", recurring.ID, recurring.CurrentCycle+1),
	}
	
	return s.jobQueue.EnqueueJob(job)
}

// ProcessRecurringInvoice executes a recurring invoice generation
func (s *AutoRecurringInvoiceService) ProcessRecurringInvoice(job *models.AutomationJob) error {
	if s.jobQueue == nil {
		return fmt.Errorf("job queue not configured")
	}
	
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return s.jobQueue.FailJob(job.ID, fmt.Sprintf("invalid payload: %v", err))
	}
	
	recurringID, _ := payload["recurring_id"].(string)
	if recurringID == "" {
		return s.jobQueue.FailJob(job.ID, "missing recurring_id")
	}
	
	recurring, err := s.GetRecurringInvoice(job.TenantID, recurringID)
	if err != nil {
		return s.jobQueue.FailJob(job.ID, "recurring not found")
	}
	
	if recurring.Status != "active" || !recurring.IsActive {
		return s.jobQueue.CompleteJob(job.ID, "skipped: not active")
	}
	
	// Validate client still exists
	var client models.Client
	if err := s.db.Where("id = ? AND tenant_id = ?", recurring.ClientID, job.TenantID).First(&client).Error; err != nil {
		return s.jobQueue.FailJob(job.ID, "client not found")
	}
	
	// Check for duplicate
	if recurring.LastInvoiceID != nil {
		var lastInvoice models.Invoice
		if err := s.db.Where("id = ?", *recurring.LastInvoiceID).First(&lastInvoice).Error; err == nil {
			if lastInvoice.CreatedAt.Format("2006-01-02") == time.Now().Format("2006-01-02") {
				return s.jobQueue.CompleteJob(job.ID, "already generated today")
			}
		}
	}
	
	// Generate invoice
	templateData := make(map[string]interface{})
	json.Unmarshal([]byte(recurring.InvoiceTemplate), &templateData)
	
	taxRate := 0.0
	if tr, ok := templateData["tax_rate"].(float64); ok {
		taxRate = tr
	}
	
	dueDate := time.Now().AddDate(0, 0, 30)
	if dd, ok := templateData["due_date"].(string); ok {
		if parsed, err := time.Parse("2006-01-02", dd); err == nil {
			dueDate = parsed
		}
	}
	
	// Parse line items from template
	lineItems := templateData["line_items"]
	var items []models.InvoiceItem
	if lis, ok := lineItems.([]interface{}); ok {
		for _, li := range lis {
			if m, ok := li.(map[string]interface{}); ok {
				item := models.InvoiceItem{
					ID:          uuid.New().String(),
					Description: getString(m, "description", "Item"),
					Quantity:    getFloat(m, "quantity", 1),
					UnitPrice:   getFloat(m, "rate", 0),
					Total:      getFloat(m, "quantity", 1) * getFloat(m, "rate", 0),
				}
				items = append(items, item)
			}
		}
	}
	
	// Calculate subtotal
	subtotal := 0.0
	for _, item := range items {
		subtotal += item.Total
	}
	taxAmount := subtotal * (taxRate / 100)
	totalAmount := subtotal + taxAmount
	
	invoice := &models.Invoice{
		ID:             uuid.New().String(),
		TenantID:       job.TenantID,
		UserID:         recurring.UserID,
		ClientID:       recurring.ClientID,
		InvoiceNumber: autoGenerateInvoiceNumber(),
		Status:        models.InvoiceStatusDraft,
		Subtotal:      subtotal,
		TaxRate:       taxRate,
		TotalTax:      taxAmount,
		Total:         totalAmount,
		PaidAmount:   0,
		BalanceDue:    totalAmount,
		DueDate:      dueDate,
		IsRecurring:  true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	// Calculate totals
	if items, ok := lineItems.([]interface{}); ok {
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				if qty, ok := m["quantity"].(float64); ok {
					if rate, ok := m["rate"].(float64); ok {
						invoice.Subtotal += qty * rate
					}
				}
			}
		}
	}
	invoice.TotalTax = invoice.Subtotal * (invoice.TaxRate / 100)
	invoice.Total = invoice.Subtotal + invoice.TotalTax
	invoice.BalanceDue = invoice.Total
	
	if err := s.db.Create(invoice).Error; err != nil {
		return s.jobQueue.FailJob(job.ID, fmt.Sprintf("failed to create invoice: %v", err))
	}
	
	// Save line items
	for i := range items {
		items[i].InvoiceID = invoice.ID
	}
	if len(items) > 0 {
		s.db.Create(&items)
	}
	
	// Update recurring
	recurring.CurrentCycle++
	now := time.Now()
	recurring.LastInvoiceID = &invoice.ID
	recurring.LastRunAt = &now
	recurring.NextRunDate = calculateNextRunDateFromLast(recurring.LastRunAt, recurring.Frequency, recurring.IntervalDays)
	s.db.Save(recurring)
	
	// Schedule next job
	s.ScheduleJob(recurring)
	
	// Complete
	s.jobQueue.CompleteJob(job.ID, fmt.Sprintf("generated invoice %s", invoice.InvoiceNumber))
	
	return nil
}

func calculateNextRunDate(startDate time.Time, frequency string, intervalDays int) time.Time {
	now := time.Now()
	if startDate.After(now) {
		return startDate
	}
	
	switch frequency {
	case models.FrequencyDaily:
		return now.AddDate(0, 0, 1)
	case models.FrequencyWeekly:
		return now.AddDate(0, 0, 7)
	case models.FrequencyMonthly:
		return now.AddDate(0, 1, 0)
	case models.FrequencyCustom:
		return now.AddDate(0, 0, intervalDays)
	default:
		return now.AddDate(0, 1, 0)
	}
}

func calculateNextRunDateFromLast(lastRun *time.Time, frequency string, intervalDays int) time.Time {
	base := time.Now()
	if lastRun != nil {
		base = *lastRun
	}
	
	switch frequency {
	case models.FrequencyDaily:
		return base.AddDate(0, 0, 1)
	case models.FrequencyWeekly:
		return base.AddDate(0, 0, 7)
	case models.FrequencyMonthly:
		return base.AddDate(0, 1, 0)
	case models.FrequencyCustom:
		return base.AddDate(0, 0, intervalDays)
	default:
		return base.AddDate(0, 1, 0)
	}
}