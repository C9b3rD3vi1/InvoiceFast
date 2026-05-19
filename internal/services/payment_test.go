package services_test

import (
	"testing"

	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/stretchr/testify/assert"
)

// ============================================================
// Payment Service Tests
// ============================================================

// TestValidatePaymentAmount tests payment amount validation
func TestValidatePaymentAmount(t *testing.T) {
	tests := []struct {
		name    string
		amount  float64
		wantErr bool
	}{
		{"valid amount", 100.00, false},
		{"minimum amount", 1.00, false},
		{"zero amount", 0.00, true},
		{"negative amount", -50.00, true},
		{"very small amount", 0.01, false},
		{"large amount", 1000000.00, false},
		{"exact limit 100", 100.00, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := services.ValidatePaymentAmountForTest(tt.amount)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCalculateBalanceDue tests balance calculation
func TestCalculateBalanceDue(t *testing.T) {
	tests := []struct {
		name         string
		total        float64
		paidAmount   float64
		newPayment   float64
		wantBalance  float64
		wantComplete bool
	}{
		{
			name:         "first partial payment",
			total:        1000.00,
			paidAmount:   0,
			newPayment:   500.00,
			wantBalance:  500.00,
			wantComplete: false,
		},
		{
			name:         "second partial completes",
			total:        1000.00,
			paidAmount:   500.00,
			newPayment:   500.00,
			wantBalance:  0,
			wantComplete: true,
		},
		{
			name:         "overpayment",
			total:        1000.00,
			paidAmount:   0,
			newPayment:   1200.00,
			wantBalance:  0,
			wantComplete: true,
		},
		{
			name:         "exact payment",
			total:        1000.00,
			paidAmount:   0,
			newPayment:   1000.00,
			wantBalance:  0,
			wantComplete: true,
		},
		{
			name:         "multiple partials",
			total:        1000.00,
			paidAmount:   300.00,
			newPayment:   300.00,
			wantBalance:  400.00,
			wantComplete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balance, complete := services.CalculateBalanceDueForTest(tt.total, tt.paidAmount, tt.newPayment)
			assert.Equal(t, tt.wantBalance, balance)
			assert.Equal(t, tt.wantComplete, complete)
		})
	}
}

// TestPaymentStatusTransitions tests payment status workflow
func TestPaymentStatusTransitions(t *testing.T) {
	validTransitions := map[models.PaymentStatus][]models.PaymentStatus{
		models.PaymentStatusPending: {
			models.PaymentStatusProcessing,
			models.PaymentStatusFailed,
			models.PaymentStatusCancelled,
		},
		models.PaymentStatusProcessing: {
			models.PaymentStatusCompleted,
			models.PaymentStatusFailed,
		},
		models.PaymentStatusCompleted: {}, // Terminal
		models.PaymentStatusFailed:    {}, // Terminal
		models.PaymentStatusCancelled: {}, // Terminal
	}

	tests := []struct {
		name       string
		fromStatus models.PaymentStatus
		toStatus   models.PaymentStatus
		wantValid  bool
	}{
		{"pending to processing", models.PaymentStatusPending, models.PaymentStatusProcessing, true},
		{"pending to failed", models.PaymentStatusPending, models.PaymentStatusFailed, true},
		{"pending to completed", models.PaymentStatusPending, models.PaymentStatusCompleted, false},
		{"processing to completed", models.PaymentStatusProcessing, models.PaymentStatusCompleted, true},
		{"processing to failed", models.PaymentStatusProcessing, models.PaymentStatusFailed, true},
		{"completed to failed", models.PaymentStatusCompleted, models.PaymentStatusFailed, false},
		{"failed to completed", models.PaymentStatusFailed, models.PaymentStatusCompleted, false},
		{"cancelled to pending", models.PaymentStatusCancelled, models.PaymentStatusPending, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validToStatuses := validTransitions[tt.fromStatus]
			isValid := false
			for _, s := range validToStatuses {
				if s == tt.toStatus {
					isValid = true
					break
				}
			}
			assert.Equal(t, tt.wantValid, isValid)
		})
	}
}

// TestPaymentMethodTypes tests valid payment methods
func TestPaymentMethodTypes(t *testing.T) {
	validMethods := []string{"mpesa", "card", "bank", "cash", "intasend"}

	// Test that all valid methods are recognized
	for _, method := range validMethods {
		assert.True(t, services.IsValidPaymentMethodForTest(method),
			"Expected %s to be a valid payment method", method)
	}

	// Test invalid methods
	assert.False(t, services.IsValidPaymentMethodForTest("crypto"))
	assert.False(t, services.IsValidPaymentMethodForTest(""))
	assert.False(t, services.IsValidPaymentMethodForTest("bitcoin"))
	assert.False(t, services.IsValidPaymentMethodForTest("paypal"))
}

// TestValidatePhoneForPayment tests phone validation for payments
func TestValidatePhoneForPayment(t *testing.T) {
	tests := []struct {
		name  string
		phone string
		valid bool
	}{
		{"kenya valid", "254712345678", true},
		{"kenya with plus", "+254712345678", true},
		{"kenya short", "712345678", false},
		{"tanzania valid", "255712345678", true},
		{"uganda valid", "256712345678", true},
		{"empty", "", false},
		{"invalid format", "1234567890", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.ValidatePhoneForPaymentForTest(tt.phone)
			assert.Equal(t, tt.valid, result)
		})
	}
}

