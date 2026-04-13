package services

import (
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

type ReportService struct {
	db *database.DB
}

func NewReportService(db *database.DB) *ReportService {
	return &ReportService{db: db}
}

type ReportFilter struct {
	Period    string // 7, 30, 90, 365
	StartDate *time.Time
	EndDate   *time.Time
}

func (s *ReportService) getDateRange(period string) (time.Time, time.Time) {
	now := time.Now()
	var start time.Time

	switch period {
	case "7":
		start = now.AddDate(0, 0, -7)
	case "30":
		start = now.AddDate(0, 0, -30)
	case "90":
		start = now.AddDate(0, 0, -90)
	case "180":
		start = now.AddDate(0, 0, -180)
	case "365":
		start = now.AddDate(0, 0, -365)
	default:
		start = now.AddDate(0, 0, -30)
	}

	return start, now
}

type ReportOverview struct {
	TotalRevenue  float64 `json:"total_revenue"`
	RevenueChange float64 `json:"revenue_change"`
	PendingAmount float64 `json:"pending_amount"`
	PendingCount  int64   `json:"pending_count"`
	PaidAmount    float64 `json:"paid_amount"`
	PaidCount     int64   `json:"paid_count"`
	OverdueAmount float64 `json:"overdue_amount"`
	OverdueCount  int64   `json:"overdue_count"`
	TotalClients  int64   `json:"total_clients"`
	TotalInvoices int64   `json:"total_invoices"`
	GSTCollected  float64 `json:"tax_collected"`
}

func (s *ReportService) GetOverview(tenantID string, period string) (*ReportOverview, error) {
	start, end := s.getDateRange(period)

	var result ReportOverview

	// Get revenue from payments
	err := s.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND status = 'completed' AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Select("COALESCE(SUM(amount), 0) as total").
		Scan(&result.TotalRevenue).Error
	if err != nil {
		return nil, err
	}

	// Get previous period for comparison
	prevStart := start.AddDate(0, 0, -int(end.Sub(start).Hours()/24))
	err = s.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND status = 'completed' AND created_at BETWEEN ? AND ?", tenantID, prevStart, start).
		Select("COALESCE(SUM(amount), 0) as total").
		Scan(&result.RevenueChange).Error

	if result.RevenueChange > 0 {
		result.RevenueChange = ((result.TotalRevenue - result.RevenueChange) / result.RevenueChange) * 100
	}

	// Pending invoices (sent but not paid)
	err = s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status IN ?", tenantID, []string{"sent", "viewed", "partially_paid"}).
		Select("COALESCE(SUM(total - paid_amount), 0) as total").
		Scan(&result.PendingAmount).Error
	if err != nil {
		return nil, err
	}

	err = s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status IN ?", tenantID, []string{"sent", "viewed", "partially_paid"}).
		Count(&result.PendingCount).Error
	if err != nil {
		return nil, err
	}

	// Paid invoices
	err = s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status = 'paid'", tenantID).
		Count(&result.PaidCount).Error
	if err != nil {
		return nil, err
	}

	// Overdue
	err = s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status = 'overdue'", tenantID).
		Select("COALESCE(SUM(total - paid_amount), 0) as total").
		Scan(&result.OverdueAmount).Error
	if err != nil {
		return nil, err
	}

	err = s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status = 'overdue'", tenantID).
		Count(&result.OverdueCount).Error
	if err != nil {
		return nil, err
	}

	// Total clients
	err = s.db.Model(&models.Client{}).
		Where("tenant_id = ?", tenantID).
		Count(&result.TotalClients).Error
	if err != nil {
		return nil, err
	}

	// Total invoices
	err = s.db.Model(&models.Invoice{}).
		Where("tenant_id = ?", tenantID).
		Count(&result.TotalInvoices).Error
	if err != nil {
		return nil, err
	}

	// Tax collected
	err = s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Select("COALESCE(SUM(tax_amount), 0) as total").
		Scan(&result.GSTCollected).Error

	return &result, nil
}

type RevenueDataPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

func (s *ReportService) GetRevenue(tenantID string, period string) ([]RevenueDataPoint, error) {
	start, end := s.getDateRange(period)

	var results []RevenueDataPoint

	// Group by date
	err := s.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND status = 'completed' AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Select("DATE(created_at) as date, COALESCE(SUM(amount), 0) as value").
		Group("DATE(created_at)").
		Order("date").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return []RevenueDataPoint{}, nil
	}

	return results, nil
}

type InvoiceDataPoint struct {
	Date   string `json:"date"`
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

func (s *ReportService) GetInvoices(tenantID string, period string) ([]InvoiceDataPoint, error) {
	start, end := s.getDateRange(period)

	var results []InvoiceDataPoint

	err := s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Select("DATE(created_at) as date, status, COUNT(*) as count").
		Group("DATE(created_at), status").
		Order("date").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return []InvoiceDataPoint{}, nil
	}

	return results, nil
}

