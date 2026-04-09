package test

import (
	"testing"

	"invoicefast/internal/utils"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeString(t *testing.T) {
	config := utils.DefaultSanitizationConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"basic string", "Hello World", "Hello World"},
		{"trims whitespace", "  Hello World  ", "Hello World"},
		{"removes null bytes", "Hello\x00World", "HelloWorld"},
		{"removes control chars", "Hello\x1bWorld", "HelloWorld"},
		{"preserves newlines when allowed", "Hello\nWorld", "Hello\nWorld"},
		{"limits length", string(make([]byte, 15000)), ""},
		{"removes HTML", "<script>alert('xss')</script>Hello", "alert('xss')Hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeString(tt.input, config)
			if tt.name == "limits length" {
				assert.LessOrEqual(t, len(result), config.MaxInputLength)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSanitizeStringStrict(t *testing.T) {
	config := StrictSanitizationConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"removes newlines", "Hello\nWorld", "Hello World"},
		{"removes special chars", "Hello@World!", "HelloWorld"},
		{"keeps letters and numbers", "Test123", "Test123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeString(tt.input, config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeEmail(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"basic email", "Test@Example.COM", "test@example.com"},
		{"trims whitespace", "  user@example.com  ", "user@example.com"},
		{"multiple spaces", "user@example.com   ", "user@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeEmail(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		email   string
		wantErr bool
	}{
		{"test@example.com", false},
		{"user.name@example.com", false},
		{"user+tag@example.co.ke", false},
		{"invalid", true},
		{"invalid@", true},
		{"@example.com", true},
		{"", true},
		{"test@example", false},                  // Basic validation accepts
		{"test..user@example.com", true},         // Double dots
		{".test@example.com", true},              // Leading dot
		{"test.@example.com", true},              // Trailing dot
		{"test@.example.com", true},              // Leading dot in domain
		{"a".repeat(255) + "@example.com", true}, // Too long
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizePhone(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0712345678", "+254712345678"},
		{"+254712345678", "+254712345678"},
		{"254712345678", "+254712345678"},
		{"712345678", "+254712345678"},
		{"0712-345-678", "+254712345678"},
		{"(0712) 345-678", "+254712345678"},
		{"+254 712 345 678", "+254712345678"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizePhone(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidatePhone(t *testing.T) {
	tests := []struct {
		phone   string
		wantErr bool
	}{
		{"+254712345678", false},
		{"254712345678", false},
		{"0712345678", false},
		{"712345678", false},
		{"+254 712 345 678", true},     // Contains spaces after sanitization
		{"123", true},                  // Too short
		{"12345678901234567890", true}, // Too long
		{"+", true},                    // Just plus
		{"", false},                    // Empty is acceptable (optional field)
	}

	for _, tt := range tests {
		t.Run(tt.phone, func(t *testing.T) {
			err := ValidatePhone(tt.phone)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"john doe", "John Doe"},
		{"  JOHN DOE  ", "John Doe"},
		{"John <script>", "John Script"},
		{"Mary-Jane", "Mary-Jane"},
		{"O'Brien", "O'brien"}, // Note: apostrophe handling
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeCurrency(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"kes", "KES"},
		{"USD", "USD"},
		{"eur", "EUR"},
		{"GBP", "GBP"},
		{"TZS", "TZS"},
		{"UGX", "UGX"},
		{"NGN", "NGN"},
		{"invalid", "KES"}, // Defaults to KES
		{"", "KES"},        // Defaults to KES
		{"XXX", "KES"},     // Invalid defaults to KES
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeCurrency(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeAmount(t *testing.T) {
	tests := []struct {
		name     string
		amount   float64
		expected float64
		wantErr  bool
	}{
		{"valid amount", 100.50, 100.50, false},
		{"rounds to 2 decimal", 100.123, 100.12, false},
		{"zero amount", 0, 0, false},
		{"large amount", 999999999.99, 999999999.99, false},
		{"negative amount", -10, 0, true},
		{"too large", 1000000000000, 0, true}, // Overflows
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SanitizeAmount(tt.amount)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSanitizeInvoiceNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"INV-2024-001", "INV-2024-001"},
		{"inv-2024-001", "INV-2024-001"},
		{"INV_2024/001", "INV_2024/001"},
		{"INV@2024#001", "INV2024001"},
		{"12345", "12345"},
		{"  INV-001  ", "INV-001"},
		{string(make([]byte, 100)), string(make([]byte, 50))}, // Limits to 50 chars
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeInvoiceNumber(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeNotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"preserves newlines", "Line1\nLine2", "Line1\nLine2"},
		{"removes HTML", "<p>Hello</p>", "Hello"},
		{"removes control chars", "Hello\x1bWorld", "HelloWorld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeNotes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeAddress(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		checkFunc func(string) bool
	}{
		{"preserves newlines", "123 Main St\nNairobi, Kenya", func(r string) bool { return len(r) > 0 }},
		{"removes HTML", "<script>alert('xss')</script>123 Main St", func(r string) bool {
			return len(r) < len("<script>alert('xss')</script>123 Main St")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeAddress(tt.input)
			assert.True(t, tt.checkFunc(result))
		})
	}
}

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/logo.png", "https://example.com/logo.png"},
		{"http://example.com/logo.png", "http://example.com/logo.png"},
		{"ftp://example.com", ""},       // Invalid protocol
		{"javascript:alert('xss')", ""}, // Invalid protocol
		{"example.com/logo.png", ""},    // No protocol
		{"", ""},                        // Empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeKRAPIN(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"A12345678B", "A12345678B"},
		{"a12345678b", "A12345678B"},
		{"  A12345678B  ", "A12345678B"},
		{"12345678", "12345678"}, // Just returns trimmed, validation should check
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeKRAPIN(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateKRAPIN(t *testing.T) {
	tests := []struct {
		pin     string
		wantErr bool
	}{
		{"A12345678B", false}, // Valid
		{"P01234567Q", false}, // Valid
		{"", false},           // Empty is acceptable (optional)
		{"A1234567B", true},   // Too short
		{"A12345678BC", true}, // Too long
		{"12345678B", true},   // Missing leading letter
		{"A12345678", true},   // Missing trailing letters
	}

	for _, tt := range tests {
		t.Run(tt.pin, func(t *testing.T) {
			err := ValidateKRAPIN(tt.pin)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizeColor(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"#ff0000", "#ff0000"},
		{"FF0000", "#ff0000"},
		{"#F00", "#f00"},
		{"F00", "#f00"},
		{"#000000", "#000000"},
		{"invalid", "#2563eb"}, // Default blue
		{"", "#2563eb"},        // Default blue
		{"#GGGGGG", "#2563eb"}, // Invalid hex
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeColor(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEscapeJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`Hello "World"`, `Hello \"World\"`},
		{`Line1\nLine2`, `Line1\\nLine2`},
		{`Tab\there`, `Tab\\there`},
		{`Path\to\file`, `Path\\to\\file`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := EscapeJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoveControlChars(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello\x00World", "HelloWorld"},
		{"Hello\x1bWorld", "HelloWorld"},
		{"Hello\nWorld", "Hello\nWorld"}, // Newline preserved
		{"Hello\tWorld", "Hello\tWorld"}, // Tab preserved
		{"Hello\rWorld", "Hello\rWorld"}, // Carriage return preserved
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := removeControlChars(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>Hello</p>", "Hello"},
		{"<script>alert('xss')</script>", "alert('xss')"},
		{"<div><span>Hello</span></div>", "Hello"},
		{"&lt;Hello&gt;", "<Hello>"},
		{"&amp;", "&"},
		{"No HTML", "No HTML"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := stripHTMLTags(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollapseSpaces(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  Hello   World  ", "Hello World"},
		{"Hello\t\tWorld", "Hello\t\tWorld"}, // Tabs not collapsed
		{"No extra spaces", "No extra spaces"},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := collapseSpaces(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"john doe", "John Doe"},
		{"JOHN DOE", "John Doe"},
		{"johnDOE", "Johndoe"},
		{"", ""},
		{"a", "A"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := toTitleCase(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark tests
func BenchmarkSanitizeString(b *testing.B) {
	config := DefaultSanitizationConfig()
	input := "Hello <script>alert('xss')</script> World! This is a test string."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SanitizeString(input, config)
	}
}

func BenchmarkValidateEmail(b *testing.B) {
	email := "test.user+tag@example.com"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidateEmail(email)
	}
}

func BenchmarkSanitizePhone(b *testing.B) {
	phone := "+254 712 345 678"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SanitizePhone(phone)
	}
}
