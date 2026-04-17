package services

import (
	"errors"
	"fmt"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"gorm.io/gorm"
)

// ItemLibraryService handles item library operations
type ItemLibraryService struct {
	db *database.DB
}

// NewItemLibraryService creates a new item library service
func NewItemLibraryService(db *database.DB) *ItemLibraryService {
	return &ItemLibraryService{db: db}
}

// CreateItem saves a new item to the library
func (s *ItemLibraryService) CreateItem(tenantID, userID string, req *CreateItemRequest) (*models.ItemLibrary, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if userID == "" {
		return nil, errors.New("user_id is required")
	}

	item := &models.ItemLibrary{
		TenantID:  tenantID,
		UserID:    userID,
		Name:      req.Name,
		UnitPrice: req.UnitPrice,
		Unit:      req.Unit,
		Taxable:   false,
		Notes:     "",
	}
	if req.Taxable != nil {
		item.Taxable = *req.Taxable
	}
	if req.Notes != nil {
		item.Notes = *req.Notes
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(item).Error; err != nil {
			return fmt.Errorf("failed to create item: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return item, nil
}

// GetItems retrieves all items for a tenant with optional search
func (s *ItemLibraryService) GetItems(tenantID string, search string) ([]models.ItemLibrary, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	var items []models.ItemLibrary
	query := s.db.Where("tenant_id = ?", tenantID)

	if search != "" {
		searchTerm := "%" + search + "%"
		query = query.Where("name ILIKE ?", searchTerm)
	}

	if err := query.Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to get items: %w", err)
	}

	return items, nil
}

// GetItemByID retrieves a specific item by ID
func (s *ItemLibraryService) GetItemByID(tenantID, itemID string) (*models.ItemLibrary, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if itemID == "" {
		return nil, errors.New("item_id is required")
	}

	var item models.ItemLibrary
	if err := s.db.Where("tenant_id = ? AND id = ?", tenantID, itemID).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("item not found")
		}
		return nil, fmt.Errorf("failed to get item: %w", err)
	}

	return &item, nil
}

// UpdateItem updates an existing item
func (s *ItemLibraryService) UpdateItem(tenantID, itemID string, req *UpdateItemRequest) (*models.ItemLibrary, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if itemID == "" {
		return nil, errors.New("item_id is required")
	}

	item, err := s.GetItemByID(tenantID, itemID)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if req.Name != nil {
		item.Name = *req.Name
	}
	if req.UnitPrice != nil {
		item.UnitPrice = *req.UnitPrice
	}
	if req.Unit != nil {
		item.Unit = *req.Unit
	}
	if req.Taxable != nil {
		item.Taxable = *req.Taxable
	}
	if req.Notes != nil {
		item.Notes = *req.Notes
	}

	if err := s.db.Save(item).Error; err != nil {
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

	return item, nil
}

// DeleteItem removes an item from the library
func (s *ItemLibraryService) DeleteItem(tenantID, itemID string) error {
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}
	if itemID == "" {
		return errors.New("item_id is required")
	}

	result := s.db.Where("tenant_id = ? AND id = ?", tenantID, itemID).Delete(&models.ItemLibrary{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete item: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("item not found")
	}

	return nil
}

// Request types
type CreateItemRequest struct {
	Name      string  `json:"name" binding:"required"`
	UnitPrice float64 `json:"unit_price" binding:"required,min=0"`
	Unit      string  `json:"unit"`
	Taxable   *bool   `json:"taxable"`
	Notes     *string `json:"notes"`
}

type UpdateItemRequest struct {
	Name      *string  `json:"name"`
	UnitPrice *float64 `json:"unit_price"`
	Unit      *string  `json:"unit"`
	Taxable   *bool    `json:"taxable"`
	Notes     *string  `json:"notes"`
}