func (s *ReportService) GetInvoiceStats(tenantID string, period string) (map[string]interface{}, error) {
	start, end := s.getDateRange(period)

	stats := make(map[string]interface{})

	// Total created
	var total int64
	err := s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Count(&total).Error
	if err != nil {
		return nil, err
	}
	stats["total"] = total

	// By status
	type statusCount struct {
		Status string
		Count  int64
	}
	var statusCounts []statusCount
	err = s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&statusCounts).Error
	if err != nil {
		return nil, err
	}

	statusMap := make(map[string]int64)
	for _, sc := range statusCounts {
		statusMap[sc.Status] = sc.Count
	}
	stats["by_status"] = statusMap

	return stats, nil
}

func (s *ReportService) GetPayments(tenantID string, period string) ([]RevenueDataPoint, error) {
	start, end := s.getDateRange(period)

	var results []RevenueDataPoint

	err := s.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Select("DATE(created_at) as date, COALESCE(SUM(amount), 0) as value").
		Group("DATE(created_at)").
		Order("date").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return []RevenueDataPoint{}, nil
	}

	return results, nil
}

func (s *ReportService) GetPaymentStats(tenantID string, period string) (map[string]interface{}, error) {
	start, end := s.getDateRange(period)

	stats := make(map[string]interface{})

	var total int64
	err := s.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Count(&total).Error
	if err != nil {
		return nil, err
	}
	stats["total"] = total

	// By method
	type methodCount struct {
		Method string
		Count  int64
	}
	var methodCounts []methodCount
	err = s.db.Model(&models.Payment{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Select("method, COUNT(*) as count").
		Group("method").
		Scan(&methodCounts).Error
	if err != nil {
		return nil, err
	}

	methodMap := make(map[string]int64)
	for _, mc := range methodCounts {
		methodMap[mc.Method] = mc.Count
	}
	stats["by_method"] = methodMap

	return stats, nil
}

func (s *ReportService) GetClients(tenantID string, period string) ([]RevenueDataPoint, error) {
	start, end := s.getDateRange(period)

	var results []RevenueDataPoint

	err := s.db.Model(&models.Client{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Select("DATE(created_at) as date, COUNT(*) as value").
		Group("DATE(created_at)").
		Order("date").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return []RevenueDataPoint{}, nil
	}

	return results, nil
}

func (s *ReportService) GetClientStats(tenantID string, period string) (map[string]interface{}, error) {
	start, end := s.getDateRange(period)

	stats := make(map[string]interface{})

	var total int64
	err := s.db.Model(&models.Client{}).
		Where("tenant_id = ?", tenantID).
		Count(&total).Error
	if err != nil {
		return nil, err
	}
	stats["total"] = total

	// New clients in period
	var newClients int64
	err = s.db.Model(&models.Client{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, start, end).
		Count(&newClients).Error
	if err != nil {
		return nil, err
	}
	stats["new"] = newClients

	return stats, nil
}

type TaxSummary struct {
	TotalSales   float64 `json:"total_sales"`
	TotalTax     float64 `json:"total_tax"`
	TaxableSales float64 `json:"taxable_sales"`
	ZeroRated    float64 `json:"zero_rated"`
	Exempt       float64 `json:"exempt"`
}

func (s *ReportService) GetTaxSummary(tenantID string, period string) (*TaxSummary, error) {
	start, end := s.getDateRange(period)

	summary := &TaxSummary{}

	// Get invoices with tax
	err := s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ? AND tax_rate > 0", tenantID, start, end).
		Select("COALESCE(SUM(total), 0) as total_sales, COALESCE(SUM(tax_amount), 0) as total_tax").
		Scan(summary).Error
	if err != nil {
		return nil, err
	}

	// Zero rated (tax_rate = 0 but would have been taxable)
	err = s.db.Model(&models.Invoice{}).
		Where("tenant_id = ? AND created_at BETWEEN ? AND ? AND tax_rate = 0", tenantID, start, end).
		Select("COALESCE(SUM(total), 0) as zero_rated").
		Scan(summary).Error
	if err != nil {
		return nil, err
	}

	summary.TaxableSales = summary.TotalSales - summary.ZeroRated

	return summary, nil
}

func (s *ReportService) ExportReport(tenantID, format, period string) ([]byte, error) {
	overview, err := s.GetOverview(tenantID, period)
	if err != nil {
		return nil, fmt.Errorf("failed to get overview: %w", err)
	}

	// Simple CSV export
	csv := "Metric,Value\n"
	csv += fmt.Sprintf("Total Revenue,%.2f\n", overview.TotalRevenue)
	csv += fmt.Sprintf("Paid Invoices,%d\n", overview.PaidCount)
	csv += fmt.Sprintf("Pending Invoices,%d\n", overview.PendingCount)
	csv += fmt.Sprintf("Overdue Invoices,%d\n", overview.OverdueCount)
	csv += fmt.Sprintf("Total Clients,%d\n", overview.TotalClients)
	csv += fmt.Sprintf("Tax Collected,%.2f\n", overview.GSTCollected)

	return []byte(csv), nil
}
