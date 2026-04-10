package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

// AuditService handles comprehensive audit logging
type AuditService struct {
	db *database.DB
}

// NewAuditService creates a new audit service
func NewAuditService(db *database.DB) *AuditService {
	return &AuditService{db: db}
}

// AuditAction types
const (
	AuditActionUserLogin       = "user_login"
	AuditActionUserLogout      = "user_logout"
	AuditActionUserRegister    = "user_register"
	AuditActionUserUpdate      = "user_update"
	AuditActionInvoiceCreate   = "invoice_create"
	AuditActionInvoiceUpdate   = "invoice_update"
	AuditActionInvoiceSend     = "invoice_send"
	AuditActionInvoiceView     = "invoice_view"
	AuditActionInvoicePay      = "invoice_paid"
	AuditActionInvoiceCancel   = "invoice_cancel"
	AuditActionPaymentInitiate = "payment_initiate"
	AuditActionPaymentComplete = "payment_complete"
	AuditActionPaymentFailed   = "payment_failed"
	AuditActionKRASubmit       = "kra_submit"
	AuditActionKRASuccess      = "kra_success"
	AuditActionKRAFailed       = "kra_failed"
	AuditActionClientCreate    = "client_create"
	AuditActionClientUpdate    = "client_update"
	AuditActionSettingsUpdate  = "settings_update"
	AuditActionAPIAccess       = "api_access"
	AuditActionSecurityEvent   = "security_event"
)

// AuditEntity types
const (
	AuditEntityUser     = "user"
	AuditEntityInvoice  = "invoice"
	AuditEntityPayment  = "payment"
	AuditEntityClient   = "client"
	AuditEntitySettings = "settings"
	AuditEntityAPI      = "api"
)

// LogAction records an audit log entry
func (s *AuditService) LogAction(ctx context.Context, tenantID, userID, action, entityType, entityID string, details map[string]interface{}) error {
	detailsJSON, _ := json.Marshal(details)

	entry := &models.AuditLog{
		TenantID:   tenantID,
		UserID:     userID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Details:    string(detailsJSON),
		CreatedAt:  time.Now(),
	}

	return s.db.Create(entry).Error
}

// LogPaymentCompleted logs a completed payment with full details
func (s *AuditService) LogPaymentCompleted(ctx context.Context, tenantID, userID, paymentID, invoiceID string, amount float64, method string) error {
	return s.LogAction(ctx, tenantID, userID, AuditActionPaymentComplete, AuditEntityPayment, paymentID, map[string]interface{}{
		"invoice_id": invoiceID,
		"amount":     amount,
		"method":     method,
	})
}

// LogSecurityEvent logs security-related events
func (s *AuditService) LogSecurityEvent(ctx context.Context, tenantID, eventType, description string, details map[string]interface{}) error {
	details["event_type"] = eventType
	return s.LogAction(ctx, tenantID, "", AuditActionSecurityEvent, "security", "", details)
}

// LogLoginAttempt logs login attempts (success and failure)
func (s *AuditService) LogLoginAttempt(ctx context.Context, tenantID, email, ipAddress string, success bool, reason string) error {
	action := AuditActionUserLogin
	if !success {
		action = "user_login_failed"
	}

	return s.LogAction(ctx, tenantID, "", action, AuditEntityUser, "", map[string]interface{}{
		"email":   email,
		"ip":      ipAddress,
		"success": success,
		"reason":  reason,
	})
}

// GetAuditLogs retrieves audit logs with filters
func (s *AuditService) GetAuditLogs(ctx context.Context, tenantID string, filter AuditFilter) ([]models.AuditLog, int64, error) {
	if tenantID == "" {
		return nil, 0, fmt.Errorf("tenant_id required")
	}

	var logs []models.AuditLog
	var total int64

	query := s.db.Where("tenant_id = ?", tenantID)

	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.EntityType != "" {
		query = query.Where("entity_type = ?", filter.EntityType)
	}
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.FromDate != nil {
		query = query.Where("created_at >= ?", filter.FromDate)
	}
	if filter.ToDate != nil {
		query = query.Where("created_at <= ?", filter.ToDate)
	}

	// Count total
	if err := query.Model(&models.AuditLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// AuditFilter for querying audit logs
type AuditFilter struct {
	Action     string
	EntityType string
	UserID     string
	FromDate   *time.Time
	ToDate     *time.Time
	Limit      int
	Offset     int
}

// CreateDefaultAuditLogs creates default audit log entries for critical actions
func (s *AuditService) CreateDefaultAuditLogs() error {
	// This would be called during migration/setup
	return nil
}

// GetSecurityAuditLogs retrieves security-related audit logs
func (s *AuditService) GetSecurityAuditLogs(ctx context.Context, tenantID string, days int) ([]models.AuditLog, error) {
	fromDate := time.Now().AddDate(0, 0, -days)

	var logs []models.AuditLog
	err := s.db.Where("tenant_id = ? AND action = ? AND created_at >= ?", tenantID, AuditActionSecurityEvent, fromDate).
		Order("created_at DESC").
		Find(&logs).Error

	return logs, err
}

// GetUserActivityLog retrieves activity logs for a specific user
func (s *AuditService) GetUserActivityLog(ctx context.Context, tenantID, userID string, limit int) ([]models.AuditLog, error) {
	var logs []models.AuditLog
	err := s.db.Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error

	return logs, err
}

// GenerateAuditReport generates an audit summary report
func (s *AuditService) GenerateAuditReport(ctx context.Context, tenantID string, fromDate, toDate time.Time) (map[string]interface{}, error) {
	var totalActions int64
	var uniqueUsers int64
	var actionCounts map[string]int64

	// Get total actions
	s.db.Model(&models.AuditLog{}).Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, fromDate, toDate).Count(&totalActions)

	// Get unique users
	s.db.Model(&models.AuditLog{}).Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, fromDate, toDate).Distinct("user_id").Count(&uniqueUsers)

	// Get action breakdown
	var actionBreakdown []struct {
		Action string
		Count  int64
	}
	s.db.Model(&models.AuditLog{}).Select("action, COUNT(*) as count").
		Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, fromDate, toDate).
		Group("action").Scan(&actionBreakdown)

	actionCounts = make(map[string]int64)
	for _, ab := range actionBreakdown {
		actionCounts[ab.Action] = ab.Count
	}

	return map[string]interface{}{
		"period": map[string]time.Time{
			"from": fromDate,
			"to":   toDate,
		},
		"total_actions":    totalActions,
		"unique_users":     uniqueUsers,
		"action_breakdown": actionCounts,
		"generated_at":     time.Now(),
	}, nil
}
