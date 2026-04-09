package services

import (
	"os"
	"strings"
	"testing"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test setup helpers
func setupTestDB(t *testing.T) *database.DB {
	// Use in-memory SQLite for tests
	cfg := &config.DatabaseConfig{
		Driver:            "sqlite3",
		DSN:               ":memory:",
		MaxOpenConns:      1,
		MaxIdleConns:      1,
		ConnMaxLifetime:   time.Minute,
		ConnMaxIdleTime:   time.Minute,
		QueryTimeout:      10 * time.Second,
	}

	db, err := database.New(cfg)
	require.NoError(t, err)
	
	// Run migrations
	err = db.Migrate()
	require.NoError(t, err)

	return db
}

func setupTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Port:         "8082",
			BaseURL:      "http://localhost:8082",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-for-testing-must-be-32-chars!",
			Expiry:        time.Hour,
			RefreshExpiry: 24 * time.Hour,
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite3",
			DSN:    ":memory:",
		},
	}
}

func teardownTestDB(t *testing.T, db *database.DB) {
	db.Close()
}

// ==================== AUTH SERVICE TESTS ====================

func TestAuthService_Register(t *testing.T) {
	db := setupTestDB(t)
	defer teardownTestDB(t, db)
	
	cfg := setupTestConfig()
	auth := NewAuthService(db, cfg)

	tests := []struct {
		name    string
		req     *RegisterRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid registration",
			req: &RegisterRequest{
				Email:    "test@example.com",
				Password: "SecurePass123!",
				Name:     "Test User",
				Phone:    "254712345678",
			},
			wantErr: false,
		},
		{
			name: "duplicate email",
			req: &RegisterRequest{
				Email:    "test@example.com",
				Password: "AnotherPass123!",
				Name:     "Another User",
			},
			wantErr: true,
			errMsg:  "email already registered",
		},
		{
			name: "weak password - too short",
			req: &RegisterRequest{
				Email:    "weak@example.com",
				Password: "Short1!",
				Name:     "Weak User",
			},
			wantErr: true,
			errMsg:  "password must be at least 8 characters",
		},
		{
			name: "weak password - no uppercase",
			req: &RegisterRequest{
				Email:    "nouppercase@example.com",
				Password: "lowercase123!",
				Name:     "No Upper",
			},
			wantErr: true,
			errMsg:  "uppercase",
		},
		{
			name: "weak password - no lowercase",
			req: &RegisterRequest{
				Email:    "nolowercase@example.com",
				Password: "UPPERCASE123!",
				Name:     "No Lower",
			},
			wantErr: true,
			errMsg:  "lowercase",
		},
		{
			name: "weak password - no number",
			req: &RegisterRequest{
				Email:    "nonumber@example.com",
				Password: "NoNumberHere!",
				Name:     "No Number",
			},
			wantErr: true,
			errMsg:  "number",
		},
		{
			name: "weak password - no special",
			req: &RegisterRequest{
				Email:    "nospecial@example.com",
				Password: "NoSpecial123",
				Name:     "No Special",
			},
			wantErr: true,
			errMsg:  "special",
		},
		{
			name: "common password",
			req: &RegisterRequest{
				Email:    "common@example.com",
				Password: "password123!",
				Name:     "Common Pass",
			},
			wantErr: true,
			errMsg:  "compromised",
		},
		{
			name: "invalid email - no @",
			req: &RegisterRequest{
				Email:    "invalidemail.com",
				Password: "SecurePass123!",
				Name:     "Invalid Email",
			},
			wantErr: true,
			errMsg:  "invalid email",
		},
		{
			name: "phone normalization - with 0",
			req: &RegisterRequest{
				Email:    "phone1@example.com",
				Password: "SecurePass123!",
				Name:     "Phone User 1",
				Phone:    "0712345678",
			},
			wantErr: false,
		},
		{
			name: "phone normalization - with +254",
			req: &RegisterRequest{
				Email:    "phone2@example.com",
				Password: "SecurePass123!",
				Name:     "Phone User 2",
				Phone:    "+254712345678",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := auth.Register(tt.req)
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, strings.ToLower(err.Error()), tt.errMsg)
				}
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.NotEmpty(t, resp.AccessToken)
				assert.NotEmpty(t, resp.RefreshToken)
				assert.NotNil(t, resp.User)
				assert.NotEmpty(t, resp.User.ID)
				assert.Equal(t, strings.ToLower(tt.req.Email), resp.User.Email)
			}
		})
	}
}

