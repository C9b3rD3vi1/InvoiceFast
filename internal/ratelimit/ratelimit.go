package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// RateLimitConfig holds rate limiting configuration for a tenant
type RateLimitConfig struct {
	RequestsPerMinute int           `json:"requests_per_minute"`
	Burst            int           `json:"burst"`
	BlockDuration   time.Duration `json:"block_duration"`
}

// PlanLimits defines rate limits per subscription plan
var PlanLimits = map[string]RateLimitConfig{
	"free": {
		RequestsPerMinute: 60,
		Burst:            10,
		BlockDuration:   5 * time.Minute,
	},
	"pro": {
		RequestsPerMinute: 300,
		Burst:            50,
		BlockDuration:   2 * time.Minute,
	},
	"agency": {
		RequestsPerMinute: 1000,
		Burst:            100,
		BlockDuration:   1 * time.Minute,
	},
	"enterprise": {
		RequestsPerMinute: 5000,
		Burst:            500,
		BlockDuration:   30 * time.Second,
	},
}

// Limiter represents a rate limiter implementation
type Limiter interface {
	Allow(ctx context.Context, key string) (allowed bool, resetAfter time.Duration)
}

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	mu           sync.RWMutex
	tokens       float64
	maxTokens    float64
	refillRate   float64 // tokens per second
	lastRefill   time.Time
	blockUntil   time.Time
	config       RateLimitConfig
}

// NewTokenBucket creates a new token bucket limiter
func NewTokenBucket(config RateLimitConfig) *TokenBucket {
	if config.RequestsPerMinute == 0 {
		config.RequestsPerMinute = 60 // default
	}
	if config.Burst == 0 {
		config.Burst = config.RequestsPerMinute / 6
	}
	if config.BlockDuration == 0 {
		config.BlockDuration = 5 * time.Minute
	}

	return &TokenBucket{
		tokens:     float64(config.Burst),
		maxTokens:  float64(config.Burst),
		refillRate: float64(config.RequestsPerMinute) / 60.0,
		lastRefill: time.Now(),
		config:     config,
	}
}

// Allow checks if a request is allowed
func (tb *TokenBucket) Allow(ctx context.Context, key string) (bool, time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// Check if blocked
	if now.Before(tb.blockUntil) {
		return false, time.Until(tb.blockUntil)
	}

	// Refill tokens
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = min(tb.maxTokens, tb.tokens+(elapsed*tb.refillRate))
	tb.lastRefill = now

	// Check if we have tokens
	if tb.tokens >= 1 {
		tb.tokens--
		// Calculate when tokens will be available again
		resetAfter := time.Duration((1.0/tb.refillRate)*1000) * time.Millisecond
		return true, resetAfter
	}

	// No tokens available, calculate wait time
	waitTime := time.Duration((1.0-tb.tokens)/tb.refillRate) * time.Second
	return false, waitTime
}

// Block blocks the limiter for the configured duration
func (tb *TokenBucket) Block(duration time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.blockUntil = time.Now().Add(duration)
	tb.tokens = 0
}

// Reset resets the limiter
func (tb *TokenBucket) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.blockUntil = time.Time{}
	tb.tokens = tb.maxTokens
	tb.lastRefill = time.Now()
}

// GetConfig returns the current configuration
func (tb *TokenBucket) GetConfig() RateLimitConfig {
	return tb.config
}

// ============================================================
// Tenant Rate Limiter Registry
// ============================================================

// Registry manages rate limiters per tenant
type Registry struct {
	mu         sync.RWMutex
	limiters   map[string]*TokenBucket
	defaultCfg RateLimitConfig
	redis      *redis.Client
	config     *RegistryConfig
}

// RegistryConfig holds registry configuration
type RegistryConfig struct {
	DefaultPlan string `json:"default_plan"`
	RedisKeyTTL time.Duration
}

// DefaultRegistryConfig is the default configuration
var DefaultRegistryConfig = &RegistryConfig{
	DefaultPlan: "free",
	RedisKeyTTL: time.Hour,
}

var (
	defaultRegistry *Registry
	registryOnce   sync.Once
)

// Default returns the default registry
func Default() *Registry {
	registryOnce.Do(func() {
		defaultRegistry = NewRegistry(nil, nil)
	})
	return defaultRegistry
}

