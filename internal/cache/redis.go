package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache provides caching with Redis
type RedisCache struct {
	client *redis.Client
	prefix string
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	URL         string
	Password    string
	DB          int
	Prefix      string
	MaxRetries  int
	PoolSize    int
	IdleTimeout time.Duration
}

// NewRedisCache creates a new Redis cache client
func NewRedisCache(cfg *CacheConfig) (*RedisCache, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("redis URL is required")
	}

	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	if cfg.Password != "" {
		opts.Password = cfg.Password
	}
	if cfg.DB > 0 {
		opts.DB = cfg.DB
	}
	if cfg.MaxRetries > 0 {
		opts.MaxRetries = cfg.MaxRetries
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "invoicefast"
	}

	log.Printf("[Redis] Connected successfully with prefix: %s", prefix)

	return &RedisCache{
		client: client,
		prefix: prefix,
	}, nil
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// key generates a prefixed cache key
func (c *RedisCache) key(key string) string {
	return fmt.Sprintf("%s:%s", c.prefix, key)
}

// Set stores a value in cache with expiration
func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return c.client.Set(ctx, c.key(key), data, expiration).Err()
}

// Get retrieves a value from cache
func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := c.client.Get(ctx, c.key(key)).Bytes()
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// GetString retrieves a string value from cache
func (c *RedisCache) GetString(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, c.key(key)).Result()
}

// SetString stores a string value in cache
func (c *RedisCache) SetString(ctx context.Context, key, value string, expiration time.Duration) error {
	return c.client.Set(ctx, c.key(key), value, expiration).Err()
}

// Delete removes a key from cache
func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	fullKeys := make([]string, len(keys))
	for i, k := range keys {
		fullKeys[i] = c.key(k)
	}
	return c.client.Del(ctx, fullKeys...).Err()
}

// DeleteByPattern removes keys matching a pattern
func (c *RedisCache) DeleteByPattern(ctx context.Context, pattern string) error {
	fullPattern := c.key(pattern)
	iter := c.client.Scan(ctx, 0, fullPattern, 0).Iterator()

	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return err
	}

	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}

	return nil
}

// Exists checks if a key exists
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	count, err := c.client.Exists(ctx, c.key(key)).Result()
	return count > 0, err
}

// Expire sets expiration for a key
func (c *RedisCache) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return c.client.Expire(ctx, c.key(key), expiration).Err()
}

// TTL gets the remaining time to live for a key
func (c *RedisCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, c.key(key)).Result()
}

// Increment increments a counter
func (c *RedisCache) Increment(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, c.key(key)).Result()
}

// IncrementBy increments a counter by a value
func (c *RedisCache) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	return c.client.IncrBy(ctx, c.key(key), value).Result()
}

// Decrement decrements a counter
func (c *RedisCache) Decrement(ctx context.Context, key string) (int64, error) {
	return c.client.Decr(ctx, c.key(key)).Result()
}

// SetNX sets a value only if the key doesn't exist (useful for locks)
func (c *RedisCache) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	return c.client.SetNX(ctx, c.key(key), data, expiration).Result()
}

// GetOrSet gets a value from cache, or sets it using the provided function
func (c *RedisCache) GetOrSet(ctx context.Context, key string, dest interface{}, expiration time.Duration, fetchFn func() (interface{}, error)) error {
	// Try to get from cache first
	err := c.Get(ctx, key, dest)
	if err == nil {
		return nil // Cache hit
	}

	if err != redis.Nil {
		return err // Error other than not found
	}

	// Cache miss - fetch the value
	value, err := fetchFn()
	if err != nil {
		return err
	}

	// Store in cache
	if err := c.Set(ctx, key, value, expiration); err != nil {
		log.Printf("Warning: failed to cache value for key %s: %v", key, err)
	}

	// Set destination
	data, _ := json.Marshal(value)
	return json.Unmarshal(data, dest)
}

// Hash operations for structured caching

// HSet sets a field in a hash
func (c *RedisCache) HSet(ctx context.Context, key string, field string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.HSet(ctx, c.key(key), field, data).Err()
}

// HGet gets a field from a hash
func (c *RedisCache) HGet(ctx context.Context, key string, field string, dest interface{}) error {
	data, err := c.client.HGet(ctx, c.key(key), field).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// HGetAll gets all fields from a hash
func (c *RedisCache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, c.key(key)).Result()
}

// HDel deletes a field from a hash
func (c *RedisCache) HDel(ctx context.Context, key string, fields ...string) error {
	fullFields := make([]string, len(fields))
	copy(fullFields, fields)
	return c.client.HDel(ctx, c.key(key), fullFields...).Err()
}

// List operations