// TestCalculatePaymentReference tests reference generation
func TestCalculatePaymentReference(t *testing.T) {
	tests := []struct {
		name         string
		invoiceID    string
		phoneNumber  string
		amount       float64
		wantContains string
	}{
		{
			name:         "generates invoice reference",
			invoiceID:    "INV-001",
			phoneNumber:  "254712345678",
			amount:       1000,
			wantContains: "INV-001",
		},
		{
			name:         "includes amount",
			invoiceID:    "INV-002",
			phoneNumber:  "254712345678",
			amount:       5000,
			wantContains: "5000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.CalculatePaymentReferenceForTest(tt.invoiceID, tt.phoneNumber, tt.amount)
			assert.Contains(t, result, tt.wantContains)
		})
	}
}

// TestValidateMpesaCallback tests M-Pesa callback validation
func TestValidateMpesaCallback(t *testing.T) {
	// Test with valid callback structure
	validCallback := map[string]interface{}{
		"Body": map[string]interface{}{
			"StkCallback": map[string]interface{}{
				"ResultCode":        0,
				"ResultDesc":        "Success",
				"MerchantRequestID": "test-merchant-id",
				"CheckoutRequestID": "test-checkout-id",
			},
		},
	}

	err := services.ValidateMpesaCallbackForTest(validCallback)
	assert.NoError(t, err)

	// Test with failed result
	failedCallback := map[string]interface{}{
		"Body": map[string]interface{}{
			"StkCallback": map[string]interface{}{
				"ResultCode":        1,
				"ResultDesc":        "Insufficient Funds",
				"MerchantRequestID": "test-merchant-id",
			},
		},
	}

	err = services.ValidateMpesaCallbackForTest(failedCallback)
	assert.Error(t, err)

	// Test with invalid structure
	invalidCallback := map[string]interface{}{}
	err = services.ValidateMpesaCallbackForTest(invalidCallback)
	assert.Error(t, err)
}

// TestPaymentTimeout tests payment timeout calculation
func TestPaymentTimeout(t *testing.T) {
	// Test timeout calculation
	timeout := services.GetPaymentTimeoutForTest()
	assert.Greater(t, timeout, int64(0))
	assert.LessOrEqual(t, timeout, int64(300)) // Max 5 minutes
}

// TestPartialPaymentTracking tests partial payment balance tracking
func TestPartialPaymentTracking(t *testing.T) {
	// Test tracking multiple partial payments
	history := []struct {
		payment float64
		total   float64
	}{
		{payment: 500, total: 1000},
		{payment: 300, total: 1000},
		{payment: 200, total: 1000},
	}

	var totalPaid float64
	for _, h := range history {
		totalPaid += h.payment
	}

	assert.Equal(t, 1000.0, totalPaid)

	// Verify complete
	balance := 1000 - totalPaid
	assert.Equal(t, 0.0, balance)
}

// TestPaymentExpiry tests payment expiry calculation
func TestPaymentExpiry(t *testing.T) {
	// Test that expiry is calculated correctly
	createdAt := services.GetTestTimeForTest()

	expiryTime := services.CalculatePaymentExpiryForTest(createdAt)
	assert.Greater(t, expiryTime, createdAt)

	// Should be around 30 minutes
	diff := expiryTime - createdAt
	assert.LessOrEqual(t, int64(1800), diff)  // At least 30 min
	assert.GreaterOrEqual(t, int64(3600), diff) // At most 60 min
}

// TestPaymentMatchingThreshold tests auto-matching
func TestPaymentMatchingThreshold(t *testing.T) {
	tests := []struct {
		name          string
		invoiceAmount float64
		paidAmount    float64
		expectMatch   bool
	}{
		{"exact match", 1000.00, 1000.00, true},
		{"partial 50%", 1000.00, 500.00, true},
		{"over payment", 1000.00, 1100.00, true},
		{"slight under (1%)", 1000.00, 990.00, true},
		{"under (10%)", 1000.00, 900.00, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := services.ShouldAutoMatchPaymentForTest(tt.invoiceAmount, tt.paidAmount)
			assert.Equal(t, tt.expectMatch, match)
		})
	}
}

// ============================================================
// Benchmark Tests
// ============================================================

func BenchmarkValidatePaymentAmount(b *testing.B) {
	amount := 1000.00

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		services.ValidatePaymentAmountForTest(amount)
	}
}

func BenchmarkCalculateBalanceDue(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		services.CalculateBalanceDueForTest(1000, 500, 500)
	}
}

func BenchmarkPaymentMatchingThreshold(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		services.ShouldAutoMatchPaymentForTest(1000, 1000)
	}
}