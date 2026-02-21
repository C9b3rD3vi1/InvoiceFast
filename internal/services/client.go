package services

import (
	"errors"
	"fmt"
	"strings"

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

// CreateClient creates a new client
func (s *ClientService) CreateClient(userID string, req *CreateClientRequest) (*models.Client, error) {
	// Validate inputs
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user ID is required")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("client name is required")
	}

	client := &models.Client{
		ID:           uuid.New().String(),
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

	if err := s.db.Create(client).Error; err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
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

// GetClient retrieves a client by ID
func (s *ClientService) GetClient(clientID, userID string) (*models.Client, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("client ID and user ID are required")
	}

	var client models.Client
	err := s.db.Preload("Invoices", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at DESC")
	}).Preload("Invoices.Items").Preload("Invoices.Payments").
		First(&client, "id = ? AND user_id = ?", clientID, userID).Error
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

// GetUserClients retrieves all clients for a user
func (s *ClientService) GetUserClients(userID string, filter ClientFilter) ([]models.Client, int64, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, 0, fmt.Errorf("user ID is required")
	}

	var clients []models.Client
	var total int64

	query := s.db.Model(&models.Client{}).Where("user_id = ?", userID)

	// Apply filters safely
	if filter.Search != "" {
		search := "%" + strings.TrimSpace(filter.Search) + "%"
		query = query.Where("name ILIKE ? OR email ILIKE ? OR phone ILIKE ?", search, search, search)
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

	// Calculate totals for each client
	for i := range clients {
		var invoices []models.Invoice
		s.db.Model(&models.Invoice{}).Where("client_id = ?", clients[i].ID).Find(&invoices)

		var totalBilled, totalPaid float64
		for _, inv := range invoices {
			totalBilled += inv.Total
			totalPaid += inv.PaidAmount
		}
		clients[i].TotalBilled = totalBilled
		clients[i].TotalPaid = totalPaid
	}

	return clients, total, nil
}

// UpdateClient updates a client
func (s *ClientService) UpdateClient(clientID, userID string, req *UpdateClientRequest) (*models.Client, error) {
	client, err := s.GetClient(clientID, userID)
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

	if err := s.db.Save(client).Error; err != nil {
		return nil, fmt.Errorf("failed to update client: %w", err)
	}

	return client, nil
}

// DeleteClient deletes a client
func (s *ClientService) DeleteClient(clientID, userID string) error {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(userID) == "" {
		return fmt.Errorf("client ID and user ID are required")
	}

	// Check if client has any invoices (including draft)
	var count int64
	s.db.Model(&models.Invoice{}).Where("client_id = ? AND user_id = ?", clientID, userID).Count(&count)
	if count > 0 {
		return fmt.Errorf("cannot delete client with existing invoices (%d invoices)", count)
	}

	result := s.db.Where("id = ? AND user_id = ?", clientID, userID).Delete(&models.Client{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete client: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("client not found")
	}

	return nil
}

// GetClientStats returns statistics for a client
func (s *ClientService) GetClientStats(clientID, userID string) (*ClientStats, error) {
	client, err := s.GetClient(clientID, userID)
	if err != nil {
		return nil, err
	}

	var stats ClientStats
	stats.Client = *client

	// Invoice counts
	s.db.Model(&models.Invoice{}).Where("client_id = ?", clientID).Count(&stats.TotalInvoices)
	s.db.Model(&models.Invoice{}).Where("client_id = ? AND status = ?", clientID, models.InvoiceStatusPaid).Count(&stats.PaidInvoices)
	s.db.Model(&models.Invoice{}).Where("client_id = ? AND status = ?", clientID, models.InvoiceStatusOverdue).Count(&stats.OverdueInvoices)

	// Calculate average payment time
	var payments []models.Payment
	s.db.Where("invoice_id IN ?",
		s.db.Model(&models.Invoice{}).Select("id").Where("client_id = ?", clientID),
	).Find(&payments)

	var totalDays float64
	var paidCount int64
	for _, p := range payments {
		if p.Status == "completed" && p.CompletedAt.Valid {
			var invoice models.Invoice
			if err := s.db.First(&invoice, p.InvoiceID).Error; err == nil {
				days := p.CompletedAt.Time.Sub(invoice.CreatedAt).Hours() / 24
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
	Name         string `json:"name" binding:"required"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Address      string `json:"address"`
	KRAPIN       string `json:"kra_pin"`
	Currency     string `json:"currency"`
	PaymentTerms int    `json:"payment_terms"`
	Notes        string `json:"notes"`
}

type UpdateClientRequest struct {
	Name         *string `json:"name"`
	Email        *string `json:"email"`
	Phone        *string `json:"phone"`
	Address      *string `json:"address"`
	KRAPIN       *string `json:"kra_pin"`
	Currency     *string `json:"currency"`
	PaymentTerms *int    `json:"payment_terms"`
	Notes        *string `json:"notes"`
}

type ClientFilter struct {
	Search string
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
