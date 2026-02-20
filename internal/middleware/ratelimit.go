package middleware

import (
	"net/http"
	"sync"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/utils"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	mu              sync.Mutex
	tokens          map[string]*tokenBucket
	config          *config.RateLimitConfig
	cleanupInterval time.Duration
	stopChan        chan bool
}

type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(cfg *config.RateLimitConfig) *RateLimiter {
	if !cfg.Enabled {
		return &RateLimiter{config: cfg}
	}

	rl := &RateLimiter{
		tokens:          make(map[string]*tokenBucket),
		config:          cfg,
		cleanupInterval: cfg.CleanupInterval,
		stopChan:        make(chan bool),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// cleanup removes old entries periodically
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, bucket := range rl.tokens {
				// Remove if not used in 10 minutes
				if now.Sub(bucket.lastRefill) > 10*time.Minute {
					delete(rl.tokens, key)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopChan:
			return
		}
	}
}

// Stop stops the rate limiter cleanup
func (rl *RateLimiter) Stop() {
	if rl == nil || !rl.config.Enabled {
		return
	}
	rl.stopChan <- true
}

// getBucket gets or creates a token bucket for the key
func (rl *RateLimiter) getBucket(key string) *tokenBucket {
	if bucket, exists := rl.tokens[key]; exists {
		return bucket
	}

	bucket := &tokenBucket{
		tokens:     float64(rl.config.Burst),
		maxTokens:  float64(rl.config.Burst),
		refillRate: float64(rl.config.RequestsPer) / rl.config.Window.Seconds(),
		lastRefill: time.Now(),
	}
	rl.tokens[key] = bucket
	return bucket
}

// refill adds tokens based on time elapsed
func (rl *RateLimiter) refill(bucket *tokenBucket) {
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	bucket.tokens = min(bucket.maxTokens, bucket.tokens+(elapsed*bucket.refillRate))
	bucket.lastRefill = now
}

// Allow checks if request is allowed and consumes a token
func (rl *RateLimiter) Allow(key string) bool {
	if rl == nil || !rl.config.Enabled {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket := rl.getBucket(key)
	rl.refill(bucket)

	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}

	return false
}

// RemainingTokens returns remaining tokens for the key
func (rl *RateLimiter) RemainingTokens(key string) int {
	if rl == nil || !rl.config.Enabled {
		return rl.config.Burst
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket := rl.getBucket(key)
	rl.refill(bucket)

	return int(bucket.tokens)
}

// RateLimitMiddleware returns the rate limiting middleware
func RateLimitMiddleware(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if rate limiting disabled
		if rl == nil || !rl.config.Enabled {
			c.Next()
			return
		}

		// Get key - use user ID if authenticated, otherwise IP
		key := c.ClientIP()
		if userID := c.GetString("user_id"); userID != "" {
			key = "user:" + userID
		}

		// Check rate limit
		if !rl.Allow(key) {
			retryAfter := time.Duration(1) * time.Minute // simplified
			utils.RespondWithRateLimited(c, retryAfter)
			c.Abort()
			return
		}

		// Set rate limit headers
		remaining := rl.RemainingTokens(key)
		c.Header("X-RateLimit-Limit", string(rune(rl.config.Burst)))
		c.Header("X-RateLimit-Remaining", string(rune(remaining)))

		c.Next()
	}
}
