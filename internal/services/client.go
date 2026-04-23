package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ClientService struct {
	db *database.DB
}

func NewClientService(db *database.DB) *ClientService {
	return &ClientService{db: db}
}

// CreateClient creates a new client (tenant-scoped)
func (s *ClientService) CreateClient(tenantID, userID string, req *CreateClientRequest) (*models.Client, error) {
	// Validate inputs
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user ID is required")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("client name is required")
	}

	client := &models.Client{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		UserID:       userID,
		Name:         strings.TrimSpace(req.Name),
		Email:        strings.TrimSpace(req.Email),
		Phone:        normalizePhone(req.Phone),
		Address:      strings.TrimSpace(req.Address),
		KRAPIN:       strings.ToUpper(strings.TrimSpace(req.KRAPIN)),
		Currency:     getValidCurrency(req.Currency),
		PaymentTerms: getValidPaymentTerms(req.PaymentTerms),
		Notes:        strings.TrimSpace(req.Notes),
	}

	// Handle tags - serialize to JSON string
	if len(req.Tags) > 0 {
		tagsJSON, err := json.Marshal(req.Tags)
		if err == nil {
			client.Tags = string(tagsJSON)
		}
	}

	if err := s.db.Create(client).Error; err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Parse tags for response
	if client.Tags != "" {
		json.Unmarshal([]byte(client.Tags), &client.TagsList)
	}

	return client, nil
}

// getValidCurrency ensures currency is valid
func getValidCurrency(currency string) string {
	valid := map[string]bool{
		"KES": true, "USD": true, "EUR": true, "GBP": true,
		"TZS": true, "UGX": true, "NGN": true,
	}

	currency = strings.ToUpper(strings.TrimSpace(currency))
	if valid[currency] {
		return currency
	}
	return "KES" // Default
}

// getValidPaymentTerms ensures payment terms is valid
func getValidPaymentTerms(terms int) int {
	if terms < 0 {
		return 0
	}
	if terms > 365 {
		return 365 // Max 1 year
	}
	if terms == 0 {
		return 30 // Default
	}
	return terms
}

func getValidPaymentMethod(method string) string {
	validMethods := map[string]bool{
		"mpesa": true,
		"bank":  true,
		"card":  true,
		"cash":  true,
	}

	method = strings.ToLower(strings.TrimSpace(method))
	if validMethods[method] {
		return method
	}
	return "mpesa" // Default
}

// GetClient retrieves a client by ID (tenant-scoped)
func (s *ClientService) GetClient(tenantID, clientID string) (*models.Client, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, fmt.Errorf("client ID is required")
	}

	var client models.Client
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Preload("Invoices", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at DESC")
		}).Preload("Invoices.Items").Preload("Invoices.Payments").
		First(&client, "id = ?", clientID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("client not found")
		}
		return nil, fmt.Errorf("failed to fetch client: %w", err)
	}

	// Calculate totals
	var totalBilled, totalPaid float64
	for _, inv := range client.Invoices {
		totalBilled += inv.Total
		totalPaid += inv.PaidAmount
	}
	client.TotalBilled = totalBilled
	client.TotalPaid = totalPaid

	return &client, nil
}

