package services

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

// KRAValidationResult holds validation results
type KRAValidationResult struct {
	Valid  bool
	Errors []string
}

// ValidateKRAPIN validates a KRA PIN format
// Format: A123456789B (11 characters, starts with A, ends with B)
func ValidateKRAPIN(pin string) *KRAValidationResult {
	result := &KRAValidationResult{Valid: true, Errors: []string{}}
	
	if pin == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "KRA PIN is required")
		return result
	}
	
	// Check length
	if len(pin) != 11 {
		result.Valid = false
		result.Errors = append(result.Errors, "KRA PIN must be 11 characters")
		return result
	}
	
	// Check format: starts with A, ends with B
	if !strings.HasPrefix(pin, "A") || !strings.HasSuffix(pin, "B") {
		result.Valid = false
		result.Errors = append(result.Errors, "KRA PIN must start with 'A' and end with 'B'")
		return result
	}
	
	// Check middle characters are alphanumeric
	middle := pin[1:len(pin)-1]
	matched, _ := regexp.MatchString("^[A-Z0-9]+$", middle)
	if !matched {
		result.Valid = false
		result.Errors = append(result.Errors, "KRA PIN middle characters must be alphanumeric")
		return result
	}

	return result
}

// KRAReportData holds KRA report data
type KRAReportData struct {
	ReportType        string    `json:"report_type"`
	ReportDate        time.Time `json:"report_date"`
	TotalInvoices     int       `json:"total_invoices"`
	TotalSales        float64   `json:"total_sales"`
	TotalVATCollected float64   `json:"total_vat_collected"`
	TotalDiscount    float64   `json:"total_discount"`
	TotalExciseDuty  float64   `json:"total_excise_duty"`
	CreditNoteCount  int       `json:"credit_note_count"`
	DebitNoteCount   int       `json:"debit_note_count"`
	CancelledCount   int       `json:"cancelled_count"`
	PendingKRA      int       `json:"pending_kra"`
	SubmittedKRA    int       `json:"submitted_kra"`
	FailedKRA       int       `json:"failed_kra"`
}

// GenerateZReport generates daily Z-Report (close of day)
func (s *InvoiceService) GenerateZReport(tenantID string, reportDate time.Time) (*KRAReportData, error) {
	startOfDay := time.Date(reportDate.Year(), reportDate.Month(), reportDate.Day(), 0, 0, 0, 0, reportDate.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	report := &KRAReportData{
		ReportType: "daily",
		ReportDate: startOfDay,
	}

	var invoices []models.Invoice
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
		Where("deleted_at IS NULL").
		Find(&invoices).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch invoices: %w", err)
	}

	for _, inv := range invoices {
		report.TotalInvoices++
		report.TotalSales += inv.Total.Float64()
		report.TotalVATCollected += inv.TotalTax.Float64()
		report.TotalDiscount += inv.Discount.Float64()
		report.TotalExciseDuty += inv.ExciseDuty.Float64()

		switch inv.InvoiceType {
		case "credit_note":
			report.CreditNoteCount++
		case "debit_note":
			report.DebitNoteCount++
		}

		if inv.Status == models.InvoiceStatusCancelled || inv.Status == models.InvoiceStatusVoid {
			report.CancelledCount++
		}

		switch inv.KRAStatus {
		case models.KRAInvoiceStatusPending:
			report.PendingKRA++
		case models.KRAInvoiceStatusSubmitted, models.KRAInvoiceStatusAccepted:
			report.SubmittedKRA++
		case models.KRAInvoiceStatusFailed, models.KRAInvoiceStatusRejected:
			report.FailedKRA++
		}
	}

	return report, nil
}

// GenerateMonthlyReport generates monthly KRA report
func (s *InvoiceService) GenerateMonthlyReport(tenantID string, reportDate time.Time) (*KRAReportData, error) {
	startOfMonth := time.Date(reportDate.Year(), reportDate.Month(), 1, 0, 0, 0, 0, reportDate.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	report := &KRAReportData{
		ReportType: "monthly",
		ReportDate: startOfMonth,
	}

	// Query invoices for the month
	var invoices []models.Invoice
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Where("created_at >= ? AND created_at < ?", startOfMonth, endOfMonth).
		Where("deleted_at IS NULL").
		Find(&invoices).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to fetch invoices: %w", err)
	}

	// Calculate totals for comparison period
	for _, inv := range invoices {
		report.TotalInvoices++
		report.TotalSales += inv.Total.Float64()
		report.TotalVATCollected += inv.TotalTax.Float64()
		report.TotalDiscount += inv.Discount.Float64()
		report.TotalExciseDuty += inv.ExciseDuty.Float64()

		switch inv.InvoiceType {
		case "credit_note":
			report.CreditNoteCount++
		case "debit_note":
			report.DebitNoteCount++
		}

		if inv.Status == models.InvoiceStatusCancelled || inv.Status == models.InvoiceStatusVoid {
			report.CancelledCount++
		}

		switch inv.KRAStatus {
		case models.KRAInvoiceStatusPending:
			report.PendingKRA++
		case models.KRAInvoiceStatusSubmitted, models.KRAInvoiceStatusAccepted:
			report.SubmittedKRA++
		case models.KRAInvoiceStatusFailed, models.KRAInvoiceStatusRejected:
			report.FailedKRA++
		}
	}

	return report, nil
}