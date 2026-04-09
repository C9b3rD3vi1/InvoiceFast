package utils

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

// SanitizationConfig holds configuration for sanitization
type SanitizationConfig struct {
	MaxInputLength      int
	AllowHTML           bool
	AllowNewlines       bool
	AllowSpecialChars   bool
	StrictEmailValidation bool
}

// DefaultSanitizationConfig returns default sanitization config
func DefaultSanitizationConfig() SanitizationConfig {
	return SanitizationConfig{
		MaxInputLength:      10000,
		AllowHTML:           false,
		AllowNewlines:       true,
		AllowSpecialChars:   true,
		StrictEmailValidation: true,
	}
}

// StrictSanitizationConfig returns strict sanitization for high-security contexts
func StrictSanitizationConfig() SanitizationConfig {
	return SanitizationConfig{
		MaxInputLength:      500,
		AllowHTML:           false,
		AllowNewlines:       false,
		AllowSpecialChars:   false,
		StrictEmailValidation: true,
	}
}

// SanitizeString sanitizes a string input
func SanitizeString(input string, config SanitizationConfig) string {
	// 1. Trim whitespace
	result := strings.TrimSpace(input)
	
	// 2. Limit length
	if len(result) > config.MaxInputLength {
		result = result[:config.MaxInputLength]
	}
	
	// 3. Remove null bytes and control characters
	result = removeControlChars(result)
	
	// 4. Handle HTML
	if !config.AllowHTML {
		result = stripHTMLTags(result)
		result = html.EscapeString(result)
	}
	
	// 5. Handle newlines
	if !config.AllowNewlines {
		result = strings.ReplaceAll(result, "\n", " ")
		result = strings.ReplaceAll(result, "\r", "")
	}
	
	// 6. Handle special characters for strict mode
	if !config.AllowSpecialChars {
		result = removeSpecialChars(result)
	}
	
	return result
}

// SanitizeEmail sanitizes and validates an email address
func SanitizeEmail(email string) string {
	email = strings.TrimSpace(email)
	email = strings.ToLower(email)
	email = removeControlChars(email)
	
	// Remove any HTML
	email = stripHTMLTags(email)
	email = html.EscapeString(email)
	
	return email
}

// ValidateEmail validates an email address format
func ValidateEmail(email string) error {
	email = strings.TrimSpace(email)
	
	if email == "" {
		return newValidationError("email", "email is required")
	}
	
	if len(email) > 254 {
		return newValidationError("email", "email address too long")
	}
	
	// Basic email regex
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return newValidationError("email", "invalid email format")
	}
	
	// Check for suspicious patterns
	if strings.Contains(email, "..") ||
		strings.HasPrefix(email, ".") ||
		strings.HasSuffix(email, ".") ||
		strings.Contains(email, "@.") ||
		strings.Contains(email, ".@") {
		return newValidationError("email", "invalid email format")
	}
	
	return nil
}

// SanitizePhone sanitizes and normalizes a phone number
func SanitizePhone(phone string) string {
	// Remove all non-digit characters except +
	var result strings.Builder
	for _, c := range phone {
		if unicode.IsDigit(c) || c == '+' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// ValidatePhone validates a phone number
func ValidatePhone(phone string) error {
	phone = SanitizePhone(phone)
	
	if phone == "" {
		return nil // Empty phone is acceptable
	}
	
	// Remove leading +
	checkPhone := strings.TrimPrefix(phone, "+")
	
	// Must be 9-15 digits
	if len(checkPhone) < 9 || len(checkPhone) > 15 {
		return newValidationError("phone", "phone number must be 9-15 digits")
	}
	
	// Must be all digits
	for _, c := range checkPhone {
		if !unicode.IsDigit(c) {
			return newValidationError("phone", "phone number contains invalid characters")
		}
	}
	
	return nil
}

// SanitizeName sanitizes a person/company name
func SanitizeName(name string) string {
	name = SanitizeString(name, SanitizationConfig{
		MaxInputLength:    200,
		AllowHTML:         false,
		AllowNewlines:     false,
		AllowSpecialChars: true,
	})
	
	// Remove multiple spaces
	name = collapseSpaces(name)
	
	// Capitalize properly (title case)
	name = toTitleCase(name)
	
	return name
}

// SanitizeCurrency sanitizes a currency code
func SanitizeCurrency(currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	currency = removeControlChars(currency)
	
	// Must be 3 characters
	if len(currency) != 3 {
		return "KES" // Default
	}
	
	// Valid currencies
	validCurrencies := map[string]bool{
		"KES": true, "USD": true, "EUR": true, "GBP": true,
		"TZS": true, "UGX": true, "NGN": true, "ZAR": true,
		"RWF": true, "BWP": true, "GHS": true, "ETB": true,
	}
	
	if !validCurrencies[currency] {
		return "KES" // Default
	}
	
	return currency
}

// SanitizeAmount sanitizes and validates a monetary amount
func SanitizeAmount(amount float64) (float64, error) {
	// Check for negative
	if amount < 0 {
		return 0, newValidationError("amount", "amount cannot be negative")
	}
	
	// Check for overflow
	if amount > 999999999999.99 {
		return 0, newValidationError("amount", "amount exceeds maximum value")
	}
	
	// Round to 2 decimal places
	rounded := float64(int(amount*100+0.5)) / 100
	
	return rounded, nil
}

// SanitizeInvoiceNumber sanitizes an invoice number
func SanitizeInvoiceNumber(number string) string {
	number = strings.ToUpper(strings.TrimSpace(number))
	number = removeControlChars(number)
	
	// Only allow alphanumeric and dash/underscore
	var result strings.Builder
	for _, c := range number {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-' || c == '_' || c == '/' {
			result.WriteRune(c)
		}
	}
	
	// Limit length
	if len(result.String()) > 50 {
		return result.String()[:50]
	}
	
	return result.String()
}

// SanitizeNotes sanitizes notes/description text
func SanitizeNotes(notes string) string {
	return SanitizeString(notes, SanitizationConfig{
		MaxInputLength:    5000,
		AllowHTML:         false,
		AllowNewlines:     true,
		AllowSpecialChars: true,
	})
}

// SanitizeAddress sanitizes an address
func SanitizeAddress(address string) string {
	return SanitizeString(address, SanitizationConfig{
		MaxInputLength:    500,
		AllowHTML:         false,
		AllowNewlines:     true,
		AllowSpecialChars: true,
	})
}

// SanitizeReference sanitizes a reference number
func SanitizeReference(reference string) string {
	reference = strings.TrimSpace(reference)
	reference = removeControlChars(reference)
	
	// Remove HTML
	reference = stripHTMLTags(reference)
	
	// Limit length
	if len(reference) > 100 {
		reference = reference[:100]
	}
	
	return reference
}

// SanitizeURL sanitizes a URL (for logo URLs, etc.)
func SanitizeURL(url string) string {
	url = strings.TrimSpace(url)
	url = removeControlChars(url)
	
	// Only allow http/https
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "" // Invalid URL
	}
	
	// Limit length
	if len(url) > 2000 {
		return url[:2000]
	}
	
	return url
}

