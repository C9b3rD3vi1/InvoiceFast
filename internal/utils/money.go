package utils

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// Money represents monetary values in cents to avoid floating-point errors
// This is the PRIMARY type for all financial calculations
type Money int64

// String returns the amount as a string (in cents)
func (m Money) String() string {
	return strconv.FormatInt(int64(m), 10)
}

// Float64 returns the amount as a float64 (for display purposes only)
func (m Money) Float64() float64 {
	return float64(m) / 100
}

// ToCents converts a float64 to Money (cents)
func ToCents(amount float64) Money {
	return Money(math.Round(amount * 100))
}

// FromCents creates Money from cents
func FromCents(cents int64) Money {
	return Money(cents)
}

// Add adds two Money values
func (m Money) Add(other Money) Money {
	return m + other
}

// Subtract subtracts two Money values
func (m Money) Subtract(other Money) Money {
	return m - other
}

// Multiply multiplies Money by a factor (e.g., for tax calculation)
func (m Money) Multiply(factor float64) Money {
	return Money(math.Round(float64(m) * factor))
}

// Divide divides Money by a divisor
func (m Money) Divide(divisor float64) Money {
	if divisor == 0 {
		return 0
	}
	return Money(math.Round(float64(m) / divisor))
}

// IsZero checks if Money is zero
func (m Money) IsZero() bool {
	return m == 0
}

// IsPositive checks if Money is greater than zero
func (m Money) IsPositive() bool {
	return m > 0
}

// FormatCurrency formats Money as currency string
func (m Money) FormatCurrency(currency string) string {
	decimal := decimal.NewFromInt(int64(m)).Div(decimal.NewFromInt(100))
	return fmt.Sprintf("%s %s", decimal.String(), currency)
}

// ParseCurrency parses a currency string like "1000 KES" to Money
func ParseCurrency(input string) (Money, error) {
	input = strings.TrimSpace(input)
	parts := strings.Split(input, " ")
	if len(parts) < 1 {
		return 0, fmt.Errorf("invalid currency format")
	}

	amountStr := strings.ReplaceAll(parts[0], ",", "")
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount: %w", err)
	}

	return ToCents(amount), nil
}

// MoneyFromDecimal creates Money from shopspring/decimal
func MoneyFromDecimal(d decimal.Decimal) Money {
	return Money(d.Mul(decimal.NewFromInt(100)).IntPart())
}

// Decimal converts Money to shopspring/decimal
func (m Money) Decimal() decimal.Decimal {
	return decimal.NewFromInt(int64(m)).Div(decimal.NewFromInt(100))
}

// ============================================================================
// Invoice Money Calculations - All return Money (cents)
// ============================================================================

// CalculateLineTotal calculates total for a single line item
func CalculateLineTotal(quantity float64, unitPrice float64, discountRate float64, taxRate float64) Money {
	// Line subtotal = quantity * unit price
	subtotal := ToCents(quantity * unitPrice)

	// Apply discount
	if discountRate > 0 {
		discount := subtotal.Multiply(discountRate / 100)
		subtotal = subtotal.Subtract(discount)
	}

	// Apply tax
	if taxRate > 0 {
		tax := subtotal.Multiply(taxRate / 100)
		subtotal = subtotal.Add(tax)
	}

	return subtotal
}

// CalculateInvoiceSubtotal calculates subtotal for all items
func CalculateInvoiceSubtotal(items []struct {
	Quantity    float64
	UnitPrice   float64
	DiscountRate float64
}) Money {
	var subtotal Money
	for _, item := range items {
		lineTotal := CalculateLineTotal(item.Quantity, item.UnitPrice, item.DiscountRate, 0)
		subtotal = subtotal.Add(lineTotal)
	}
	return subtotal
}

// CalculateInvoiceTotal calculates total with tax and discount
func CalculateInvoiceTotal(subtotal Money, taxRate float64, discount float64) Money {
	// Apply discount
	total := subtotal.Subtract(ToCents(discount))

	// Apply tax on discounted amount
	if taxRate > 0 {
		tax := total.Multiply(taxRate / 100)
		total = total.Add(tax)
	}

	return total
}

