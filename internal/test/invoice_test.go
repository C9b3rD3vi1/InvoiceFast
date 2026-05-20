package services_test

import (
	"math"
	"testing"

	"invoicefast/internal/services"

	"github.com/stretchr/testify/assert"
)

// ============================================================
// Invoice Service Tests
// ============================================================

// TestCalculateLineTotal tests line item total calculations
func TestCalculateLineTotal(t *testing.T) {
	tests := []struct {
		name       string
		quantity   float64
		unitPrice  float64
		discount   float64
		taxRate    float64
		wantTotal  float64
		wantTax    float64
	}{
		{
			name:      "basic calculation with 16% tax",
			quantity:  2,
			unitPrice: 100.00,
			discount:  0,
			taxRate:   16,
			wantTotal: 232.00,
			wantTax:   32.00,
		},
		{
			name:      "with 10% discount",
			quantity:  2,
			unitPrice: 100.00,
			discount:  10,
			taxRate:   16,
			wantTotal: 208.80,
			wantTax:   28.80,
		},
		{
			name:      "zero tax rate",
			quantity:  1,
			unitPrice: 50.00,
			discount:  0,
			taxRate:   0,
			wantTotal: 50.00,
			wantTax:   0,
		},
		{
			name:      "exempt tax rate (-1)",
			quantity:  1,
			unitPrice: 100.00,
			discount:  0,
			taxRate:   -1,
			wantTotal: 100.00,
			wantTax:   0,
		},
		{
			name:      "zero rated (-2)",
			quantity:  1,
			unitPrice: 100.00,
			discount:  0,
			taxRate:   -2,
			wantTotal: 100.00,
			wantTax:   0,
		},
		{
			name:      "combined discount and tax",
			quantity:  3,
			unitPrice: 100.00,
			discount:  15,
			taxRate:   16,
			wantTotal: 295.80,
			wantTax:   40.80,
		},
		{
			name:      "large quantity",
			quantity:  100,
			unitPrice: 500.00,
			discount:  20,
			taxRate:   16,
			wantTotal: 46400.00,
			wantTax:   6400.00,
		},
		{
			name:      "negative quantity defaults to zero",
			quantity:  -5,
			unitPrice: 100.00,
			discount:  0,
			taxRate:   16,
			wantTotal: 0,
			wantTax:   0,
		},
		{
			name:      "negative unit price defaults to zero",
			quantity:  2,
			unitPrice: -50.00,
			discount:  0,
			taxRate:   16,
			wantTotal: 0,
			wantTax:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := services.InvoiceItemRequest{
				Quantity:     tt.quantity,
				UnitPrice:    tt.unitPrice,
				DiscountRate: tt.discount,
				TaxRate:      tt.taxRate,
			}
			total, tax := services.CalculateLineTotalForTest(item)
			assert.InDelta(t, tt.wantTotal, total, 0.01, "total mismatch")
			assert.InDelta(t, tt.wantTax, tax, 0.01, "tax mismatch")
		})
	}
}

// TestCalculateInvoiceTotal tests invoice total calculations
func TestCalculateInvoiceTotal(t *testing.T) {
	tests := []struct {
		name          string
		items         []services.InvoiceItemRequest
		wantSubtotal  float64
		wantTax       float64
		wantTotal     float64
	}{
		{
			name: "multiple items with different tax rates",
			items: []services.InvoiceItemRequest{
				{Description: "Item 1", Quantity: 2, UnitPrice: 100.00, TaxRate: 16},
				{Description: "Item 2", Quantity: 1, UnitPrice: 50.00, TaxRate: 0},
				{Description: "Item 3", Quantity: 3, UnitPrice: 25.00, TaxRate: 16, DiscountRate: 10},
			},
			wantSubtotal: 317.50,
			wantTax:      42.80,
			wantTotal:    360.30,
		},
		{
			name: "single item",
			items: []services.InvoiceItemRequest{
				{Description: "Item 1", Quantity: 1, UnitPrice: 1000.00, TaxRate: 16},
			},
			wantSubtotal: 1000.00,
			wantTax:      160.00,
			wantTotal:    1160.00,
		},
		{
			name: "all exempt items",
			items: []services.InvoiceItemRequest{
				{Description: "Item 1", Quantity: 2, UnitPrice: 100.00, TaxRate: -1},
				{Description: "Item 2", Quantity: 1, UnitPrice: 50.00, TaxRate: -1},
			},
			wantSubtotal: 250.00,
			wantTax:      0,
			wantTotal:    250.00,
		},
		{
			name: "all zero rated items",
			items: []services.InvoiceItemRequest{
				{Description: "Export Item", Quantity: 5, UnitPrice: 200.00, TaxRate: -2},
			},
			wantSubtotal: 1000.00,
			wantTax:      0,
			wantTotal:    1000.00,
		},
		{
			name: "mixed tax with large discount",
			items: []services.InvoiceItemRequest{
				{Description: "Item 1", Quantity: 10, UnitPrice: 100.00, TaxRate: 16, DiscountRate: 50},
			},
			wantSubtotal: 500.00,
			wantTax:      80.00,
			wantTotal:    580.00,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subtotal, tax, total := services.CalculateInvoiceTotalForTest(tt.items)
			assert.InDelta(t, tt.wantSubtotal, subtotal, 0.01)
			assert.InDelta(t, tt.wantTax, tax, 0.01)
			assert.InDelta(t, tt.wantTotal, total, 0.01)
		})
	}
}

