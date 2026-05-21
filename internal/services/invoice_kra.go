package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func internalInvoiceToKRA(invoice *models.Invoice) *kraInvoice {
	invItems := make([]kraInvoiceItem, len(invoice.Items))
	for i, item := range invoice.Items {
		invItems[i] = kraInvoiceItem{
			ID:          item.ID,
			Description: item.Description,
			Quantity:    item.Quantity,
			Unit:        item.Unit,
			UnitPrice:   item.UnitPrice,
			Total:       item.Total,
		}
	}
	return &kraInvoice{
		ID:            invoice.ID,
		InvoiceNumber: invoice.InvoiceNumber,
		Currency:      invoice.Currency,
		Subtotal:      invoice.Subtotal,
		TaxRate:       invoice.TaxRate,
		TaxAmount:     invoice.TaxAmount,
		Discount:      invoice.Discount,
		Total:         invoice.Total,
		PaidAmount:    invoice.PaidAmount,
		CreatedAt:     invoice.CreatedAt,
		DueDate:       invoice.DueDate,
		Status:        string(invoice.Status),
		Items:         invItems,
	}
}

func internalUserToKRA(user *models.User) *kraUser {
	return &kraUser{
		ID:          user.ID,
		Email:       user.Email,
		Phone:       user.Phone,
		CompanyName: user.CompanyName,
		KRAPIN:      user.KRAPIN,
	}
}

func internalClientToKRA(client *models.Client) *kraClient {
	return &kraClient{
		ID:      client.ID,
		Name:    client.Name,
		Email:   client.Email,
		Phone:   client.Phone,
		Address: client.Address,
		KRAPIN:  client.KRAPIN,
	}
}

