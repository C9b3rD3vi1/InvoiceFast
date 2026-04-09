package middleware

import (
	"sync"
	"time"

	"invoicefast/internal/config"

	"github.com/gofiber/fiber/v2"
)

type FiberRateLimiter struct {
	mu       sync.RWMutex
	requests map[string][]time.Time
	limits   map[string]int
	window   time.Duration
	cleanup  *time.Ticker
	cfg      *config.RateLimitConfig
}

// Default plan limits
var defaultPlanLimits = map[string]int{
	"free":       100,   // 100 requests/min for free tier
	"pro":        500,   // 500 requests/min for pro
	"agency":     2000,  // 2000 requests/min for agency
	"enterprise": 10000, // unlimited-ish for enterprise
}

var defaultBurstLimits = map[string]int{
	"free":       20,
	"pro":        100,
	"agency":     500,
	"enterprise": 2000,
}

func NewFiberRateLimiterWithConfig(cfg *config.RateLimitConfig) *FiberRateLimiter {
	// Apply config overrides to defaults
	planLimits := make(map[string]int)
	planBurstLimits := make(map[string]int)

	for k, v := range defaultPlanLimits {
		planLimits[k] = v
	}
	for k, v := range defaultBurstLimits {
		planBurstLimits[k] = v
	}

	// Override with config values if provided
	if cfg.FreeLimit > 0 {
		planLimits["free"] = cfg.FreeLimit
	}
	if cfg.ProLimit > 0 {
		planLimits["pro"] = cfg.ProLimit
	}
	if cfg.AgencyLimit > 0 {
		planLimits["agency"] = cfg.AgencyLimit
	}
	if cfg.EnterpriseLimit > 0 {
		planLimits["enterprise"] = cfg.EnterpriseLimit
	}

	rl := &FiberRateLimiter{
		requests: make(map[string][]time.Time),
		limits:   make(map[string]int),
		window:   time.Minute,
		cfg:      cfg,
	}

	// Store limits for use in Limit method
	for k, v := range planLimits {
		rl.limits["plan:"+k] = v
	}

	go rl.cleanupLoop()
	return rl
}

// NewFiberRateLimiter creates default rate limiter (for backward compatibility)
func NewFiberRateLimiter() *FiberRateLimiter {
	return NewFiberRateLimiterWithConfig(&config.RateLimitConfig{
		Enabled:     true,
		RequestsPer: 100,
		Window:      time.Minute,
		Burst:       20,
	})
}

func (rl *FiberRateLimiter) cleanupLoop() {
	rl.cleanup = time.NewTicker(5 * time.Minute)
	defer rl.cleanup.Stop()

	for range rl.cleanup.C {
		rl.mu.Lock()
		now := time.Now()
		for key, times := range rl.requests {
			var valid []time.Time
			for _, t := range times {
				if now.Sub(t) < rl.window {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = valid
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *FiberRateLimiter) getLimitForKey(key string) int {
	// Check if custom limit set (e.g., from plan)
	if limit, ok := rl.limits[key]; ok {
		return limit
	}
	// Default to free tier
	return defaultPlanLimits["free"]
}

func (rl *FiberRateLimiter) Limit(requests int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Build key based on user context
		key := c.IP()

		// Use user_id if authenticated for more granular limiting
		if userID := c.Locals("user_id"); userID != nil {
			if id, ok := userID.(string); ok {
				key = "user:" + id
			}
		}

		// Check for tenant-level rate limiting (use plan from JWT claims)
		if plan := c.Locals("plan"); plan != nil {
			if planStr, ok := plan.(string); ok {
				if limit, exists := defaultPlanLimits[planStr]; exists {
					requests = limit // Override with plan limit
				}
			}
		}

		rl.mu.Lock()
		limit := requests
		if l, ok := rl.limits[key]; ok {
			limit = l
		}

		now := time.Now()
		valid := now.Add(-window)

		var validReqs []time.Time
		for _, t := range rl.requests[key] {
			if t.After(valid) {
				validReqs = append(validReqs, t)
			}
		}

		if len(validReqs) >= limit {
			rl.mu.Unlock()
			// Set standard rate limit headers
			c.Set("X-RateLimit-Limit", string(rune(limit)))
			c.Set("X-RateLimit-Remaining", "0")
			c.Set("X-RateLimit-Reset", string(rune(time.Now().Add(window).Unix())))

			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       "rate limit exceeded",
				"code":        "RATE_LIMITED",
				"retry_after": window.Seconds(),
			})
		}

		rl.requests[key] = append(validReqs, now)
		rl.mu.Unlock()

		// Set rate limit headers for client
		c.Set("X-RateLimit-Limit", string(rune(limit)))
		c.Set("X-RateLimit-Remained", string(rune(limit-len(validReqs)-1)))

		return c.Next()
	}
}

// SetCustomLimit allows setting a custom rate limit for a specific key (e.g., per plan)
func (rl *FiberRateLimiter) SetCustomLimit(key string, limit int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limits[key] = limit
}

func (rl *FiberRateLimiter) Stop() {
	if rl.cleanup != nil {
		rl.cleanup.Stop()
	}
}
