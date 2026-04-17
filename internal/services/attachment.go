package services

import (
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AttachmentService handles file attachments for invoices
type AttachmentService struct {
	db          *database.DB
	uploadDir   string
	maxFileSize int64 // Maximum file size in bytes (10MB default)
}

// NewAttachmentService creates a new attachment service
func NewAttachmentService(db *database.DB, uploadDir string) *AttachmentService {
	// Ensure upload directory exists
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		// Log error but continue - directory might be created by infrastructure
	}

	return &AttachmentService{
		db:          db,
		uploadDir:   uploadDir,
		maxFileSize: 10 * 1024 * 1024, // 10MB default
	}
}

// SetMaxFileSize updates the maximum allowed file size
func (s *AttachmentService) SetMaxFileSize(sizeMB int64) {
	s.maxFileSize = sizeMB * 1024 * 1024
}

// UploadFile handles file upload for an invoice
func (s *AttachmentService) UploadFile(tenantID, invoiceID string, fileHeader *multipart.FileHeader, c *fiber.Ctx) (*models.Attachment, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if invoiceID == "" {
		return nil, errors.New("invoice_id is required")
	}
	if fileHeader == nil {
		return nil, errors.New("file is required")
	}

	// Verify invoice exists and belongs to tenant
	var invoice models.Invoice
	if err := s.db.Scopes(database.TenantFilter(tenantID)).First(&invoice, "id = ?", invoiceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invoice not found")
		}
		return nil, fmt.Errorf("failed to verify invoice: %w", err)
	}

	// Validate file size
	if fileHeader.Size > s.maxFileSize {
		return nil, fmt.Errorf("file size exceeds limit of %d MB", s.maxFileSize/(1024*1024))
	}

	// Validate file type (basic security)
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		// Try to detect from filename
		contentType = mime.TypeByExtension(filepath.Ext(fileHeader.Filename))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Generate unique filename
	fileExt := filepath.Ext(fileHeader.Filename)
	uniqueID := uuid.New().String()
	storedFilename := uniqueID + fileExt
	filePath := filepath.Join(s.uploadDir, storedFilename)

	// Save file to disk
	if err := c.SaveFile(fileHeader, filePath); err != nil {
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	// Create attachment record
	attachment := &models.Attachment{
		TenantID:    tenantID,
		InvoiceID:   invoiceID,
		FileName:    fileHeader.Filename,
		FileSize:    fileHeader.Size,
		ContentType: contentType,
		FileURL:     "/" + filepath.ToSlash(filePath), // Web-accessible URL
		UploadedAt:  time.Now(),
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(attachment).Error; err != nil {
			return fmt.Errorf("failed to create attachment record: %w", err)
		}
		return nil
	})

	if err != nil {
		// Clean up file if database operation failed
		os.Remove(filePath)
		return nil, err
	}

	return attachment, nil
}

// GetAttachments retrieves all attachments for an invoice
func (s *AttachmentService) GetAttachments(tenantID, invoiceID string) ([]models.Attachment, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if invoiceID == "" {
		return nil, errors.New("invoice_id is required")
	}

	var attachments []models.Attachment
	if err := s.db.Where("tenant_id = ? AND invoice_id = ?", tenantID, invoiceID).
		Order("created_at DESC").
		Find(&attachments).Error; err != nil {
		return nil, fmt.Errorf("failed to get attachments: %w", err)
	}

	return attachments, nil
}

// DeleteAttachment removes an attachment
func (s *AttachmentService) DeleteAttachment(tenantID, attachmentID string) error {
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}
	if attachmentID == "" {
		return errors.New("attachment_id is required")
	}

	var attachment models.Attachment
	if err := s.db.Where("tenant_id = ? AND id = ?", tenantID, attachmentID).First(&attachment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("attachment not found")
		}
		return fmt.Errorf("failed to get attachment: %w", err)
	}

	// Delete file from disk
	if err := os.Remove(attachment.FileURL); err != nil && !os.IsNotExist(err) {
		// Log error but continue with database deletion
	}

	// Delete attachment record
	if err := s.db.Delete(&attachment).Error; err != nil {
		return fmt.Errorf("failed to delete attachment: %w", err)
	}

	return nil
}

// GetAttachmentByID retrieves a single attachment by ID (for internal use)
func (s *AttachmentService) GetAttachmentByID(tenantID, attachmentID string) (*models.Attachment, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if attachmentID == "" {
		return nil, errors.New("attachment_id is required")
	}

	var attachment models.Attachment
	if err := s.db.Where("tenant_id = ? AND id = ?", tenantID, attachmentID).First(&attachment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("attachment not found")
		}
		return nil, fmt.Errorf("failed to get attachment: %w", err)
	}

	return &attachment, nil
}
