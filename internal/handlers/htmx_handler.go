package handlers

import (
	"invoicefast/internal/services"
	"invoicefast/internal/utils"

	"github.com/gofiber/fiber/v2"
)

// HTMXHandler handles HTMX-enabled frontend requests
type HTMXHandler struct {
	invoiceService *services.InvoiceService
	clientService  *services.ClientService
}

// NewHTMXHandler creates a new HTMX handler
func NewHTMXHandler(invoiceService *services.InvoiceService, clientService *services.ClientService) *HTMXHandler {
	return &HTMXHandler{
		invoiceService: invoiceService,
		clientService:  clientService,
	}
}

// DashboardHTMX handles the dashboard with HTMX support
func (h *HTMXHandler) Dashboard(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	userID := c.Locals("user_id")

	stats, err := h.invoiceService.GetDashboardStats(tenantID.(string), "30d")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Error loading dashboard")
	}

	data := fiber.Map{
		"Title":    "Dashboard",
		"Stats":    stats,
		"UserID":   userID,
		"TenantID": tenantID,
	}

	if utils.IsHTMXRequest(c) {
		return c.Render("components/dashboard-content", data)
	}

	return c.Render("dashboard", data)
}

// InvoiceListHTMX renders invoice list with HTMX support
func (h *HTMXHandler) InvoiceList(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")

	filter := services.InvoiceFilter{
		Status: c.Query("status"),
	}

	invoices, _, err := h.invoiceService.GetUserInvoices(tenantID.(string), filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Error loading invoices")
	}

	data := fiber.Map{
		"Invoices": invoices,
		"Status":   filter.Status,
	}

	if utils.IsHTMXRequest(c) {
		return c.Render("components/invoice-list", data)
	}

	return c.Render("invoices/list", data)
}

// InvoiceRowHTMX renders a single invoice row for HTMX OOB swaps
func (h *HTMXHandler) InvoiceRow(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	invoiceID := c.Params("id")

	invoice, err := h.invoiceService.GetInvoiceByID(tenantID.(string), invoiceID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Invoice not found")
	}

	data := fiber.Map{
		"Invoice": invoice,
	}

	return c.Render("components/invoice-row", data)
}

// CreateInvoiceHTMX handles invoice creation via HTMX
func (h *HTMXHandler) CreateInvoice(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	userID := c.Locals("user_id")

	var req services.CreateInvoiceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	invoice, err := h.invoiceService.CreateInvoice(tenantID.(string), userID.(string), req.ClientID, &req)
	if err != nil {
		if utils.IsHTMXRequest(c) {
			data := fiber.Map{"Error": err.Error()}
			return c.Render("components/alert-error", data)
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	data := fiber.Map{
		"Invoice": invoice,
	}

	if utils.IsHTMXRequest(c) {
		utils.SetHXRetarget(c, "#invoice-list")
		utils.SetHXReswap(c, "afterbegin")
		return c.Render("components/invoice-row", data)
	}

	return c.Status(fiber.StatusCreated).JSON(invoice)
}

// FilterInvoicesHTMX handles invoice filtering via HTMX
func (h *HTMXHandler) FilterInvoices(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	status := c.Query("status")

	filter := services.InvoiceFilter{Status: status}
	invoices, _, err := h.invoiceService.GetUserInvoices(tenantID.(string), filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Error loading invoices")
	}

	data := fiber.Map{
		"Invoices": invoices,
		"Status":   status,
	}

	if utils.IsHTMXRequest(c) {
		return c.Render("components/invoice-list", data)
	}

	return c.Render("invoices/list", data)
}