// TestValidateCurrency tests currency validation
func TestValidateCurrency(t *testing.T) {
	tests := []struct {
		name      string
		currency  string
		wantValid bool
	}{
		{"valid KES", "KES", true},
		{"valid USD", "USD", true},
		{"valid EUR", "EUR", true},
		{"valid GBP", "GBP", true},
		{"valid TZS", "TZS", true},
		{"valid UGX", "UGX", true},
		{"valid NGN", "NGN", true},
		{"lowercase normalized", "kes", true},
		{"uppercase normalized", "KES", true},
		{"with spaces", " KES ", true},
		{"invalid currency", "XYZ", false},
		{"empty currency", "", false},
		{"partial currency", "KE", false},
		{"numbers", "123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := services.IsValidCurrencyForTest(tt.currency)
			assert.Equal(t, tt.wantValid, isValid)
		})
	}
}

// TestValidateTaxRate tests tax rate validation
func TestValidateTaxRate(t *testing.T) {
	tests := []struct {
		name    string
		taxRate float64
		wantOk  bool
	}{
		{"standard rate 16%", 16.0, true},
		{"standard rate 8%", 8.0, true},
		{"standard rate 12%", 12.0, true},
		{"zero rate", 0.0, true},
		{"exempt rate", -1.0, true},
		{"zero rated", -2.0, true},
		{"negative invalid", -0.5, false},
		{"negative other", -3.0, false},
		{"over 100%", 101.0, false},
		{"exactly 100%", 100.0, true},
		{"negative 100", -100.0, false},
		{"decimal rate 16.5%", 16.5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := services.ValidateTaxRateForTest(tt.taxRate)
			assert.Equal(t, tt.wantOk, isValid, "taxRate: %v", tt.taxRate)
		})
	}
}

