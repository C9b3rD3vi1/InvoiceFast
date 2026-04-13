package services

import (
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
)

// AutomationService handles workflow automation functionality
type AutomationService struct {
	db            *database.DB
	emailService  *EmailService
	invoiceService *InvoiceService
	clientService  *ClientService
}

// NewAutomationService creates a new automation service
func NewAutomationService(db *database.DB, emailSvc *EmailService, invoiceSvc *InvoiceService, clientSvc *ClientService) *AutomationService {
	return &AutomationService{
		db:            db,
		emailService:   emailSvc,
		invoiceService: invoiceSvc,
		clientService:  clientSvc,
	}
}

// SimpleAutomationService creates automation service with just database
type SimpleAutomationService struct {
	db            *database.DB
	emailService  *EmailService
}

// NewSimpleAutomationService creates a simplified automation service
func NewSimpleAutomationService(db *database.DB) *AutomationService {
	return &AutomationService{
		db: db,
	}
}

// GetAutomations returns all automations for a tenant
func (s *AutomationService) GetAutomations(tenantID string) ([]models.Automation, error) {
	var automations []models.Automation
	if err := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&automations).Error; err != nil {
		return nil, err
	}
	return automations, nil
}

// GetAutomation returns a single automation
func (s *AutomationService) GetAutomation(tenantID, id string) (*models.Automation, error) {
	var automation models.Automation
	if err := s.db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&automation).Error; err != nil {
		return nil, err
	}
	return &automation, nil
}

// GetActiveAutomations returns all active (enabled) automations
func (s *AutomationService) GetActiveAutomations() ([]models.Automation, error) {
	var automations []models.Automation
	if err := s.db.Where("is_active = ?", true).Find(&automations).Error; err != nil {
		return nil, err
	}
	return automations, nil
}

// CreateAutomationRequest for creating new automations
type CreateAutomationRequest struct {
	Name          string                `json:"name"`
	Description   string                `json:"description"`
	TriggerType   string                `json:"trigger_type"`
	TriggerConfig interface{}           `json:"trigger_config"`
	Conditions    []AutomationCondition `json:"conditions"`
	Actions       []AutomationAction    `json:"actions"`
	IsActive      bool                  `json:"is_active"`
}

// UpdateAutomationRequest for updating automations
type UpdateAutomationRequest struct {
	Name          *string                `json:"name,omitempty"`
	Description   *string                `json:"description,omitempty"`
	TriggerType   *string                `json:"trigger_type,omitempty"`
	TriggerConfig interface{}            `json:"trigger_config,omitempty"`
	Conditions    *[]AutomationCondition `json:"conditions,omitempty"`
	Actions       *[]AutomationAction    `json:"actions,omitempty"`
	IsActive      *bool                  `json:"is_active,omitempty"`
}

// AutomationCondition for filtering when automation runs
type AutomationCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// AutomationAction defines what to do when automation triggers
type AutomationAction struct {
	ActionType string      `json:"action_type"`
	Config     interface{} `json:"config"`
	Order      int         `json:"order"`
}