// GetUserClients retrieves all clients for a tenant (tenant-scoped)
func (s *ClientService) GetUserClients(tenantID string, filter ClientFilter) ([]models.Client, int64, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, 0, fmt.Errorf("tenant ID is required")
	}

	var clients []models.Client
	var total int64

	query := s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Client{})

	// Apply filters safely
	if filter.Search != "" {
		search := "%" + strings.TrimSpace(filter.Search) + "%"
		if s.db.IsPostgres() {
			query = query.Where("name ILIKE ? OR email ILIKE ? OR phone ILIKE ?", search, search, search)
		} else {
			query = query.Where("LOWER(name) LIKE LOWER(?) OR LOWER(email) LIKE LOWER(?) OR LOWER(phone) LIKE LOWER(?)", search, search, search)
		}
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count clients: %w", err)
	}

	// Apply pagination and ordering
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit)

	if err := query.Find(&clients).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch clients: %w", err)
	}

	// PERFORMANCE FIX: Use Preload to fetch all invoices in single query instead of N+1
	// SECURITY: Added tenant filter to prevent cross-tenant data leakage
	var invoices []models.Invoice
	if err := s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Find(&invoices).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch invoices: %w", err)
	}

	// Build invoice lookup map for O(1) access
	invoiceMap := make(map[string][]models.Invoice)
	for _, inv := range invoices {
		invoiceMap[inv.ClientID] = append(invoiceMap[inv.ClientID], inv)
	}

	// Calculate totals using the preloaded data
	for i := range clients {
		clientInvoices := invoiceMap[clients[i].ID]
		var totalBilled, totalPaid float64
		for _, inv := range clientInvoices {
			totalBilled += inv.Total
			totalPaid += inv.PaidAmount
		}
		clients[i].TotalBilled = totalBilled
		clients[i].TotalPaid = totalPaid
		clients[i].InvoiceCount = int64(len(clientInvoices))
		// Parse tags for response
		if clients[i].Tags != "" {
			json.Unmarshal([]byte(clients[i].Tags), &clients[i].TagsList)
		}
	}

	return clients, total, nil
}

// UpdateClient updates a client (tenant-scoped)
func (s *ClientService) UpdateClient(tenantID, clientID string, req *UpdateClientRequest) (*models.Client, error) {
	client, err := s.GetClient(tenantID, clientID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		client.Name = name
	}
	if req.Email != nil {
		client.Email = strings.TrimSpace(*req.Email)
	}
	if req.Phone != nil {
		client.Phone = normalizePhone(*req.Phone)
	}
	if req.Address != nil {
		client.Address = strings.TrimSpace(*req.Address)
	}
	if req.KRAPIN != nil {
		client.KRAPIN = strings.ToUpper(strings.TrimSpace(*req.KRAPIN))
	}
	if req.Currency != nil {
		client.Currency = getValidCurrency(*req.Currency)
	}
	if req.PaymentTerms != nil {
		client.PaymentTerms = getValidPaymentTerms(*req.PaymentTerms)
	}
	if req.Notes != nil {
		client.Notes = strings.TrimSpace(*req.Notes)
	}
	if req.DefaultPaymentMethod != nil {
		client.DefaultPaymentMethod = getValidPaymentMethod(*req.DefaultPaymentMethod)
	}
	if req.InternalNotes != nil {
		client.InternalNotes = strings.TrimSpace(*req.InternalNotes)
	}
	if req.Status != nil {
		client.Status = models.ClientStatus(*req.Status)
	}
	if req.Tags != nil {
		tagsJSON, err := json.Marshal(req.Tags)
		if err == nil {
			client.Tags = string(tagsJSON)
		}
	}

	if err := s.db.Save(client).Error; err != nil {
		return nil, fmt.Errorf("failed to update client: %w", err)
	}

	// Parse tags for response
	if client.Tags != "" {
		json.Unmarshal([]byte(client.Tags), &client.TagsList)
	}

	return client, nil
}

// DeleteClient deletes a client (tenant-scoped)
func (s *ClientService) DeleteClient(tenantID, clientID string) error {
	if strings.TrimSpace(tenantID) == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return fmt.Errorf("client ID is required")
	}

	// Check if client has any invoices (including draft)
	var count int64
	s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Invoice{}).Where("client_id = ?", clientID).Count(&count)
	if count > 0 {
		return fmt.Errorf("cannot delete client with existing invoices (%d invoices)", count)
	}

	result := s.db.Scopes(database.TenantFilter(tenantID)).
		Where("id = ?", clientID).Delete(&models.Client{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete client: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("client not found")
	}

	return nil
}

