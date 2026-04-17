package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ItemLibrary represents a saved product/service for quick reuse
type ItemLibrary struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID  string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	UserID    string    `json:"user_id" gorm:"type:uuid;index;not null"`
	Name      string    `json:"name" gorm:"not null"`        // Item name/description
	UnitPrice float64   `json:"unit_price" gorm:"not null"`  // Default price
	Unit      string    `json:"unit"`                        // e.g., "hours", "items", "pieces"
	Taxable   bool      `json:"taxable" gorm:"default:true"` // Whether item is taxable
	Notes     string    `json:"notes"`                       // Optional notes
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate hook to generate UUID
func (i *ItemLibrary) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	return nil
}