// NewRegistry creates a new rate limiter registry
func NewRegistry(redisClient *redis.Client, config *RegistryConfig) *Registry {
	if config == nil {
		config = DefaultRegistryConfig
	}

	cfg := PlanLimits[config.DefaultPlan]
	if cfg.RequestsPerMinute == 0 {
		cfg = PlanLimits["free"]
	}

	return &Registry{
		limiters:   make(map[string]*TokenBucket),
		defaultCfg: cfg,
		redis:      redisClient,
		config:     config,
	}
}

// GetLimiter gets or creates a rate limiter for a tenant
func (r *Registry) GetLimiter(tenantID, plan string) *TokenBucket {
	r.mu.RLock()
	if limiter, exists := r.limiters[tenantID]; exists {
		r.mu.RUnlock()
		return limiter
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := r.limiters[tenantID]; exists {
		return limiter
	}

	// Get plan limits or use default
	cfg, exists := PlanLimits[plan]
	if !exists {
		cfg = PlanLimits["free"]
	}

	limiter := NewTokenBucket(cfg)
	r.limiters[tenantID] = limiter

	return limiter
}

// RemoveLimiter removes a tenant's rate limiter
func (r *Registry) RemoveLimiter(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.limiters, tenantID)
}

// UpdatePlan updates rate limits for a tenant when plan changes
func (r *Registry) UpdatePlan(tenantID, plan string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cfg, exists := PlanLimits[plan]
	if !exists {
		cfg = PlanLimits["free"]
	}

	if limiter, exists := r.limiters[tenantID]; exists {
		// Update existing limiter config
		limiter.tokens = float64(cfg.Burst)
		limiter.maxTokens = float64(cfg.Burst)
		limiter.refillRate = float64(cfg.RequestsPerMinute) / 60.0
		limiter.config = cfg
	}
}

// CheckTenantLimit checks if a request is allowed for a tenant
func (r *Registry) CheckTenantLimit(ctx context.Context, tenantID, plan string) (allowed bool, resetAfter time.Duration, err error) {
	limiter := r.GetLimiter(tenantID, plan)
	allowed, resetAfter = limiter.Allow(ctx, tenantID)

	if !allowed {
		err = fmt.Errorf("rate limit exceeded")
	}

	return allowed, resetAfter, err
}

// BlockTenant blocks a tenant's rate limiter
func (r *Registry) BlockTenant(tenantID, plan string, duration time.Duration) {
	limiter := r.GetLimiter(tenantID, plan)
	limiter.Block(duration)
}

// ResetTenant resets a tenant's rate limiter
func (r *Registry) ResetTenant(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limiter, exists := r.limiters[tenantID]; exists {
		limiter.Reset()
	}
}

// GetTenantStatus returns rate limit status for a tenant
func (r *Registry) GetTenantStatus(tenantID, plan string) map[string]interface{} {
	limiter := r.GetLimiter(tenantID, plan)
	cfg := limiter.GetConfig()

	return map[string]interface{}{
		"tenant_id":             tenantID,
		"plan":                  plan,
		"requests_per_minute":   cfg.RequestsPerMinute,
		"burst":                 cfg.Burst,
		"block_duration":        cfg.BlockDuration,
	}
}

// GetAllStatus returns status for all tenants
func (r *Registry) GetAllStatus() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := make([]map[string]interface{}, 0, len(r.limiters))
	for tenantID, limiter := range r.limiters {
		cfg := limiter.GetConfig()
		status = append(status, map[string]interface{}{
			"tenant_id":           tenantID,
			"requests_per_minute": cfg.RequestsPerMinute,
			"burst":               cfg.Burst,
		})
	}
	return status
}

// ============================================================
// Redis-backed Rate Limiter (for distributed systems)
// ============================================================

// RedisLimiter implements rate limiting using Redis
type RedisLimiter struct {
	redis     *redis.Client
	keyPrefix string
	config    RateLimitConfig
}

// NewRedisLimiter creates a Redis-backed rate limiter
func NewRedisLimiter(redisClient *redis.Client, keyPrefix string, config RateLimitConfig) *RedisLimiter {
	return &RedisLimiter{
		redis:     redisClient,
		keyPrefix: keyPrefix,
		config:    config,
	}
}

