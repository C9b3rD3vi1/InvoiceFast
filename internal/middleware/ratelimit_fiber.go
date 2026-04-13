package middleware

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// FiberRateLimiter implements a production-ready token bucket rate limiter for Fiber
type FiberRateLimiter struct {
	mu              sync.RWMutex
	tokens          map[string]*clientTokens
	rate            int
	window          time.Duration
	burst           int
	cleanupInterval time.Duration
	stopCleanup     chan bool
}

type clientTokens struct {
	tokens    []time.Time
	lastCheck time.Time
}

// EndpointConfig allows different rate limits per endpoint
type EndpointConfig struct {
	Path   string
	Rate   int
	Window time.Duration
	Burst  int
}

// Default endpoint configs
var defaultEndpointConfigs = []EndpointConfig{
	{Path: "/webhook/", Rate: 30, Window: time.Minute, Burst: 10},   // Webhooks - stricter
	{Path: "/auth/", Rate: 10, Window: time.Minute, Burst: 5},         // Auth endpoints
	{Path: "/api/v1/invoices", Rate: 60, Window: time.Minute, Burst: 15}, // Invoice operations
	{Path: "/api/v1/", Rate: 100, Window: time.Minute, Burst: 20},    // Default API
}

// NewFiberRateLimiter creates a new rate limiter with default settings
func NewFiberRateLimiter() *FiberRateLimiter {
	return NewFiberRateLimiterWithConfig(100, time.Minute, 20)
}

// NewFiberRateLimiterWithConfig creates a rate limiter with custom settings
func NewFiberRateLimiterWithConfig(rate int, window time.Duration, burst int) *FiberRateLimiter {
	rl := &FiberRateLimiter{
		tokens:          make(map[string]*clientTokens),
		rate:            rate,
		window:          window,
		burst:           burst,
		cleanupInterval: 5 * time.Minute,
		stopCleanup:     make(chan bool),
	}
	go rl.cleanup()
	return rl
}

// Middleware returns a Fiber middleware handler
func (rl *FiberRateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := rl.getClientKey(c)
		endpoint := c.Path()

		if !rl.AllowForEndpoint(key, endpoint) {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       "Rate limit exceeded",
				"retry_after": rl.window.Seconds(),
			})
		}
		return c.Next()
	}
}

// AuthRateLimiter is stricter for auth endpoints
func (rl *FiberRateLimiter) AuthRateLimiter() fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := rl.getClientKey(c)
		
		rl.mu.Lock()
		defer rl.mu.Unlock()
		
		now := time.Now()
		windowStart := now.Add(-rl.window)
		authRate := 10 // Stricter for auth
		
		client, exists := rl.tokens[key]
		if exists {
			var valid []time.Time
			for _, t := range client.tokens {
				if t.After(windowStart) {
					valid = append(valid, t)
				}
			}
			
			if len(valid) >= authRate {
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error":       "Too many authentication attempts",
					"retry_after": "60 seconds",
				})
			}
			
			client.tokens = append(valid, now)
		} else {
			rl.tokens[key] = &clientTokens{
				tokens:    []time.Time{now},
				lastCheck: now,
			}
		}
		
		return c.Next()
	}
}

// WebhookRateLimiter is stricter for webhooks
func (rl *FiberRateLimiter) WebhookRateLimiter() fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := rl.getClientKey(c)
		endpoint := c.Path()

		if !rl.AllowForWebhook(key, endpoint) {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       "Webhook rate limit exceeded",
				"retry_after": "60 seconds",
			})
		}
		return c.Next()
	}
}

// AllowForWebhook uses stricter limits
func (rl *FiberRateLimiter) AllowForWebhook(key, endpoint string) bool {
	return rl.allowWithRate(key, 30) // 30 reqs/min for webhooks
}

