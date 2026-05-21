package middleware

import (
	"context"
	"time"

	"invoicefast/internal/cache"

	"github.com/gofiber/fiber/v2"
)

// RedisRateLimiter wraps cache.RateLimiter as a Fiber middleware for
// horizontal scaling across multiple server instances.
type RedisRateLimiter struct {
	limiter *cache.RateLimiter
}

// NewRedisRateLimiter creates a Redis-backed rate limiter.
// Returns nil when redisCache is nil so callers can fall back.
func NewRedisRateLimiter(redisCache *cache.RedisCache) *RedisRateLimiter {
	if redisCache == nil {
		return nil
	}
	return &RedisRateLimiter{limiter: cache.NewRateLimiter(redisCache)}
}

// Middleware returns a sliding-window rate limiter using Redis INCR + EXPIRE.
func (r *RedisRateLimiter) Middleware(limit int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := getClientKeyFromCtx(c)
		allowed, err := r.limiter.Allow(context.Background(), key, limit, window)
		if err != nil {
			return c.Next()
		}
		if !allowed {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       "Rate limit exceeded",
				"retry_after": window.Seconds(),
			})
		}
		return c.Next()
	}
}

func getClientKeyFromCtx(c *fiber.Ctx) string {
	if forwarded := c.Get("X-Forwarded-For"); forwarded != "" {
		if idx := indexByte(forwarded, ','); idx > 0 {
			return forwarded[:idx]
		}
		return forwarded
	}
	if cfRay := c.Get("CF-Connecting-IP"); cfRay != "" {
		return cfRay
	}
	return c.IP()
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