// GetClientStats returns statistics for a client (tenant-scoped)
func (s *ClientService) GetClientStats(tenantID, clientID string) (*ClientStats, error) {
	client, err := s.GetClient(tenantID, clientID)
	if err != nil {
		return nil, err
	}

	var stats ClientStats
	stats.Client = *client

	// Invoice counts - tenant-scoped
	s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Invoice{}).Where("client_id = ?", clientID).Count(&stats.TotalInvoices)
	s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Invoice{}).Where("client_id = ? AND status = ?", clientID, models.InvoiceStatusPaid).Count(&stats.PaidInvoices)
	s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Invoice{}).Where("client_id = ? AND status = ?", clientID, models.InvoiceStatusOverdue).Count(&stats.OverdueInvoices)

	// Calculate average payment time
	var payments []models.Payment
	s.db.Scopes(database.TenantFilter(tenantID)).
		Where("invoice_id IN ?",
			s.db.Model(&models.Invoice{}).Select("id").Where("client_id = ?", clientID),
		).Find(&payments)

	var totalDays float64
	var paidCount int64
	for _, p := range payments {
		if p.Status == "completed" && p.CompletedAt != nil {
			var invoice models.Invoice
			if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&invoice, p.InvoiceID).Error; err == nil {
				days := (*p.CompletedAt).Sub(invoice.CreatedAt).Hours() / 24
				totalDays += days
				paidCount++
			}
		}
	}

	if paidCount > 0 {
		stats.AveragePaymentDays = int(totalDays / float64(paidCount))
	}

	return &stats, nil
}

// Request types
type CreateClientRequest struct {
	Name         string   `json:"name" binding:"required"`
	Email        string   `json:"email"`
	Phone        string   `json:"phone"`
	Address      string   `json:"address"`
	KRAPIN       string   `json:"kra_pin"`
	Currency     string   `json:"currency"`
	PaymentTerms int      `json:"payment_terms"`
	Notes        string   `json:"notes"`
	Tags         []string `json:"tags"`
}

type UpdateClientRequest struct {
	Name                 *string  `json:"name"`
	Email                *string  `json:"email"`
	Phone                *string  `json:"phone"`
	Address              *string  `json:"address"`
	KRAPIN               *string  `json:"kra_pin"`
	Currency             *string  `json:"currency"`
	PaymentTerms         *int     `json:"payment_terms"`
	Notes                *string  `json:"notes"`
	DefaultPaymentMethod *string  `json:"default_payment_method"`
	InternalNotes        *string  `json:"internal_notes"`
	Status               *string  `json:"status"`
	Tags                 []string `json:"tags"`
}

type ClientFilter struct {
	Search string
	Status string
	Offset int
	Limit  int
}

type ClientStats struct {
	Client             models.Client
	TotalInvoices      int64 `json:"total_invoices"`
	PaidInvoices       int64 `json:"paid_invoices"`
	OverdueInvoices    int64 `json:"overdue_invoices"`
	AveragePaymentDays int   `json:"average_payment_days"`
}

// ClientDashboardStats holds aggregate stats for all clients
type ClientDashboardStats struct {
	TotalClients     int64   `json:"total_clients"`
	ActiveClients    int64   `json:"active_clients"`
	InactiveClients  int64   `json:"inactive_clients"`
	ArchivedClients  int64   `json:"archived_clients"`
	TotalRevenue     float64 `json:"total_revenue"`
	TotalOutstanding float64 `json:"total_outstanding"`
}

// GetClientDashboardStats returns aggregate statistics for all clients (tenant-scoped)
func (s *ClientService) GetClientDashboardStats(tenantID string) (*ClientDashboardStats, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}

	var stats ClientDashboardStats

	// Total clients
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Client{}).Count(&stats.TotalClients)

	// Count by status
	s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Client{}).
		Where("status = ?", models.ClientStatusActive).
		Count(&stats.ActiveClients)

	s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Client{}).
		Where("status = ?", models.ClientStatusInactive).
		Count(&stats.InactiveClients)

	s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Client{}).
		Where("status = ?", models.ClientStatusArchived).
		Count(&stats.ArchivedClients)

	// Total revenue (all paid invoices)
	s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Invoice{}).
		Where("status = ?", models.InvoiceStatusPaid).
		Select("COALESCE(SUM(total), 0)").
		Scan(&stats.TotalRevenue)

	// Total outstanding (unpaid invoices)
	s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Invoice{}).
		Where("status NOT IN ?", []string{string(models.InvoiceStatusPaid), string(models.InvoiceStatusCancelled), string(models.InvoiceStatusVoid), string(models.InvoiceStatusDraft)}).
		Select("COALESCE(SUM(total - paid_amount), 0)").
		Scan(&stats.TotalOutstanding)

	return &stats, nil
}

