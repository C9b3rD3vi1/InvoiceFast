package services

import (
	"fmt"
	"math"
	"strings"
	"time"

	"invoicefast/internal/utils"
)

// DB interface for test package to use
type DB interface {
	Close() error
}

// ============================================================
// TEST HELPERS - Exported functions for test packages
// These wrap internal logic for external testing
// ============================================================

// ValidateCreateRequestForTest wraps internal validation
func (s *InvoiceService) ValidateCreateRequestForTest(userID, clientID string, req *CreateInvoiceRequest) error {
	return s.validateCreateRequest(userID, clientID, req)
}

// CalculateLineTotalForTest exports line total calculation
func CalculateLineTotalForTest(item InvoiceItemRequest) (total, tax float64) {
	return calculateLineTotal(item)
}

// CalculateInvoiceTotalForTest exports invoice total calculation
func CalculateInvoiceTotalForTest(items []InvoiceItemRequest) (subtotal, tax, total float64) {
	return calculateInvoiceTotal(items)
}

// IsValidCurrencyForTest exports currency validation
func IsValidCurrencyForTest(currency string) bool {
	if currency == "" {
		return false
	}
	normalized := strings.ToUpper(strings.TrimSpace(currency))
	return validCurrencies[normalized]
}

// ValidateTaxRateForTest exports tax rate validation
func ValidateTaxRateForTest(taxRate float64) bool {
	return validateTaxRate(taxRate)
}

// NormalizePhoneNumberForTest exports phone normalization
func NormalizePhoneNumberForTest(phone string) string {
	return normalizePhoneNumber(phone)
}

// GenerateInvoiceNumberForTest exports invoice number generation
func GenerateInvoiceNumberForTest(prefix string, sequence int64) string {
	// Format: PREFIX-0001 (4-digit zero-padded)
	return fmt.Sprintf("%s-%04d", prefix, sequence)
}

// GetValidCurrenciesForTest exports valid currencies list
func GetValidCurrenciesForTest() []string {
	currencies := make([]string, 0, len(validCurrencies))
	for k := range validCurrencies {
		currencies = append(currencies, k)
	}
	return currencies
}

// calculateLineTotal calculates total for a single line item
func calculateLineTotal(item InvoiceItemRequest) (float64, float64) {
	quantity := item.Quantity
	if quantity < 0 {
		quantity = 0
	}

	unitPrice := item.UnitPrice
	if unitPrice < 0 {
		unitPrice = 0
	}

	// Calculate base total
	baseTotal := quantity * unitPrice

	// Apply item-level discount
	discountRate := item.DiscountRate
	if discountRate < 0 {
		discountRate = 0
	}
	if discountRate > 100 {
		discountRate = 100
	}

	discountAmount := baseTotal * (discountRate / 100)
	subtotal := baseTotal - discountAmount

	// Calculate tax
	taxRate := item.TaxRate
	if taxRate < 0 {
		taxRate = 0 // Treat negative as exempt
	}
	if taxRate > 100 {
		taxRate = 100
	}

	var tax float64
	// Only apply tax to non-exempt items
	// taxRate: -1 = exempt, -2 = zero rated, 0+ = standard rate
	if taxRate >= 0 {
		tax = subtotal * (taxRate / 100)
	}

	total := subtotal + tax

	// Round to 2 decimal places
	return roundToTwoDecimals(total), roundToTwoDecimals(tax)
}

// calculateInvoiceTotal calculates totals for entire invoice
func calculateInvoiceTotal(items []InvoiceItemRequest) (subtotal, tax, total float64) {
	var subTotal, totalTax float64

	for _, item := range items {
		lineTotal, lineTax := calculateLineTotal(item)
		subTotal += lineTotal - lineTax // Add back tax to get subtotal
		totalTax += lineTax
	}

	subtotal = roundToTwoDecimals(subTotal)
	tax = roundToTwoDecimals(totalTax)
	total = roundToTwoDecimals(subtotal + tax)

	return
}

// validateTaxRate validates tax rate is within acceptable range
func validateTaxRate(taxRate float64) bool {
	// Valid tax rates:
	// -1 (exempt), -2 (zero rated), 0-100 (standard rates)
	if taxRate == -1 || taxRate == -2 {
		return true
	}
	return taxRate >= 0 && taxRate <= 100
}

// normalizePhoneNumber normalizes phone number to E.164 format
func normalizePhoneNumber(phone string) string {
	if phone == "" {
		return ""
	}

	// Remove common formatting characters
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	phone = strings.ReplaceAll(phone, "_", "")

	// Remove + prefix for processing
	if strings.HasPrefix(phone, "+") {
		phone = phone[1:]
	}

	// Strip leading zeros to normalize
	phone = strings.TrimLeft(phone, "0")

	if phone == "" {
		return ""
	}

	// If starts with known country code (254, 255, 256, 234), keep as-is
	if strings.HasPrefix(phone, "254") || strings.HasPrefix(phone, "255") ||
		strings.HasPrefix(phone, "256") || strings.HasPrefix(phone, "234") {
		return phone
	}

	// Kenya mobile without country code (7xx or 1xx)
	if strings.HasPrefix(phone, "7") || strings.HasPrefix(phone, "1") {
		return "254" + phone
	}

	// Unknown format — return as-is
	return phone
}

