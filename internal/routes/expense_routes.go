package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// ExpenseRoutes configures /api/v1/tenant/expenses endpoints
func ExpenseRoutes(app *fiber.App, h *handlers.ExpenseHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/expenses")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Post("/", h.CreateExpense)
	group.Get("/", h.GetExpenses)
	group.Get("/:id", h.GetExpense)
	group.Put("/:id", h.UpdateExpense)
	group.Delete("/:id", h.DeleteExpense)
	group.Get("/categories", h.GetCategories)
	group.Post("/categories", h.CreateCategory)
	group.Get("/summary", h.GetExpenseSummary)

	return group
}
