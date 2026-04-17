package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
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
		"start_date":  c.Query("start_date"),
		"end_date":    c.Query("end_date"),
		"search":      c.Query("search"),
		"page":        c.QueryInt("page", 1),
		"limit":       c.QueryInt("limit", 15),
	}

	expenses, total, err := h.expenseService.GetExpenses(tenantID, filters)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"expenses": expenses,
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

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	total, err := h.expenseService.GetTotalExpenses(tenantID, startDate, endDate)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	byCategory, _ := h.expenseService.GetExpensesByCategory(tenantID, startDate, endDate)

	return c.JSON(fiber.Map{
		"total":       total,
		"by_category": byCategory,
		"start_date":  startDate,
		"end_date":    endDate,
	})
}