// roundToTwoDecimals rounds float64 to 2 decimal places
func roundToTwoDecimals(val float64) float64 {
	return math.Round(val*100) / 100
}

// GetDefaultCurrency returns the default currency
func GetDefaultCurrency() string {
	return utils.DefaultCurrency
}

// CalculateInvoiceNumberSequence generates sequential invoice numbers
func CalculateInvoiceNumberSequence(prefix string, sequence int64) string {
	return fmt.Sprintf("%s-%04d", prefix, sequence)
}

// ValidateInvoiceStatusTransition validates status changes
func ValidateInvoiceStatusTransition(from, to string) bool {
	validTransitions := map[string][]string{
		"draft":            {"sent", "cancelled"},
		"sent":             {"viewed", "partially_paid", "overdue", "cancelled"},
		"viewed":           {"partially_paid", "paid", "overdue", "cancelled"},
		"partially_paid":   {"partially_paid", "paid", "overdue"},
		"paid":             {},
		"overdue":          {"paid", "cancelled"},
		"cancelled":        {},
		"void":             {},
		"credit_note":      {},
		"debit_note":       {},
	}

	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, status := range allowed {
		if status == to {
			return true
		}
	}
	return false
}

// ValidateInvoiceDueDate validates due date is valid
func ValidateInvoiceDueDate(dueDate *time.Time) error {
	if dueDate == nil {
		return fmt.Errorf("due date is required")
	}
	if dueDate.Before(time.Now().AddDate(0, 0, -1)) {
		return fmt.Errorf("due date cannot be in the past")
	}
	return nil
}

// SanitizeInvoiceDescription sanitizes description text
func SanitizeInvoiceDescription(desc string) string {
	if strings.TrimSpace(desc) == "" {
		return "Item"
	}
	// Trim whitespace and limit length
	desc = strings.TrimSpace(desc)
	if len(desc) > 500 {
		desc = desc[:500]
	}
	return desc
}

// ValidateInvoiceItems validates all invoice items
func ValidateInvoiceItems(items []InvoiceItemRequest) error {
	if len(items) == 0 {
		return ErrEmptyItems
	}

	for i, item := range items {
		if item.Quantity < 0 {
			return fmt.Errorf("item %d: quantity cannot be negative", i+1)
		}
		if item.UnitPrice < 0 {
			return fmt.Errorf("item %d: unit price cannot be negative", i+1)
		}
		if item.TaxRate < -2 || item.TaxRate > 100 {
			return fmt.Errorf("item %d: invalid tax rate", i+1)
		}
	}

	return nil
}

// GetDefaultPaymentTerms returns default payment terms in days
func GetDefaultPaymentTerms() int {
	return 30
}

// CalculateDueDate calculates due date from invoice date and terms
func CalculateDueDate(invoiceDate time.Time, paymentTerms int) time.Time {
	return invoiceDate.AddDate(0, 0, paymentTerms)
}

// IsInvoiceEditable checks if invoice can be edited based on status
func IsInvoiceEditable(status string) bool {
	editableStatuses := map[string]bool{
		"draft":         true,
		"sent":          false,
		"viewed":        false,
		"partially_paid": false,
		"paid":          false,
		"overdue":       false,
		"cancelled":     false,
		"void":          false,
	}
	return editableStatuses[status]
}

// IsInvoiceCancellable checks if invoice can be cancelled
func IsInvoiceCancellable(status string) bool {
	cancellableStatuses := map[string]bool{
		"draft":    true,
		"sent":     true,
		"viewed":   true,
		"overdue":  true,
		"cancelled": false,
		"void":      false,
		"paid":     false,
	}
	return cancellableStatuses[status]
}

// FormatCurrencyAmount formats amount for display
func FormatCurrencyAmount(amount float64, currency string) string {
	symbols := map[string]string{
		"KES": "KSh ",
		"USD": "$",
		"EUR": "€",
		"GBP": "£",
		"TZS": "TSh ",
		"UGX": "USh ",
		"NGN": "₦",
	}

	symbol := symbols[currency]
	if symbol == "" {
		symbol = currency + " "
	}

	return fmt.Sprintf("%s%.2f", symbol, amount)
}

// CalculateExchangeRateAmount converts amount using exchange rate
func CalculateExchangeRateAmount(amount, rate float64) float64 {
	return roundToTwoDecimals(amount * rate)
}

// ValidateInvoiceNumberFormat validates custom invoice number format
func ValidateInvoiceNumberFormat(format string) bool {
	if format == "" {
		return false
	}
	// Should contain placeholder for sequence
	return strings.Contains(format, "{sequence}") || strings.Contains(format, "{year}")
}

// ParseInvoiceNumberFormat parses custom format string
func ParseInvoiceNumberFormat(format string, sequence int64) (string, error) {
	result := format

	// Replace {sequence} with zero-padded sequence
	if strings.Contains(result, "{sequence}") {
		result = strings.ReplaceAll(result, "{sequence}", fmt.Sprintf("%04d", sequence))
	}

	// Replace {year} with current year
	if strings.Contains(result, "{year}") {
		year := time.Now().Year()
		result = strings.ReplaceAll(result, "{year}", fmt.Sprintf("%d", year))
	}

	// Replace {month} with current month
	if strings.Contains(result, "{month}") {
		month := int(time.Now().Month())
		result = strings.ReplaceAll(result, "{month}", fmt.Sprintf("%02d", month))
	}

	return result, nil
}