package handlers

import (
	"fmt"
	"strconv"
	"strings"

	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ExpenseHandler struct {
	expenseService *services.ExpenseService
}

func NewExpenseHandler(expenseService *services.ExpenseService) *ExpenseHandler {
	return &ExpenseHandler{expenseService: expenseService}
}

func (h *ExpenseHandler) CreateExpense(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	userID := middleware.GetUserID(c)

	var req services.CreateExpenseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Title == "" || req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "title and amount required"})
	}

	expense, err := h.expenseService.CreateExpense(tenantID, userID, &req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(expense)
}

func (h *ExpenseHandler) GetExpenses(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	filters := map[string]interface{}{
		"category_id": c.Query("category_id"),
		"status":      c.Query("status"),
		"period":      c.Query("period"),
		"start_date":  c.Query("start_date"),
		"end_date":    c.Query("end_date"),
		"search":      c.Query("search"),
		"page":        c.QueryInt("page", 1),
		"limit":       c.QueryInt("limit", 15),
	}

	// Parse amount filters as float64
	if minAmt := c.Query("min_amount"); minAmt != "" {
		if f, err := strconv.ParseFloat(minAmt, 64); err == nil {
			filters["min_amount"] = f
		}
	}
	if maxAmt := c.Query("max_amount"); maxAmt != "" {
		if f, err := strconv.ParseFloat(maxAmt, 64); err == nil {
			filters["max_amount"] = f
		}
	}

	expenses, total, err := h.expenseService.GetExpenses(tenantID, filters)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Get categories map
	categories, _ := h.expenseService.GetExpenseCategories(tenantID)
	catMap := make(map[string]string)
	for _, c := range categories {
		catMap[c.ID] = c.Name
	}

	// Transform expenses to include category_name
	type expenseWithCategory struct {
		models.Expense
		CategoryName string `json:"category_name"`
	}
	result := make([]expenseWithCategory, len(expenses))
	for i, e := range expenses {
		result[i] = expenseWithCategory{
			Expense:      e,
			CategoryName: catMap[e.CategoryID],
		}
	}

	return c.JSON(fiber.Map{
		"expenses": result,
		"total":    total,
		"page":     filters["page"],
		"limit":    filters["limit"],
	})
}

func (h *ExpenseHandler) GetExpense(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	expenseID := c.Params("id")
	if expenseID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "expense ID required"})
	}

	expense, err := h.expenseService.GetExpenseByID(tenantID, expenseID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(expense)
}

func (h *ExpenseHandler) UpdateExpense(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	expenseID := c.Params("id")
	if expenseID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "expense ID required"})
	}

	var req services.UpdateExpenseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	expense, err := h.expenseService.UpdateExpense(tenantID, expenseID, &req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(expense)
}

func (h *ExpenseHandler) DeleteExpense(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	expenseID := c.Params("id")
	if expenseID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "expense ID required"})
	}

	if err := h.expenseService.DeleteExpense(tenantID, expenseID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "expense deleted"})
}

func (h *ExpenseHandler) GetCategories(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	categories, err := h.expenseService.GetExpenseCategories(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"categories": categories})
}

func (h *ExpenseHandler) CreateCategory(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "category name required"})
	}

	category, err := h.expenseService.CreateCategory(tenantID, req.Name, req.Description)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(category)
}

func (h *ExpenseHandler) GetExpenseSummary(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	period := c.Query("period")

	summary, err := h.expenseService.GetExpenseSummary(tenantID, period)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(summary)
}

// UploadExpenseAttachment handles file upload for an expense
func (h *ExpenseHandler) UploadExpenseAttachment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	expenseID := c.Params("id")
	if expenseID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "expense ID required"})
	}

	// Validate expense ID is a valid UUID
	if _, err := uuid.Parse(expenseID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid expense ID"})
	}

	// Get the file from the request
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no file provided"})
	}

	// Upload the file
	attachment, err := h.expenseService.UploadExpenseAttachment(tenantID, expenseID, fileHeader, c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(attachment)
}

// GetExpenseAttachments retrieves all attachments for an expense
func (h *ExpenseHandler) GetExpenseAttachments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	expenseID := c.Params("id")
	if expenseID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "expense ID required"})
	}

	// Validate expense ID is a valid UUID
	if _, err := uuid.Parse(expenseID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid expense ID"})
	}

	attachments, err := h.expenseService.GetExpenseAttachments(tenantID, expenseID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(attachments)
}

