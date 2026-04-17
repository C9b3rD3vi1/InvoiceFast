package services

import (
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

type ActivityItem struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // invoice_created, invoice_sent, payment_received, client_created
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	EntityType  string                 `json:"entity_type"`
	EntityID    string                 `json:"entity_id"`
	UserID      string                 `json:"user_id"`
	UserName    string                 `json:"user_name"`
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type ActivityService struct {
	db *database.DB
}

func NewActivityService(db *database.DB) *ActivityService {
	return &ActivityService{db: db}
}

func (s *ActivityService) GetRecentActivity(tenantID string, limit int) ([]ActivityItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var activities []ActivityItem

	// Get audit logs
	var auditLogs []models.AuditLog
	s.db.Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Find(&auditLogs)

	for _, log := range auditLogs {
		userName := s.getUserName(log.UserID)
		activities = append(activities, ActivityItem{
			ID:          log.ID,
			Type:        log.Action,
			Title:       formatActionTitle(log.Action, log.EntityType),
			Description: log.Details,
			EntityType:  log.EntityType,
			EntityID:    log.EntityID,
			UserID:      log.UserID,
			UserName:    userName,
			Timestamp:   log.CreatedAt,
		})
	}

	// Also get recent invoices
	var invoices []models.Invoice
	s.db.Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Find(&invoices)

	for _, inv := range invoices {
		userName := s.getUserName(inv.UserID)
		activities = append(activities, ActivityItem{
			ID:          inv.ID,
			Type:        "invoice_created",
			Title:       fmt.Sprintf("Invoice %s created", inv.InvoiceNumber),
			Description: fmt.Sprintf("%s - %s %.2f", inv.ClientID, inv.Currency, inv.Total),
			EntityType:  "invoice",
			EntityID:    inv.ID,
			UserID:      inv.UserID,
			UserName:    userName,
			Timestamp:   inv.CreatedAt,
			Metadata: map[string]interface{}{
				"status": inv.Status,
				"total":  inv.Total,
			},
		})
	}

	// Get recent payments
	var payments []models.Payment
	s.db.Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Find(&payments)

	for _, pay := range payments {
		var inv models.Invoice
		s.db.First(&inv, "id = ?", pay.InvoiceID)
		activities = append(activities, ActivityItem{
			ID:          pay.ID,
			Type:        "payment_received",
			Title:       fmt.Sprintf("Payment of %s %.2f received", pay.Currency, pay.Amount),
			Description: fmt.Sprintf("Invoice: %s", inv.InvoiceNumber),
			EntityType:  "payment",
			EntityID:    pay.ID,
			UserID:      pay.UserID,
			Timestamp:   pay.CreatedAt,
			Metadata: map[string]interface{}{
				"status": pay.Status,
				"method": pay.Method,
			},
		})
	}

	// Sort by timestamp descending
	for i := 0; i < len(activities)-1; i++ {
		for j := i + 1; j < len(activities); j++ {
			if activities[j].Timestamp.After(activities[i].Timestamp) {
				activities[i], activities[j] = activities[j], activities[i]
			}
		}
	}

	if len(activities) > limit {
		activities = activities[:limit]
	}

	return activities, nil
}

func (s *ActivityService) getUserName(userID string) string {
	if userID == "" {
		return "System"
	}
	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		return "User"
	}
	return user.Name
}

func formatActionTitle(action, entityType string) string {
	switch action {
	case "invoice.created":
		return "Invoice created"
	case "invoice.sent":
		return "Invoice sent"
	case "invoice.paid":
		return "Invoice paid"
	case "invoice.viewed":
		return "Invoice viewed"
	case "payment.received":
		return "Payment received"
	case "client.created":
		return "Client added"
	case "client.updated":
		return "Client updated"
	default:
		return fmt.Sprintf("%s %s", action, entityType)
	}
}