// CreateAutomation creates a new automation
func (s *AutomationService) CreateAutomation(tenantID, userID string, req *CreateAutomationRequest) (*models.Automation, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.TriggerType == "" {
		return nil, fmt.Errorf("trigger type is required")
	}

	automation := &models.Automation{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		TriggerType: req.TriggerType,
		IsActive:    req.IsActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.db.Create(automation).Error; err != nil {
		return nil, err
	}

	return automation, nil
}

// UpdateAutomation updates an existing automation
func (s *AutomationService) UpdateAutomation(tenantID, id string, req *UpdateAutomationRequest) (*models.Automation, error) {
	automation, err := s.GetAutomation(tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		automation.Name = *req.Name
	}
	if req.Description != nil {
		automation.Description = *req.Description
	}
	if req.IsActive != nil {
		automation.IsActive = *req.IsActive
	}
	automation.UpdatedAt = time.Now()

	if err := s.db.Save(automation).Error; err != nil {
		return nil, err
	}

	return automation, nil
}

// DeleteAutomation removes an automation
func (s *AutomationService) DeleteAutomation(tenantID, id string) error {
	automation, err := s.GetAutomation(tenantID, id)
	if err != nil {
		return err
	}

	return s.db.Delete(automation).Error
}

// RunAutomation executes an automation
func (s *AutomationService) RunAutomation(tenantID, id string) (map[string]interface{}, error) {
	automation, err := s.GetAutomation(tenantID, id)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	
	// Create execution log
	log := &models.AutomationLog{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		AutomationID: id,
		Status:       "running",
		StartedAt:    now,
	}

	if err := s.db.Create(log).Error; err != nil {
		return nil, err
	}

	// Execute the automation actions
	actionsRun := 0
	errors := []string{}
	
	// Example: Send email action
	if s.emailService != nil {
		switch automation.TriggerType {
		case "invoice_due":
			// Get overdue invoices and send reminders
			actionsRun++
		case "payment_received":
			// Send thank you email
			actionsRun++
		case "payment_failed":
			// Notify admin
			actionsRun++
		case "invoice_created":
			// Log creation
			actionsRun++
		}
	}

	// Update log with results
	status := "completed"
	if len(errors) > 0 {
		status = "completed_with_errors"
	}
	
	log.Status = status
	log.CompletedAt = &now
	s.db.Save(log)

	result := map[string]interface{}{
		"automation_id": id,
		"status":        status,
		"log_id":        log.ID,
		"actions_run":   actionsRun,
		"errors":        errors,
		"executed_at":   now,
	}

	return result, nil
}

// ProcessAutomationTrigger checks if any automations should fire for a trigger
type AutomationTriggerContext struct {
	TenantID    string
	UserID      string
	InvoiceID   *string
	ClientID    *string
	PaymentID   *string
	TriggerType string
	Data        map[string]interface{}
}

// ProcessTrigger processes a trigger event and runs matching automations
func (s *AutomationService) ProcessTrigger(ctx *AutomationTriggerContext) error {
	// Find matching active automations
	automations, err := s.findMatchingAutomations(ctx.TenantID, ctx.TriggerType, ctx.Data)
	if err != nil {
		return err
	}

	// Run each matching automation
	for _, automation := range automations {
		// Run in goroutine for async processing
		go func(automationID string) {
			_, err := s.RunAutomation(ctx.TenantID, automationID)
			if err != nil {
				// Log error but don't fail
				fmt.Printf("[Automation] Error running %s: %v\n", automationID, err)
			}
		}(automation.ID)
	}

	return nil
}

// findMatchingAutomations finds automations that match trigger criteria
func (s *AutomationService) findMatchingAutomations(tenantID, triggerType string, data map[string]interface{}) ([]models.Automation, error) {
	var automations []models.Automation
	
	// Find active automations for this tenant with this trigger type
	if err := s.db.Where(
		"tenant_id = ? AND is_active = ? AND trigger_type = ?",
		tenantID, true, triggerType,
	).Find(&automations).Error; err != nil {
		return nil, err
	}

	// TODO: Filter by conditions if any

	return automations, nil
}

// GetLogs returns execution logs for an automation
func (s *AutomationService) GetLogs(tenantID, automationID string, limit int) ([]models.AutomationLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	
	var logs []models.AutomationLog
	if err := s.db.Where("tenant_id = ? AND automation_id = ?", tenantID, automationID).
		Order("started_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

// GetAutomationStats returns statistics for an automation
func (s *AutomationService) GetAutomationStats(tenantID, automationID string) (map[string]interface{}, error) {
	var totalRuns int64
	var successRuns int64
	var failedRuns int64
	
	s.db.Model(&models.AutomationLog{}).
		Where("tenant_id = ? AND automation_id = ?", tenantID, automationID).
		Count(&totalRuns)
	
	s.db.Model(&models.AutomationLog{}).
		Where("tenant_id = ? AND automation_id = ? AND status = ?", tenantID, automationID, "completed").
		Count(&successRuns)
	
	s.db.Model(&models.AutomationLog{}).
		Where("tenant_id = ? AND automation_id = ? AND status = ?", tenantID, automationID, "failed").
		Count(&failedRuns)
	
	var avgDuration float64
	s.db.Raw(
		"SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (completed_at - started_at))), 0) FROM automation_logs WHERE tenant_id = ? AND automation_id = ?",
		tenantID, automationID,
	).Scan(&avgDuration)
	
	return map[string]interface{}{
		"total_runs":   totalRuns,
		"success_runs": successRuns,
		"failed_runs":  failedRuns,
		"success_rate": func() float64 {
			if totalRuns == 0 {
				return 100
			}
			return float64(successRuns) / float64(totalRuns) * 100
		}(),
		"avg_duration": avgDuration,
	}, nil
}

// TriggerInvoiceCreated fires when an invoice is created
func (s *AutomationService) TriggerInvoiceCreated(tenantID, userID, invoiceID string, invoiceTotal float64) error {
	return s.ProcessTrigger(&AutomationTriggerContext{
		TenantID:    tenantID,
		UserID:      userID,
		InvoiceID:   &invoiceID,
		TriggerType: "invoice_created",
		Data: map[string]interface{}{
			"invoice_id": invoiceID,
			"total":      invoiceTotal,
		},
	})
}

// TriggerInvoiceSent fires when an invoice is sent
func (s *AutomationService) TriggerInvoiceSent(tenantID, userID, invoiceID string) error {
	return s.ProcessTrigger(&AutomationTriggerContext{
		TenantID:    tenantID,
		UserID:      userID,
		InvoiceID:   &invoiceID,
		TriggerType: "invoice_sent",
		Data: map[string]interface{}{
			"invoice_id": invoiceID,
		},
	})
}

// TriggerInvoicePaid fires when an invoice is paid
func (s *AutomationService) TriggerInvoicePaid(tenantID, userID, invoiceID, paymentID string, amount float64) error {
	return s.ProcessTrigger(&AutomationTriggerContext{
		TenantID:    tenantID,
		UserID:      userID,
		InvoiceID:   &invoiceID,
		PaymentID:   &paymentID,
		TriggerType: "invoice_paid",
		Data: map[string]interface{}{
			"invoice_id":  invoiceID,
			"payment_id":  paymentID,
			"amount":      amount,
		},
	})
}

// TriggerInvoiceOverdue fires when an invoice becomes overdue
func (s *AutomationService) TriggerInvoiceOverdue(tenantID, userID, invoiceID string, daysOverdue int) error {
	return s.ProcessTrigger(&AutomationTriggerContext{
		TenantID:    tenantID,
		UserID:      userID,
		InvoiceID:   &invoiceID,
		TriggerType: "invoice_overdue",
		Data: map[string]interface{}{
			"invoice_id":   invoiceID,
			"days_overdue": daysOverdue,
		},
	})
}

// TriggerPaymentFailed fires when a payment attempt fails
func (s *AutomationService) TriggerPaymentFailed(tenantID, userID, invoiceID string, reason string) error {
	return s.ProcessTrigger(&AutomationTriggerContext{
		TenantID:    tenantID,
		UserID:      userID,
		InvoiceID:   &invoiceID,
		TriggerType: "payment_failed",
		Data: map[string]interface{}{
			"invoice_id": invoiceID,
			"reason":     reason,
		},
	})
}

// TriggerClientAdded fires when a new client is added
func (s *AutomationService) TriggerClientAdded(tenantID, userID, clientID string) error {
	return s.ProcessTrigger(&AutomationTriggerContext{
		TenantID:    tenantID,
		UserID:      userID,
		ClientID:    &clientID,
		TriggerType: "client_added",
		Data: map[string]interface{}{
			"client_id": clientID,
		},
	})
}