// TestNormalizePhoneNumber tests phone number normalization
func TestNormalizePhoneNumber(t *testing.T) {
	tests := []struct {
		name  string
		phone string
		want  string
	}{
		{"kenya with plus", "+254712345678", "254712345678"},
		{"kenya without plus", "254712345678", "254712345678"},
		{"kenya with 07", "0712345678", "254712345678"},
		{"kenya with 01", "012345678", "25412345678"},
		{"kenya with 254", "254712345678", "254712345678"},
		{"tanzania with plus", "+255712345678", "255712345678"},
		{"uganda with plus", "+256712345678", "256712345678"},
		{"nigeria with plus", "+2348031234567", "2348031234567"},
		{"kenya with spaces", "+254 712 345 678", "254712345678"},
		{"kenya with dash", "254-712-345-678", "254712345678"},
		{"kenya with parens", "(254) 712 345 678", "254712345678"},
		{"leading zeros", "000254712345678", "254712345678"},
		{"invalid empty", "", ""},
		{"whitespace only", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.NormalizePhoneNumberForTest(tt.phone)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestGenerateInvoiceNumber tests invoice number generation
func TestGenerateInvoiceNumber(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		sequence    int64
		wantPattern string
	}{
		{"first invoice", "INV", 1, "INV-0001"},
		{"tenth invoice", "INV", 10, "INV-0010"},
		{"hundredth invoice", "INV", 100, "INV-0100"},
		{"thousandth invoice", "INV", 1000, "INV-1000"},
		{"with year", "INV-2026", 5, "INV-2026-0005"},
		{"with custom prefix", "RENT", 42, "RENT-0042"},
		{"credit note prefix", "CN", 1, "CN-0001"},
		{"debit note prefix", "DN", 99, "DN-0099"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.GenerateInvoiceNumberForTest(tt.prefix, tt.sequence)
			assert.Equal(t, tt.wantPattern, result)
		})
	}
}

// TestGetValidCurrencies tests currency list
func TestGetValidCurrencies(t *testing.T) {
	currencies := services.GetValidCurrenciesForTest()
	
	// Verify expected currencies are present
	expected := []string{"KES", "USD", "EUR", "GBP", "TZS", "UGX", "NGN"}
	for _, exp := range expected {
		found := false
		for _, c := range currencies {
			if c == exp {
				found = true
				break
			}
		}
		assert.True(t, found, "expected currency %s not found", exp)
	}
	
	// Verify we have multiple currencies
	assert.GreaterOrEqual(t, len(currencies), 7, "should have at least 7 currencies")
}

// TestValidateInvoiceStatusTransition tests status transitions
func TestValidateInvoiceStatusTransition(t *testing.T) {
	tests := []struct {
		name       string
		fromStatus string
		toStatus   string
		wantValid  bool
	}{
		{"draft to sent", "draft", "sent", true},
		{"draft to cancelled", "draft", "cancelled", true},
		{"draft to paid", "draft", "paid", false},
		{"sent to viewed", "sent", "viewed", true},
		{"sent to paid", "sent", "paid", false},
		{"viewed to paid", "viewed", "paid", true},
		{"viewed to partially paid", "viewed", "partially_paid", true},
		{"partially paid to paid", "partially_paid", "paid", true},
		{"partially paid to overdue", "partially_paid", "overdue", true},
		{"paid to anything", "paid", "draft", false},
		{"cancelled to anything", "cancelled", "sent", false},
		{"overdue to paid", "overdue", "paid", true},
		{"invalid from", "invalid", "sent", false},
		{"invalid to", "draft", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := services.ValidateInvoiceStatusTransition(tt.fromStatus, tt.toStatus)
			assert.Equal(t, tt.wantValid, isValid)
		})
	}
}

