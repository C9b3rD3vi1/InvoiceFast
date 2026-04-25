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
// ReminderRuleService - Handles payment reminder automation
// ============================================================================

// AutoReminderService handles payment reminder automation
type AutoReminderService struct {
	db        *database.DB
	jobQueue *JobQueueService
}

// NewAutoReminderService creates a new reminder service
func NewAutoReminderService(db *database.DB, jobQueue *JobQueueService) *AutoReminderService {
	return &AutoReminderService{
		db:        db,
		jobQueue: jobQueue,
	}
}

// CreateReminderRuleRequest defines request for creating reminder rule
type CreateReminderRuleRequest struct {
	Name           string     `json:"name"`
	Description   string     `json:"description"`
	ReminderType  string     `json:"reminder_type"` // before_due, on_due, after_due
	DaysBeforeDue int        `json:"days_before_due"`
	DaysAfterDue  int        `json:"days_after_due"`
	Frequency    int        `json:"frequency"`     // Days between reminders
	MaxReminders int        `json:"max_reminders"`
	Channels    []string    `json:"channels"`  // email, sms, whatsapp
	Templates    map[string]string `json:"templates"`
	InvoiceStatus string    `json:"invoice_status"`
}

// UpdateReminderRuleRequest defines request for updating reminder rule
type UpdateReminderRuleRequest struct {
	Name           *string     `json:"name,omitempty"`
	Description   *string     `json:"description,omitempty"`
	ReminderType  *string     `json:"reminder_type,omitempty"`
	DaysBeforeDue *int        `json:"days_before_due,omitempty"`
	DaysAfterDue *int        `json:"days_after_due,omitempty"`
	Frequency    *int        `json:"frequency,omitempty"`
	MaxReminders *int        `json:"max_reminders,omitempty"`
	Channels     *[]string   `json:"channels,omitempty"`
	IsActive     *bool       `json:"is_active,omitempty"`
}

// GetReminderRules returns all reminder rules for a tenant
func (s *AutoReminderService) GetReminderRules(tenantID string, activeOnly bool) ([]models.ReminderRule, error) {
	var rules []models.ReminderRule
	query := s.db.Where("tenant_id = ?", tenantID)
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	err := query.Order("created_at DESC").Find(&rules).Error
	return rules, err
}

// GetReminderRule returns a single reminder rule
func (s *AutoReminderService) GetReminderRule(tenantID, id string) (*models.ReminderRule, error) {
	var rule models.ReminderRule
	err := s.db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&rule).Error
	return &rule, err
}

// CreateReminderRule creates a new reminder rule
func (s *AutoReminderService) CreateReminderRule(tenantID, userID string, req *CreateReminderRuleRequest) (*models.ReminderRule, error) {
	if req.ReminderType == "" {
		return nil, fmt.Errorf("reminder type is required")
	}
	
	channelsJSON, _ := json.Marshal(req.Channels)
	templatesJSON, _ := json.Marshal(req.Templates)
	
	rule := &models.ReminderRule{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		UserID:         userID,
		Name:           req.Name,
		Description:    req.Description,
		ReminderType:   req.ReminderType,
		DaysBeforeDue:  req.DaysBeforeDue,
		DaysAfterDue:   req.DaysAfterDue,
		Frequency:      req.Frequency,
		MaxReminders:   req.MaxReminders,
		Channels:       string(channelsJSON),
		Templates:      string(templatesJSON),
		InvoiceStatus:  req.InvoiceStatus,
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	
	if err := s.db.Create(rule).Error; err != nil {
		return nil, err
	}
	
	return rule, nil
}

// UpdateReminderRule updates a reminder rule
func (s *AutoReminderService) UpdateReminderRule(tenantID, id string, req *UpdateReminderRuleRequest) (*models.ReminderRule, error) {
	rule, err := s.GetReminderRule(tenantID, id)
	if err != nil {
		return nil, err
	}
	
	if req.Name != nil {
		rule.Name = *req.Name
	}
	if req.Description != nil {
		rule.Description = *req.Description
	}
	if req.ReminderType != nil {
		rule.ReminderType = *req.ReminderType
	}
	if req.DaysBeforeDue != nil {
		rule.DaysBeforeDue = *req.DaysBeforeDue
	}
	if req.DaysAfterDue != nil {
		rule.DaysAfterDue = *req.DaysAfterDue
	}
	if req.Frequency != nil {
		rule.Frequency = *req.Frequency
	}
	if req.MaxReminders != nil {
		rule.MaxReminders = *req.MaxReminders
	}
	if req.Channels != nil {
		channelsJSON, _ := json.Marshal(req.Channels)
		rule.Channels = string(channelsJSON)
	}
	if req.IsActive != nil {
		rule.IsActive = *req.IsActive
	}
	
	rule.UpdatedAt = time.Now()
	if err := s.db.Save(rule).Error; err != nil {
		return nil, err
	}
	
	return rule, nil
}

// DeleteReminderRule deletes a reminder rule
func (s *AutoReminderService) DeleteReminderRule(tenantID, id string) error {
	rule, err := s.GetReminderRule(tenantID, id)
	if err != nil {
		return err
	}
	
	return s.db.Delete(rule).Error
}

// GetReminderStats returns reminder statistics
func (s *AutoReminderService) GetReminderStats(tenantID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	var totalRules, activeRules, sent, failed int64
	s.db.Model(&models.ReminderRule{}).Where("tenant_id = ?", tenantID).Count(&totalRules)
	s.db.Model(&models.ReminderRule{}).Where("tenant_id = ? AND is_active = ?", tenantID, true).Count(&activeRules)
	s.db.Model(&models.ReminderStatus{}).Where("tenant_id = ? AND status = ?", tenantID, "sent").Count(&sent)
	s.db.Model(&models.ReminderStatus{}).Where("tenant_id = ? AND status = ?", tenantID, "failed").Count(&failed)
	
	stats["total_rules"] = totalRules
	stats["active_rules"] = activeRules
	stats["reminders_sent"] = sent
	stats["reminders_failed"] = failed
	
	return stats, nil
}

// ============================================================================
// WorkflowService - Event-driven automation
// ============================================================================

// AutoWorkflowService handles event-driven workflow automation
type AutoWorkflowService struct {
	db        *database.DB
	jobQueue *JobQueueService
}

// NewAutoWorkflowService creates a new workflow service
func NewAutoWorkflowService(db *database.DB, jobQueue *JobQueueService) *AutoWorkflowService {
	return &AutoWorkflowService{
		db:        db,
		jobQueue: jobQueue,
	}
}

// CreateWorkflowRequest defines request for creating workflow
type CreateWorkflowRequest struct {
	Name         string                    `json:"name"`
	Description  string                    `json:"description"`
	TriggerEvent string                 `json:"trigger_event"` // invoice_created, invoice_paid, etc
	Conditions  []models.WorkflowCondition `json:"conditions"`
	Actions     []models.WorkflowAction    `json:"actions"`
	Priority    int                     `json:"priority"`
}

// UpdateWorkflowRequest defines request for updating workflow
type UpdateWorkflowRequest struct {
	Name        *string                  `json:"name,omitempty"`
	Description *string                  `json:"description,omitempty"`
	IsActive    *bool                    `json:"is_active,omitempty"`
	Priority   *int                     `json:"priority,omitempty"`
	Actions    *[]models.WorkflowAction `json:"actions,omitempty"`
}

// GetWorkflows returns all workflows for a tenant
func (s *AutoWorkflowService) GetWorkflows(tenantID string, activeOnly bool) ([]models.AutomationWorkflow, error) {
	var workflows []models.AutomationWorkflow
	query := s.db.Where("tenant_id = ?", tenantID)
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	err := query.Order("priority DESC, created_at DESC").Find(&workflows).Error
	return workflows, err
}

// GetWorkflow returns a single workflow
func (s *AutoWorkflowService) GetWorkflow(tenantID, id string) (*models.AutomationWorkflow, error) {
	var workflow models.AutomationWorkflow
	err := s.db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&workflow).Error
	return &workflow, err
}