// AllowForEndpoint checks rate limit for a specific endpoint
func (rl *FiberRateLimiter) AllowForEndpoint(key, endpoint string) bool {
	config := rl.getEndpointConfig(endpoint)
	return rl.allowWithRate(key, config.Rate)
}

// Allow checks default rate limit
func (rl *FiberRateLimiter) Allow(key string) bool {
	return rl.allowWithRate(key, rl.rate)
}

// allowWithRate checks if a request should be allowed
func (rl *FiberRateLimiter) allowWithRate(key string, rate int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	client, exists := rl.tokens[key]
	if !exists {
		rl.tokens[key] = &clientTokens{
			tokens:    []time.Time{now},
			lastCheck: now,
		}
		return true
	}

	// Remove expired tokens
	var valid []time.Time
	for _, t := range client.tokens {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}

	// Check if under rate limit
	if len(valid) >= rate {
		client.tokens = valid
		return false
	}

	// Add new token
	client.tokens = append(valid, now)
	client.lastCheck = now

	return true
}

func (rl *FiberRateLimiter) getEndpointConfig(endpoint string) EndpointConfig {
	for _, config := range defaultEndpointConfigs {
		if len(endpoint) >= len(config.Path) && endpoint[:len(config.Path)] == config.Path {
			return config
		}
	}
	return EndpointConfig{
		Path:   "/api/v1/",
		Rate:   rl.rate,
		Window: rl.window,
		Burst:  rl.burst,
	}
}

func (rl *FiberRateLimiter) getClientKey(c *fiber.Ctx) string {
	// Check for forwarded headers
	if forwarded := c.Get("X-Forwarded-For"); forwarded != "" {
		// Take first IP
		if idx := strings.Index(forwarded, ","); idx > 0 {
			return strings.TrimSpace(forwarded[:idx])
		}
		return forwarded
	}

	if cfRay := c.Get("CF-Connecting-IP"); cfRay != "" {
		return cfRay
	}

	// Fall back to IP
	return c.IP()
}

func (rl *FiberRateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			windowStart := now.Add(-rl.window)

			for key, client := range rl.tokens {
				// Remove if no valid tokens and last check was > 1 hour ago
				if now.Sub(client.lastCheck) > time.Hour {
					delete(rl.tokens, key)
					continue
				}

				var valid []time.Time
				for _, t := range client.tokens {
					if t.After(windowStart) {
						valid = append(valid, t)
					}
				}
				client.tokens = valid
			}
			rl.mu.Unlock()

		case <-rl.stopCleanup:
			return
		}
	}
}

// Stop stops the cleanup goroutine
func (rl *FiberRateLimiter) Stop() {
	if rl.stopCleanup != nil {
		close(rl.stopCleanup)
	}
}

// GetRateLimitInfo returns current rate limit status
func (rl *FiberRateLimiter) GetRateLimitInfo(c *fiber.Ctx) map[string]interface{} {
	key := rl.getClientKey(c)

	rl.mu.RLock()
	defer rl.mu.RUnlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	client, exists := rl.tokens[key]
	if !exists {
		return map[string]interface{}{
			"remaining": rl.rate,
			"reset":     now.Add(rl.window).Unix(),
			"limit":     rl.rate,
		}
	}

	var valid int
	for _, t := range client.tokens {
		if t.After(windowStart) {
			valid++
		}
	}

	return map[string]interface{}{
		"remaining": rl.rate - valid,
		"reset":     now.Add(rl.window).Unix(),
		"limit":     rl.rate,
		"used":      valid,
	}
}

// HeadersMiddleware adds rate limit headers to response
func (rl *FiberRateLimiter) HeadersMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()
		
		info := rl.GetRateLimitInfo(c)
		c.Set("X-RateLimit-Limit", fmt.Sprintf("%v", info["limit"]))
		c.Set("X-RateLimit-Remaining", fmt.Sprintf("%v", info["remaining"]))
		c.Set("X-RateLimit-Reset", fmt.Sprintf("%v", info["reset"]))
		
		return err
	}
}