func TestAuthService_Login(t *testing.T) {
	db := setupTestDB(t)
	defer teardownTestDB(t, db)
	
	cfg := setupTestConfig()
	auth := NewAuthService(db, cfg)

	// Register a test user
	registered, err := auth.Register(&RegisterRequest{
		Email:    "login@example.com",
		Password: "LoginPass123!",
		Name:     "Login Test",
	})
	require.NoError(t, err)
	require.NotNil(t, registered)

	tests := []struct {
		name     string
		email    string
		password string
		wantErr  bool
	}{
		{
			name:     "valid login",
			email:    "login@example.com",
			password: "LoginPass123!",
			wantErr:  false,
		},
		{
			name:     "wrong password",
			email:    "login@example.com",
			password: "WrongPassword!",
			wantErr:  true,
		},
		{
			name:     "non-existent user",
			email:    "nonexistent@example.com",
			password: "SomePassword!",
			wantErr:  true,
		},
		{
			name:     "case insensitive email",
			email:    "LOGIN@EXAMPLE.COM",
			password: "LoginPass123!",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := auth.Login(tt.email, tt.password)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.NotEmpty(t, resp.AccessToken)
				assert.NotEmpty(t, resp.RefreshToken)
				assert.Equal(t, registered.User.ID, resp.User.ID)
			}
		})
	}
}

func TestAuthService_TokenValidation(t *testing.T) {
	db := setupTestDB(t)
	defer teardownTestDB(t, db)
	
	cfg := setupTestConfig()
	auth := NewAuthService(db, cfg)

	// Register and login
	_, err := auth.Register(&RegisterRequest{
		Email:    "token@example.com",
		Password: "TokenPass123!",
		Name:     "Token Test",
	})
	require.NoError(t, err)

	loginResp, err := auth.Login("token@example.com", "TokenPass123!")
	require.NoError(t, err)

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid access token",
			token:   loginResp.AccessToken,
			wantErr: false,
		},
		{
			name:    "invalid token",
			token:   "invalid.token.here",
			wantErr: true,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "malformed token",
			token:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := auth.ValidateToken(tt.token)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, claims)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, claims)
				assert.NotEmpty(t, claims.UserID)
				assert.Equal(t, "token@example.com", claims.Email)
			}
		})
	}
}

func TestAuthService_RefreshToken(t *testing.T) {
	db := setupTestDB(t)
	defer teardownTestDB(t, db)
	
	cfg := setupTestConfig()
	auth := NewAuthService(db, cfg)

	// Register user
	registerResp, err := auth.Register(&RegisterRequest{
		Email:    "refresh@example.com",
		Password: "RefreshPass123!",
		Name:     "Refresh Test",
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid refresh token",
			token:   registerResp.RefreshToken,
			wantErr: false,
		},
		{
			name:    "invalid refresh token",
			token:   "invalid-refresh-token",
			wantErr: true,
		},
		{
			name:    "empty refresh token",
			token:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := auth.RefreshToken(tt.token)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.NotEmpty(t, resp.AccessToken)
				assert.NotEmpty(t, resp.RefreshToken)
				
				// Verify new token is valid
				claims, err := auth.ValidateToken(resp.AccessToken)
				assert.NoError(t, err)
				assert.Equal(t, registerResp.User.ID, claims.UserID)
				
				// Old refresh token should be invalidated
				_, err = auth.RefreshToken(tt.token)
				assert.Error(t, err, "old refresh token should be invalidated")
			}
		})
	}
}

func TestAuthService_ChangePassword(t *testing.T) {
	db := setupTestDB(t)
	defer teardownTestDB(t, db)
	
	cfg := setupTestConfig()
	auth := NewAuthService(db, cfg)

	// Register user
	_, err := auth.Register(&RegisterRequest{
		Email:    "changepass@example.com",
		Password: "OldPass123!",
		Name:     "Change Pass Test",
	})
	require.NoError(t, err)

	userID, err := getUserIDByEmail(db, "changepass@example.com")
	require.NoError(t, err)

	tests := []struct {
		name        string
		oldPassword string
		newPassword string
		wantErr     bool
	}{
		{
			name:        "valid password change",
			oldPassword: "OldPass123!",
			newPassword: "NewPass456!",
			wantErr:     false,
		},
		{
			name:        "wrong old password",
			oldPassword: "WrongOldPass!",
			newPassword: "AnotherNew123!",
			wantErr:     true,
		},
		{
			name:        "weak new password",
			oldPassword: "NewPass456!",
			newPassword: "weak",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := auth.ChangePassword(userID, tt.oldPassword, tt.newPassword)
			
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				// Verify can login with new password
				_, err := auth.Login("changepass@example.com", tt.newPassword)
				assert.NoError(t, err, "should be able to login with new password")
			}
		})
	}
}

