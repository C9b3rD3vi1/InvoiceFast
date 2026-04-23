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

// ValidateInvoiceNumber validates invoice number format
// Format: INV-000001 or prefix-sequence
func ValidateInvoiceNumber(invNum string) *KRAValidationResult {
	result := &KRAValidationResult{Valid: true, Errors: []string{}}
	
	if invNum == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "Invoice number is required")
		return result
	}
	
	// KRA expects alphanumeric with hyphen, max 20 chars
	if len(invNum) > 20 {
		result.Valid = false
		result.Errors = append(result.Errors, "Invoice number must be <= 20 characters")
		return result
	}
	
	// Check format is alphanumeric with allowed special chars
	matched, _ := regexp.MatchString("^[A-Za-z0-9-]+$", invNum)
	if !matched {
		result.Valid = false
		result.Errors = append(result.Errors, "Invoice number must be alphanumeric with hyphens only")
		return result
	}
	
	return result
}

// ValidateDSN validates Device Serial Number
// Format: DSN-XXXX-XXXX-XXXX
func ValidateDSN(dsn string) *KRAValidationResult {
	result := &KRAValidationResult{Valid: true, Errors: []string{}}
	
	if dsn == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "Device Serial Number is required")
		return result
	}
	
	// DSN format: alphanumeric, 10-20 chars
	if len(dsn) < 10 || len(dsn) > 20 {
		result.Valid = false
		result.Errors = append(result.Errors, "DSN must be 10-20 characters")
		return result
	}
	
	return result
}

// ValidateBuyerClassification validates buyer classification
func ValidateBuyerClassification(classification string) *KRAValidationResult {
	result := &KRAValidationResult{Valid: true, Errors: []string{}}
	
	validTypes := []string{"B2C", "B2B", "B2E", "EXPORT"}
	
	found := false
	for _, v := range validTypes {
		if classification == v {
			found = true
			break
		}
	}
	
	if !found {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Buyer classification must be one of: %s", strings.Join(validTypes, ", ")))
	}
	
	return result
}

// KRAReportData holds KRA report data
type KRAReportData struct {
	ReportType        string    `json:"report_type"`        // daily, monthly
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
	// Start of day
	startOfDay := time.Date(reportDate.Year(), reportDate.Month(), reportDate.Day(), 0, 0, 0, 0, reportDate.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	report := &KRAReportData{
		ReportType: "daily",
		ReportDate: startOfDay,
	}

	// Query invoices for the day
	var invoices []models.Invoice
	err := s.db.Scopes(database.TenantFilter(tenantID)).
		Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
		Where("deleted_at IS NULL").
		Find(&invoices).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to fetch invoices: %w", err)
	}

	// Calculate totals
	for _, inv := range invoices {
		report.TotalInvoices++
		report.TotalSales += inv.Total
		report.TotalVATCollected += inv.TotalTax
		report.TotalDiscount += inv.Discount
		report.TotalExciseDuty += inv.ExciseDuty

		// Count by type
		switch inv.InvoiceType {
		case "credit_note":
			report.CreditNoteCount++
		case "debit_note":
			report.DebitNoteCount++
		}

		// Count by status
		if inv.Status == "cancelled" || inv.Status == "voided" {
			report.CancelledCount++
		}

		// Count by KRA status
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

// GenerateDailySummary generates daily summary report
func (s *InvoiceService) GenerateDailySummary(tenantID string, reportDate time.Time) (*KRAReportData, error) {
	return s.GenerateZReport(tenantID, reportDate)
}

// GenerateMonthlySummary generates monthly summary report
func (s *InvoiceService) GenerateMonthlySummary(tenantID string, year int, month int) (*KRAReportData, error) {
	// Start of month
	startOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
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

	// Calculate totals
	for _, inv := range invoices {
		report.TotalInvoices++
		report.TotalSales += inv.Total
		report.TotalVATCollected += inv.TotalTax
		report.TotalDiscount += inv.Discount
		report.TotalExciseDuty += inv.ExciseDuty

		switch inv.InvoiceType {
		case "credit_note":
			report.CreditNoteCount++
		case "debit_note":
			report.DebitNoteCount++
		}

		if inv.Status == "cancelled" || inv.Status == "voided" {
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