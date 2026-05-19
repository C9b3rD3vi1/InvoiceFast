package services

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// ============================================================
// Payment Test Helpers - Exported functions for test packages
// ============================================================

// ValidatePaymentAmountForTest validates payment amount
func ValidatePaymentAmountForTest(amount float64) error {
	if amount <= 0 {
		return errors.New("invalid payment amount")
	}
	return nil
}

// CalculateBalanceDueForTest calculates remaining balance
func CalculateBalanceDueForTest(total, paidAmount, newPayment float64) (float64, bool) {
	newPaid := paidAmount + newPayment
	if newPaid >= total {
		return 0, true
	}
	return total - newPaid, false
}

// IsValidPaymentMethodForTest validates payment method
func IsValidPaymentMethodForTest(method string) bool {
	validMethods := map[string]bool{
		"mpesa":    true,
		"card":     true,
		"bank":     true,
		"cash":     true,
		"intasend": true,
	}
	return validMethods[method]
}

// ValidatePhoneForPaymentForTest validates phone for payment
func ValidatePhoneForPaymentForTest(phone string) bool {
	if phone == "" {
		return false
	}

	// Remove formatting
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")

	if strings.HasPrefix(phone, "+") {
		phone = phone[1:]
	}

	// Check valid length and prefix
	phone = strings.TrimLeft(phone, "0")

	// Kenya: 254 + 9 digits = 12
	// Tanzania: 255 + 9 digits = 12
	// Uganda: 256 + 9 digits = 12
	// Nigeria: 234 + 10 digits = 13
	validPrefixes := []string{"254", "255", "256", "234"}

	for _, prefix := range validPrefixes {
		if strings.HasPrefix(phone, prefix) {
			return len(phone) >= 11 && len(phone) <= 13
		}
	}

	return false
}

// CalculatePaymentReferenceForTest generates payment reference
func CalculatePaymentReferenceForTest(invoiceID, phoneNumber string, amount float64) string {
	// Format: INV-001-2547XXX-1000.00
	ref := invoiceID
	if phoneNumber != "" {
		// Show last 4 digits of phone
		if len(phoneNumber) > 4 {
			ref += "-" + phoneNumber[len(phoneNumber)-4:]
		}
	}
	ref += "-" + formatAmount(amount)
	return ref
}

// ValidateMpesaCallbackForTest validates M-Pesa callback
func ValidateMpesaCallbackForTest(callback map[string]interface{}) error {
	body, ok := callback["Body"].(map[string]interface{})
	if !ok {
		return errors.New("invalid callback structure")
	}

	stkCallback, ok := body["StkCallback"].(map[string]interface{})
	if !ok {
		return errors.New("invalid stk callback")
	}

	resultCode, ok := stkCallback["ResultCode"].(float64)
	if !ok {
		// Try int
		resultCodeInt, ok := stkCallback["ResultCode"].(int)
		if !ok {
			return errors.New("invalid result code")
		}
		resultCode = float64(resultCodeInt)
	}

	if resultCode != 0 {
		resultDesc, _ := stkCallback["ResultDesc"].(string)
		return errors.New(resultDesc)
	}

	return nil
}

// GetPaymentTimeoutForTest returns payment timeout in seconds
func GetPaymentTimeoutForTest() int64 {
	return 1800 // 30 minutes
}

// GetTestTimeForTest returns current time for testing
func GetTestTimeForTest() int64 {
	return time.Now().Unix()
}

// CalculatePaymentExpiryForTest calculates payment expiry time
func CalculatePaymentExpiryForTest(createdAt int64) int64 {
	return createdAt + 1800 // 30 minutes from creation
}

// ShouldAutoMatchPaymentForTest determines if payment should auto-match
func ShouldAutoMatchPaymentForTest(invoiceAmount, paidAmount float64) bool {
	diff := paidAmount - invoiceAmount
	if diff >= 0 {
		return true // Exact or over payment
	}
	// Check if within 5% threshold
	threshold := invoiceAmount * 0.05
	return diff >= -threshold
}

// ============================================================
// Helper functions
// ============================================================

func formatAmount(amount float64) string {
	return strconv.FormatFloat(amount, 'f', 2, 64)
}

// ValidateSTKPushRequest validates STK push request
func ValidateSTKPushRequest(req STKPushRequest) error {
	if req.BusinessShortCode == "" {
		return errors.New("business short code required")
	}
	if req.Amount == "0" || req.Amount == "" {
		return errors.New("amount must be positive")
	}
	if req.PhoneNumber == "" {
		return errors.New("phone number required")
	}
	return nil
}

// PaymentStatus constants
const (
	PaymentStatusPending   = "pending"
	PaymentStatusProcessing = "processing"
	PaymentStatusCompleted = "completed"
	PaymentStatusFailed    = "failed"
	PaymentStatusCancelled = "cancelled"
)

// ValidateMPesaAmount validates M-Pesa amount
func ValidateMPesaAmount(amount string) error {
	amountInt, err := strconv.Atoi(amount)
	if err != nil {
		return errors.New("invalid amount format")
	}
	if amountInt < 1 {
		return errors.New("minimum amount is 1")
	}
	if amountInt > 999999 {
		return errors.New("maximum amount exceeded")
	}
	return nil
}

// ExtractMpesaReceipt extracts receipt number from callback
func ExtractMpesaReceipt(callback map[string]interface{}) (string, error) {
	body, ok := callback["Body"].(map[string]interface{})
	if !ok {
		return "", errors.New("invalid callback")
	}

	stkCallback, ok := body["StkCallback"].(map[string]interface{})
	if !ok {
		return "", errors.New("invalid stk callback")
	}

	metadata, ok := stkCallback["CallbackMetadata"].(map[string]interface{})
	if !ok {
		return "", errors.New("no callback metadata")
	}

	items, ok := metadata["Item"].([]interface{})
	if !ok {
		return "", errors.New("no items in metadata")
	}

	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := itemMap["Name"].(string)
		if name == "MpesaReceiptNumber" {
			value, _ := itemMap["Value"].(string)
			return value, nil
		}
	}

	return "", errors.New("receipt number not found")
}

// FormatPaymentStatus formats payment status for display
func FormatPaymentStatus(status string) string {
	statusDisplay := map[string]string{
		"pending":    "Pending",
		"processing": "Processing",
		"completed":  "Completed",
		"failed":     "Failed",
		"cancelled":  "Cancelled",
	}

	if display, ok := statusDisplay[status]; ok {
		return display
	}
	return status
}

// CalculatePaymentFee calculates payment processing fee
func CalculatePaymentFee(amount float64, method string) float64 {
	rates := map[string]float64{
		"mpesa":    0.0, // Free for business
		"card":     0.029,
		"bank":     0.0,
		"cash":     0.0,
		"intasend": 0.025,
	}

	rate := rates[method]
	if rate == 0 {
		return 0
	}

	return roundToTwoDecimals(amount * rate)
}

// GetMPesaLimit returns M-Pesa transaction limits
func GetMPesaLimit() (min, max int) {
	return 10, 150000 // KES
}