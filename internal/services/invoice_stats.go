package services

import (
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

)

// GetDashboardStats returns dashboard statistics for a tenant (tenant-scoped)
func (s *InvoiceService) GetDashboardStats(tenantID, period string) (*DashboardStats, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	var stats DashboardStats

	// Determine date range
	now := time.Now()
	var startDate, prevStartDate time.Time
	var periodDays int
	switch period {
	case "week":
		startDate = now.AddDate(0, 0, -7)
		prevStartDate = now.AddDate(0, 0, -14)
		periodDays = 7
	case "month":
		startDate = now.AddDate(0, -1, 0)
		prevStartDate = now.AddDate(0, -2, 0)
		periodDays = 30
	case "quarter":
		startDate = now.AddDate(0, -3, 0)
		prevStartDate = now.AddDate(0, -6, 0)
		periodDays = 90
	case "year":
		startDate = now.AddDate(-1, 0, 0)
		prevStartDate = now.AddDate(-2, 0, 0)
		periodDays = 365
	default:
		startDate = now.AddDate(0, -1, 0)
		prevStartDate = now.AddDate(0, -2, 0)
		periodDays = 30
	}

	// Total revenue (all time paid)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ?", models.InvoiceStatusPaid).
		Select("COALESCE(SUM(total), 0)").
		Scan(&stats.TotalRevenue)

	// Revenue this period
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ? AND paid_at >= ?", models.InvoiceStatusPaid, startDate).
		Select("COALESCE(SUM(total), 0)").
		Scan(&stats.RevenueThisPeriod)

	// Revenue previous period (for comparison)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ? AND paid_at >= ? AND paid_at < ?", models.InvoiceStatusPaid, prevStartDate, startDate).
		Select("COALESCE(SUM(total), 0)").
		Scan(&stats.RevenuePreviousPeriod)

	// Calculate revenue growth
	if stats.RevenuePreviousPeriod > 0 {
		stats.RevenueGrowth = ((stats.RevenueThisPeriod - stats.RevenuePreviousPeriod) / stats.RevenuePreviousPeriod) * 100
	} else if stats.RevenueThisPeriod > 0 {
		stats.RevenueGrowth = 100
	}

	// Outstanding (unpaid + partially paid)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status IN ?", []string{string(models.InvoiceStatusSent), string(models.InvoiceStatusViewed), string(models.InvoiceStatusPartiallyPaid), string(models.InvoiceStatusOverdue)}).
		Select("COALESCE(SUM(total - paid_amount), 0)").
		Scan(&stats.Outstanding)

	// Count by status
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusDraft).Count(&stats.DraftCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusSent).Count(&stats.SentCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusPaid).Count(&stats.PaidCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusOverdue).Count(&stats.OverdueCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusPartiallyPaid).Count(&stats.PartialCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusCancelled).Count(&stats.CancelledCount)

	// Total clients
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Client{}).Count(&stats.TotalClients)

	// Total invoices
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Count(&stats.TotalInvoices)

	// Invoice growth (this period vs previous)
	var currentInvoices, prevInvoices int64
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("created_at >= ?", startDate).Count(&currentInvoices)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("created_at >= ? AND created_at < ?", prevStartDate, startDate).Count(&prevInvoices)
	if prevInvoices > 0 {
		stats.InvoiceGrowth = (float64(currentInvoices-prevInvoices) / float64(prevInvoices)) * 100
	}

	// Average invoice value
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ?", models.InvoiceStatusPaid).
		Select("COALESCE(AVG(total), 0)").
		Scan(&stats.AvgInvoiceValue)

	// Collection rate (paid / total billed)
	var totalBilled, totalPaid float64
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Select("COALESCE(SUM(total), 0)").Scan(&totalBilled)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ?", models.InvoiceStatusPaid).
		Select("COALESCE(SUM(total), 0)").Scan(&totalPaid)
	if totalBilled > 0 {
		stats.CollectionRate = (totalPaid / totalBilled) * 100
	}

	// Average payment days
	var avgDays float64
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("status = ? AND paid_at IS NOT NULL", models.InvoiceStatusPaid).
		Select("COALESCE(AVG(EXTRACT(EPOCH FROM (paid_at - created_at)) / 86400), 0)").Scan(&avgDays)
	stats.AvgPaymentDays = avgDays

	// Get monthly revenue for last 12 months
	stats.MonthlyRevenue = s.getMonthlyRevenue(tenantID, 12)
	stats.MonthlyInvoices = s.getMonthlyInvoices(tenantID, 12)

	// Get daily revenue for current period
	stats.DailyRevenue = s.getDailyRevenue(tenantID, startDate, periodDays)
	stats.DailyInvoices = s.getDailyInvoices(tenantID, startDate, periodDays)

	// Get top paying clients
	stats.TopPayingClients = s.getTopPayingClients(tenantID, 5)

	// Recent invoices
	s.db.Scopes(database.TenantFilter(tenantID)).Preload("Client").
		Order("created_at DESC").
		Limit(5).
		Find(&stats.RecentInvoices)

	return &stats, nil
}

