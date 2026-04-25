package handlers

import (
	"strconv"

	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// ============================================================================
// AUTOMATION HANDLER - Enterprise automation module
// ============================================================================

type AutomationHandler struct {
	jobQueue           *services.JobQueueService
	recurringInvoice  *services.AutoRecurringInvoiceService
	reminderService   *services.AutoReminderService
	workflowService  *services.AutoWorkflowService
}

func NewAutomationHandler(
	jobQueue *services.JobQueueService,
	recurringInvoice *services.AutoRecurringInvoiceService,
	reminder *services.AutoReminderService,
	workflow *services.AutoWorkflowService,
) *AutomationHandler {
	return &AutomationHandler{
		jobQueue:           jobQueue,
		recurringInvoice:  recurringInvoice,
		reminderService:    reminder,
		workflowService:   workflow,
	}
}

// ============================================================================
// RECURRING INVOICE HANDLERS
// ============================================================================

func (h *AutomationHandler) GetRecurringInvoices(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	recurring, err := h.recurringInvoice.GetRecurringInvoices(tenantID, "")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"recurring_invoices": recurring})
}

func (h *AutomationHandler) GetRecurringInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	recurring, err := h.recurringInvoice.GetRecurringInvoice(tenantID, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "recurring invoice not found"})
	}

	return c.JSON(recurring)
}

func (h *AutomationHandler) CreateRecurringInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	userID := middleware.GetUserID(c)

	var req services.CreateRecurringInvoiceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	recurring, err := h.recurringInvoice.CreateRecurringInvoice(tenantID, userID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(recurring)
}

func (h *AutomationHandler) DeleteRecurringInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	if err := h.recurringInvoice.DeleteRecurringInvoice(tenantID, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *AutomationHandler) PauseRecurringInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	if err := h.recurringInvoice.PauseRecurringInvoice(tenantID, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "paused"})
}

func (h *AutomationHandler) ResumeRecurringInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	if err := h.recurringInvoice.ResumeRecurringInvoice(tenantID, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "active"})
}

// ============================================================================
// REMINDER RULE HANDLERS
// ============================================================================

func (h *AutomationHandler) GetReminderRules(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	activeOnly := c.Query("active") == "true"
	rules, err := h.reminderService.GetReminderRules(tenantID, activeOnly)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"reminder_rules": rules})
}

func (h *AutomationHandler) GetReminderRule(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	rule, err := h.reminderService.GetReminderRule(tenantID, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "reminder rule not found"})
	}

	return c.JSON(rule)
}

func (h *AutomationHandler) CreateReminderRule(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	userID := middleware.GetUserID(c)

	var req services.CreateReminderRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	rule, err := h.reminderService.CreateReminderRule(tenantID, userID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(rule)
}

func (h *AutomationHandler) UpdateReminderRule(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")

	var req services.UpdateReminderRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	rule, err := h.reminderService.UpdateReminderRule(tenantID, id, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(rule)
}

func (h *AutomationHandler) DeleteReminderRule(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	if err := h.reminderService.DeleteReminderRule(tenantID, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *AutomationHandler) GetReminderStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	stats, err := h.reminderService.GetReminderStats(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

// ============================================================================
// WORKFLOW HANDLERS
// ============================================================================

func (h *AutomationHandler) GetWorkflows(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	activeOnly := c.Query("active") == "true"
	workflows, err := h.workflowService.GetWorkflows(tenantID, activeOnly)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"workflows": workflows})
}

func (h *AutomationHandler) GetWorkflow(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	workflow, err := h.workflowService.GetWorkflow(tenantID, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "workflow not found"})
	}

	return c.JSON(workflow)
}

func (h *AutomationHandler) CreateWorkflow(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	userID := middleware.GetUserID(c)

	var req services.CreateWorkflowRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	workflow, err := h.workflowService.CreateWorkflow(tenantID, userID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(workflow)
}

func (h *AutomationHandler) UpdateWorkflow(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")

	var req services.UpdateWorkflowRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	workflow, err := h.workflowService.UpdateWorkflow(tenantID, id, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(workflow)
}

func (h *AutomationHandler) DeleteWorkflow(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	if err := h.workflowService.DeleteWorkflow(tenantID, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *AutomationHandler) GetWorkflowStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	stats, err := h.workflowService.GetWorkflowStats(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

// ============================================================================
// JOB QUEUE HANDLERS
// ============================================================================

func (h *AutomationHandler) GetJobs(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	status := c.Query("status", "")
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	jobs, total, err := h.jobQueue.GetJobsByTenant(tenantID, status, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"jobs":   jobs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *AutomationHandler) GetJob(c *fiber.Ctx) error {
	id := c.Params("id")
	job, err := h.jobQueue.GetJob(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "job not found"})
	}

	tenantID := middleware.GetTenantID(c)
	if job.TenantID != tenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "unauthorized"})
	}

	return c.JSON(job)
}

func (h *AutomationHandler) GetJobStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	stats, err := h.jobQueue.GetJobStats(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

func (h *AutomationHandler) RetryJob(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	
	job, err := h.jobQueue.GetJob(id)
	if err != nil || job.TenantID != tenantID {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "job not found"})
	}
	
	if err := h.jobQueue.RetryDeadLetter(id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "retry_scheduled"})
}

func (h *AutomationHandler) CancelJob(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	id := c.Params("id")
	
	job, err := h.jobQueue.GetJob(id)
	if err != nil || job.TenantID != tenantID {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "job not found"})
	}
	
	if err := h.jobQueue.MoveToDeadLetter(id, "manually cancelled"); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "cancelled"})
}

func (h *AutomationHandler) GetFailedJobs(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	jobs, total, err := h.jobQueue.GetJobsByTenant(tenantID, "failed", limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"failed_jobs": jobs,
		"total":      total,
		"limit":      limit,
		"offset":     offset,
	})
}

func (h *AutomationHandler) GetRecentJobs(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	jobs, _, err := h.jobQueue.GetJobsByTenant(tenantID, "", limit, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"jobs": jobs})
}

// ============================================================================
// AUTOMATION OVERVIEW HANDLER
// ============================================================================

func (h *AutomationHandler) GetAutomationOverview(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	jobStats, _ := h.jobQueue.GetJobStats(tenantID)
	reminderStats, _ := h.reminderService.GetReminderStats(tenantID)
	workflowStats, _ := h.workflowService.GetWorkflowStats(tenantID)

	recurring, _ := h.recurringInvoice.GetRecurringInvoices(tenantID, "")
	reminders, _ := h.reminderService.GetReminderRules(tenantID, false)
	workflows, _ := h.workflowService.GetWorkflows(tenantID, false)

	return c.JSON(fiber.Map{
		"jobs":       jobStats,
		"reminders":   reminderStats,
		"workflows":  workflowStats,
		"counts": fiber.Map{
			"recurring_invoices": len(recurring),
			"reminder_rules":    len(reminders),
			"workflows":        len(workflows),
		},
	})
}