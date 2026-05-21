package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type CSRFConfig struct {
	TokenLookup    string        `json:"token_lookup"`
	CookieName     string        `json:"cookie_name"`
	CookieDomain   string        `json:"cookie_domain"`
	CookiePath     string        `json:"cookie_path"`
	CookieSecure   bool          `json:"cookie_secure"`
	CookieHTTPOnly bool          `json:"cookie_http_only"`
	CookieSameSite string        `json:"cookie_same_site"`
	Expiration     time.Duration `json:"expiration"`
	store          *csrfStore
	SingleToken    bool `json:"single_token"`
	stopCleanup    chan struct{}
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

func (s *csrfStore) StartCleanup(interval time.Duration, stopCh chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.cleanup()
			case <-stopCh:
				return
			}
		}
	}()
}

func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		TokenLookup:    "header:X-CSRF-Token",
		CookieName:     "csrf_token",
		CookiePath:     "/",
		CookieSecure:   true,
		CookieHTTPOnly: false, // SPA needs to read this for header-based submission
		CookieSameSite: "Strict",
		Expiration:     24 * time.Hour,
		store:          newCSRFStore(),
		SingleToken:    false,
		stopCleanup:    make(chan struct{}),
	}
}

func CSRF() fiber.Handler {
	config := DefaultCSRFConfig()
	config.store.StartCleanup(10*time.Minute, config.stopCleanup)

	return func(c *fiber.Ctx) error {
		// Skip safe methods (GET, HEAD, OPTIONS)
		if c.Method() == "GET" || c.Method() == "HEAD" || c.Method() == "OPTIONS" {
			return c.Next()
		}

		// Skip health endpoints
		if c.Path() == "/api/v1/health" || c.Path() == "/health" ||
			c.Path() == "/ready" || c.Path() == "/api/v1/metrics" ||
			c.Path() == "/metrics" {
			return c.Next()
		}

		// ALL state-changing requests require CSRF validation, authenticated or not
		cookieToken := c.Cookies(config.CookieName)

		// If no cookie exists, generate one but still reject the request
		if cookieToken == "" {
			token := generateCSRFToken(c)
			setCSRFCookie(c, config, token)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "CSRF token required",
				"code":  "CSRF_MISSING",
			})
		}

		// Look up the token in the store
		storedToken := config.store.Get(cookieToken)
		if storedToken == nil || storedToken.ExpiresAt.Before(time.Now()) {
			token := generateCSRFToken(c)
			setCSRFCookie(c, config, token)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "CSRF token expired or invalid",
				"code":  "CSRF_INVALID",
			})
		}

		// Get request token from header, query, or form
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

		// Validate: must match stored token exactly
		if config.store.Get(requestToken) == nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Invalid CSRF token",
				"code":  "CSRF_INVALID",
			})
		}

		return c.Next()
	}
}

func setCSRFCookie(c *fiber.Ctx, config CSRFConfig, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     config.CookieName,
		Value:    token,
		Expires:  time.Now().Add(config.Expiration),
		Path:     config.CookiePath,
		Domain:   config.CookieDomain,
		Secure:   config.CookieSecure,
		HTTPOnly: config.CookieHTTPOnly,
		SameSite: config.CookieSameSite,
	})
}

func generateCSRFToken(c *fiber.Ctx) string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}


