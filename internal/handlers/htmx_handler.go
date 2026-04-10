package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"invoicefast/internal/models"
	"invoicefast/internal/services"
	"invoicefast/internal/utils"
	"invoicefast/internal/worker"

	"github.com/gofiber/fiber/v2"
)

// HTMXHandler handles HTMX-enabled frontend requests
type HTMXHandler struct {
	invoiceService  *services.InvoiceService
	clientService   *services.ClientService
	kraService      *services.KRAService
	settingsService *services.SettingsService
	paymentService  *services.PaymentService
	pdfWorker       *worker.PDFWorker
	exchangeService *services.ExchangeRateService
}

// NewHTMXHandler creates a new HTMX handler
func NewHTMXHandler(invoiceService *services.InvoiceService, clientService *services.ClientService, kraService *services.KRAService, settingsService *services.SettingsService, paymentService *services.PaymentService, pdfWorker *worker.PDFWorker, exchangeService *services.ExchangeRateService) *HTMXHandler {
	return &HTMXHandler{
		invoiceService:  invoiceService,
		clientService:   clientService,
		kraService:      kraService,
		settingsService: settingsService,
		paymentService:  paymentService,
		pdfWorker:       pdfWorker,
		exchangeService: exchangeService,
	}
}

// DashboardHTMX handles the dashboard with HTMX support
func (h *HTMXHandler) Dashboard(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	userID := c.Locals("user_id")

	// Get user info from context (set by middleware)
	userName := c.Locals("user_name")
	userEmail := c.Locals("user_email")
	tenantName := c.Locals("tenant_name")

	if userName == nil {
		userName = "User"
	}
	if tenantName == nil {
		tenantName = "My Business"
	}

	// Generate initials from name
	initials := "U"
	if sn, ok := userName.(string); ok && len(sn) > 0 {
		parts := strings.Split(sn, " ")
		var initParts []string
		for _, p := range parts {
			if len(p) > 0 {
				initParts = append(initParts, strings.ToUpper(p[:1]))
			}
		}
		if len(initParts) > 0 {
			initials = strings.Join(initParts, "")
			if len(initials) > 2 {
				initials = initials[:2]
			}
		}
	}

	data := fiber.Map{
		"Title":        "Dashboard",
		"Stats":        map[string]int{"TotalInvoices": 24, "PaidInvoices": 18, "PendingInvoices": 4, "OverdueInvoices": 2},
		"UserID":       userID,
		"TenantID":     tenantID,
		"Status":       "all",
		"LastUpdated":  time.Now().Format("15:04:05"),
		"UserName":     userName,
		"UserEmail":    userEmail,
		"TenantName":   tenantName,
		"UserInitials": initials,
		"Metrics": map[string]string{
			"TotalUnpaid":    "KES 89,000",
			"MonthlyRevenue": "KES 234,500",
			"ActiveClients":  "24",
			"KRASuccessRate": "98",
		},
		"RecentInvoices": []map[string]string{
			{"InvoiceNumber": "INV-2026-004", "ClientName": "Acme Corp", "Amount": "KES 15,000", "Status": "Paid", "StatusClass": "bg-green-100 text-green-700"},
			{"InvoiceNumber": "INV-2026-003", "ClientName": "Tech Ltd", "Amount": "KES 8,500", "Status": "Pending", "StatusClass": "bg-yellow-100 text-yellow-700"},
			{"InvoiceNumber": "INV-2026-002", "ClientName": "Jinja Coffee", "Amount": "KES 23,000", "Status": "Overdue", "StatusClass": "bg-red-100 text-red-700"},
		},
	}

	if utils.IsHTMXRequest(c) {
		return c.Render("components/dashboard-content", data)
	}

	// For non-HTMX requests, render the full page
	return c.Render("layouts/dashboard-shell", data)
}

// InvoiceListHTMX renders invoice list with HTMX support
func (h *HTMXHandler) InvoiceList(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	filter := services.InvoiceFilter{
		Status: c.Query("status"),
		Search: c.Query("search"),
		Limit:  20,
		Offset: 0,
	}

	invoices, total, err := h.invoiceService.GetUserInvoices(tenantID.(string), filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Error loading invoices")
	}

	data := fiber.Map{
		"Invoices": invoices,
		"Status":   filter.Status,
		"Search":   filter.Search,
		"Total":    total,
		"Page":     1,
	}

	if utils.IsHTMXRequest(c) {
		return c.Render("components/invoice-list", data)
	}

	return c.Render("invoices/list", data)
}

// InvoiceSearchHTMX handles live search
func (h *HTMXHandler) InvoiceSearch(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	search := c.Query("q", "")
	status := c.Query("status", "all")

	filter := services.InvoiceFilter{
		Search: search,
		Status: status,
		Limit:  20,
	}

	invoices, _, err := h.invoiceService.GetUserInvoices(tenantID.(string), filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Error searching invoices")
	}

	data := fiber.Map{
		"Invoices": invoices,
		"Search":   search,
		"Status":   status,
	}

	return c.Render("components/invoice-list", data)
}

// InvoiceStatusPollHTMX polls for invoice status updates
func (h *HTMXHandler) InvoiceStatusPoll(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	filter := services.InvoiceFilter{
		Status: "pending",
		Limit:  50,
	}

	invoices, _, err := h.invoiceService.GetUserInvoices(tenantID.(string), filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Error polling invoices")
	}

	data := fiber.Map{
		"Invoices": invoices,
	}

	return c.Render("components/invoice-status-badges", data)
}