func TestAuthService_APIKeys(t *testing.T) {
	db := setupTestDB(t)
	defer teardownTestDB(t, db)
	
	cfg := setupTestConfig()
	auth := NewAuthService(db, cfg)

	// Register user
	_, err := auth.Register(&RegisterRequest{
		Email:    "apikey@example.com",
		Password: "ApiKeyPass123!",
		Name:     "API Key Test",
	})
	require.NoError(t, err)

	userID, err := getUserIDByEmail(db, "apikey@example.com")
	require.NoError(t, err)

	// Generate API key
	key, err := auth.GenerateAPIKey(userID, "Test Key")
	require.NoError(t, err)
	assert.NotEmpty(t, key)
	assert.True(t, strings.HasPrefix(key, "if_sk_"))

	// Validate API key
	user, err := auth.ValidateAPIKey(key)
	require.NoError(t, err)
	assert.Equal(t, userID, user.ID)

	// Invalid API key
	_, err = auth.ValidateAPIKey("if_sk_invalid_key")
	assert.Error(t, err)

	// Empty API key
	_, err = auth.ValidateAPIKey("")
	assert.Error(t, err)
}

// ==================== INPUT VALIDATION TESTS ====================

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
		{"test@example", false}, // Basic validation accepts this
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			err := validateEmail(tt.email)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		password string
		wantErr  bool
	}{
		{"SecurePass123!", false},
		{"AnotherGood1@", false},
		{"short", true},           // too short
		{"NOLOWER123!", true},     // no lowercase
		{"noupper123!", true},     // no uppercase
		{"NoNumbers!", true},      // no numbers
		{"NoSpecial123", true},    // no special chars
		{"password123!", true},    // common password
	}

	for _, tt := range tests {
		t.Run(tt.password, func(t *testing.T) {
			err := validatePassword(tt.password)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0712345678", "254712345678"},
		{"+254712345678", "254712345678"},
		{"254712345678", "254712345678"},
		{"712345678", "254712345678"},
		{"+254 712 345 678", "254712345678"},
		{"0712-345-678", "254712345678"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizePhone(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ==================== HELPER FUNCTIONS ====================

func getUserIDByEmail(db *database.DB, email string) (string, error) {
	var user models.User
	if err := db.First(&user, "email = ?", email).Error; err != nil {
		return "", err
	}
	return user.ID, nil
}

// ==================== BENCHMARK TESTS ====================

func BenchmarkAuthService_Register(b *testing.B) {
	cfg := setupTestConfig()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		db := setupTestDB(nil)
		auth := NewAuthService(db, cfg)
		b.StartTimer()
		
		auth.Register(&RegisterRequest{
			Email:    "bench@example.com",
			Password: "BenchPass123!",
			Name:     "Bench User",
		})		
		
		b.StopTimer()
		db.Close()
		b.StartTimer()
	}
}

func BenchmarkAuthService_Login(b *testing.B) {
	cfg := setupTestConfig()
	db := setupTestDB(nil)
	defer db.Close()
	
	auth := NewAuthService(db, cfg)
	auth.Register(&RegisterRequest{
		Email:    "benchlogin@example.com",
		Password: "BenchPass123!",
		Name:     "Bench Login",
	})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auth.Login("benchlogin@example.com", "BenchPass123!")
	}
}

func BenchmarkAuthService_TokenValidation(b *testing.B) {
	cfg := setupTestConfig()
	db := setupTestDB(nil)
	defer db.Close()
	
	auth := NewAuthService(db, cfg)
	resp, _ := auth.Register(&RegisterRequest{
		Email:    "benchtoken@example.com",
		Password: "BenchPass123!",
		Name:     "Bench Token",
	})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auth.ValidateToken(resp.AccessToken)
	}
}