// TestSanitizeInvoiceDescription tests description sanitization
func TestSanitizeInvoiceDescription(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        string
	}{
		{"normal description", "Web Development Services", "Web Development Services"},
		{"empty string", "", "Item"},
		{"whitespace only", "   ", "Item"},
		{"trims whitespace", "  Product  ", "Product"},
		{"truncates long description", string(makeString(600)), string(makeString(500))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.SanitizeInvoiceDescription(tt.input)
			if tt.want == "Item" {
				assert.Equal(t, "Item", result)
			} else {
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

// TestValidateInvoiceItems tests item validation
func TestValidateInvoiceItems(t *testing.T) {
	tests := []struct {
		name    string
		items   []services.InvoiceItemRequest
		wantErr bool
	}{
		{
			name: "valid items",
			items: []services.InvoiceItemRequest{
				{Description: "Item 1", Quantity: 1, UnitPrice: 100, TaxRate: 16},
			},
			wantErr: false,
		},
		{
			name: "empty items",
			items: []services.InvoiceItemRequest{},
			wantErr: true,
		},
		{
			name: "negative quantity",
			items: []services.InvoiceItemRequest{
				{Description: "Item 1", Quantity: -1, UnitPrice: 100, TaxRate: 16},
			},
			wantErr: true,
		},
		{
			name: "negative unit price",
			items: []services.InvoiceItemRequest{
				{Description: "Item 1", Quantity: 1, UnitPrice: -100, TaxRate: 16},
			},
			wantErr: true,
		},
		{
			name: "invalid tax rate",
			items: []services.InvoiceItemRequest{
				{Description: "Item 1", Quantity: 1, UnitPrice: 100, TaxRate: 101},
			},
			wantErr: true,
		},
		{
			name: "multiple valid items",
			items: []services.InvoiceItemRequest{
				{Description: "Item 1", Quantity: 1, UnitPrice: 100, TaxRate: 16},
				{Description: "Item 2", Quantity: 2, UnitPrice: 50, TaxRate: 8},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := services.ValidateInvoiceItems(tt.items)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestIsInvoiceEditable tests editable status check
func TestIsInvoiceEditable(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"draft is editable", "draft", true},
		{"sent is not editable", "sent", false},
		{"viewed is not editable", "viewed", false},
		{"paid is not editable", "paid", false},
		{"cancelled is not editable", "cancelled", false},
		{"overdue is not editable", "overdue", false},
		{"unknown status", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.IsInvoiceEditable(tt.status)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestIsInvoiceCancellable tests cancellable status check
func TestIsInvoiceCancellable(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"draft is cancellable", "draft", true},
		{"sent is cancellable", "sent", true},
		{"viewed is cancellable", "viewed", true},
		{"paid is not cancellable", "paid", false},
		{"cancelled is not cancellable", "cancelled", false},
		{"overdue is cancellable", "overdue", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.IsInvoiceCancellable(tt.status)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestFormatCurrencyAmount tests currency formatting
func TestFormatCurrencyAmount(t *testing.T) {
	tests := []struct {
		name     string
		amount   float64
		currency string
		want     string
	}{
		{"KES format", 1000.50, "KES", "KSh 1000.50"},
		{"USD format", 1000.00, "USD", "$1000.00"},
		{"EUR format", 500.25, "EUR", "€500.25"},
		{"GBP format", 250.00, "GBP", "£250.00"},
		{"TZS format", 100000.00, "TZS", "TSh 100000.00"},
		{"default format", 100.00, "XYZ", "XYZ 100.00"},
		{"zero amount", 0, "KES", "KSh 0.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.FormatCurrencyAmount(tt.amount, tt.currency)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestCalculateExchangeRateAmount tests exchange rate conversion
func TestCalculateExchangeRateAmount(t *testing.T) {
	result := services.CalculateExchangeRateAmount(1000, 0.0093) // KES to USD approx
	assert.Equal(t, 9.30, result)
	
	// Test backward precision
	result2 := services.CalculateExchangeRateAmount(1234.56, 0.0093)
	assert.InDelta(t, 11.48, result2, 0.01)
}

// TestValidateInvoiceNumberFormat tests number format validation
func TestValidateInvoiceNumberFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   bool
	}{
		{"valid with sequence", "INV-{sequence}", true},
		{"valid with year", "INV-{year}-{sequence}", true},
		{"empty format", "", false},
		{"no placeholder", "INV-2026-001", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.ValidateInvoiceNumberFormat(tt.format)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestParseInvoiceNumberFormat tests number format parsing
func TestParseInvoiceNumberFormat(t *testing.T) {
	format := "INV-{year}-{sequence}"
	result, err := services.ParseInvoiceNumberFormat(format, 42)
	assert.NoError(t, err)
	assert.Contains(t, result, "INV-")
	assert.Contains(t, result, "-0042")
}

// ============================================================
// Benchmark Tests
// ============================================================

func BenchmarkCalculateLineTotal(b *testing.B) {
	item := services.InvoiceItemRequest{
		Quantity:     10,
		UnitPrice:    1000.00,
		DiscountRate: 5,
		TaxRate:      16,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		services.CalculateLineTotalForTest(item)
	}
}

func BenchmarkCalculateInvoiceTotal(b *testing.B) {
	items := []services.InvoiceItemRequest{
		{Quantity: 2, UnitPrice: 100.00, TaxRate: 16},
		{Quantity: 1, UnitPrice: 50.00, TaxRate: 0},
		{Quantity: 3, UnitPrice: 25.00, TaxRate: 16, DiscountRate: 10},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		services.CalculateInvoiceTotalForTest(items)
	}
}

func BenchmarkNormalizePhoneNumber(b *testing.B) {
	phone := "+254712345678"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		services.NormalizePhoneNumberForTest(phone)
	}
}

func BenchmarkValidateCurrency(b *testing.B) {
	currency := "KES"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		services.IsValidCurrencyForTest(currency)
	}
}

// Helper function
func makeString(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += "a"
	}
	return s
}

// math.Round is available in Go 1.20+
var _ = math.Round