// LPush pushes to the left of a list
func (c *RedisCache) LPush(ctx context.Context, key string, values ...interface{}) error {
	return c.client.LPush(ctx, c.key(key), values...).Err()
}

// RPush pushes to the right of a list
func (c *RedisCache) RPush(ctx context.Context, key string, values ...interface{}) error {
	return c.client.RPush(ctx, c.key(key), values...).Err()
}

// LRange gets a range of elements from a list
func (c *RedisCache) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.LRange(ctx, c.key(key), start, stop).Result()
}

// LPop pops from the left of a list
func (c *RedisCache) LPop(ctx context.Context, key string) (string, error) {
	return c.client.LPop(ctx, c.key(key)).Result()
}

// RPop pops from the right of a list
func (c *RedisCache) RPop(ctx context.Context, key string) (string, error) {
	return c.client.RPop(ctx, c.key(key)).Result()
}

// Sorted Set operations (for worker queues)

// ZAdd adds a member to sorted set
func (c *RedisCache) ZAdd(ctx context.Context, key string, z *redis.Z) error {
	return c.client.ZAdd(ctx, c.key(key), *z).Err()
}

// ZPopMin removes and returns lowest score members
func (c *RedisCache) ZPopMin(ctx context.Context, key string, count int64) ([]redis.Z, error) {
	return c.client.ZPopMin(ctx, c.key(key), count).Result()
}

// ZCard returns sorted set cardinality
func (c *RedisCache) ZCard(ctx context.Context, key string) (int64, error) {
	return c.client.ZCard(ctx, c.key(key)).Result()
}

// Pub/Sub operations

// Publish publishes a message to a channel
func (c *RedisCache) Publish(ctx context.Context, channel string, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return c.client.Publish(ctx, c.key(channel), data).Err()
}

// Subscribe subscribes to a channel
func (c *RedisCache) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	return c.client.Subscribe(ctx, c.key(channel))
}

// Distributed Lock

// Lock acquires a distributed lock
func (c *RedisCache) Lock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	return c.client.SetNX(ctx, c.key("lock:"+key), "1", expiration).Result()
}

// Unlock releases a distributed lock
func (c *RedisCache) Unlock(ctx context.Context, key string) error {
	return c.client.Del(ctx, c.key("lock:"+key)).Err()
}

// Stats returns cache statistics
func (c *RedisCache) Stats(ctx context.Context) (*CacheStats, error) {
	_, err := c.client.Info(ctx, "stats").Result()
	if err != nil {
		return nil, err
	}

	stats := &CacheStats{}
	// Parse basic stats from info string
	// This is simplified - in production, parse all relevant metrics
	stats.Connected = true

	dbSize, _ := c.client.DBSize(ctx).Result()
	stats.KeyCount = dbSize

	return stats, nil
}

// CacheStats holds cache statistics
type CacheStats struct {
	Connected bool   `json:"connected"`
	KeyCount  int64  `json:"key_count"`
	Hits      int64  `json:"hits"`
	Misses    int64  `json:"misses"`
	Memory    string `json:"memory"`
}

// Cache keys for InvoiceFast entities
const (
	CacheKeyUser         = "user"
	CacheKeyInvoice      = "invoice"
	CacheKeyClient       = "client"
	CacheKeyDashboard    = "dashboard"
	CacheKeyRateLimit    = "ratelimit"
	CacheKeySession      = "session"
	CacheKeyAPIKey       = "apikey"
	CacheKeyInvoiceCount = "invoice:count"
)

// Cache durations
const (
	DurationShort  = 5 * time.Minute
	DurationMedium = 30 * time.Minute
	DurationLong   = 2 * time.Hour
	DurationDay    = 24 * time.Hour
)

// CacheableInvoice wraps invoice data for caching
type CacheableInvoice struct {
	ID            string                 `json:"id"`
	InvoiceNumber string                 `json:"invoice_number"`
	ClientID      string                 `json:"client_id"`
	Total         float64                `json:"total"`
	Status        string                 `json:"status"`
	Data          map[string]interface{} `json:"data"`
}

// CacheableUser wraps user data for caching
type CacheableUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	CompanyName string `json:"company_name"`
	Plan        string `json:"plan"`
}

// NoOpCache is a no-operation cache for when Redis is not available
type NoOpCache struct{}

func NewNoOpCache() *NoOpCache {
	return &NoOpCache{}
}

func (c *NoOpCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return nil
}

func (c *NoOpCache) Get(ctx context.Context, key string, dest interface{}) error {
	return fmt.Errorf("cache miss")
}

func (c *NoOpCache) Delete(ctx context.Context, keys ...string) error {
	return nil
}

func (c *NoOpCache) Close() error {
	return nil
}