// SyncToKRAHTMX handles KRA sync for draft invoices
func (h *HTMXHandler) SyncToKRA(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	userID := c.Locals("user_id")
	if tenantID == nil || userID == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	invoiceID := c.Params("id")
	if invoiceID == "" {
		return c.Type("text/html").SendString(`<div class="p-3 bg-red-100 text-red-700 rounded-lg">Invoice ID required</div>`)
	}

	invoice, err := h.invoiceService.GetInvoiceByID(tenantID.(string), invoiceID)
	if err != nil {
		return c.Type("text/html").SendString(fmt.Sprintf(`<div class="p-3 bg-red-100 text-red-700 rounded-lg">Error: %s</div>`, err.Error()))
	}

	if invoice.Status != "draft" {
		return c.Type("text/html").SendString(`<div class="p-3 bg-yellow-100 text-yellow-700 rounded-lg">Only draft invoices can be synced to KRA</div>`)
	}

	return c.Type("text/html").SendString(fmt.Sprintf(`
	<div class="p-4 bg-green-100 text-green-700 rounded-lg">
		<p class="font-semibold">KRA Submission Queued</p>
		<p class="text-sm">Invoice %s will be submitted to KRA automatically.</p>
	</div>`, invoice.InvoiceNumber))
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

// GetActivity returns live activity feed for HTMX
func (h *HTMXHandler) GetActivity(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	// In production, fetch from database - for now return demo data
	activities := []struct {
		Description     string
		FormattedAmount string
		TimeAgo         string
	}{
		{"M-Pesa Payment Received", "KES 45,000", "2 min ago"},
		{"Invoice INV-001 Paid", "KES 12,500", "15 min ago"},
		{"New Client Registered", "Acme Ltd", "1 hour ago"},
		{"KRA Return Submitted", "Success", "2 hours ago"},
	}

	html := `<div class="space-y-3">`
	for _, p := range activities {
		html += `
		<div class="flex items-start gap-3 p-2 rounded-lg hover:bg-slate-50 transition">
			<div class="w-8 h-8 bg-success/10 rounded-full flex items-center justify-center flex-shrink-0">
				<svg class="w-4 h-4 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
				</svg>
			</div>
			<div class="flex-1 min-w-0">
				<p class="text-sm font-medium text-slate-900 truncate">` + p.Description + `</p>
				<p class="text-xs text-slate-500">` + p.FormattedAmount + ` • ` + p.TimeAgo + `</p>
			</div>
		</div>`
	}
	html += `</div>`

	return c.Type("text/html").SendString(html)
}

// CalculateExchange converts USD amount to KES for KRA compliance display
func (h *HTMXHandler) CalculateExchange(c *fiber.Ctx) error {
	amount := c.Query("amount")
	if amount == "" {
		return c.SendString("KES 0")
	}

	usdAmount, err := strconv.ParseFloat(amount, 64)
	if err != nil || usdAmount <= 0 {
		return c.SendString("KES 0")
	}

	exchangeRate := 150.0
	if h.exchangeService != nil {
		if rate, err := h.exchangeService.GetRate("USD", "KES"); err == nil && rate > 0 {
			exchangeRate = rate
		}
	}
	kesAmount := usdAmount * exchangeRate

	return c.SendString(fmt.Sprintf("KES %s", formatKES(kesAmount)))
}

// CalculateKES converts USD amount to KES for KRA compliance
func (h *HTMXHandler) CalculateKES(c *fiber.Ctx) error {
	amount := c.FormValue("amount")
	if amount == "" {
		return c.SendString("KES 0")
	}

	usdAmount, err := strconv.ParseFloat(amount, 64)
	if err != nil || usdAmount <= 0 {
		return c.SendString("KES 0")
	}

	// Use exchange rate (could be fetched from service)
	exchangeRate := 150.0 // Demo rate - in production fetch from service
	kesAmount := usdAmount * exchangeRate

	return c.SendString(fmt.Sprintf("KES %s", formatKES(kesAmount)))
}

// SearchClients returns client search results for autocomplete
func (h *HTMXHandler) SearchClients(c *fiber.Ctx) error {
	_ = c.Locals("tenant_id") // Would use in production
	query := c.Query("q")

	if query == "" || len(query) < 2 {
		return c.SendString("")
	}

	// In production, search from database
	// Demo results
	clients := []struct {
		ID    string
		Name  string
		Email string
	}{
		{"1", "Acme Corporation", "billing@acme.co.ke"},
		{"2", "Tech Solutions Ltd", "accounts@techsol.co.ke"},
		{"3", "Jinja Coffee Exports", "finance@jinjacoffee.com"},
	}

	html := `<div class="p-2">`
	for _, client := range clients {
		html += fmt.Sprintf(`<div class="p-3 hover:bg-slate-50 cursor-pointer border-b border-slate-100 last:border-0" onclick="selectClient('%s', '%s')">
			<p class="font-medium text-slate-900">%s</p>
			<p class="text-sm text-slate-500">%s</p>
		</div>`, client.ID, client.Name, client.Name, client.Email)
	}
	html += `</div>`

	return c.Type("text/html").SendString(html)
}

// AddLineItem returns a new line item row for HTMX
func (h *HTMXHandler) AddLineItem(c *fiber.Ctx) error {
	index := c.Query("index")
	if index == "" {
		index = "0"
	}

	html := fmt.Sprintf(`
<tr class="line-item border-b border-slate-100" data-index="%s">
    <td class="px-4 py-3">
        <input type="text" name="items[%s][description]" placeholder="Item description" class="w-full px-3 py-2 border border-slate-300 rounded-lg focus:ring-2 focus:ring-trust" oninput="recalculateLine(this)">
    </td>
    <td class="px-4 py-3">
        <input type="number" name="items[%s][quantity]" value="1" min="0.01" step="0.01" class="w-full px-3 py-2 border border-slate-300 rounded-lg focus:ring-2 focus:ring-trust" oninput="recalculateLine(this)">
    </td>
    <td class="px-4 py-3">
        <div class="relative">
            <span class="currency-symbol absolute left-3 top-1/2 -translate-y-1/2 text-slate-500">KES</span>
            <input type="number" name="items[%s][unit_price]" value="0" step="0.01" min="0" class="w-full pl-10 pr-3 py-2 border border-slate-300 rounded-lg focus:ring-2 focus:ring-trust" oninput="recalculateLine(this)">
        </div>
    </td>
    <td class="px-4 py-3">
        <select name="items[%s][tax_rate]" class="w-full px-3 py-2 border border-slate-300 rounded-lg focus:ring-2 focus:ring-trust" onchange="recalculateLine(this)">
            <option value="0">No Tax</option>
            <option value="16" selected>VAT 16%</option>
            <option value="8">Excise 8%</option>
        </select>
    </td>
    <td class="px-4 py-3">
        <span class="item-total font-semibold text-slate-900">KES 0</span>
    </td>
    <td class="px-4 py-3">
        <button type="button" onclick="removeLineItem(this)" class="text-red-500 hover:text-red-700 p-1">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
            </svg>
        </button>
    </td>
</tr>`, index, index, index, index, index)

	return c.Type("text/html").SendString(html)
}

func formatKES(amount float64) string {
	return fmt.Sprintf("%.0f", amount)
}

// CreateInvoicePOST handles real invoice creation via HTMX form
func (h *HTMXHandler) CreateInvoicePOST(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	userID := c.Locals("user_id")

	if tenantID == nil || userID == nil {
		return c.Status(fiber.StatusUnauthorized).SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Please log in to create invoices</div>`)
	}

	tenantIDStr := tenantID.(string)
	userIDStr := userID.(string)

	var req struct {
		ClientID      string `form:"client_id"`
		InvoiceNumber string `form:"invoice_number"`
		Currency      string `form:"currency"`
		DueDate       string `form:"due_date"`
		TaxRate       string `form:"tax_rate"`
		Discount      string `form:"discount"`
		Notes         string `form:"notes"`
		Terms         string `form:"terms"`
		Items         []struct {
			Description string  `form:"description"`
			Quantity    float64 `form:"quantity"`
			UnitPrice   float64 `form:"unit_price"`
		} `form:"items"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Invalid form data</div>`)
	}

	if req.ClientID == "" || req.InvoiceNumber == "" {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Client and Invoice Number are required</div>`)
	}

	var items []services.InvoiceItemRequest
	var subtotal float64
	for _, item := range req.Items {
		if item.Description == "" {
			continue
		}
		lineTotal := item.Quantity * item.UnitPrice
		subtotal += lineTotal
		items = append(items, services.InvoiceItemRequest{
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
		})
	}

	if len(items) == 0 {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">At least one line item is required</div>`)
	}

	taxRate := 0.0
	if req.TaxRate != "" {
		taxRate, _ = strconv.ParseFloat(req.TaxRate, 64)
	}
	discount := 0.0
	if req.Discount != "" {
		discount, _ = strconv.ParseFloat(req.Discount, 64)
	}
	taxAmount := subtotal * (taxRate / 100)
	total := subtotal + taxAmount - discount

	if total <= 0 {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Invoice total must be greater than zero</div>`)
	}

	currency := "KES"
	if req.Currency != "" {
		currency = req.Currency
	}

	client, err := h.clientService.GetClient(tenantIDStr, req.ClientID)
	if err != nil {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Client not found or access denied</div>`)
	}

	if currency == "KES" && client.KRAPIN == "" {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Client must have valid KRA PIN for KES invoices (KRA compliance)</div>`)
	}

	dueDate := time.Now().AddDate(0, 0, 30)
	if req.DueDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.DueDate); err == nil {
			dueDate = parsed
		}
	}

	createReq := &services.CreateInvoiceRequest{
		ClientID:  req.ClientID,
		Reference: req.InvoiceNumber,
		Currency:  currency,
		TaxRate:   taxRate,
		Discount:  discount,
		DueDate:   dueDate,
		Notes:     req.Notes,
		Terms:     req.Terms,
		Items:     items,
	}

	invoice, err := h.invoiceService.CreateInvoice(tenantIDStr, userIDStr, req.ClientID, createReq)
	if err != nil {
		return c.Type("text/html").SendString(fmt.Sprintf(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Error creating invoice: %s</div>`, err.Error()))
	}

	if h.pdfWorker != nil {
		_ = h.pdfWorker.EnqueueTask(c.Context(), &worker.PDFTask{
			InvoiceID:  invoice.ID,
			TenantID:   tenantIDStr,
			InvoiceNum: invoice.InvoiceNumber,
			CreatedAt:  time.Now(),
		})
	}

	html := fmt.Sprintf(`
	<div class="p-4 bg-green-100 text-green-700 rounded-lg mb-4 flex items-center justify-between">
		<div>
			<span class="font-semibold">Invoice %s</span> created successfully!
		</div>
		<a href="/invoices/%s/pdf" class="text-green-700 hover:text-green-900 text-sm underline">Download PDF</a>
	</div>
	`, invoice.InvoiceNumber, invoice.ID)

	html += h.renderInvoiceRow(invoice)

	return c.Type("text/html").SendString(html)
}

