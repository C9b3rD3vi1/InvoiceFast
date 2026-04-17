package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Attachment represents a file attached to an invoice
type Attachment struct {
	ID          string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID    string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	InvoiceID   string    `json:"invoice_id" gorm:"type:uuid;index;not null"`
	FileName    string    `json:"file_name" gorm:"not null"`
	FileSize    int64     `json:"file_size" gorm:"not null"`    // in bytes
	ContentType string    `json:"content_type" gorm:"not null"` // MIME type
	FileURL     string    `json:"file_url" gorm:"not null"`     // Path to stored file
	UploadedAt  time.Time `json:"uploaded_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Relations
	Invoice Invoice `json:"-" gorm:"foreignKey:InvoiceID"`
}

// BeforeCreate hook to generate UUID
func (a *Attachment) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