// GetInvoiceStats returns invoice-specific statistics for the invoices list page
func (s *InvoiceService) GetInvoiceStats(tenantID string) (*InvoiceStats, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	var stats InvoiceStats

	// Total invoices count
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Count(&stats.TotalInvoices)

	// Count by status
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusDraft).Count(&stats.DraftCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusSent).Count(&stats.SentCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusViewed).Count(&stats.ViewedCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusPaid).Count(&stats.PaidCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusPartiallyPaid).Count(&stats.PartiallyPaidCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusOverdue).Count(&stats.OverdueCount)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusCancelled).Count(&stats.CancelledCount)

	// Total value by status
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusPaid).Select("COALESCE(SUM(total), 0)").Scan(&stats.TotalPaid)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status IN ?", []string{string(models.InvoiceStatusSent), string(models.InvoiceStatusViewed), string(models.InvoiceStatusPartiallyPaid), string(models.InvoiceStatusOverdue)}).Select("COALESCE(SUM(total - paid_amount), 0)").Scan(&stats.TotalOutstanding)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusOverdue).Select("COALESCE(SUM(total - paid_amount), 0)").Scan(&stats.TotalOverdue)

	// Calculate totals
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Select("COALESCE(SUM(total), 0)").Scan(&stats.TotalValue)

	// Overdue invoices count
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("status = ?", models.InvoiceStatusOverdue).Count(&stats.OverdueInvoicesCount)

	// Recently created invoices (last 7 days)
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("created_at >= ?", sevenDaysAgo).Count(&stats.Last7DaysInvoices)
	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Where("created_at >= ?", sevenDaysAgo).Select("COALESCE(SUM(total), 0)").Scan(&stats.Last7DaysValue)

	// Average invoice value
	if stats.TotalInvoices > 0 {
		stats.AverageInvoiceValue = stats.TotalValue / float64(stats.TotalInvoices)
	}

	// Collection rate
	if stats.TotalValue > 0 {
		stats.CollectionRate = (stats.TotalPaid / stats.TotalValue) * 100
	}

	return &stats, nil
}

type KRADashboardStats struct {
	Total     int64 `json:"total"`
	Submitted int64 `json:"submitted"`
	Pending   int64 `json:"pending"`
	Failed    int64 `json:"failed"`
}

func (s *InvoiceService) GetKRADashboardStats(tenantID string) (*KRADashboardStats, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	var stats KRADashboardStats

	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).Count(&stats.Total)

	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("kra_status = 'submitted'").
		Count(&stats.Submitted)

	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("kra_status = 'pending'").
		Count(&stats.Pending)

	s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
		Where("kra_status = 'failed'").
		Count(&stats.Failed)

	return &stats, nil
}

// Helper functions for dashboard stats

