package services

import (
	"fmt"
	"mime/multipart"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ExpenseService struct {
	db          *database.DB
	attachmentService *ExpenseAttachmentService
}

func NewExpenseService(db *database.DB) *ExpenseService {
	return &ExpenseService{
		db: db,
		attachmentService: NewExpenseAttachmentService(db, "./uploads/expense_attachments"),
	}
}

type CreateExpenseRequest struct {
	CategoryID      string  `json:"category_id"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	Date            string  `json:"date"`
	Status          string  `json:"status"`
	PaymentMethod   string  `json:"payment_method"`
	Reference       string  `json:"reference"`
	Vendor          string  `json:"vendor"`
	TaxAmount       float64 `json:"tax_amount"`
	TaxRate         float64 `json:"tax_rate"`
	IsRecurring     bool    `json:"is_recurring"`
	RecurringPeriod string  `json:"recurring_period"`
	Notes           string  `json:"notes"`
}

type UpdateExpenseRequest struct {
	CategoryID      *string  `json:"category_id"`
	Title           *string  `json:"title"`
	Description     *string  `json:"description"`
	Amount          *float64 `json:"amount"`
	Currency        *string  `json:"currency"`
	Date            *string  `json:"date"`
	Status          *string  `json:"status"`
	PaymentMethod   *string  `json:"payment_method"`
	Reference       *string  `json:"reference"`
	Vendor          *string  `json:"vendor"`
	TaxAmount       *float64 `json:"tax_amount"`
	TaxRate         *float64 `json:"tax_rate"`
	IsRecurring     *bool    `json:"is_recurring"`
	RecurringPeriod *string  `json:"recurring_period"`
	Notes           *string  `json:"notes"`
	ApprovedBy      *string  `json:"approved_by"`
}

func (s *ExpenseService) CreateExpense(tenantID, userID string, req *CreateExpenseRequest) (*models.Expense, error) {
	expenseDate := time.Now()
	if req.Date != "" {
		expenseDate, _ = time.Parse("2006-01-02", req.Date)
	}

	expense := &models.Expense{
		ID:              uuid.New().String(),
		TenantID:        tenantID,
		CategoryID:      req.CategoryID,
		Title:           req.Title,
		Description:     req.Description,
		Amount:          req.Amount,
		Currency:        req.Currency,
		Date:            expenseDate,
		Status:          "pending",
		PaymentMethod:   req.PaymentMethod,
		Reference:       req.Reference,
		Vendor:          req.Vendor,
		TaxAmount:       req.TaxAmount,
		TaxRate:         req.TaxRate,
		IsRecurring:     req.IsRecurring,
		RecurringPeriod: req.RecurringPeriod,
		Notes:           req.Notes,
		CreatedBy:       userID,
	}

	if req.Status != "" {
		expense.Status = req.Status
	}

	if err := s.db.Create(expense).Error; err != nil {
		return nil, fmt.Errorf("failed to create expense: %w", err)
	}

	return expense, nil
}

func (s *ExpenseService) GetExpenses(tenantID string, filters map[string]interface{}) ([]models.Expense, int64, error) {
	query := s.db.Where("tenant_id = ?", tenantID)

	if categoryID, ok := filters["category_id"].(string); ok && categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}
	if status, ok := filters["status"].(string); ok && status != "" {
		query = query.Where("status = ?", status)
	}
	
	// Date range
	startDate := ""
	endDate := ""
	
	// Handle "this_month" quick filter
	if period, ok := filters["period"].(string); ok && period != "" {
		now := time.Now()
		if period == "today" {
			startDate = now.Format("2006-01-02")
			endDate = now.Format("2006-01-02")
		} else if period == "week" {
			startOfWeek := now
			startOfWeek.Weekday()
			day := int(now.Weekday())
			startOfWeek = now.AddDate(0, 0, -day)
			startDate = startOfWeek.Format("2006-01-02")
			endDate = now.Format("2006-01-02")
		} else if period == "month" {
			startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			startDate = startOfMonth.Format("2006-01-02")
			endDate = now.Format("2006-01-02")
		}
	}
	
	if sd, ok := filters["start_date"].(string); ok && sd != "" {
		startDate = sd
	}
	if ed, ok := filters["end_date"].(string); ok && ed != "" {
		endDate = ed
	}
	
	if startDate != "" {
		parsed, _ := time.Parse("2006-01-02", startDate)
		query = query.Where("date >= ?", parsed)
	}
	if endDate != "" {
		parsed, _ := time.Parse("2006-01-02", endDate)
		query = query.Where("date <= ?", parsed)
	}
	
	// Amount range
	if minAmt, ok := filters["min_amount"].(float64); ok && minAmt > 0 {
		query = query.Where("amount >= ?", minAmt)
	}
	if maxAmt, ok := filters["max_amount"].(float64); ok && maxAmt > 0 {
		query = query.Where("amount <= ?", maxAmt)
	}
	
	if search, ok := filters["search"].(string); ok && search != "" {
		query = query.Where("title ILIKE ? OR description ILIKE ? OR vendor ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	var total int64
	query.Model(&models.Expense{}).Count(&total)

	var expenses []models.Expense
	page := 1
	limit := 15
	if p, ok := filters["page"].(int); ok && p > 0 {
		page = p
	}
	if l, ok := filters["limit"].(int); ok && l > 0 {
		limit = l
	}

	offset := (page - 1) * limit
	if err := query.Order("date desc").Offset(offset).Limit(limit).Find(&expenses).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get expenses: %w", err)
	}

	return expenses, total, nil
}

func (s *ExpenseService) GetExpenseByID(tenantID, expenseID string) (*models.Expense, error) {
	var expense models.Expense
	if err := s.db.Where("id = ? AND tenant_id = ?", expenseID, tenantID).First(&expense).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("expense not found")
		}
		return nil, fmt.Errorf("failed to get expense: %w", err)
	}
	return &expense, nil
}

func (s *ExpenseService) UpdateExpense(tenantID, expenseID string, req *UpdateExpenseRequest) (*models.Expense, error) {
	expense, err := s.GetExpenseByID(tenantID, expenseID)
	if err != nil {
		return nil, err
	}

	if req.CategoryID != nil {
		expense.CategoryID = *req.CategoryID
	}
	if req.Title != nil {
		expense.Title = *req.Title
	}
	if req.Description != nil {
		expense.Description = *req.Description
	}
	if req.Amount != nil {
		expense.Amount = *req.Amount
	}
	if req.Currency != nil {
		expense.Currency = *req.Currency
	}
	if req.Date != nil {
		expense.Date, _ = time.Parse("2006-01-02", *req.Date)
	}
	if req.Status != nil {
		expense.Status = *req.Status
		if *req.Status == "approved" {
			now := time.Now()
			expense.ApprovedAt = &now
		}
		if *req.Status == "paid" {
			now := time.Now()
			expense.PaidAt = &now
		}
	}
	if req.PaymentMethod != nil {
		expense.PaymentMethod = *req.PaymentMethod
	}
	if req.Reference != nil {
		expense.Reference = *req.Reference
	}
	if req.Vendor != nil {
		expense.Vendor = *req.Vendor
	}
	if req.TaxAmount != nil {
		expense.TaxAmount = *req.TaxAmount
	}
	if req.TaxRate != nil {
		expense.TaxRate = *req.TaxRate
	}
	if req.IsRecurring != nil {
		expense.IsRecurring = *req.IsRecurring
	}
	if req.RecurringPeriod != nil {
		expense.RecurringPeriod = *req.RecurringPeriod
	}
	if req.Notes != nil {
		expense.Notes = *req.Notes
	}
	if req.ApprovedBy != nil {
		expense.ApprovedBy = *req.ApprovedBy
		now := time.Now()
		expense.ApprovedAt = &now
	}

	if err := s.db.Save(expense).Error; err != nil {
		return nil, fmt.Errorf("failed to update expense: %w", err)
	}

	return expense, nil
}

func (s *ExpenseService) DeleteExpense(tenantID, expenseID string) error {
	result := s.db.Where("id = ? AND tenant_id = ?", expenseID, tenantID).Delete(&models.Expense{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete expense: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("expense not found")
	}
	return nil
}

func (s *ExpenseService) GetExpenseCategories(tenantID string) ([]models.ExpenseCategory, error) {
	var categories []models.ExpenseCategory
	if err := s.db.Where("tenant_id = ? AND is_active = ?", tenantID, true).Order("name").Find(&categories).Error; err != nil {
		return nil, fmt.Errorf("failed to get categories: %w", err)
	}
	return categories, nil
}

func (s *ExpenseService) CreateCategory(tenantID string, name, description string) (*models.ExpenseCategory, error) {
	category := &models.ExpenseCategory{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		Name:        name,
		Description: description,
		IsActive:    true,
	}
	if err := s.db.Create(category).Error; err != nil {
		return nil, fmt.Errorf("failed to create category: %w", err)
	}
	return category, nil
}

func (s *ExpenseService) GetExpenseSummary(tenantID, period string) (map[string]interface{}, error) {
	now := time.Now()
	var startDate, endDate string
	
	// Handle period quick filter for filtering results
	if period == "today" {
		startDate = now.Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	} else if period == "week" {
		weekday := int(now.Weekday())
		startOfWeek := now.AddDate(0, 0, -weekday)
		startDate = startOfWeek.Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	} else if period == "month" {
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		startDate = startOfMonth.Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}

	// Get total all time
	var totalAll float64
	err := s.db.Model(&models.Expense{}).Where("tenant_id = ? AND status IN ('approved', 'paid')", tenantID).
		Select("COALESCE(SUM(amount), 0)").Scan(&totalAll).Error
	if err != nil {
		totalAll = 0
	}

	// ALWAYS get this month's total (regardless of period filter)
	thisMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	thisMonthEnd := now
	var thisMonth float64
	err = s.db.Model(&models.Expense{}).Where("tenant_id = ? AND status IN ('approved', 'paid') AND date >= ? AND date <= ?", tenantID, thisMonthStart, thisMonthEnd).
		Select("COALESCE(SUM(amount), 0)").Scan(&thisMonth).Error
	if err != nil {
		thisMonth = 0
	}

	// Get counts by status - use period if provided
	var pending, approved, paid, rejected int64
	if startDate != "" && endDate != "" {
		parsed, _ := time.Parse("2006-01-02", startDate)
		parsed2, _ := time.Parse("2006-01-02", endDate)
		s.db.Model(&models.Expense{}).Where("tenant_id = ? AND date >= ? AND date <= ?", tenantID, parsed, parsed2).Where("status = ?", "pending").Count(&pending)
		s.db.Model(&models.Expense{}).Where("tenant_id = ? AND date >= ? AND date <= ?", tenantID, parsed, parsed2).Where("status = ?", "approved").Count(&approved)
		s.db.Model(&models.Expense{}).Where("tenant_id = ? AND date >= ? AND date <= ?", tenantID, parsed, parsed2).Where("status = ?", "paid").Count(&paid)
		s.db.Model(&models.Expense{}).Where("tenant_id = ? AND date >= ? AND date <= ?", tenantID, parsed, parsed2).Where("status = ?", "rejected").Count(&rejected)
	} else {
		s.db.Model(&models.Expense{}).Where("tenant_id = ?", tenantID).Where("status = ?", "pending").Count(&pending)
		s.db.Model(&models.Expense{}).Where("tenant_id = ?", tenantID).Where("status = ?", "approved").Count(&approved)
		s.db.Model(&models.Expense{}).Where("tenant_id = ?", tenantID).Where("status = ?", "paid").Count(&paid)
		s.db.Model(&models.Expense{}).Where("tenant_id = ?", tenantID).Where("status = ?", "rejected").Count(&rejected)
	}

	// Get by category with names
	type categoryResult struct {
		CategoryID   string
		CategoryName string
		Total       float64
	}
	byCategory := make(map[string]float64)
	byCategoryRaw := make(map[string]string)
	
	var results []categoryResult
	query := s.db.Model(&models.Expense{}).Where("tenant_id = ? AND status IN ('approved', 'paid')", tenantID)
	if startDate != "" && endDate != "" {
		parsed, _ := time.Parse("2006-01-02", startDate)
		parsed2, _ := time.Parse("2006-01-02", endDate)
		query = query.Where("date >= ? AND date <= ?", parsed, parsed2)
	}
	query.Select("category_id, SUM(amount) as total").Group("category_id").Find(&results)
	
	for _, r := range results {
		catName := "Uncategorized"
		if r.CategoryID != "" {
			var cat models.ExpenseCategory
			if err := s.db.Where("id = ?", r.CategoryID).First(&cat).Error; err == nil {
				catName = cat.Name
			}
		}
		byCategory[catName] = r.Total
		byCategoryRaw[r.CategoryID] = catName
	}

	return map[string]interface{}{
		"total":          totalAll,
		"this_month":    thisMonth,
		"pending":        pending,
		"approved":       approved,
		"paid":           paid,
		"rejected":       rejected,
		"by_category":     byCategory,
		"by_category_raw": byCategoryRaw,
	}, nil
}

func (s *ExpenseService) GetExpensesByCategory(tenantID, startDate, endDate string) (map[string]float64, error) {
	query := s.db.Model(&models.Expense{}).Where("tenant_id = ? AND status IN ('approved', 'paid')", tenantID)

	if startDate != "" {
		parsed, _ := time.Parse("2006-01-02", startDate)
		query = query.Where("date >= ?", parsed)
	}
	if endDate != "" {
		parsed, _ := time.Parse("2006-01-02", endDate)
		query = query.Where("date <= ?", parsed)
	}

	type categoryTotal struct {
		CategoryID string
		Total      float64
	}

	var results []categoryTotal
	if err := query.Select("category_id, SUM(amount) as total").Group("category_id").Find(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get by category: %w", err)
	}

	result := make(map[string]float64)
	for _, r := range results {
		result[r.CategoryID] = r.Total
	}
	return result, nil
}

// UploadExpenseAttachment handles file upload for an expense
func (s *ExpenseService) UploadExpenseAttachment(tenantID, expenseID string, fileHeader *multipart.FileHeader, c *fiber.Ctx) (*models.ExpenseAttachment, error) {
	return s.attachmentService.UploadFile(tenantID, expenseID, fileHeader, c)
}

// GetExpenseAttachments retrieves all attachments for an expense
func (s *ExpenseService) GetExpenseAttachments(tenantID, expenseID string) ([]models.ExpenseAttachment, error) {
	return s.attachmentService.GetAttachments(tenantID, expenseID)
}

// DeleteExpenseAttachment removes an attachment from an expense
func (s *ExpenseService) DeleteExpenseAttachment(tenantID, attachmentID string) error {
	return s.attachmentService.DeleteAttachment(tenantID, attachmentID)
}

// GetExpenseAttachmentByID retrieves a single attachment by ID
func (s *ExpenseService) GetExpenseAttachmentByID(tenantID, attachmentID string) (*models.ExpenseAttachment, error) {
	return s.attachmentService.GetAttachmentByID(tenantID, attachmentID)
}
