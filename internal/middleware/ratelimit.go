package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a token bucket rate limiter with per-endpoint configuration
type RateLimiter struct {
	mu            sync.RWMutex
	tokens        map[string]*clientTokens
	rate          int
	window        time.Duration
	burst         int
	cleanupInterval time.Duration
	stopCleanup   chan bool
}

// clientTokens tracks tokens for a specific client
type clientTokens struct {
	tokens    []time.Time
	lastCheck time.Time
}

// EndpointConfig allows different rate limits per endpoint
type EndpointConfig struct {
	Path    string
	Rate    int
	Window  time.Duration
	Burst   int
}

// Default endpoint configs
var defaultEndpointConfigs = []EndpointConfig{
	{Path: "/webhook/", Rate: 30, Window: time.Minute, Burst: 10},    // Webhooks - stricter
	{Path: "/auth/", Rate: 10, Window: time.Minute, Burst: 5},          // Auth endpoints
	{Path: "/api/v1/invoices", Rate: 60, Window: time.Minute, Burst: 15}, // Invoice operations
	{Path: "/api/v1/", Rate: 100, Window: time.Minute, Burst: 20},     // Default API
}

// NewRateLimiter creates a new rate limiter with configurable settings
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		tokens:        make(map[string]*clientTokens),
		rate:          100,
		window:        time.Minute,
		burst:         20,
		cleanupInterval: 5 * time.Minute,
		stopCleanup:   make(chan bool),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// NewRateLimiterWithConfig creates a rate limiter with custom settings
func NewRateLimiterWithConfig(rate int, window time.Duration, burst int) *RateLimiter {
	rl := &RateLimiter{
		tokens:        make(map[string]*clientTokens),
		rate:          rate,
		window:        window,
		burst:         burst,
		cleanupInterval: 5 * time.Minute,
		stopCleanup:   make(chan bool),
	}

	go rl.cleanup()

	return rl
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow(key string) bool {
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

	// Check if under rate limit (allow burst)
	maxTokens := rl.rate
	if len(valid) >= maxTokens {
		client.tokens = valid
		return false
	}

	// Add new token
	client.tokens = append(valid, now)
	client.lastCheck = now

	return true
}

// AllowForEndpoint checks rate limit for a specific endpoint
func (rl *RateLimiter) AllowForEndpoint(key, endpoint string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Get config for endpoint
	config := rl.getEndpointConfig(endpoint)
	rate := config.Rate

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

	// Check rate limit
	if len(valid) >= rate {
		client.tokens = valid
		return false
	}

	client.tokens = append(valid, now)
	client.lastCheck = now

	return true
}

func (rl *RateLimiter) getEndpointConfig(endpoint string) EndpointConfig {
	for _, config := range defaultEndpointConfigs {
		if len(endpoint) >= len(config.Path) && endpoint[:len(config.Path)] == config.Path {
			return config
		}
	}
	// Default config
	return EndpointConfig{
		Path:   "/api/v1/",
		Rate:   rl.rate,
		Window: rl.window,
		Burst:  rl.burst,
	}
}

// ServeHTTP applies rate limiting to the request
func (rl *RateLimiter) ServeHTTP(c *gin.Context) {
	key := rl.getClientKey(c)

	if !rl.Allow(key) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error":       "Rate limit exceeded",
			"retry_after": rl.window.Seconds(),
		})
		return
	}

	c.Next()
}

// WebhookRateLimiter is a stricter rate limiter for webhooks
func (rl *RateLimiter) WebhookRateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := rl.getClientKey(c)
		endpoint := c.FullPath()

		if !rl.AllowForEndpoint(key, endpoint) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "Webhook rate limit exceeded",
				"retry_after": "60 seconds",
			})
			return
		}

		c.Next()
	}
}

// AuthRateLimiter is stricter for auth endpoints (login/register)
func (rl *RateLimiter) AuthRateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := rl.getClientKey(c)
		
		rl.mu.Lock()
		now := time.Now()
		windowStart := now.Add(-rl.window)
		
		client, exists := rl.tokens[key]
		authRate := 10 // Stricter for auth
		
		if exists {
			var valid []time.Time
			for _, t := range client.tokens {
				if t.After(windowStart) {
					valid = append(valid, t)
				}
			}
			
			if len(valid) >= authRate {
				rl.mu.Unlock()
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error":       "Too many authentication attempts",
					"retry_after": "60 seconds",
				})
				return
			}
			
			client.tokens = append(valid, now)
		} else {
			rl.tokens[key] = &clientTokens{
				tokens:    []time.Time{now},
				lastCheck: now,
			}
		}
		rl.mu.Unlock()
		
		c.Next()
	}
}

// getClientKey extracts a unique key for rate limiting
func (rl *RateLimiter) getClientKey(c *gin.Context) string {
	// Check for forwarded headers (reverse proxy)
	if forwarded := c.GetHeader("X-Forwarded-For"); forwarded != "" {
		// Take first IP (original client)
		for i, c := range forwarded {
			if c == ',' {
				return forwarded[:i]
			}
		}
		return forwarded
	}

	if cfRay := c.GetHeader("CF-Connecting-IP"); cfRay != "" {
		return cfRay
	}

	// Fall back to remote addr
	return c.ClientIP()
}

// cleanup removes expired entries periodically
func (rl *RateLimiter) cleanup() {
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

				// Clean up old tokens
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

// Stop stops the rate limiter cleanup goroutine
func (rl *RateLimiter) Stop() {
	if rl.stopCleanup != nil {
		close(rl.stopCleanup)
	}
}

// GetRateLimitInfo returns current rate limit status for debugging
func (rl *RateLimiter) GetRateLimitInfo(c *gin.Context) map[string]interface{} {
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

// RateLimitHeaders adds standard rate limit headers to response
func RateLimitHeaders(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Add headers to response
		info := rl.GetRateLimitInfo(c)
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%v", info["limit"]))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%v", info["remaining"]))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%v", info["reset"]))
	}
}