// CreateWorkflow creates a new workflow
func (s *AutoWorkflowService) CreateWorkflow(tenantID, userID string, req *CreateWorkflowRequest) (*models.AutomationWorkflow, error) {
	if req.TriggerEvent == "" {
		return nil, fmt.Errorf("trigger event is required")
	}
	if len(req.Actions) == 0 {
		return nil, fmt.Errorf("at least one action is required")
	}
	
	conditionsJSON, _ := json.Marshal(req.Conditions)
	actionsJSON, _ := json.Marshal(req.Actions)
	
	workflow := &models.AutomationWorkflow{
		ID:            uuid.New().String(),
		TenantID:       tenantID,
		UserID:         userID,
		Name:          req.Name,
		Description:   req.Description,
		TriggerEvent:  req.TriggerEvent,
		Conditions:   string(conditionsJSON),
		Actions:      string(actionsJSON),
		IsActive:      true,
		Priority:    req.Priority,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	if err := s.db.Create(workflow).Error; err != nil {
		return nil, err
	}
	
	return workflow, nil
}

// UpdateWorkflow updates a workflow
func (s *AutoWorkflowService) UpdateWorkflow(tenantID, id string, req *UpdateWorkflowRequest) (*models.AutomationWorkflow, error) {
	workflow, err := s.GetWorkflow(tenantID, id)
	if err != nil {
		return nil, err
	}
	
	if req.Name != nil {
		workflow.Name = *req.Name
	}
	if req.Description != nil {
		workflow.Description = *req.Description
	}
	if req.IsActive != nil {
		workflow.IsActive = *req.IsActive
	}
	if req.Priority != nil {
		workflow.Priority = *req.Priority
	}
	if req.Actions != nil {
		actionsJSON, _ := json.Marshal(req.Actions)
		workflow.Actions = string(actionsJSON)
	}
	
	workflow.UpdatedAt = time.Now()
	if err := s.db.Save(workflow).Error; err != nil {
		return nil, err
	}
	
	return workflow, nil
}

// DeleteWorkflow deletes a workflow
func (s *AutoWorkflowService) DeleteWorkflow(tenantID, id string) error {
	workflow, err := s.GetWorkflow(tenantID, id)
	if err != nil {
		return err
	}
	
	return s.db.Delete(workflow).Error
}

// GetWorkflowStats returns workflow statistics
func (s *AutoWorkflowService) GetWorkflowStats(tenantID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	var total, active, totalRuns, successRuns int64
	s.db.Model(&models.AutomationWorkflow{}).Where("tenant_id = ?", tenantID).Count(&total)
	s.db.Model(&models.AutomationWorkflow{}).Where("tenant_id = ? AND is_active = ?", tenantID, true).Count(&active)
	s.db.Model(&models.AutomationWorkflow{}).Where("tenant_id = ?", tenantID).
		Select("COALESCE(SUM(total_runs), 0)").Scan(&totalRuns)
	s.db.Model(&models.AutomationWorkflow{}).Where("tenant_id = ?", tenantID).
		Select("COALESCE(SUM(success_runs), 0)").Scan(&successRuns)
	
	stats["total_workflows"] = total
	stats["active_workflows"] = active
	stats["total_runs"] = totalRuns
	stats["success_runs"] = successRuns
	
	return stats, nil
}