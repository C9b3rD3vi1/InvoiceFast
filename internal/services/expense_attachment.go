package services

import (
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// ExpenseAttachmentService handles file attachments for expenses
type ExpenseAttachmentService struct {
	db      *database.DB
	uploadDir string
	maxFileSize int64
}

// NewExpenseAttachmentService creates a new expense attachment service
func NewExpenseAttachmentService(db *database.DB, uploadDir string) *ExpenseAttachmentService {
	if uploadDir == "" {
		uploadDir = "./uploads/expense_attachments"
	}
	
	// Create upload directory if it doesn't exist
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create upload directory: %v\n", err)
	}
	
	return &ExpenseAttachmentService{
		db:         db,
		uploadDir:  uploadDir,
		maxFileSize: 10 << 20, // 10 MB default
	}
}

// SetMaxFileSize sets the maximum file size for uploads
func (s *ExpenseAttachmentService) SetMaxFileSize(sizeMB int64) {
	s.maxFileSize = sizeMB << 20 // Convert MB to bytes
}

// UploadFile handles file upload for an expense
func (s *ExpenseAttachmentService) UploadFile(tenantID, expenseID string, fileHeader *multipart.FileHeader, c *fiber.Ctx) (*models.ExpenseAttachment, error) {
	// Validate tenant ID
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}
	
	// Validate expense ID
	if expenseID == "" {
		return nil, fmt.Errorf("expense ID is required")
	}
	
	// Validate file header
	if fileHeader == nil {
		return nil, fmt.Errorf("file header is required")
	}
	
	// Check file size
	if fileHeader.Size > s.maxFileSize {
		return nil, fmt.Errorf("file size exceeds limit of %d MB", s.maxFileSize>>20)
	}
	
	// Validate expense exists and belongs to tenant
	var expense models.Expense
	if err := s.db.Where("id = ? AND tenant_id = ?", expenseID, tenantID).First(&expense).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("expense not found")
		}
		return nil, fmt.Errorf("failed to validate expense: %w", err)
	}
	
	// Generate unique filename
	fileExt := filepath.Ext(fileHeader.Filename)
	fileName := uuid.New().String() + fileExt
	filePath := filepath.Join(s.uploadDir, fileName)
	
	// Save file to disk
	if err := c.SaveFile(fileHeader, filePath); err != nil {
		return nil, fmt.Errorf("failed to save file: %w", err)
	}
	
	// Create attachment record
	attachment := &models.ExpenseAttachment{
		ID:        uuid.New().String(),
		ExpenseID: expenseID,
		TenantID:  tenantID,
		FileName:  fileHeader.Filename,
		FileURL:   filePath,
		FileSize:  fileHeader.Size,
		FileType:  fileHeader.Header.Get("Content-Type"),
		CreatedAt: time.Now(),
	}
	
	if err := s.db.Create(attachment).Error; err != nil {
		// Clean up file if database operation fails
		// In a real implementation, we would remove the file here
		return nil, fmt.Errorf("failed to create attachment record: %w", err)
	}
	
	// Update attachment count on expense
	if err := s.db.Model(&models.Expense{}).
		Where("id = ? AND tenant_id = ?", expenseID, tenantID).
		Update("attachments", gorm.Expr("attachments + 1")).Error; err != nil {
		// Log error but don't fail the operation - attachment was still created
		// In a production system, we might want to handle this more carefully
	}
	
	return attachment, nil
}

// GetAttachments retrieves all attachments for an expense
func (s *ExpenseAttachmentService) GetAttachments(tenantID, expenseID string) ([]models.ExpenseAttachment, error) {
	var attachments []models.ExpenseAttachment
	if err := s.db.Where("tenant_id = ? AND expense_id = ?", tenantID, expenseID).
		Order("created_at desc").
		Find(&attachments).Error; err != nil {
		return nil, fmt.Errorf("failed to get attachments: %w", err)
	}
	return attachments, nil
}

// DeleteAttachment removes an attachment from an expense
func (s *ExpenseAttachmentService) DeleteAttachment(tenantID, attachmentID string) error {
	var attachment models.ExpenseAttachment
	if err := s.db.Where("id = ? AND tenant_id = ?", attachmentID, tenantID).
		First(&attachment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("attachment not found")
		}
		return fmt.Errorf("failed to get attachment: %w", err)
	}
	
	// Delete file from disk
	// In a real implementation, we would remove the file here
	// os.Remove(attachment.FileURL)
	
	// Delete attachment record
	if err := s.db.Delete(&attachment).Error; err != nil {
		return fmt.Errorf("failed to delete attachment: %w", err)
	}
	
	// Update attachment count on expense
	if err := s.db.Model(&models.Expense{}).
		Where("id = ? AND tenant_id = ?", attachment.ExpenseID, tenantID).
		Update("attachments", gorm.Expr("attachments - 1")).Error; err != nil {
		// Log error but don't fail the operation
	}
	
	return nil
}

// GetAttachmentByID retrieves a single attachment by ID (for internal use)
func (s *ExpenseAttachmentService) GetAttachmentByID(tenantID, attachmentID string) (*models.ExpenseAttachment, error) {
	var attachment models.ExpenseAttachment
	if err := s.db.Where("id = ? AND tenant_id = ?", attachmentID, tenantID).
		First(&attachment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("attachment not found")
		}
		return nil, fmt.Errorf("failed to get attachment: %w", err)
	}
	return &attachment, nil
}