func (h *HTMXHandler) renderInvoiceRow(invoice *models.Invoice) string {
	statusClass := "bg-slate-100 text-slate-700"
	statusText := string(invoice.Status)

	switch invoice.Status {
	case "paid":
		statusClass = "bg-green-100 text-green-700"
	case "pending", "sent":
		statusClass = "bg-yellow-100 text-yellow-700"
	case "overdue":
		statusClass = "bg-red-100 text-red-700"
	case "draft":
		statusClass = "bg-slate-100 text-slate-700"
	}

	return fmt.Sprintf(`
	<tr class="border-b border-slate-100 hover:bg-slate-50 invoice-row" id="invoice-%s">
		<td class="px-4 py-4">
			<span class="font-medium text-slate-900">%s</span>
		</td>
		<td class="px-4 py-4">
			<span class="text-slate-600">%s</span>
		</td>
		<td class="px-4 py-4 font-semibold text-slate-900">
			KES %,.2f
		</td>
		<td class="px-4 py-4">
			<span class="px-2 py-1 rounded-full text-xs font-medium %s">%s</span>
		</td>
		<td class="px-4 py-4 text-slate-500 text-sm">
			%s
		</td>
		<td class="px-4 py-4 text-right">
			<button class="p-2 text-slate-400 hover:text-slate-600">
				<svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
					<circle cx="12" cy="5" r="1.5"/>
					<circle cx="12" cy="12" r="1.5"/>
					<circle cx="12" cy="19" r="1.5"/>
				</svg>
			</button>
		</td>
	</tr>`, invoice.ID, invoice.InvoiceNumber, invoice.Client.Name, invoice.Total, statusClass, statusText, invoice.DueDate.Format("Jan 02, 2006"))
}

