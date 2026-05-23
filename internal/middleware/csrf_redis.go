package middleware

import (
	"context"
	"strings"
	"time"

	"invoicefast/internal/cache"

	"github.com/gofiber/fiber/v2"
)

const csrfRedisPrefix = "csrf:"

// RedisCSRFStore implements a shared CSRF token store in Redis so that
// multiple server instances accept each other's tokens.
type RedisCSRFStore struct {
	cache  *cache.RedisCache
	prefix string
}

type csrfTokenData struct {
	Value     string    `json:"value"`
	ExpiresAt time.Time `json:"expires_at"`
	UserID    string    `json:"user_id"`
}

// NewRedisCSRFStore creates a Redis-backed CSRF store.
// Returns nil when redisCache is nil so callers can fall back.
func NewRedisCSRFStore(redisCache *cache.RedisCache) *RedisCSRFStore {
	if redisCache == nil {
		return nil
	}
	return &RedisCSRFStore{cache: redisCache, prefix: csrfRedisPrefix}
}

// Get retrieves a token from Redis into the dest struct.
func (s *RedisCSRFStore) Get(ctx context.Context, token string, dest *csrfTokenData) error {
	return s.cache.Get(ctx, s.prefix+token, dest)
}

// Set stores a token in Redis with the configured TTL.
func (s *RedisCSRFStore) Set(ctx context.Context, token string, data *csrfTokenData, ttl time.Duration) error {
	return s.cache.Set(ctx, s.prefix+token, data, ttl)
}

// Del removes a token from Redis.
func (s *RedisCSRFStore) Del(ctx context.Context, token string) error {
	return s.cache.Delete(ctx, s.prefix+token)
}

// NewRedisCSRFMiddleware creates a CSRF middleware handler backed by Redis.
// Falls back to in-memory store when redisCache is nil.
func NewRedisCSRFMiddleware(redisCache *cache.RedisCache) (fiber.Handler, func()) {
	store := NewRedisCSRFStore(redisCache)
	if store == nil {
		return NewCSRFMiddleware()
	}

	config := DefaultCSRFConfig()
	return csrfHandlerWithRedis(config, store), func() {}
}

func csrfHandlerWithRedis(config CSRFConfig, store *RedisCSRFStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() == "GET" || c.Method() == "HEAD" || c.Method() == "OPTIONS" {
			return c.Next()
		}

		if strings.HasPrefix(c.Path(), "/api/v1/webhook/") ||
			c.Path() == "/api/v1/health" || c.Path() == "/health" ||
			c.Path() == "/ready" || c.Path() == "/api/v1/metrics" ||
			c.Path() == "/metrics" {
			return c.Next()
		}

		if strings.HasPrefix(c.Path(), "/api/v1/auth/") {
			return c.Next()
		}

		if strings.HasPrefix(c.Get("Authorization"), "Bearer ") {
			return c.Next()
		}

		cookieToken := c.Cookies(config.CookieName)
		if cookieToken == "" {
			token := generateCSRFToken(c)
			setCSRFCookie(c, config, token)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "CSRF token required",
				"code":  "CSRF_MISSING",
			})
		}

		ctx := context.Background()
		var stored csrfTokenData
		if err := store.Get(ctx, cookieToken, &stored); err != nil {
			token := generateCSRFToken(c)
			setCSRFCookie(c, config, token)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "CSRF token expired or invalid",
				"code":  "CSRF_INVALID",
			})
		}

		if time.Now().After(stored.ExpiresAt) {
			store.Del(ctx, cookieToken)
			token := generateCSRFToken(c)
			setCSRFCookie(c, config, token)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "CSRF token expired",
				"code":  "CSRF_INVALID",
			})
		}

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

		if requestToken != cookieToken {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Invalid CSRF token",
				"code":  "CSRF_INVALID",
			})
		}

		return c.Next()
	}
}
