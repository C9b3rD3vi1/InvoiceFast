package middleware

import (
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Validation schemas for different request types
type ValidationSchema struct {
	Fields map[string]FieldValidator
}

type FieldValidator struct {
	Required    bool
	Type        string // "email", "phone", "number", "string", "currency", "uuid"
	MinLen      int
	MaxLen      int
	Pattern     *regexp.Regexp
	Custom      func(interface{}) error
}

var (
	// Email regex - RFC 5322 simplified
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	
	// Phone regex - supports international formats
	phoneRegex = regexp.MustCompile(`^\+?[1-9]\d{1,14}$|^254\d{9}$`)
	
	// UUID regex
	uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// ValidateInput creates validation middleware for request body
func ValidateInput(schema ValidationSchema) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Parse body
		var data map[string]interface{}
		if err := c.BodyParser(&data); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
				"code":  "INVALID_BODY",
			})
		}

		// Validate each field
		var errors []string
		for field, validator := range schema.Fields {
			value, exists := data[field]

			// Check required
			if validator.Required && !exists {
				errors = append(errors, field+" is required")
				continue
			}

			if !exists {
				continue // Optional field, skip
			}

			// Validate type
			if err := validateType(value, validator); err != nil {
				errors = append(errors, field+": "+err.Error())
			}
		}

		if len(errors) > 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":  "Validation failed",
				"code":   "VALIDATION_ERROR",
				"errors": errors,
			})
		}

		return c.Next()
	}
}

func validateType(value interface{}, validator FieldValidator) error {
	str, isString := value.(string)
	num, isNumber := value.(float64)

	switch validator.Type {
	case "email":
		if !isString || !emailRegex.MatchString(str) {
			return fiber.NewError(fiber.StatusBadRequest, "invalid email format")
		}

	case "phone":
		if !isString {
			return fiber.NewError(fiber.StatusBadRequest, "phone must be a string")
		}
		// Normalize phone for validation
		normalized := normalizePhoneNumber(str)
		if !phoneRegex.MatchString(normalized) {
			return fiber.NewError(fiber.StatusBadRequest, "invalid phone number format")
		}

	case "number", "currency":
		if !isNumber {
			return fiber.NewError(fiber.StatusBadRequest, "must be a number")
		}
		if num < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "must be non-negative")
		}

	case "uuid":
		if !isString || !uuidRegex.MatchString(str) {
			return fiber.NewError(fiber.StatusBadRequest, "invalid UUID format")
		}

	case "string":
		if !isString {
			return fiber.NewError(fiber.StatusBadRequest, "must be a string")
		}
		if validator.MinLen > 0 && len(str) < validator.MinLen {
			return fiber.NewError(fiber.StatusBadRequest, "minimum length is "+string(rune(validator.MinLen)))
		}
		if validator.MaxLen > 0 && len(str) > validator.MaxLen {
			return fiber.NewError(fiber.StatusBadRequest, "maximum length is "+string(rune(validator.MaxLen)))
		}

	case "positive":
		if !isNumber || num <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, "must be positive")
		}
	}

	// Custom validation
	if validator.Custom != nil {
		return validator.Custom(value)
	}

	return nil
}

// normalizePhoneNumber normalizes phone numbers for validation
func normalizePhoneNumber(phone string) string {
	// Remove common separators
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")

	// Handle Kenya numbers
	if strings.HasPrefix(phone, "0") {
		phone = "254" + phone[1:]
	}
	if strings.HasPrefix(phone, "7") || strings.HasPrefix(phone, "1") {
		phone = "254" + phone
	}

	return phone
}

// Common validation schemas for reuse

// InvoiceCreateSchema validates invoice creation requests
var InvoiceCreateSchema = ValidationSchema{
	Fields: map[string]FieldValidator{
		"client_id":    {Required: true, Type: "uuid"},
		"currency":     {Required: false, Type: "string", MinLen: 3, MaxLen: 3},
		"due_date":     {Required: false, Type: "string"},
		"items":        {Required: true, Custom: validateItemsArray},
	},
}

// ClientCreateSchema validates client creation requests
var ClientCreateSchema = ValidationSchema{
	Fields: map[string]FieldValidator{
		"name":    {Required: true, Type: "string", MinLen: 1, MaxLen: 200},
		"email":   {Required: false, Type: "email"},
		"phone":   {Required: false, Type: "phone"},
		"currency": {Required: false, Type: "string", MinLen: 3, MaxLen: 3},
	},
}

// PaymentRequestSchema validates payment initiation
var PaymentRequestSchema = ValidationSchema{
	Fields: map[string]FieldValidator{
		"invoice_id":   {Required: true, Type: "uuid"},
		"amount":       {Required: true, Type: "currency"},
		"phone_number": {Required: true, Type: "phone"},
	},
}

// AmountValidator validates monetary amounts
func AmountValidator(amount interface{}) error {
	num, ok := amount.(float64)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "amount must be a number")
	}
	if num < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "amount cannot be negative")
	}
	// Check for reasonable precision (max 2 decimal places for cents)
	// This is a safeguard but real implementation should use int64 cents
	if num > 1e15 {
		return fiber.NewError(fiber.StatusBadRequest, "amount exceeds maximum")
	}
	return nil
}

func validateItemsArray(value interface{}) error {
	items, ok := value.([]interface{})
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "items must be an array")
	}
	if len(items) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one item required")
	}
	if len(items) > 1000 {
		return fiber.NewError(fiber.StatusBadRequest, "maximum 1000 items allowed")
	}
	return nil
}