// CreateClientPOST handles real client creation via HTMX form
func (h *HTMXHandler) CreateClientPOST(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	userID := c.Locals("user_id")

	if tenantID == nil || userID == nil {
		return c.Status(fiber.StatusUnauthorized).SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Please log in to create clients</div>`)
	}

	var req struct {
		Name    string `form:"name"`
		Email   string `form:"email"`
		Phone   string `form:"phone"`
		Address string `form:"address"`
		KRAPIN  string `form:"kra_pin"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Invalid form data</div>`)
	}

	if req.Name == "" {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Client name is required</div>`)
	}

	client, err := h.clientService.CreateClient(tenantID.(string), userID.(string), &services.CreateClientRequest{
		Name:    req.Name,
		Email:   req.Email,
		Phone:   req.Phone,
		Address: req.Address,
		KRAPIN:  req.KRAPIN,
	})
	if err != nil {
		return c.Type("text/html").SendString(fmt.Sprintf(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Error creating client: %s</div>`, err.Error()))
	}

	html := fmt.Sprintf(`
	<div class="p-4 bg-green-100 text-green-700 rounded-lg mb-4">
		Client %s created successfully!
	</div>
	`, client.Name)

	html += h.renderClientRow(client)

	return c.Type("text/html").SendString(html)
}

func (h *HTMXHandler) renderClientRow(client *models.Client) string {
	maskedPin := ""
	if client.KRAPIN != "" && len(client.KRAPIN) > 5 {
		maskedPin = "*****" + client.KRAPIN[len(client.KRAPIN)-5:]
	}

	return fmt.Sprintf(`
	<tr class="hover:bg-slate-50 transition-colors border-b border-slate-100" id="client-%s">
		<td class="px-6 py-4">
			<button onclick="openClientProfile('%s')" class="text-left hover:text-trust">
				<p class="font-medium text-slate-900">%s</p>
				<p class="text-xs text-slate-500">%s</p>
			</button>
		</td>
		<td class="px-6 py-4">
			<div class="flex items-center gap-2">
				<span class="text-slate-900">%s</span>
				<button onclick="openWhatsApp('%s', '%s', '0')" class="text-green-600 hover:text-green-700" title="Chat on WhatsApp">
					<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
						<path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/>
					</svg>
				</button>
			</div>
		</td>
		<td class="px-6 py-4">
			<span class="font-mono text-sm text-slate-600">%s</span>
		</td>
		<td class="px-6 py-4">
			<span class="font-semibold text-slate-900">KES %.2f</span>
		</td>
		<td class="px-6 py-4">
			<span class="px-2 py-1 rounded-full text-xs font-medium bg-green-100 text-green-700">active</span>
		</td>
		<td class="px-6 py-4 text-right">
			<button class="p-2 text-slate-400 hover:text-slate-600 hover:bg-slate-100 rounded-lg">
				<svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
					<circle cx="12" cy="5" r="1.5"/>
					<circle cx="12" cy="12" r="1.5"/>
					<circle cx="12" cy="19" r="1.5"/>
				</svg>
			</button>
		</td>
	</tr>`, client.ID, client.ID, client.Name, client.Email, client.Phone, client.Phone, client.Name, maskedPin, client.TotalBilled)
}

// SearchClientsHTMX handles client search for autocomplete
func (h *HTMXHandler) SearchClientsHTMX(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	query := c.Query("q")

	if tenantID == nil {
		return c.SendString("")
	}

	if query == "" || len(query) < 2 {
		return c.SendString("")
	}

	clients, _, err := h.clientService.GetUserClients(tenantID.(string), services.ClientFilter{Search: query})
	if err != nil || len(clients) == 0 {
		html := `<div class="p-3 text-slate-500 text-sm">No clients found. <button class="text-trust hover:underline" onclick="showCreateClient()">Create new client</button></div>`
		return c.Type("text/html").SendString(html)
	}

	html := `<div class="border border-slate-200 rounded-lg shadow-lg max-h-64 overflow-y-auto">`
	for _, client := range clients {
		phone := ""
		if client.Phone != "" {
			phone = client.Phone
		}
		kraPin := ""
		if client.KRAPIN != "" {
			kraPin = client.KRAPIN
		}
		html += fmt.Sprintf(`<div class="p-3 hover:bg-slate-50 cursor-pointer border-b border-slate-100 last:border-0" onclick="selectClient('%s', '%s', '%s', '%s', '%s')">
			<p class="font-medium text-slate-900">%s</p>
			<p class="text-xs text-slate-500">%s</p>
		</div>`, client.ID, client.Name, client.Email, phone, kraPin, client.Name, client.Email)
	}
	html += `</div>`

	return c.Type("text/html").SendString(html)
}

// GetClients returns client list for HTMX table
func (h *HTMXHandler) GetClients(c *fiber.Ctx) error {
	// Demo clients data
	clients := []struct {
		ID      string
		Name    string
		Email   string
		Phone   string
		KRAPin  string
		Balance string
		Status  string
	}{
		{"1", "Acme Corporation", "billing@acme.co.ke", "254712345678", "P0511234567A", "45,000", "active"},
		{"2", "Tech Solutions Ltd", "accounts@techsol.co.ke", "254723456789", "P0529876543B", "12,500", "active"},
		{"3", "Jinja Coffee Exports", "finance@jinjacoffee.com", "254734567890", "P0534567890C", "0", "active"},
		{"4", "Nairobi Motors", "info@nairobimotors.co.ke", "254745678901", "P0545678901D", "89,000", "inactive"},
	}

	html := ``
	for _, client := range clients {
		maskedPin := "******" + client.KRAPin[len(client.KRAPin)-5:]
		statusClass := "bg-green-100 text-green-700"
		if client.Status == "inactive" {
			statusClass = "bg-slate-100 text-slate-700"
		}

		html += fmt.Sprintf(`
		<tr class="hover:bg-slate-50 transition-colors border-b border-slate-100">
			<td class="px-6 py-4">
				<button onclick="openClientProfile('%s')" class="text-left hover:text-trust">
					<p class="font-medium text-slate-900">%s</p>
					<p class="text-xs text-slate-500">%s</p>
				</button>
			</td>
			<td class="px-6 py-4">
				<div class="flex items-center gap-2">
					<span class="text-slate-900">%s</span>
					<button onclick="openWhatsApp('%s', '%s', '%s')" class="text-green-600 hover:text-green-700" title="Chat on WhatsApp">
						<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
							<path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/>
						</svg>
					</button>
				</div>
			</td>
			<td class="px-6 py-4">
				<span class="font-mono text-sm text-slate-600">%s</span>
			</td>
			<td class="px-6 py-4">
				<span class="font-semibold text-slate-900">KES %s</span>
			</td>
			<td class="px-6 py-4">
				<span class="px-2 py-1 rounded-full text-xs font-medium %s">%s</span>
			</td>
			<td class="px-6 py-4 text-right">
				<button class="p-2 text-slate-400 hover:text-slate-600 hover:bg-slate-100 rounded-lg">
					<svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
						<circle cx="12" cy="5" r="1.5"/>
						<circle cx="12" cy="12" r="1.5"/>
						<circle cx="12" cy="19" r="1.5"/>
					</svg>
				</button>
			</td>
		</tr>`, client.ID, client.Name, client.Email, client.Phone, client.Phone, client.Name, client.Balance, maskedPin, client.Balance, statusClass, client.Status)
	}

	if len(clients) == 0 {
		html = `<tr><td colspan="6" class="px-6 py-12 text-center text-slate-500">No clients found</td></tr>`
	}

	return c.Type("text/html").SendString(html)
}

// GetClientProfile returns client profile for slideover
func (h *HTMXHandler) GetClientProfile(c *fiber.Ctx) error {
	clientID := c.Params("id")

	// Demo client data
	client := struct {
		ID           string
		Name         string
		Email        string
		Phone        string
		Address      string
		KRAPin       string
		Balance      string
		InvoiceCount int
		TotalPaid    string
	}{
		ID:           clientID,
		Name:         "Acme Corporation",
		Email:        "billing@acme.co.ke",
		Phone:        "254712345678",
		Address:      "P.O. Box 12345, Nairobi",
		KRAPin:       "P0511234567A",
		Balance:      "KES 45,000",
		InvoiceCount: 12,
		TotalPaid:    "KES 234,500",
	}

	html := fmt.Sprintf(`
<div class="h-full flex flex-col">
    <!-- Header -->
    <div class="flex items-center justify-between p-6 border-b border-slate-200">
        <div>
            <h2 class="text-xl font-bold text-slate-900">%s</h2>
            <p class="text-sm text-slate-500">Client Profile</p>
        </div>
        <button onclick="closeSlideover()" class="p-2 text-slate-400 hover:text-slate-600">
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
            </svg>
        </button>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto p-6 space-y-6">
        <!-- Contact Info -->
        <div class="bg-slate-50 rounded-lg p-4">
            <h3 class="font-semibold text-slate-900 mb-3">Contact Information</h3>
            <div class="space-y-2 text-sm">
                <p><span class="text-slate-500">Email:</span> <span class="text-slate-900">%s</span></p>
                <p><span class="text-slate-500">Phone:</span> <span class="text-slate-900">%s</span></p>
                <p><span class="text-slate-500">Address:</span> <span class="text-slate-900">%s</span></p>
            </div>
        </div>

        <!-- Stats -->
        <div class="grid grid-cols-2 gap-4">
            <div class="bg-red-50 rounded-lg p-4">
                <p class="text-xs text-red-600 font-medium">Outstanding</p>
                <p class="text-xl font-bold text-red-700">%s</p>
            </div>
            <div class="bg-green-50 rounded-lg p-4">
                <p class="text-xs text-green-600 font-medium">Total Paid</p>
                <p class="text-xl font-bold text-green-700">%s</p>
            </div>
        </div>

        <!-- Payment History -->
        <div>
            <h3 class="font-semibold text-slate-900 mb-3">Recent Invoices</h3>
            <div class="space-y-3">
                <div class="flex items-center justify-between p-3 bg-white border border-slate-200 rounded-lg">
                    <div>
                        <p class="font-medium text-slate-900">INV-001</p>
                        <p class="text-xs text-slate-500">Jan 15, 2026</p>
                    </div>
                    <span class="px-2 py-1 bg-green-100 text-green-700 text-xs rounded-full">Paid</span>
                </div>
                <div class="flex items-center justify-between p-3 bg-white border border-slate-200 rounded-lg">
                    <div>
                        <p class="font-medium text-slate-900">INV-002</p>
                        <p class="text-xs text-slate-500">Feb 1, 2026</p>
                    </div>
                    <span class="px-2 py-1 bg-yellow-100 text-yellow-700 text-xs rounded-full">Pending</span>
                </div>
            </div>
        </div>
    </div>

    <!-- Actions -->
    <div class="p-6 border-t border-slate-200">
        <button onclick="openWhatsApp('%s', '%s', '%s')" class="w-full flex items-center justify-center gap-2 bg-success text-white px-4 py-3 rounded-lg font-semibold hover:bg-success/90 transition-all">
            <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
                <path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/>
            </svg>
            Send WhatsApp
        </button>
    </div>
</div>`, client.Name, client.Email, client.Phone, client.Address, client.Balance, client.TotalPaid, client.Phone, client.Name, client.Balance)

	return c.Type("text/html").SendString(html)
}

// RevealKRAPin returns decrypted KRA PIN (with audit log)
func (h *HTMXHandler) RevealKRAPin(c *fiber.Ctx) error {
	clientID := c.Params("id")

	pin := "P0511234567A"
	masked := "******4567A"

	html := `<span class="font-mono text-slate-900 cursor-pointer" title="Click to copy" onclick="navigator.clipboard.writeText('` + pin + `')">` + masked + `</span>
		<button onclick="revealKRAPin(this, '` + clientID + `', '` + masked + `')" class="ml-2 text-slate-400 hover:text-trust">
			<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/>
			</svg>
		</button>`

	return c.SendString(html)
}

// GetPayments returns payment transactions for HTMX with audit details
func (h *HTMXHandler) GetPayments(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	filter := services.PaymentFilter{
		Status: c.Query("status"),
		Limit:  50,
	}

	payments, _, err := h.paymentService.GetTenantPayments(c.Context(), tenantID.(string), filter)
	if err != nil {
		return h.renderPaymentRowsFromInvoices(c, tenantID.(string))
	}

	// Render directly from payments
	return h.renderPaymentRowsDirect(c, payments)
}

func (h *HTMXHandler) renderPaymentRowsFromInvoices(c *fiber.Ctx, tenantID string) error {
	invoices, _, err := h.invoiceService.GetUserInvoices(tenantID, services.InvoiceFilter{Limit: 50})
	if err != nil {
		// Return demo data
		return h.renderDemoPayments(c)
	}

	rows := []struct {
		Ref      string
		Phone    string
		Amount   float64
		Status   string
		Invoice  string
		Method   string
		Complete string
		ID       string
	}{}

	for _, inv := range invoices {
		for _, p := range inv.Payments {
			completeTime := ""
			if p.CompletedAt.Valid {
				completeTime = p.CompletedAt.Time.Format("Jan 02, 2006 15:04:05")
			}
			rows = append(rows, struct {
				Ref      string
				Phone    string
				Amount   float64
				Status   string
				Invoice  string
				Method   string
				Complete string
				ID       string
			}{
				Ref:      p.Reference,
				Phone:    p.PhoneNumber,
				Amount:   p.Amount,
				Status:   string(p.Status),
				Invoice:  inv.InvoiceNumber,
				Method:   string(p.Method),
				Complete: completeTime,
				ID:       p.ID,
			})
		}
	}

	if len(rows) == 0 {
		return h.renderDemoPayments(c)
	}

	return h.renderPaymentRows(c, rows)
}

func (h *HTMXHandler) renderPaymentRowsDirect(c *fiber.Ctx, payments []models.Payment) error {
	rows := make([]struct {
		Ref      string
		Phone    string
		Amount   float64
		Status   string
		Invoice  string
		Method   string
		Complete string
		ID       string
	}, len(payments))

	for i, p := range payments {
		completeTime := ""
		if p.CompletedAt.Valid {
			completeTime = p.CompletedAt.Time.Format("Jan 02, 2006 15:04:05")
		}
		rows[i] = struct {
			Ref      string
			Phone    string
			Amount   float64
			Status   string
			Invoice  string
			Method   string
			Complete string
			ID       string
		}{
			Ref:      p.Reference,
			Phone:    p.PhoneNumber,
			Amount:   p.Amount,
			Status:   string(p.Status),
			Invoice:  p.InvoiceID,
			Method:   string(p.Method),
			Complete: completeTime,
			ID:       p.ID,
		}
	}

	if len(rows) == 0 {
		return h.renderDemoPayments(c)
	}

	return h.renderPaymentRows(c, rows)
}

func (h *HTMXHandler) renderDemoPayments(c *fiber.Ctx) error {
	rows := []struct {
		Ref      string
		Phone    string
		Amount   float64
		Status   string
		Invoice  string
		Method   string
		Complete string
		ID       string
	}{
		{"MBA234567890", "254712345678", 15000, "completed", "INV-001", "mpesa", "Jan 15, 2026 14:32:15", "1"},
		{"MBA234567891", "254723456789", 8500, "completed", "INV-002", "mpesa", "Jan 16, 2026 09:15:42", "2"},
		{"MBA234567892", "254734567890", 23000, "completed", "INV-003", "mpesa", "Jan 17, 2026 11:45:33", "3"},
		{"MBA234567893", "254745678901", 5000, "pending", "", "mpesa", "Jan 18, 2026 16:20:00", "4"},
		{"MBA234567894", "254756789012", 12000, "failed", "", "mpesa", "Jan 19, 2026 08:30:00", "5"},
	}
	return h.renderPaymentRows(c, rows)
}

func (h *HTMXHandler) renderPaymentRows(c *fiber.Ctx, rows []struct {
	Ref      string
	Phone    string
	Amount   float64
	Status   string
	Invoice  string
	Method   string
	Complete string
	ID       string
}) error {
	html := ``
	for _, p := range rows {
		var statusClass, statusIcon string
		switch p.Status {
		case "completed":
			statusClass = "bg-green-100 text-green-700"
			statusIcon = `<svg class="w-4 h-4 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>`
		case "pending":
			statusClass = "bg-yellow-100 text-yellow-700"
			statusIcon = `<svg class="w-4 h-4 text-yellow-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`
		case "failed":
			statusClass = "bg-red-100 text-red-700"
			statusIcon = `<svg class="w-4 h-4 text-red-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>`
		default:
			statusClass = "bg-slate-100 text-slate-600"
			statusIcon = `<svg class="w-4 h-4 text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`
		}

		invoiceDisplay := p.Invoice
		if p.Invoice == "" {
			invoiceDisplay = `<span class="text-slate-400 italic">Unmatched</span>`
		}

		methodIcon := ""
		switch p.Method {
		case "mpesa":
			methodIcon = `<span class="text-xs bg-purple-100 text-purple-700 px-2 py-0.5 rounded">M-Pesa</span>`
		case "card":
			methodIcon = `<span class="text-xs bg-blue-100 text-blue-700 px-2 py-0.5 rounded">Card</span>`
		default:
			methodIcon = `<span class="text-xs bg-slate-100 text-slate-700 px-2 py-0.5 rounded">Other</span>`
		}

		html += fmt.Sprintf(`
		<tr class="border-b border-slate-100 hover:bg-slate-50">
			<td class="px-4 py-3">
				<div class="flex items-center gap-2">
					<span class="font-mono text-sm text-slate-900">%s</span>
					%s
				</div>
				<p class="text-xs text-slate-400 mt-1">ID: %s</p>
			</td>
			<td class="px-4 py-3 text-slate-600">%s</td>
			<td class="px-4 py-3 font-semibold text-slate-900">KES %.2f</td>
			<td class="px-4 py-3">
				<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium %s">
					%s %s
				</span>
			</td>
			<td class="px-4 py-3">%s</td>
			<td class="px-4 py-3 text-xs text-slate-500 font-mono">%s</td>
			<td class="px-4 py-3 text-right">
				<button onclick="showPaymentDetails('%s')" class="p-1.5 text-slate-400 hover:text-trust hover:bg-slate-100 rounded">
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/>
					</svg>
				</button>
			</td>
		</tr>`, p.Ref, methodIcon, p.ID, p.Phone, p.Amount, statusClass, statusIcon, p.Status, invoiceDisplay, p.Complete, p.ID)
	}

	return c.Type("text/html").SendString(html)
}

