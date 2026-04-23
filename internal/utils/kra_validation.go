package utils

import (
	"fmt"
	"strings"

	"invoicefast/internal/models"
)

// ClassifyBuyerType determines if invoice is B2B, B2C, EXPORT, or B2E based on buyer information
func ClassifyBuyerType(buyer models.Client, kraPin string) string {
	if kraPin == "" {
		return "B2C" // Consumer (no KRA PIN)
	}

	// Check if buyer has KRA PIN
	if buyer.KRAPIN != "" && ValidateKRAPIN(buyer.KRAPIN) == nil {
		// Decrypt buyer KRA PIN (would need decryption key in real implementation)
		// For now, we'll check if it matches the tenant's KRA PIN pattern
		if strings.EqualFold(buyer.KRAPIN, kraPin) {
			return "B2E" // Business to Employee (same entity)
		}
		return "B2B" // Business to Business
	}

	// Check if buyer is foreign/export (non-Kenyan address or special indicators)
	if buyer.Address != "" {
		addressLower := strings.ToLower(buyer.Address)
		if strings.Contains(addressLower, "export") || strings.Contains(addressLower, "foreign") ||
			strings.Contains(addressLower, "usa") || strings.Contains(addressLower, "uk") ||
			strings.Contains(addressLower, "uganda") || strings.Contains(addressLower, "tanzania") {
			return "EXPORT"
		}
	}

	// Default to B2C if no clear business indicators
	return "B2C"
}

// CalculateTaxBreakdown calculates VAT, zero-rated, exempt, and excise amounts
func CalculateTaxBreakdown(subtotal float64, vatRate float64, items []models.InvoiceItem) (vatAmount, zeroRated, exempt, excise float64) {
	vatAmount = subtotal * (vatRate / 100)

	// Calculate zero-rated and exempt amounts from items
	for _, item := range items {
		// Check item classification for zero-rated or exempt
		if item.Description != "" {
			descLower := strings.ToLower(item.Description)
			if strings.Contains(descLower, "zero-rated") || strings.Contains(descLower, "zero rated") {
				zeroRated += item.Total
			} else if strings.Contains(descLower, "exempt") || strings.Contains(descLower, "vat exempt") {
				exempt += item.Total
			}
		}

		// Calculate excise duty (if applicable)
		if item.TaxAmount > 0 { // Using TaxAmount instead of ExciseDuty for now
			excise += item.TaxAmount
		}
	}

	return
}

// GenerateInvoiceNumber generates sequential invoice number per tenant
func GenerateInvoiceNumber(tenantID string, lastInvoiceNumber string) string {
	if lastInvoiceNumber == "" {
		// Start with INV/TENANTID/000001
		return fmt.Sprintf("INV/%s/%06d", tenantID[:8], 1)
	}

	// Extract numeric part and increment
	var num int
	_, err := fmt.Sscanf(lastInvoiceNumber, "INV/%s/%d", &num)
	if err != nil {
		// Fallback: increment last number
		num = 1
		_, err = fmt.Sscanf(lastInvoiceNumber, "%*[^/]/%*[^/]/%d", &num)
		if err != nil {
			num = 1
		}
	}

	return fmt.Sprintf("INV/%s/%06d", tenantID[:8], num+1)
}

// ValidateInvoiceSequentialNumbering ensures invoice numbers are sequential per tenant
func ValidateInvoiceSequentialNumbering(tenantID, currentNumber, lastNumber string) bool {
	if lastNumber == "" {
		return true // First invoice
	}

	// Extract numbers
	var current, last int
	_, err1 := fmt.Sscanf(currentNumber, "INV/%s/%d", &current)
	_, err2 := fmt.Sscanf(lastNumber, "INV/%s/%d", &last)

	if err1 != nil || err2 != nil {
		// Try alternative format
		_, err1 = fmt.Sscanf(currentNumber, "%*[^/]/%*[^/]/%d", &current)
		_, err2 = fmt.Sscanf(lastNumber, "%*[^/]/%*[^/]/%d", &last)
		if err1 != nil || err2 != nil {
			return false // Invalid format
		}
	}

	return current == last+1 // Should be exactly one more than last
}



// MaskKRAPIN masks KRA PIN for display (show first 3 and last 1 chars)
func MaskKRAPIN(pin string) string {
	if pin == "" || len(pin) < 4 {
		return "****"
	}
	if len(pin) <= 4 {
		return strings.Repeat("*", len(pin))
	}
	firstThree := pin[:3]
	lastOne := pin[len(pin)-1:]
	middle := strings.Repeat("*", len(pin)-4)
	return firstThree + middle + lastOne
}