// Allow checks if a request is allowed using Redis atomic operations
func (r *RedisLimiter) Allow(ctx context.Context, key string) (bool, time.Duration) {
	fullKey := fmt.Sprintf("%s:%s", r.keyPrefix, key)

	// Increment counter
	count, err := r.redis.Incr(ctx, fullKey).Result()
	if err != nil {
		// On Redis error, allow request but log
		return true, 0
	}

	// Set expiry on first request
	if count == 1 {
		r.redis.Expire(ctx, fullKey, time.Minute)
	}

	// Check limit
	if count > int64(r.config.RequestsPerMinute) {
		// Get TTL for retry-after header
		ttl, _ := r.redis.TTL(ctx, fullKey).Result()
		return false, ttl
	}

	return true, 0
}

// Block blocks the key for the configured duration
func (r *RedisLimiter) Block(ctx context.Context, key string, duration time.Duration) error {
	fullKey := fmt.Sprintf("%s:block:%s", r.keyPrefix, key)
	return r.redis.Set(ctx, fullKey, "1", duration).Err()
}

// IsBlocked checks if a key is blocked
func (r *RedisLimiter) IsBlocked(ctx context.Context, key string) bool {
	fullKey := fmt.Sprintf("%s:block:%s", r.keyPrefix, key)
	result, err := r.redis.Exists(ctx, fullKey).Result()
	return err == nil && result > 0
}

// Reset resets the rate limit for a key
func (r *RedisLimiter) Reset(ctx context.Context, key string) error {
	fullKey := fmt.Sprintf("%s:%s", r.keyPrefix, key)
	return r.redis.Del(ctx, fullKey).Err()
}

// ============================================================
// Fiber Middleware for Per-Tenant Rate Limiting
// ============================================================

// Middleware returns a Fiber middleware for per-tenant rate limiting
func Middleware(registry *Registry) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get tenant ID from context (set by auth middleware)
		tenantID := c.Locals("tenant_id")
		if tenantID == nil {
			tenantID = c.IP() // Fallback to IP for unauthenticated requests
		}

		// Get plan from context (set by subscription middleware)
		plan := c.Locals("plan")
		if plan == nil {
			plan = "free"
		}

		tenantStr, _ := tenantID.(string)
		planStr, _ := plan.(string)

		// Check rate limit
		_, resetAfter, err := registry.CheckTenantLimit(c.Context(), tenantStr, planStr)
		if err != nil {
			// Add rate limit headers
			c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", registry.GetPlanLimit(planStr)))
			c.Set("X-RateLimit-Remaining", "0")
			if resetAfter > 0 {
				c.Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(resetAfter).Unix()))
				c.Set("Retry-After", fmt.Sprintf("%d", int64(resetAfter.Seconds())))
			}

			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       "Rate limit exceeded",
				"code":        "RATE_LIMIT_EXCEEDED",
				"retry_after": resetAfter.Seconds(),
			})
		}

		// Add rate limit headers even when allowed
		c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", registry.GetPlanLimit(planStr)))
		c.Set("X-RateLimit-Remaining", "0") // Simplified
		c.Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Minute).Unix()))

		return c.Next()
	}
}

// GetPlanLimit returns the rate limit for a plan
func (r *Registry) GetPlanLimit(plan string) int {
	cfg, exists := PlanLimits[plan]
	if !exists {
		return PlanLimits["free"].RequestsPerMinute
	}
	return cfg.RequestsPerMinute
}

// ============================================================
// Helper Functions
// ============================================================

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// GetRateLimitConfigForPlan returns the rate limit config for a plan
func GetRateLimitConfigForPlan(plan string) RateLimitConfig {
	cfg, exists := PlanLimits[plan]
	if !exists {
		return PlanLimits["free"]
	}
	return cfg
}

// CreateTenantLimiter creates a rate limiter with configured limits
func CreateTenantLimiter(requestsPerMinute, burst int, blockDuration time.Duration) *TokenBucket {
	return NewTokenBucket(RateLimitConfig{
		RequestsPerMinute: requestsPerMinute,
		Burst:            burst,
		BlockDuration:   blockDuration,
	})
}