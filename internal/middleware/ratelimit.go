package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type RateLimiter struct {
	mu       sync.Mutex
	tokens   map[string][]time.Time
	rate     int
	window   time.Duration
	burst    int
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		tokens: make(map[string][]time.Time),
		rate:   100,
		window: time.Minute,
		burst:  20,
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	var valid []time.Time
	for _, t := range rl.tokens[key] {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.rate {
		rl.tokens[key] = valid
		return false
	}

	rl.tokens[key] = append(valid, now)
	return true
}

func (rl *RateLimiter) ServeHTTP(c *gin.Context) {
	key := c.ClientIP()
	if c.GetHeader("X-Forwarded-For") != "" {
		key = c.GetHeader("X-Forwarded-For")
	}

	if !rl.Allow(key) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": "Rate limit exceeded",
		})
		return
	}

	c.Next()
}

func (rl *RateLimiter) Stop() {}