// DeleteExpenseAttachment removes an attachment from an expense
func (h *ExpenseHandler) DeleteExpenseAttachment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	expenseID := c.Params("id")
	if expenseID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "expense ID required"})
	}

	// Validate expense ID is a valid UUID
	if _, err := uuid.Parse(expenseID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid expense ID"})
	}

	attachmentID := c.Params("attachmentId")
	if attachmentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "attachment ID required"})
	}

	// Validate attachment ID is a valid UUID
	if _, err := uuid.Parse(attachmentID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid attachment ID"})
	}

	if err := h.expenseService.DeleteExpenseAttachment(tenantID, attachmentID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "attachment deleted"})
}

// GetExpenseAttachmentFile serves the attachment file
func (h *ExpenseHandler) GetExpenseAttachmentFile(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	attachmentID := c.Params("attachmentId")
	if attachmentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "attachment ID required"})
	}

	if _, err := uuid.Parse(attachmentID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid attachment ID"})
	}

	attachment, err := h.expenseService.GetExpenseAttachmentByID(tenantID, attachmentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "attachment not found"})
	}

	c.Set("Content-Type", attachment.FileType)
	return c.SendFile(attachment.FileURL)
}

// BulkExpenseAction handles bulk operations on expenses
func (h *ExpenseHandler) BulkExpenseAction(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req struct {
		Action string   `json:"action"` // approve, delete, export
		IDs    []string `json:"ids"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if len(req.IDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no expense IDs provided"})
	}

	// Validate all IDs are valid UUIDs
	for _, id := range req.IDs {
		if _, err := uuid.Parse(id); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid expense ID: " + id})
		}
	}

	switch req.Action {
	case "approve":
		return h.bulkApproveExpenses(c, tenantID, req.IDs)
	case "delete":
		return h.bulkDeleteExpenses(c, tenantID, req.IDs)
	case "export":
		return h.bulkExportExpenses(c, tenantID, req.IDs)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unsupported action: " + req.Action})
	}
}

// bulkApproveExpenses approves multiple expenses
func (h *ExpenseHandler) bulkApproveExpenses(c *fiber.Ctx, tenantID string, ids []string) error {
	var approvedCount int
	var failedCount int

	for _, id := range ids {
		// Get the expense first to check if it exists and belongs to tenant
		expense, err := h.expenseService.GetExpenseByID(tenantID, id)
		if err != nil {
			failedCount++
			continue
		}

		// Only approve if it's pending
		if expense.Status == "pending" {
			updateReq := services.UpdateExpenseRequest{
				Status: func() *string { s := "approved"; return &s }(),
			}

			if _, err := h.expenseService.UpdateExpense(tenantID, id, &updateReq); err != nil {
				failedCount++
			} else {
				approvedCount++
			}
		} else {
			// Skip if not pending
			failedCount++
		}
	}

	message := fmt.Sprintf("Approved %d expenses", approvedCount)
	if failedCount > 0 {
		message += fmt.Sprintf(", %d failed", failedCount)
	}

	return c.JSON(fiber.Map{
		"message":  message,
		"approved": approvedCount,
		"failed":   failedCount,
	})
}

// bulkDeleteExpenses deletes multiple expenses
func (h *ExpenseHandler) bulkDeleteExpenses(c *fiber.Ctx, tenantID string, ids []string) error {
	var deletedCount int
	var failedCount int

	for _, id := range ids {
		if err := h.expenseService.DeleteExpense(tenantID, id); err != nil {
			failedCount++
		} else {
			deletedCount++
		}
	}

	message := fmt.Sprintf("Deleted %d expenses", deletedCount)
	if failedCount > 0 {
		message += fmt.Sprintf(", %d failed", failedCount)
	}

	return c.JSON(fiber.Map{
		"message":  message,
		"deleted":  deletedCount,
		"failed":   failedCount,
	})
}

// bulkExportExpenses exports multiple expenses (placeholder for now)
func (h *ExpenseHandler) bulkExportExpenses(c *fiber.Ctx, tenantID string, ids []string) error {
	// For now, we'll just return a success message
	// In a real implementation, this would generate a CSV or PDF file
	message := fmt.Sprintf("Export initiated for %d expenses", len(ids))
	
	return c.JSON(fiber.Map{
		"message": message,
		"count":   len(ids),
		// In a real implementation, we'd provide a download URL here
		"url":     "/api/v1/tenant/expenses/export?ids=" + strings.Join(ids, ","),
	})
}
