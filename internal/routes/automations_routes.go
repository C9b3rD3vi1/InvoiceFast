package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// AutomationRoutes configures automation API routes - Enterprise edition
func AutomationRoutes(app *fiber.App, db *database.DB, authService *services.AuthService) fiber.Router {
	// Initialize services
	jobQueue := services.NewJobQueueService(db)
	recurringInvoice := services.NewAutoRecurringInvoiceService(db, jobQueue)
	reminderService := services.NewAutoReminderService(db, jobQueue)
	workflowService := services.NewAutoWorkflowService(db, jobQueue)
	
	// Create handler
	handler := handlers.NewAutomationHandler(jobQueue, recurringInvoice, reminderService, workflowService)
	
	group := app.Group("/api/v1/tenant/automations")
	group.Use(middleware.TenantMiddleware(authService, db))
	
	// ==========================================================================
	// OVERVIEW
	// ==========================================================================
	group.Get("/", handler.GetAutomationOverview)
	
	// ==========================================================================
	// RECURRING INVOICES
	// ==========================================================================
	recurring := group.Group("/recurring")
	recurring.Get("/", handler.GetRecurringInvoices)
	recurring.Get("/:id", handler.GetRecurringInvoice)
	recurring.Post("/", handler.CreateRecurringInvoice)
	recurring.Post("/:id/pause", handler.PauseRecurringInvoice)
	recurring.Post("/:id/resume", handler.ResumeRecurringInvoice)
	recurring.Delete("/:id", handler.DeleteRecurringInvoice)
	
	// ==========================================================================
	// REMINDER RULES
	// ==========================================================================
	reminders := group.Group("/reminders")
	reminders.Get("/", handler.GetReminderRules)
	reminders.Get("/stats", handler.GetReminderStats)
	reminders.Get("/:id", handler.GetReminderRule)
	reminders.Post("/", handler.CreateReminderRule)
	reminders.Put("/:id", handler.UpdateReminderRule)
	reminders.Delete("/:id", handler.DeleteReminderRule)
	
	// ==========================================================================
	// WORKFLOWS
	// ==========================================================================
	workflows := group.Group("/workflows")
	workflows.Get("/", handler.GetWorkflows)
	workflows.Get("/stats", handler.GetWorkflowStats)
	workflows.Get("/:id", handler.GetWorkflow)
	workflows.Post("/", handler.CreateWorkflow)
	workflows.Put("/:id", handler.UpdateWorkflow)
	workflows.Delete("/:id", handler.DeleteWorkflow)
	
	// ==========================================================================
	// JOB QUEUE
	// ==========================================================================
	jobs := group.Group("/jobs")
	jobs.Get("/", handler.GetJobs)
	jobs.Get("/stats", handler.GetJobStats)
	jobs.Get("/failed", handler.GetFailedJobs)
	jobs.Get("/recent", handler.GetRecentJobs)
	jobs.Get("/:id", handler.GetJob)
	jobs.Post("/:id/retry", handler.RetryJob)
	jobs.Post("/:id/cancel", handler.CancelJob)
	
	return group
}