// SubmitInvoiceToKRA submits an invoice to KRA with idempotency
func (s *InvoiceService) SubmitInvoiceToKRA(tenantID, invoiceID string) (*KRAResponse, error) {
	if s.kraService == nil {
		return nil, errors.New("KRA service not configured")
	}

	var result *KRAResponse

	// Use transaction for idempotent submission
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var invoice models.Invoice
		if err := tx.Scopes(database.TenantFilter(tenantID)).
			Preload("Items").
			First(&invoice, "id = ?", invoiceID).Error; err != nil {
			return ErrInvoiceNotFound
		}

		// PRE-KRA VALIDATION: Validate invoice before submission
		if err := s.validateInvoiceForKRA(&invoice); err != nil {
			return fmt.Errorf("invoice validation failed: %w", err)
		}

		// Idempotency check: if already submitted, return existing result
		if invoice.KRAStatus == models.KRAInvoiceStatusSubmitted || invoice.KRAStatus == models.KRAInvoiceStatusAccepted {
			if invoice.KRAICN != "" {
				return nil // Already submitted successfully
			}
		}

		// Check for pending idempotency key to prevent duplicate submissions
		if invoice.KRAIdempotencyKey != "" {
			// Check if we have a recent submission attempt
			if invoice.KRASubmittedAt != nil {
				timeSince := time.Since(*invoice.KRASubmittedAt)
				if timeSince < 5*time.Minute {
					// Still processing or failed, don't resubmit
					if invoice.KRAStatus == models.KRAInvoiceStatusFailed {
						return fmt.Errorf("KRA submission in progress or failed, please wait or retry later")
					}
					return nil // Already processing
				}
			}
		}

		// Generate idempotency key
		idempotencyKey := uuid.New().String()
		tx.Model(&models.Invoice{}).Where("id = ?", invoiceID).Update("kra_idempotency_key", idempotencyKey)

		// Get related data with error handling
		var cli models.Client
		if err := tx.Scopes(database.TenantFilter(tenantID)).First(&cli, "id = ?", invoice.ClientID).Error; err != nil {
			return fmt.Errorf("failed to fetch client: %w", err)
		}
		
		var usr models.User
		if err := tx.Scopes(database.TenantFilter(tenantID)).First(&usr, "id = ?", invoice.UserID).Error; err != nil {
			return fmt.Errorf("failed to fetch user: %w", err)
		}

dbItems := make([]models.InvoiceItem, 0)
		if err := tx.Model(&models.InvoiceItem{}).Where("invoice_id = ?", invoiceID).Find(&dbItems).Error; err != nil {
			return fmt.Errorf("failed to fetch invoice items: %w", err)
		}

		// Convert to KRAItem format for KRA payload
		kraPayloadItems := make([]KRAItem, len(dbItems))
		for i, item := range dbItems {
			kraPayloadItems[i] = KRAItem{
				ItemCode:        item.ItemCode,
				ItemDescription: item.Description,
				Quantity:      item.Quantity,
				UnitOfMeasure:  item.Unit,
				UnitPrice:    item.UnitPrice,
				Discount:     item.DiscountAmt,
				DiscountRate: item.DiscountRate,
				VATRate:      item.TaxRate,
				VATAmount:    item.TaxAmount,
				Total:       item.Total,
			}
		}

		// Determine buyer classification based on client
		buyerClassification := determineBuyerClassification(&cli)

		// Get seller address from user - use CompanyName as fallback
		sellerAddress := usr.CompanyName
		// Note: PhysicalAddress should be added to User model for full KRA compliance
		// For now using CompanyName as it typically contains business location

		// Determine invoice classification
		invoiceClassification := invoice.InvoiceClassification
		if invoiceClassification == "" {
			invoiceClassification = "normal"
		}

		// Determine payment mode from payments (simplified)
		paymentMode := "CASH"
		
		kraData := &KRAInvoiceData{
			InvoiceNumber: invoice.InvoiceNumber,
			InvoiceDate:   invoice.CreatedAt.Format("2006-01-02"),
			InvoiceTime:   invoice.CreatedAt.Format("15:04:05"),
			Seller: KRASeller{
				RegistrationNumber: usr.KRAPIN,
				BusinessName:       usr.CompanyName,
				Address:            sellerAddress,
				ContactMobile:      usr.Phone,
				ContactEmail:       usr.Email,
			},
			Buyer: KRABuyer{
				BuyerType:          buyerClassification,
				CustomerName:       cli.Name,
				Address:            cli.Address,
				ContactMobile:      cli.Phone,
				ContactEmail:       cli.Email,
				RegistrationNumber: cli.KRAPIN,
			},
			Items:              kraPayloadItems,
			SubTotal:          invoice.Subtotal,
			TotalExcludingVAT: invoice.Subtotal - invoice.Discount,
			VATRate:           invoice.TaxRate,
			VATAmount:         invoice.TotalTax,
			TotalIncludingVAT: invoice.Total,
			Currency:          invoice.Currency,
			PaymentMode:       paymentMode,
		}

		kraResp, err := s.kraService.SubmitInvoice(kraData, invoice.TenantID, invoice.ID)
		if err != nil {
			tx.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
				"kra_status":      models.KRAInvoiceStatusFailed,
				"kra_error":      err.Error(),
				"kra_retry_count": gorm.Expr("kra_retry_count + 1"),
			})
			tx.Create(&models.AuditLog{
				ID:         uuid.New().String(),
				TenantID:   tenantID,
				UserID:     invoice.UserID,
				Action:     "kra_failed",
				EntityType: "invoice",
				EntityID:   invoiceID,
				Details:    fmt.Sprintf(`{"invoice_number": "%s", "error": "%s"}`, invoice.InvoiceNumber, err.Error()),
			})
			return err
		}

		if kraResp == nil {
			tx.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
				"kra_status": models.KRAInvoiceStatusFailed,
				"kra_error":  "nil response from KRA service",
			})
			return fmt.Errorf("KRA submission returned nil response")
		}

		now := time.Now()
		tx.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
			"kra_icn":              kraResp.ICN,
			"kra_qr_code":          kraResp.QRCode,
			"kra_status":           models.KRAInvoiceStatusSubmitted,
			"kra_submitted_at":     now,
			"kra_error":            "",
			"kra_idempotency_key":  "",
		})

		tx.Create(&models.AuditLog{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			UserID:     invoice.UserID,
			Action:     "kra_success",
			EntityType: "invoice",
			EntityID:   invoiceID,
			Details:    fmt.Sprintf(`{"invoice_number": "%s", "icn": "%s"}`, invoice.InvoiceNumber, kraResp.ICN),
		})

		logger.Get().Info(context.Background(), "KRA invoice submitted", "category", "kra", "invoice_number", invoice.InvoiceNumber, "icn", kraResp.ICN)

		result = kraResp
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// ClearKRAData clears KRA submission data for retry
func (s *InvoiceService) ClearKRAData(tenantID, invoiceID string) error {
	if tenantID == "" {
		return ErrTenantRequired
	}
	return s.db.Model(&models.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{
		"kra_icn":          "",
		"kra_qr_code":      "",
		"kra_status":       "",
		"kra_submitted_at": nil,
		"kra_error":        "",
	}).Error
}

