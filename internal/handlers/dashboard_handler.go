package handlers

import (
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// ChartData represents chart data for visualization
type ChartData struct {
	Labels   []string  `json:"labels"`
	Values   []float64 `json:"values"`
	Currency string    `json:"currency"`
}

// DashboardHandler handles dashboard API endpoints
type DashboardHandler struct {
	invoiceService *services.InvoiceService
	clientService  *services.ClientService
}

// NewDashboardHandler creates DashboardHandler
func NewDashboardHandler(invoiceSvc *services.InvoiceService, clientSvc *services.ClientService) *DashboardHandler {
	return &DashboardHandler{
		invoiceService: invoiceSvc,
		clientService:  clientSvc,
	}
}

// GetDashboard returns full dashboard data with charts
func (h *DashboardHandler) GetDashboard(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	period := c.Query("period", "month")

	stats, err := h.invoiceService.GetDashboardStats(tenantID, period)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	recentInvoices, _, err := h.invoiceService.GetUserInvoices(tenantID, services.InvoiceFilter{
		Offset: 0,
		Limit:  10,
	})
	var invoiceList []models.Invoice
	if err != nil {
		invoiceList = []models.Invoice{}
	} else {
		invoiceList = recentInvoices
	}

	recentClients, _, err := h.clientService.GetUserClients(tenantID, services.ClientFilter{
		Offset: 0,
		Limit:  10,
	})
	var clientList []models.Client
	if err != nil {
		clientList = []models.Client{}
	} else {
		clientList = recentClients
	}

	// Get chart data
	revenueChart := h.getRevenueChartData(tenantID, period)
	statusChart := h.getStatusChartData(tenantID)
	clientChart := h.getClientRevenueChartData(tenantID)

	return c.JSON(fiber.Map{
		"stats":           stats,
		"recent_invoices": invoiceList,
		"recent_clients":  clientList,
		"revenue_chart":   revenueChart,
		"status_chart":    statusChart,
		"client_chart":    clientChart,
	})
}

// GetDashboardSummary returns quick summary without charts (for initial load)
func (h *DashboardHandler) GetDashboardSummary(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	period := c.Query("period", "month")
	stats, err := h.invoiceService.GetDashboardStats(tenantID, period)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

// GetRevenueTrend returns revenue trend data (monthly)
func (h *DashboardHandler) GetRevenueTrend(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	stats, _ := h.invoiceService.GetDashboardStats(tenantID, "month")

	return c.JSON(fiber.Map{
		"monthly_revenue":  stats.MonthlyRevenue,
		"monthly_invoices": stats.MonthlyInvoices,
	})
}

// GetDailyTrend returns daily revenue/invoice data
func (h *DashboardHandler) GetDailyTrend(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	period := c.Query("period", "month")
	stats, _ := h.invoiceService.GetDashboardStats(tenantID, period)

	return c.JSON(fiber.Map{
		"daily_revenue":  stats.DailyRevenue,
		"daily_invoices": stats.DailyInvoices,
	})
}

// GetTopClients returns top paying clients
func (h *DashboardHandler) GetTopClients(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	limit := c.QueryInt("limit", 5)
	stats, _ := h.invoiceService.GetDashboardStats(tenantID, "month")

	var topClients []services.ClientRevenue
	if limit <= len(stats.TopPayingClients) {
		topClients = stats.TopPayingClients[:limit]
	} else {
		topClients = stats.TopPayingClients
	}

	return c.JSON(fiber.Map{"clients": topClients})
}

// GetActivityLog returns recent activity
func (h *DashboardHandler) GetActivityLog(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	limit := c.QueryInt("limit", 10)

	// Get recent invoices
	invoices, _, _ := h.invoiceService.GetUserInvoices(tenantID, services.InvoiceFilter{
		Offset: 0,
		Limit:  limit,
	})

	// Get recent payments
	payments := h.getRecentPayments(tenantID, limit)

	return c.JSON(fiber.Map{
		"invoices": invoices,
		"payments": payments,
	})
}

func (h *DashboardHandler) getRecentPayments(tenantID string, limit int) []map[string]interface{} {
	var payments []map[string]interface{}

	rows, err := h.invoiceService.GetDB().
		Scopes(database.TenantFilter(tenantID)).
		Model(&models.Payment{}).
		Preload("Invoice").
		Order("created_at DESC").
		Limit(limit).
		Rows()

	if err != nil {
		return payments
	}
	defer rows.Close()

	for rows.Next() {
		var payment models.Payment
		if err := rows.Scan(&payment); err == nil {
			payments = append(payments, map[string]interface{}{
				"id":         payment.ID,
				"invoice_id": payment.InvoiceID,
				"amount":     payment.Amount,
				"status":     payment.Status,
				"created_at": payment.CreatedAt,
				"method":     payment.Method,
			})
		}
	}
	return payments
}

// GetStats returns only statistics
func (h *DashboardHandler) GetStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	period := c.Query("period", "month")
	stats, err := h.invoiceService.GetDashboardStats(tenantID, period)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

// GetRecentInvoices returns recent invoices
func (h *DashboardHandler) GetRecentInvoices(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoices, total, err := h.invoiceService.GetUserInvoices(tenantID, services.InvoiceFilter{
		Offset: 0,
		Limit:  10,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"invoices": invoices, "total": total})
}

// GetHTMXInvoices returns invoices for HTMX partial rendering
func (h *DashboardHandler) GetHTMXInvoices(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	status := c.Query("status", "")

	filter := services.InvoiceFilter{
		Offset: 0,
		Limit:  10,
	}

	if status != "" {
		filter.Status = status
	}

	invoices, _, err := h.invoiceService.GetUserInvoices(tenantID, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Render the partial HTML
	return c.Render("partials/invoice_list", fiber.Map{
		"invoices": invoices,
	})
}

// GetRecentClients returns recent clients
func (h *DashboardHandler) GetRecentClients(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	clients, total, err := h.clientService.GetUserClients(tenantID, services.ClientFilter{
		Offset: 0,
		Limit:  10,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"clients": clients, "total": total})
}

// GetRevenueChart returns revenue chart data
func (h *DashboardHandler) GetRevenueChart(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	period := c.Query("period", "month")
	chartData := h.getRevenueChartData(tenantID, period)

	return c.JSON(chartData)
}

// GetStatusChart returns status distribution chart
func (h *DashboardHandler) GetStatusChart(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	chartData := h.getStatusChartData(tenantID)

	return c.JSON(chartData)
}

// GetClientRevenueChart returns client revenue chart
func (h *DashboardHandler) GetClientRevenueChart(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	chartData := h.getClientRevenueChartData(tenantID)

	return c.JSON(chartData)
}

// Helper functions for chart data

func (h *DashboardHandler) getRevenueChartData(tenantID, period string) ChartData {
	now := time.Now()
	var days int
	var startDate time.Time
	switch period {
	case "week":
		days = 7
		startDate = now.AddDate(0, 0, -7)
	case "month":
		days = 30
		startDate = now.AddDate(0, -1, 0)
	case "quarter":
		days = 90
		startDate = now.AddDate(0, -3, 0)
	case "year":
		days = 365
		startDate = now.AddDate(-1, 0, 0)
	default:
		days = 30
		startDate = now.AddDate(0, -1, 0)
	}

	labels := make([]string, days)
	values := make([]float64, days)

	// Generate labels
	for i := 0; i < days; i++ {
		date := startDate.AddDate(0, 0, i+1)
		if i == days-1 {
			labels[i] = "Today"
		} else if i == days-2 {
			labels[i] = "Yesterday"
		} else {
			labels[i] = date.Format("Jan 02")
		}
	}

	// Get daily revenue from database
	db := h.invoiceService.GetDB()
	rows, err := db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Invoice{}).
		Select("DATE(paid_at) as date, COALESCE(SUM(total), 0) as total").
		Where("status = ? AND paid_at >= ?", models.InvoiceStatusPaid, startDate).
		Group("DATE(paid_at)").
		Order("date").
		Rows()

	if err == nil {
		defer rows.Close()
		dateRevenue := make(map[string]float64)
		for rows.Next() {
			var date time.Time
			var total float64
			if err := rows.Scan(&date, &total); err == nil {
				dateRevenue[date.Format("2006-01-02")] = total
			}
		}
		// Map to values
		for i := 0; i < days; i++ {
			d := startDate.AddDate(0, 0, i+1)
			key := d.Format("2006-01-02")
			values[i] = dateRevenue[key]
		}
	}

	return ChartData{
		Labels:   labels,
		Values:   values,
		Currency: "KES",
	}
}

func (h *DashboardHandler) getStatusChartData(tenantID string) map[string]interface{} {
	// This will be populated from the dashboard stats
	// Initialize with zeros, will be filled in GetDashboard call
	return map[string]interface{}{
		"labels": []string{"Draft", "Sent", "Paid", "Overdue"},
		"values": []int{0, 0, 0, 0},
		"colors": []string{"#94a3b8", "#3b82f6", "#22c55e", "#ef4444"},
	}
}

func (h *DashboardHandler) getClientRevenueChartData(tenantID string) ChartData {
	clients, _, _ := h.clientService.GetUserClients(tenantID, services.ClientFilter{
		Limit: 10,
	})

	labels := make([]string, len(clients))
	values := make([]float64, len(clients))

	for i, client := range clients {
		labels[i] = client.Name
		values[i] = client.TotalBilled
	}

	return ChartData{
		Labels:   labels,
		Values:   values,
		Currency: "KES",
	}
}
