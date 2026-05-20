package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// CSRFConfig holds CSRF middleware configuration
type CSRFConfig struct {
	// TokenLookup is how to look up the CSRF token
	// Options: "header:Name", "query:Name", "form:Name"
	TokenLookup string `json:"token_lookup"`

	// CookieName is the name of the CSRF cookie
	CookieName string `json:"cookie_name"`

	// CookieDomain is the domain for the CSRF cookie
	CookieDomain string `json:"cookie_domain"`

	// CookiePath is the path for the CSRF cookie
	CookiePath string `json:"cookie_path"`

	// CookieSecure sets the secure flag on the CSRF cookie
	CookieSecure bool `json:"cookie_secure"`

	// CookieHTTPOnly sets the HttpOnly flag on the CSRF cookie
	CookieHTTPOnly bool `json:"cookie_http_only"`

	// CookieSameSite sets the SameSite mode on the CSRF cookie
	CookieSameSite string `json:"cookie_same_site"`

	// Expiration is how long the CSRF token is valid
	Expiration time.Duration `json:"expiration"`

	// Storage for tokens (in production, use Redis)
	store *csrfStore

	// Single token per user (set to true for higher security)
	SingleToken bool `json:"single_token"`
}

type csrfToken struct {
	Value     string
	ExpiresAt time.Time
	UserID    string
}

type csrfStore struct {
	tokens map[string]*csrfToken
	mu     sync.RWMutex
}

func newCSRFStore() *csrfStore {
	return &csrfStore{
		tokens: make(map[string]*csrfToken),
	}
}

func (s *csrfStore) Get(token string) *csrfToken {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tokens[token]
}

func (s *csrfStore) Set(userID string, token *csrfToken) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean up expired tokens periodically
	s.cleanup()

	s.tokens[token.Value] = token
	s.tokens[token.Value].UserID = userID
}

func (s *csrfStore) cleanup() {
	now := time.Now()
	for token, csrf := range s.tokens {
		if csrf.ExpiresAt.Before(now) {
			delete(s.tokens, token)
		}
	}
}

func (s *csrfStore) StartCleanup(interval time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("PANIC in CSRF cleanup: %v\n", r)
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			s.cleanup()
		}
	}()
}

// DefaultCSRFConfig returns default CSRF configuration
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		TokenLookup:    "header:X-CSRF-Token",
		CookieName:     "csrf_token",
		CookiePath:     "/",
		CookieSecure:   true,
		CookieHTTPOnly: true,
		CookieSameSite: "Strict",
		Expiration:     24 * time.Hour,
		store:          newCSRFStore(),
		SingleToken:    false,
	}
}

// CSRF returns a CSRF protection middleware
func CSRF() fiber.Handler {
	config := DefaultCSRFConfig()
	config.store.StartCleanup(10 * time.Minute)

	return func(c *fiber.Ctx) error {
		// Skip for GET, HEAD, OPTIONS (safe methods)
		if c.Method() == "GET" || c.Method() == "HEAD" || c.Method() == "OPTIONS" {
			return c.Next()
		}

		// Skip for API paths without authentication
		if c.Path() == "/api/v1/health" || c.Path() == "/health" {
			return c.Next()
		}

		// Check if user is authenticated (skip for public endpoints)
		if c.Locals("user_id") == nil {
			return c.Next() // Skip for public endpoints
		}

		// Get the token from storage
		cookieToken := c.Cookies(config.CookieName)

		// Validate token
		if cookieToken == "" {
			// Generate new token for initial request
			token := generateCSRFToken(c)
			c.Cookie(&fiber.Cookie{
				Name:      config.CookieName,
				Value:     token,
				Expires:   time.Now().Add(config.Expiration),
				Path:      config.CookiePath,
				Domain:    config.CookieDomain,
				Secure:    config.CookieSecure,
				HTTPOnly:  config.CookieHTTPOnly,
				SameSite:  config.CookieSameSite,
			})
			return c.Next()
		}

		// Validate the token
		storedToken := config.store.Get(cookieToken)
		if storedToken == nil || storedToken.ExpiresAt.Before(time.Now()) {
			// Token expired or invalid, generate new one
			token := generateCSRFToken(c)
			c.Cookie(&fiber.Cookie{
				Name:      config.CookieName,
				Value:     token,
				Expires:   time.Now().Add(config.Expiration),
				Path:      config.CookiePath,
				Domain:    config.CookieDomain,
				Secure:    config.CookieSecure,
				HTTPOnly:  config.CookieHTTPOnly,
				SameSite:  config.CookieSameSite,
			})
			return c.Next()
		}

		// Check token in request (can be in header, query, or form)
		requestToken := c.Get("X-CSRF-Token")
		if requestToken == "" {
			requestToken = c.Query("csrf_token")
		}
		if requestToken == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "CSRF token required",
				"code":  "CSRF_MISSING",
			})
		}

		// Validate request token matches stored token
		if requestToken != cookieToken {
			// Also check if it's in our store
			if config.store.Get(requestToken) == nil {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": "Invalid CSRF token",
					"code":  "CSRF_INVALID",
				})
			}
		}

		return c.Next()
	}
}

// generateCSRFToken generates a secure CSRF token
func generateCSRFToken(c *fiber.Ctx) string {
	// Create unique token based on user session + random
	userID := ""
	if u := c.Locals("user_id"); u != nil {
		userID = u.(string)
	}

	raw := fmt.Sprintf("%s:%s:%d:%s", userID, c.IP(), time.Now().UnixNano(), uuid.New().String())
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}