// GetClientInvoices retrieves all invoices for a client (tenant-scoped)
func (s *ClientService) GetClientInvoices(tenantID, clientID string, limit int) ([]models.Invoice, int64, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, 0, fmt.Errorf("tenant ID is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, 0, fmt.Errorf("client ID is required")
	}

	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var invoices []models.Invoice
	var total int64

	query := s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("client_id = ?", clientID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count invoices: %w", err)
	}

	if err := query.Preload("Items").Preload("Payments").Order("created_at DESC").Limit(limit).Find(&invoices).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch invoices: %w", err)
	}

	return invoices, total, nil
}

// GetClientPayments retrieves all payments for a client (tenant-scoped)
func (s *ClientService) GetClientPayments(tenantID, clientID string, limit int) ([]models.Payment, int64, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, 0, fmt.Errorf("tenant ID is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, 0, fmt.Errorf("client ID is required")
	}

	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var payments []models.Payment
	var total int64

	query := s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Payment{}).
		Joins("JOIN invoices ON invoices.id = payments.invoice_id AND invoices.client_id = ?", clientID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count payments: %w", err)
	}

	if err := query.Preload("Invoice").Order("created_at DESC").Limit(limit).Find(&payments).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch payments: %w", err)
	}

	return payments, total, nil
}

// GetClientActivity retrieves activity timeline for a client (tenant-scoped)
func (s *ClientService) GetClientActivity(tenantID, clientID string, limit int) ([]map[string]interface{}, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, fmt.Errorf("client ID is required")
	}

	if limit <= 0 || limit > 50 {
		limit = 20
	}

	activities := []map[string]interface{}{}

	// Get recent invoices
	var invoices []models.Invoice
	s.db.Scopes(database.TenantFilter(tenantID)).Where("client_id = ?", clientID).
		Order("created_at DESC").Limit(limit).Find(&invoices)

	for _, inv := range invoices {
		activities = append(activities, map[string]interface{}{
			"type":        "invoice_created",
			"description": "Invoice " + inv.InvoiceNumber + " created",
			"timestamp":   inv.CreatedAt,
			"data": map[string]interface{}{
				"invoice_id":     inv.ID,
				"invoice_number": inv.InvoiceNumber,
				"amount":         inv.Total,
				"status":         inv.Status,
			},
		})

		if inv.SentAt != nil {
			activities = append(activities, map[string]interface{}{
				"type":        "invoice_sent",
				"description": "Invoice " + inv.InvoiceNumber + " sent",
				"timestamp":   *inv.SentAt,
				"data": map[string]interface{}{
					"invoice_id":     inv.ID,
					"invoice_number": inv.InvoiceNumber,
				},
			})
		}

		if inv.PaidAt != nil {
			activities = append(activities, map[string]interface{}{
				"type":        "payment_received",
				"description": "Payment received for " + inv.InvoiceNumber,
				"timestamp":   *inv.PaidAt,
				"data": map[string]interface{}{
					"invoice_id":     inv.ID,
					"invoice_number": inv.InvoiceNumber,
					"amount":         inv.PaidAmount,
				},
			})
		}
	}

	// Sort by timestamp descending
	for i := 0; i < len(activities)-1; i++ {
		for j := i + 1; j < len(activities); j++ {
			iTime := activities[i]["timestamp"].(time.Time)
			jTime := activities[j]["timestamp"].(time.Time)
			if jTime.After(iTime) {
				activities[i], activities[j] = activities[j], activities[i]
			}
		}
	}

	if len(activities) > limit {
		activities = activities[:limit]
	}

	return activities, nil
}
