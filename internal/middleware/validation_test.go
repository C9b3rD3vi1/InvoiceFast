package middleware_test

import (
	"testing"

	"invoicefast/internal/middleware"

	"github.com/stretchr/testify/assert"
)

// TestValidateEmail tests email validation
func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		valid bool
	}{
		// Valid emails
		{"valid simple", "test@example.com", true},
		{"valid with subdomain", "test@mail.example.com", true},
		{"valid with plus", "test+tag@example.com", true},
		{"valid with dot", "first.last@example.com", true},
		{"valid kenya domain", "user@simuxtech.co.ke", true},
		{"valid with dash", "test-user@example.com", true},
		{"valid government", "user@go.ke", true},
		{"valid TLD", "test@example.org", true},

		// Invalid emails
		{"invalid no @", "testexample.com", false},
		{"invalid no domain", "test@", false},
		{"invalid no local", "@example.com", false},
		{"invalid empty", "", false},
		{"invalid TLD only", "test@.com", false},
		{"invalid spaces", "test @example.com", false},
		{"invalid double @", "test@@example.com", false},
		{"invalid special chars", "test@exam!ple.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidateEmail(tt.email)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestValidatePhone tests phone validation
func TestValidatePhone(t *testing.T) {

	tests := []struct {
		name  string
		phone string
		valid bool
	}{
		// Kenya
		{"kenya with plus", "+254712345678", true},
		{"kenya without plus", "254712345678", true},
		{"kenya with 07", "0712345678", true},
		{"kenya with 01", "012345678", true},
		{"kenya with 254", "254712345678", true},

		// Tanzania
		{"tanzania with plus", "+255712345678", true},
		{"tanzania without plus", "255712345678", true},

		// Uganda
		{"uganda with plus", "+256712345678", true},

		// Nigeria
		{"nigeria with plus", "+2348031234567", true},
		{"nigeria without plus", "2348031234567", true},

		// Invalid
		{"invalid too short", "254712345", false},
		{"invalid letters", "254abcdefgh", false},
		{"invalid empty", "", false},
		{"invalid international", "+1234567890", false},
		{"invalid format", "12345", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidatePhone(tt.phone)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestValidateUUID tests UUID validation
func TestValidateUUID(t *testing.T) {
	tests := []struct {
		name  string
		uuid  string
		valid bool
	}{
		{"valid UUID v4", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid with newlines", "\n550e8400-e29b-41d4-a716-446655440000\n", true},
		{"valid uppercase", "550E8400-E29B-41D4-A716-446655440000", true},
		{"invalid format", "not-a-uuid", false},
		{"invalid empty", "", false},
		{"partial uuid", "550e8400-e29b", false},
		{"invalid hex", "550e8400-e29b-41d4-a716-44665544000g", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidateUUID(tt.uuid)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestValidateCurrency tests currency validation
func TestValidateCurrency(t *testing.T) {
	tests := []struct {
		name     string
		currency string
		valid    bool
	}{
		{"KES", "KES", true},
		{"USD", "USD", true},
		{"EUR", "EUR", true},
		{"GBP", "GBP", true},
		{"TZS", "TZS", true},
		{"UGX", "UGX", true},
		{"NGN", "NGN", true},
		{"lowercase", "kes", true},
		{"with spaces", " KES ", true},
		{"invalid", "XYZ", false},
		{"empty", "", false},
		{"partial", "K", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidateCurrency(tt.currency)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestValidateNumber tests number validation
func TestValidateNumber(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		valid  bool
	}{
		{"valid integer", "100", true},
		{"valid decimal", "100.50", true},
		{"valid negative", "-50", true},
		{"valid zero", "0", true},
		{"valid large", "1000000", true},
		{"valid decimal 2 places", "100.12", true},
		{"invalid letters", "100abc", false},
		{"invalid mixed", "100.50.50", false},
		{"invalid empty", "", false},
		{"invalid decimal start", ".50", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidateNumber(tt.input)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestValidateRequired tests required field validation
func TestValidateRequired(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid value", "test-value", false},
		{"valid number", "123", false},
		{"empty string", "", true},
		{"whitespace only", "   ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidateRequired(tt.value, "test_field")
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "test_field")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateMinLength tests minimum length validation
func TestValidateMinLength(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		minLen  int
		wantErr bool
	}{
		{"valid length", "hello", 3, false},
		{"exact length", "hi", 2, false},
		{"too short", "a", 3, true},
		{"empty string", "", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidateMinLength(tt.value, tt.minLen)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateMaxLength tests maximum length validation
func TestValidateMaxLength(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		maxLen  int
		wantErr bool
	}{
		{"valid length", "hello", 10, false},
		{"exact length", "hello", 5, false},
		{"too long", "hello world", 5, true},
		{"empty is valid", "", 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidateMaxLength(tt.value, tt.maxLen)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateRange tests range validation
func TestValidateRange(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		min     float64
		max     float64
		wantErr bool
	}{
		{"within range", "50", 0, 100, false},
		{"at min boundary", "0", 0, 100, false},
		{"at max boundary", "100", 0, 100, false},
		{"below min", "-1", 0, 100, true},
		{"above max", "101", 0, 100, true},
		{"invalid number", "abc", 0, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidateRange(tt.value, tt.min, tt.max)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateURL tests URL validation
func TestValidateURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		valid bool
	}{
		{"valid https", "https://example.com", true},
		{"valid http", "http://example.com", true},
		{"valid with path", "https://example.com/path", true},
		{"valid with query", "https://example.com?query=1", true},
		{"invalid no scheme", "example.com", false},
		{"invalid scheme", "ftp://example.com", false},
		{"invalid empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidateURL(tt.url)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestValidateDate tests date validation
func TestValidateDate(t *testing.T) {
	tests := []struct {
		name    string
		date    string
		format  string
		wantErr bool
	}{
		{"valid ISO8601", "2026-12-31T23:59:59Z", "", false},
		{"valid date only", "2026-12-31", "", false},
		{"valid with time", "2026-12-31 23:59:59", "", false},
		{"invalid format", "31-12-2026", "", true},
		{"invalid empty", "", "", true},
		{"invalid date", "2026-13-01", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := tt.format
			if format == "" {
				format = "2006-01-02"
			}
			err := middleware.ValidateDate(tt.date, format)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateDateRange tests date range validation
func TestValidateDateRange(t *testing.T) {
	tests := []struct {
		name     string
		dateStr  string
		startStr string
		endStr   string
		wantErr  bool
	}{
		{"within range", "2026-06-15", "2026-01-01", "2026-12-31", false},
		{"at start", "2026-01-01", "2026-01-01", "2026-12-31", false},
		{"at end", "2026-12-31", "2026-01-01", "2026-12-31", false},
		{"before start", "2025-06-15", "2026-01-01", "2026-12-31", true},
		{"after end", "2027-06-15", "2026-01-01", "2026-12-31", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := middleware.ValidateDateRange(tt.dateStr, tt.startStr, tt.endStr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============================================================
// Role-based Access Control Tests
// ============================================================

// TestHasRole tests role hierarchy
func TestHasRole(t *testing.T) {
	tests := []struct {
		name       string
		userRole   string
		required   string
		wantAccess bool
	}{
		// Admin accesses
		{"admin to admin", "admin", "admin", true},
		{"admin to manager", "admin", "manager", true},
		{"admin to user", "admin", "user", true},

		// Manager accesses
		{"manager to admin", "manager", "admin", false},
		{"manager to manager", "manager", "manager", true},
		{"manager to user", "manager", "user", true},

		// User accesses
		{"user to admin", "user", "admin", false},
		{"user to manager", "user", "manager", false},
		{"user to user", "user", "user", true},

		// Owner accesses
		{"owner to admin", "owner", "admin", true},
		{"owner to owner", "owner", "owner", true},

		// Viewer accesses
		{"viewer to viewer", "viewer", "viewer", true},
		{"viewer to user", "viewer", "user", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasRole := middleware.HasRoleForTest(tt.userRole, tt.required)
			assert.Equal(t, tt.wantAccess, hasRole)
		})
	}
}

// TestRoleHierarchyValues tests role hierarchy values
func TestRoleHierarchyValues(t *testing.T) {
	// Verify hierarchy values are correct
	assert.Equal(t, 5, middleware.GetRoleValueForTest("admin"))
	assert.Equal(t, 5, middleware.GetRoleValueForTest("owner"))
	assert.Equal(t, 4, middleware.GetRoleValueForTest("manager"))
	assert.Equal(t, 3, middleware.GetRoleValueForTest("staff"))
	assert.Equal(t, 2, middleware.GetRoleValueForTest("user"))
	assert.Equal(t, 1, middleware.GetRoleValueForTest("viewer"))
	assert.Equal(t, 0, middleware.GetRoleValueForTest("invalid"))
}

// ============================================================
// Security Tests
// ============================================================

// TestSanitizeInput tests input sanitization
func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal text", "Hello World", "Hello World"},
		{"with quotes", `Hello "World"`, `Hello &quot;World&quot;`},
		{"with apostrophe", "It's working", "It&#39;s working"},
		{"with html tags", "<script>alert('xss')</script>", "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := middleware.SanitizeInputForTest(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestContainsSQLInjection tests SQL injection detection
func TestContainsSQLInjection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"normal text", "Hello World", false},
		{"with numbers", "Item 123", false},
		{"SQL DROP", "test'; DROP TABLE users;--", true},
		{"SQL UNION", "test' UNION SELECT--", true},
		{"SQL SELECT", "SELECT * FROM users", true},
		{"SQL INSERT", "INSERT INTO users", true},
		{"SQL DELETE", "DELETE FROM users", true},
		{"SQL UPDATE", "UPDATE users SET", true},
		{"OR injection", "test' OR '1'='1'", true},
		{"comment", "test-- comment", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := middleware.ContainsSQLInjectionForTest(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

// ============================================================
// Benchmark Tests
// ============================================================

func BenchmarkValidateEmail(b *testing.B) {
	email := "test@example.com"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		middleware.ValidateEmail(email)
	}
}

func BenchmarkValidatePhone(b *testing.B) {
	phone := "+254712345678"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		middleware.ValidatePhone(phone)
	}
}

func BenchmarkValidateUUID(b *testing.B) {
	uuid := "550e8400-e29b-41d4-a716-446655440000"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		middleware.ValidateUUID(uuid)
	}
}

func BenchmarkSanitizeInput(b *testing.B) {
	input := "<script>alert('xss')</script>"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		middleware.SanitizeInputForTest(input)
	}
}