// CalculateBalanceDue calculates remaining balance
func CalculateBalanceDue(total Money, paid Money) Money {
	if total.Subtract(paid).IsZero() {
		return 0
	}
	return total.Subtract(paid)
}

// CalculateLateFee calculates late fee with cap
func CalculateLateFee(balance Money, rate float64, cap Money, gracePeriodDays int, overdueDays int) Money {
	if overdueDays <= gracePeriodDays {
		return 0
	}

	fee := balance.Multiply(rate / 100)

	// Apply cap if set
	if cap.IsPositive() && fee.GreaterThan(cap) {
		return cap
	}

	return fee
}

// GreaterThan compares two Money values
func (m Money) GreaterThan(other Money) bool {
	return m > other
}

// LessThan compares two Money values
func (m Money) LessThan(other Money) bool {
	return m < other
}

// Equals compares two Money values
func (m Money) Equals(other Money) bool {
	return m == other
}

// ============================================================================
// Currency Conversion
// ============================================================================

// ConvertMoney converts Money from one currency to another
func ConvertMoney(amount Money, fromRate, toRate float64) Money {
	if fromRate <= 0 || toRate <= 0 {
		return amount
	}
	// Convert to base (KES), then to target
	return amount.Multiply(toRate / fromRate)
}

// ============================================================================
// Exchange Rate handling
// ============================================================================

// ExchangeRate represents an exchange rate between two currencies
type ExchangeRate struct {
	FromCurrency string
	ToCurrency   string
	Rate         float64  // e.g., 0.0091 for USD to KES
	UpdatedAt    int64    // Unix timestamp
}

// ConvertWithRate converts amount using given rate
func ConvertWithRate(amount Money, rate float64) Money {
	if rate <= 0 {
		return amount
	}
	return amount.Multiply(rate)
}

// ValidateRate validates that an exchange rate is reasonable
func ValidateRate(rate float64, fromCurrency, toCurrency string) error {
	if rate <= 0 {
		return fmt.Errorf("invalid exchange rate: must be positive")
	}

	// Sanity check - rates shouldn't differ by more than 50% daily
	// This is a basic check, real implementation would check vs previous rate
	maxRate := 10.0 // Very rough upper bound for any currency to KES
	if rate > maxRate {
		return fmt.Errorf("exchange rate exceeds maximum allowed")
	}

	_ = fromCurrency
	_ = toCurrency
	return nil
}

// ============================================================================
// VAT/Tax Calculations
// ============================================================================

// CalculateVAT calculates VAT amount from net amount
func CalculateVAT(netAmount Money, vatRate float64) Money {
	if vatRate <= 0 {
		return 0
	}
	return netAmount.Multiply(vatRate / 100)
}

// CalculateVATInclusive calculates net amount from VAT-inclusive amount
func CalculateVATInclusive(grossAmount Money, vatRate float64) (Money, Money) {
	if vatRate <= 0 {
		return grossAmount, 0
	}

	// net = gross / (1 + rate/100)
	divisor := 1 + (vatRate / 100)
	net := grossAmount.Divide(divisor)
	vat := grossAmount.Subtract(net)

	return net, vat
}

// ExtractVAT extracts VAT from inclusive prices and returns (net, vat)
func ExtractVAT(inclusiveAmount Money, vatRate float64) (Money, Money) {
	return CalculateVATInclusive(inclusiveAmount, vatRate)
}

// ============================================================================
// Rounding - Always round to nearest cent
// ============================================================================

// RoundMoney rounds Money to nearest cent
func RoundMoney(amount float64) Money {
	return ToCents(math.Round(amount*100) / 100)
}

// RoundToCurrency rounds to appropriate decimal places for currency
func RoundToCurrency(amount float64, currency string) Money {
	switch currency {
	case "KES", "TZS", "UGX", "NGN":
		// East African currencies - no decimal places
		return Money(int64(math.Round(amount)))
	case "USD", "EUR", "GBP":
		// International currencies - 2 decimal places
		return ToCents(amount)
	default:
		return ToCents(amount)
	}
}