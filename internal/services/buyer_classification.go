package services

import (
	"strings"

	"invoicefast/internal/models"
)

// BuyerTypeResult contains the detected and suggested buyer types
type BuyerTypeResult struct {
	Detected  string `json:"detected"`  // Auto-detected based on client data
	Suggested string `json:"suggested"` // Suggested (considers user preference)
}

// DetectBuyerType automatically determines buyer classification based on client data
// Rules:
// - Valid KRA PIN (format AxxxxxxxB) → B2B
// - Foreign country OR non-KES currency → EXPORT
// - Employee flag set → B2E
// - Otherwise → B2C
func DetectBuyerType(client *models.Client) string {
	if client == nil {
		return string(models.BuyerClassificationB2C)
	}

	// Check for B2B (valid KRA PIN)
	if client.KRAPIN != "" && isValidKRAPIN(client.KRAPIN) {
		return string(models.BuyerClassificationB2B)
	}

	// Check for EXPORT (foreign country or non-KES currency)
	if client.Country != "" && client.Country != "KE" && client.Country != "KENYA" {
		return string(models.BuyerClassificationEXPORT)
	}

	// Check for non-KES currency
	if client.Currency != "" && client.Currency != "KES" {
		return string(models.BuyerClassificationEXPORT)
	}

	// Check for employee (B2E)
	if client.IsEmployee {
		return string(models.BuyerClassificationB2E)
	}

	// Default to B2C (consumer)
	return string(models.BuyerClassificationB2C)
}

// SuggestBuyerType returns the recommended buyer type considering user preferences
func SuggestBuyerType(client *models.Client) string {
	if client == nil {
		return string(models.BuyerClassificationB2C)
	}

	// Use preferred buyer type if explicitly set
	if client.PreferredBuyerType != "" {
		// Validate it's a valid type
		bt := models.BuyerClassification(client.PreferredBuyerType)
		if bt.IsValid() {
			return string(bt)
		}
	}

	// Fall back to auto-detection
	return DetectBuyerType(client)
}

// GetBuyerTypeResult returns both detected and suggested buyer types
func GetBuyerTypeResult(client *models.Client) *BuyerTypeResult {
	if client == nil {
		return &BuyerTypeResult{
			Detected:  string(models.BuyerClassificationB2C),
			Suggested: string(models.BuyerClassificationB2C),
		}
	}

	detected := DetectBuyerType(client)
	suggested := SuggestBuyerType(client)

	return &BuyerTypeResult{
		Detected:  detected,
		Suggested: suggested,
	}
}

// ValidateBuyerType validates that buyer type is appropriate for the client
// Returns error if there's a mismatch
func ValidateBuyerType(buyerType string, client *models.Client) error {
	if buyerType == "" {
		return nil // No validation needed
	}

	bt := models.BuyerClassification(buyerType)
	if !bt.IsValid() {
		return nil // Will be caught by other validation
	}

	switch bt {
	case models.BuyerClassificationB2B:
		// B2B requires valid KRA PIN
		if client == nil || client.KRAPIN == "" {
			return &ValidationError{
				Field:   "buyer_type",
				Message: "B2B classification requires a valid KRA PIN",
				Code:    "INVALID_BUYER_TYPE",
			}
		}
		if !isValidKRAPIN(client.KRAPIN) {
			return &ValidationError{
				Field:   "buyer_type",
				Message: "Invalid KRA PIN format for B2B classification",
				Code:    "INVALID_BUYER_TYPE",
			}
		}

	case models.BuyerClassificationEXPORT:
		// EXPORT requires foreign currency or country
		if client == nil {
			return nil // Allow for now
		}
		isForeign := (client.Country != "" && client.Country != "KE" && client.Country != "KENYA") ||
			(client.Currency != "" && client.Currency != "KES")
		if !isForeign {
			return &ValidationError{
				Field:   "buyer_type",
				Message: "EXPORT classification requires foreign country or non-KES currency",
				Code:    "INVALID_BUYER_TYPE",
			}
		}

	case models.BuyerClassificationB2E:
		// B2E should have employee flag
		if client == nil || !client.IsEmployee {
			// Warning only, not error - allow override
		}
	}

	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
	Code    string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// isValidKRAPIN checks if KRA PIN format is valid
// Format: AxxxxxxxxB (starts with A, ends with B, 11 chars)
func isValidKRAPIN(pin string) bool {
	if pin == "" {
		return false
	}

	// Must be 11 characters
	if len(pin) != 11 {
		return false
	}

	// Must start with A and end with B
	if !strings.HasPrefix(pin, "A") || !strings.HasSuffix(pin, "B") {
		return false
	}

	// Middle must be alphanumeric
	middle := pin[1 : len(pin)-1]
	for _, c := range middle {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}

	return true
}

// GetBuyerTypeLabel returns human-readable label for buyer type
func GetBuyerTypeLabel(buyerType string) string {
	switch buyerType {
	case "B2B":
		return "Business (B2B)"
	case "B2C":
		return "Consumer (B2C)"
	case "B2E":
		return "Employee (B2E)"
	case "EXPORT":
		return "Export (EXPORT)"
	default:
		return "Unknown"
	}
}

// GetBuyerTypeRequirements returns requirements for each buyer type
func GetBuyerTypeRequirements(buyerType string) string {
	switch buyerType {
	case "B2B":
		return "Valid KRA PIN required (format: AxxxxxxxxB)"
	case "B2C":
		return "No special requirements"
	case "B2E":
		return "Employee flag should be set"
	case "EXPORT":
		return "Foreign country or non-KES currency required"
	default:
		return ""
	}
}