func (s *InvoiceService) getMonthlyRevenue(tenantID string, months int) []MonthlyData {
	var data []MonthlyData
	now := time.Now()

	for i := months - 1; i >= 0; i-- {
		monthStart := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, now.Location())
		monthEnd := monthStart.AddDate(0, 1, 0)

		var total float64
		s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
			Where("status = ? AND paid_at >= ? AND paid_at < ?", models.InvoiceStatusPaid, monthStart, monthEnd).
			Select("COALESCE(SUM(total), 0)").
			Scan(&total)

		data = append(data, MonthlyData{
			Month:  monthStart.Format("2006-01"),
			Amount: total,
			Label:  monthStart.Format("Jan"),
		})
	}
	return data
}

func (s *InvoiceService) getMonthlyInvoices(tenantID string, months int) []MonthlyData {
	var data []MonthlyData
	now := time.Now()

	for i := months - 1; i >= 0; i-- {
		monthStart := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, now.Location())
		monthEnd := monthStart.AddDate(0, 1, 0)

		var count int64
		s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
			Where("created_at >= ? AND created_at < ?", monthStart, monthEnd).
			Count(&count)

		data = append(data, MonthlyData{
			Month:  monthStart.Format("2006-01"),
			Amount: float64(count),
			Label:  monthStart.Format("Jan"),
		})
	}
	return data
}

func (s *InvoiceService) getDailyRevenue(tenantID string, startDate time.Time, days int) []DailyData {
	var data []DailyData

	for i := 0; i < days; i++ {
		day := startDate.AddDate(0, 0, i)
		nextDay := day.AddDate(0, 0, 1)

		var total float64
		s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
			Where("status = ? AND paid_at >= ? AND paid_at < ?", models.InvoiceStatusPaid, day, nextDay).
			Select("COALESCE(SUM(total), 0)").
			Scan(&total)

		data = append(data, DailyData{
			Date:   day.Format("2006-01-02"),
			Amount: total,
			Label:  day.Format("Jan 02"),
		})
	}
	return data
}

func (s *InvoiceService) getDailyInvoices(tenantID string, startDate time.Time, days int) []DailyData {
	var data []DailyData

	for i := 0; i < days; i++ {
		day := startDate.AddDate(0, 0, i)
		nextDay := day.AddDate(0, 0, 1)

		var count int64
		s.db.Scopes(database.TenantFilter(tenantID)).Model(&models.Invoice{}).
			Where("created_at >= ? AND created_at < ?", day, nextDay).
			Count(&count)

		data = append(data, DailyData{
			Date:   day.Format("2006-01-02"),
			Amount: float64(count),
			Label:  day.Format("Jan 02"),
		})
	}
	return data
}

func (s *InvoiceService) getTopPayingClients(tenantID string, limit int) []ClientRevenue {
	var results []ClientRevenue

	type clientTotals struct {
		ClientID     string
		ClientName   string
		TotalPaid    float64
		InvoiceCount int64
	}

	rows, err := s.db.Scopes(database.TenantFilter(tenantID)).
		Model(&models.Invoice{}).
		Select("client_id, client.name as client_name, COALESCE(SUM(total), 0) as total_paid, COUNT(*) as invoice_count").
		Joins("LEFT JOIN clients client ON client.id = invoices.client_id").
		Where("invoices.status = ?", models.InvoiceStatusPaid).
		Group("client_id").
		Order("total_paid DESC").
		Limit(limit).
		Rows()

	if err != nil {
		return results
	}
	defer rows.Close()

	for rows.Next() {
		var ct clientTotals
		if err := rows.Scan(&ct.ClientID, &ct.ClientName, &ct.TotalPaid, &ct.InvoiceCount); err == nil {
			results = append(results, ClientRevenue{
				ClientID:     ct.ClientID,
				ClientName:   ct.ClientName,
				TotalPaid:    ct.TotalPaid,
				InvoiceCount: ct.InvoiceCount,
			})
		}
	}
	return results
}