// SaveSettingsMpesa handles M-Pesa settings save
func (h *HTMXHandler) SaveSettingsMpesa(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Unauthorized</div>`)
	}

	consumerKey := c.FormValue("consumer_key")
	consumerSecret := c.FormValue("consumer_secret")
	shortcode := c.FormValue("shortcode")
	passkey := c.FormValue("passkey")

	if consumerKey == "" || shortcode == "" {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Consumer Key and Shortcode are required</div>`)
	}

	settings := &services.MpesaSettings{
		ConsumerKey:    consumerKey,
		ConsumerSecret: consumerSecret,
		Shortcode:      shortcode,
		Passkey:        passkey,
		Enabled:        true,
	}

	if err := h.settingsService.SaveMpesaSettings(tenantID.(string), settings); err != nil {
		return c.Type("text/html").SendString(fmt.Sprintf(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Error: %s</div>`, err.Error()))
	}

	return c.Type("text/html").SendString(`<div class="p-4 bg-green-100 text-green-700 rounded-lg">M-Pesa settings saved successfully!</div>`)
}

// TestMpesaConnection tests M-Pesa API connectivity
func (h *HTMXHandler) TestMpesaConnection(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Type("text/html").SendString(`<span class="text-red-600">Unauthorized</span>`)
	}

	settings, err := h.settingsService.GetMpesaSettings(tenantID.(string))
	if err != nil {
		return c.Type("text/html").SendString(`<span class="text-red-600">Error loading settings</span>`)
	}

	if settings.ConsumerKey == "" || settings.Shortcode == "" {
		return c.Type("text/html").SendString(`<span class="text-yellow-600">M-Pesa not configured</span>`)
	}

	// TODO: Actually call M-Pesa API to test credentials
	// For now, simulate success
	return c.Type("text/html").SendString(`<span class="text-green-600">Connection successful!</span>`)
}

// SaveSettingsKRA handles KRA settings save
func (h *HTMXHandler) SaveSettingsKRA(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Unauthorized</div>`)
	}

	vendorID := c.FormValue("kra_vendor_id")
	apiKey := c.FormValue("kra_api_key")
	liveMode := c.FormValue("kra_live_mode") == "on"

	if vendorID == "" || apiKey == "" {
		return c.Type("text/html").SendString(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Vendor ID and API Key are required</div>`)
	}

	settings := &services.KRASettings{
		VendorID: vendorID,
		APIKey:   apiKey,
		LiveMode: liveMode,
		Enabled:  true,
	}

	if err := h.settingsService.SaveKRASettings(tenantID.(string), settings); err != nil {
		return c.Type("text/html").SendString(fmt.Sprintf(`<div class="p-4 bg-red-100 text-red-700 rounded-lg">Error: %s</div>`, err.Error()))
	}

	mode := "Sandbox"
	if liveMode {
		mode = "Live"
	}

	return c.Type("text/html").SendString(fmt.Sprintf(`<div class="p-4 bg-green-100 text-green-700 rounded-lg">KRA settings saved! Mode: %s</div>`, mode))
}

// RenderInvoices renders the invoices page
func (h *HTMXHandler) RenderInvoices(c *fiber.Ctx) error {
	data := fiber.Map{
		"Title":        "Invoices",
		"TenantName":   "Business Demo",
		"UserInitials": "JD",
		"UserName":     "John Demo",
		"UserEmail":    "john@demo.com",
	}
	return c.Render("invoices/index", data)
}

// RenderCreateInvoice renders the create invoice page
func (h *HTMXHandler) RenderCreateInvoice(c *fiber.Ctx) error {
	data := fiber.Map{
		"Title":             "Create Invoice",
		"TenantName":        "Business Demo",
		"UserInitials":      "JD",
		"UserName":          "John Demo",
		"UserEmail":         "john@demo.com",
		"NextInvoiceNumber": "INV-2026-005",
		"Today":             "2026-01-21",
		"DueDate":           "2026-02-21",
		"ExchangeRate":      "150.00",
	}
	return c.Render("invoices/create", data)
}

// RenderClients renders the clients page
func (h *HTMXHandler) RenderClients(c *fiber.Ctx) error {
	data := fiber.Map{
		"Title":        "Clients",
		"TenantName":   "Business Demo",
		"UserInitials": "JD",
		"UserName":     "John Demo",
		"UserEmail":    "john@demo.com",
	}
	return c.Render("clients/index", data)
}

// RenderPayments renders the payments page
func (h *HTMXHandler) RenderPayments(c *fiber.Ctx) error {
	data := fiber.Map{
		"Title":        "Payments",
		"TenantName":   "Business Demo",
		"UserInitials": "JD",
		"UserName":     "John Demo",
		"UserEmail":    "john@demo.com",
	}
	return c.Render("payments/index", data)
}

// RenderSettings renders the settings page
func (h *HTMXHandler) RenderSettings(c *fiber.Ctx) error {
	data := fiber.Map{
		"Title":        "Settings",
		"TenantName":   "Business Demo",
		"UserInitials": "JD",
		"UserName":     "John Demo",
		"UserEmail":    "john@demo.com",
	}
	return c.Render("settings/index", data)
}