// KRAActivityEvent represents a KRA activity event
type KRAActivityEvent struct {
	ID            string    `json:"id"`
	InvoiceID     string    `json:"invoice_id"`
	InvoiceNumber string    `json:"invoice_number"`
	Action        string    `json:"action"` // submitted, failed, retried
	Status        string    `json:"status"`
	ICN           string    `json:"icn,omitempty"`
	Error         string    `json:"error,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// GetKRAActivityFeed returns recent KRA activity for the tenant
func (s *InvoiceService) GetKRAActivityFeed(tenantID string, limit int) ([]KRAActivityEvent, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var events []KRAActivityEvent

	// Get invoices with KRA activity in the last 24 hours
	s.db.Scopes(database.TenantFilter(tenantID)).
		Where("kra_status IN ? OR kra_icn IS NOT NULL", []interface{}{models.KRAInvoiceStatusSubmitted, models.KRAInvoiceStatusFailed}).
		Order("updated_at DESC").
		Limit(limit).
		Find(&events)

	// Build activity events from audit logs for more detail
	var auditLogs []models.AuditLog
	s.db.Scopes(database.TenantFilter(tenantID)).
		Where("action IN ('kra_success', 'kra_failed')").
		Order("created_at DESC").
		Limit(limit).
		Find(&auditLogs)

	// Merge audit logs with invoice data
	activityMap := make(map[string]*KRAActivityEvent)
	for _, inv := range events {
		ts := inv.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		activityMap[inv.InvoiceID] = &inv
	}

	// Add more recent events from audit logs
	for _, log := range auditLogs {
		if existing, ok := activityMap[log.EntityID]; ok {
			if existing.Error == "" {
				var details map[string]interface{}
				json.Unmarshal([]byte(log.Details), &details)
				if err, ok := details["error"].(string); ok && err != "" {
					existing.Error = err
				}
			}
			continue
		}
		// Create event from audit log
		var inv models.Invoice
		if err := s.db.First(&inv, "id = ?", log.EntityID).Error; err != nil {
			continue
		}
		action := "submitted"
		if log.Action == "kra_failed" {
			action = "failed"
		}
		var details map[string]interface{}
		json.Unmarshal([]byte(log.Details), &details)
		errMsg := ""
		if err, ok := details["error"].(string); ok {
			errMsg = err
		}
		events = append(events, KRAActivityEvent{
			ID:            log.ID,
			InvoiceID:     log.EntityID,
			InvoiceNumber: inv.InvoiceNumber,
			Action:        action,
			Status:        string(inv.KRAStatus),
			ICN:           inv.KRAICN,
			Error:         errMsg,
			Timestamp:     log.CreatedAt,
		})
	}

	// Sort by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	if len(events) > limit {
		events = events[:limit]
	}

	return events, nil
}

// SubmitAllPendingToKRA submits all pending invoices to KRA
func (s *InvoiceService) SubmitAllPendingToKRA(tenantID string) (submitted, failed int, err error) {
	if tenantID == "" {
		return 0, 0, ErrTenantRequired
	}

	var pendingInvoices []models.Invoice
	s.db.Scopes(database.TenantFilter(tenantID)).
		Where("(kra_status IS NULL OR kra_status = '') AND (kra_icn IS NULL OR kra_icn = '')").
		Find(&pendingInvoices)

	for _, inv := range pendingInvoices {
		_, err := s.SubmitInvoiceToKRA(tenantID, inv.ID)
		if err != nil {
			failed++
			logger.Get().Error(context.Background(), "KRA submission failed", "category", "kra", "invoice_number", inv.InvoiceNumber, "error", err)
		} else {
			submitted++
		}
	}

	return submitted, failed, nil
}

// validateInvoiceForKRA validates invoice before KRA submission
func (s *InvoiceService) validateInvoiceForKRA(invoice *models.Invoice) error {
	// Check required fields
	if invoice.InvoiceNumber == "" {
		return errors.New("invoice number is required")
	}
	
	if invoice.Total <= 0 {
		return errors.New("invoice total must be greater than zero")
	}
	
	// Validate seller KRA PIN
	if invoice.UserID == "" {
		return errors.New("user ID is required")
	}
	
	// Validate client
	if invoice.ClientID == "" {
		return errors.New("client is required")
	}
	
	// Tax validation
	if invoice.TaxRate < 0 || invoice.TaxRate > 100 {
		return errors.New("invalid tax rate (must be 0-100)")
	}
	
	// For credit/debit notes, require original ICN
	if invoice.InvoiceType == "credit_note" || invoice.InvoiceType == "debit_note" {
		if invoice.OriginalICN == "" {
			return errors.New("credit/debit notes must reference original invoice ICN")
		}
	}
	
	return nil
}

// determineBuyerClassification determines KRA buyer classification based on client
func determineBuyerClassification(client *models.Client) string {
	if client == nil {
		return string(models.BuyerClassificationB2C)
	}
	
	// B2B if client has KRA PIN with valid format
	if client.KRAPIN != "" {
		// Validate PIN format: starts with A, ends with B
		if strings.HasPrefix(client.KRAPIN, "A") && strings.HasSuffix(client.KRAPIN, "B") {
			return string(models.BuyerClassificationB2B)
		}
		// Invalid PIN format, treat as B2C
		return string(models.BuyerClassificationB2C)
	}
	
	// Check for export indicators in email/address
	email := strings.ToLower(client.Email)
	address := strings.ToLower(client.Address)
	
	if strings.Contains(email, ".export") || 
	   strings.Contains(email, "abroad") ||
	   strings.Contains(address, "export") ||
	   strings.Contains(address, "duty free") {
		return string(models.BuyerClassificationEXPORT)
	}
	
	// Default to B2C for consumers
	return string(models.BuyerClassificationB2C)
}