// SanitizeKRAPIN sanitizes a KRA PIN number
func SanitizeKRAPIN(pin string) string {
	pin = strings.ToUpper(strings.TrimSpace(pin))
	pin = removeControlChars(pin)
	pin = strings.TrimSpace(pin)
	
	// KRA PIN format: 12 characters (letter + 9 digits + letter)
	// Example: A001234567B
	if len(pin) != 12 {
		return pin // Return as-is, validation should check
	}
	
	return pin
}

// ValidateKRAPIN validates a KRA PIN format
func ValidateKRAPIN(pin string) error {
	pin = SanitizeKRAPIN(pin)
	
	if pin == "" {
		return nil // Empty is acceptable (optional field)
	}
	
	// KRA PIN: 1 letter + 9 digits + 2 letters
	pinRegex := regexp.MustCompile(`^[A-Z][0-9]{9}[A-Z]{2}$`)
	if !pinRegex.MatchString(pin) {
		return newValidationError("kra_pin", "invalid KRA PIN format")
	}
	
	return nil
}

// SanitizeColor sanitizes a brand color (hex color)
func SanitizeColor(color string) string {
	color = strings.TrimSpace(color)
	
	// Remove #
	color = strings.TrimPrefix(color, "#")
	
	// Must be 3 or 6 hex characters
	matched, _ := regexp.MatchString(`^[0-9A-Fa-f]{3}$|^[0-9A-Fa-f]{6}$`, color)
	if !matched {
		return "#2563eb" // Default blue
	}
	
	return "#" + strings.ToLower(color)
}

// EscapeJSON escapes a string for JSON output
func EscapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// Helper functions

func removeControlChars(s string) string {
	var result strings.Builder
	for _, c := range s {
		// Allow printable ASCII and whitespace (tab, newline, carriage return)
		if c == '\t' || c == '\n' || c == '\r' || c >= 32 && c <= 126 {
			result.WriteRune(c)
		}
	}
	return result.String()
}

func stripHTMLTags(s string) string {
	// Remove HTML tags
	re := regexp.MustCompile(`<[^>]*>`)
	s = re.ReplaceAllString(s, "")
	
	// Remove HTML entities
	htmlEntities := map[string]string{
		"&lt;":   "<",
		"&gt;":   ">",
		"&amp;":  "&",
		"&quot;": "\"",
		"&#39;":  "'",
		"&#x27;": "'",
		"&#x2F;": "/",
		"&#x5C;": "\\",
	}
	for entity, char := range htmlEntities {
		s = strings.ReplaceAll(s, entity, char)
	}
	s = html.UnescapeString(s)
	
	return s
}

func removeSpecialChars(s string) string {
	var result strings.Builder
	for _, c := range s {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == ' ' || c == '-' || c == '_' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

func collapseSpaces(s string) string {
	// Replace multiple spaces with single space
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(strings.TrimSpace(s), " ")
}

func toTitleCase(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func newValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// IsValidationError checks if an error is a validation error
func IsValidationError(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}