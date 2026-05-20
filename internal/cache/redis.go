package cache

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

const (
	DurationShort  = 5 * time.Minute
	DurationMedium = 30 * time.Minute
	DurationLong   = 24 * time.Hour
)

var (
	ErrCacheMiss        = errors.New("cache miss")
	ErrLockNotAcquired  = errors.New("lock not acquired")
	ErrLockNotOwned     = errors.New("lock not owned")
	ErrInvalidTenant    = errors.New("invalid tenant")
	ErrInvalidNamespace = errors.New("invalid namespace")
	ErrInvalidKey       = errors.New("invalid cache key")
)

type CacheConfig struct {
	RedisAddr            string
	RedisPassword        string
	RedisDB              int
	Prefix          string
	DefaultTTL      time.Duration
	MaxRetries      int
	PoolSize        int
	MinIdleConns    int
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	EnableTLS       bool
	Environment     string
	CompressionSize int
}

type RedisCache struct {
	client redis.UniversalClient
	config CacheConfig
	log    zerolog.Logger
	sf     singleflight.Group
}

type CacheItem[T any] struct {
	Data      T         `json:"data"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type KeyBuilder struct {
	Environment string
	TenantID    string
	Module      string
	Resource    string
	ID          string
	Version     string
}

type DistributedLock struct {
	cache      *RedisCache
	key        string
	token      string
	expiration time.Duration
}

type HealthStatus struct {
	Status  string        `json:"status"`
	Latency time.Duration `json:"latency"`
}

type RateLimiter struct {
	cache *RedisCache
}

// ============================================================
// INITIALIZATION
// ============================================================

func NewRedisCache(cfg CacheConfig) (*RedisCache, error) {
	options := &redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		MaxRetries:   cfg.MaxRetries,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	if cfg.EnableTLS {
		options.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	rdb := redis.NewClient(options)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = DurationMedium
	}

	cache := &RedisCache{
		client: rdb,
		config: cfg,
		log: log.With().
			Str("service", "redis-cache").
			Str("environment", cfg.Environment).
			Logger(),
	}

	cache.log.Info().
		Str("addr", cfg.RedisAddr).
		Msg("redis cache initialized")

	return cache, nil
}

func (c *RedisCache) Client() redis.UniversalClient {
	return c.client
}

// ============================================================
// KEY HELPERS
// ============================================================

func (c *RedisCache) buildKey(key string) string {
	if c.config.Prefix == "" {
		return key
	}

	return fmt.Sprintf("%s:%s", c.config.Prefix, key)
}

func validateKey(key string) error {
	if key == "" {
		return ErrInvalidKey
	}

	if strings.Contains(key, "..") {
		return ErrInvalidKey
	}

	if len(key) > 512 {
		return ErrInvalidKey
	}

	return nil
}

func (k KeyBuilder) Build() (string, error) {
	if k.TenantID == "" {
		return "", ErrInvalidTenant
	}

	if strings.Contains(k.TenantID, ":") {
		return "", ErrInvalidTenant
	}

	if k.Module == "" {
		return "", ErrInvalidNamespace
	}

	if k.Version == "" {
		k.Version = "v1"
	}

	return fmt.Sprintf(
		"%s:%s:%s:%s:%s:%s",
		k.Environment,
		k.TenantID,
		k.Module,
		k.Resource,
		k.ID,
		k.Version,
	), nil
}

// ============================================================
// CORE CACHE OPERATIONS
// ============================================================

func (c *RedisCache) Set(
	ctx context.Context,
	key string,
	value interface{},
	ttl time.Duration,
) error {

	if err := validateKey(key); err != nil {
		return err
	}

	if ttl <= 0 {
		ttl = c.config.DefaultTTL
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}

	key = c.buildKey(key)

	if err := c.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}

	return nil
}

func (c *RedisCache) Get(
	ctx context.Context,
	key string,
	dest interface{},
) error {

	if err := validateKey(key); err != nil {
		return err
	}

	key = c.buildKey(key)

	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}

		return fmt.Errorf("redis get failed: %w", err)
	}

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		return fmt.Errorf("unmarshal failed: %w", err)
	}

	return nil
}

func (c *RedisCache) SetString(
	ctx context.Context,
	key string,
	value string,
	ttl time.Duration,
) error {

	if err := validateKey(key); err != nil {
		return err
	}

	if ttl <= 0 {
		ttl = c.config.DefaultTTL
	}

	key = c.buildKey(key)

	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) GetString(
	ctx context.Context,
	key string,
) (string, error) {

	if err := validateKey(key); err != nil {
		return "", err
	}

	key = c.buildKey(key)

	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrCacheMiss
		}

		return "", fmt.Errorf("redis get failed: %w", err)
	}

	return val, nil
}

func (c *RedisCache) Delete(
	ctx context.Context,
	keys ...string,
) error {

	if len(keys) == 0 {
		return nil
	}

	prefixed := make([]string, 0, len(keys))

	for _, key := range keys {
		if err := validateKey(key); err != nil {
			return err
		}

		prefixed = append(prefixed, c.buildKey(key))
	}

	return c.client.Del(ctx, prefixed...).Err()
}

func (c *RedisCache) DeleteByPattern(
	ctx context.Context,
	pattern string,
) error {

	if pattern == "" {
		return nil
	}

	pattern = c.buildKey(pattern)

	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()

	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			c.log.Error().
				Err(err).
				Str("key", iter.Val()).
				Msg("cache invalidation failed")
		}
	}

	return iter.Err()
}

func (c *RedisCache) Exists(ctx context.Context,key string) (bool, error) {

	if err := validateKey(key); err != nil {
		return false, err
	}

	key = c.buildKey(key)

	count, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (c *RedisCache) Unlock(ctx context.Context,key string,token string) error {

	lockKey := c.buildKey("lock:" + key)

	script := `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end
`

	result, err := c.client.Eval(ctx,script,[]string{lockKey}, token).Int()

	if err != nil {
		return err
	}

	if result == 0 {
		return ErrLockNotOwned
	}

	return nil
}

func (c *RedisCache) Lock(ctx context.Context,key string,expiration time.Duration) (*DistributedLock, error) {

	return c.AcquireLock(ctx, key, expiration)
}

func (c *RedisCache) Increment(ctx context.Context,key string) (int64, error) {

	if err := validateKey(key); err != nil {
		return 0, err
	}

	key = c.buildKey(key)

	return c.client.Incr(ctx, key).Result()
}


func (c *RedisCache) Expire(ctx context.Context,key string,ttl time.Duration,) error {

	if err := validateKey(key); err != nil {
		return err
	}

	key = c.buildKey(key)

	return c.client.Expire(ctx, key, ttl).Err()
}


// ZAdd adds members to a sorted set
func (c *RedisCache) ZAdd(ctx context.Context,key string,members ...redis.Z) error {

	if err := validateKey(key); err != nil {
		return err
	}

	if len(members) == 0 {
		return nil
	}

	key = c.buildKey(key)

	return c.client.ZAdd(ctx, key, members...).Err()
}


// ZRange returns sorted set members
func (c *RedisCache) ZRange(ctx context.Context,key string,start int64,stop int64) ([]string, error) {

	if err := validateKey(key); err != nil {
		return nil, err
	}

	key = c.buildKey(key)

	return c.client.ZRange(ctx, key, start, stop).Result()
}

// ZRevRange returns sorted set members in reverse order
func (c *RedisCache) ZRevRange(
	ctx context.Context,
	key string,
	start int64,
	stop int64,
) ([]string, error) {

	if err := validateKey(key); err != nil {
		return nil, err
	}

	key = c.buildKey(key)

	return c.client.ZRevRange(ctx, key, start, stop).Result()
}

// ZRem removes sorted set members
func (c *RedisCache) ZRem(
	ctx context.Context,
	key string,
	members ...interface{},
) error {

	if err := validateKey(key); err != nil {
		return err
	}

	if len(members) == 0 {
		return nil
	}

	key = c.buildKey(key)

	return c.client.ZRem(ctx, key, members...).Err()
}

// ZCard gets sorted set count
func (c *RedisCache) ZCard(ctx context.Context,key string) (int64, error) {

	if err := validateKey(key); err != nil {
		return 0, err
	}

	key = c.buildKey(key)

	return c.client.ZCard(ctx, key).Result()
}

func (c *RedisCache) ZPopMin(ctx context.Context,key string, count int64) ([]redis.Z, error) {

	if err := validateKey(key); err != nil {
		return nil, err
	}

	key = c.buildKey(key)

	return c.client.ZPopMin(ctx, key, count).Result()
}


func (c *RedisCache) ZPopMax(ctx context.Context,key string,count int64) ([]redis.Z, error) {

	if err := validateKey(key); err != nil {
		return nil, err
	}

	key = c.buildKey(key)

	return c.client.ZPopMax(ctx, key, count).Result()
}

func (c *RedisCache) ZScore(ctx context.Context,key string,member string) (float64, error) {

	if err := validateKey(key); err != nil {
		return 0, err
	}

	key = c.buildKey(key)

	return c.client.ZScore(ctx, key, member).Result()
}

// ============================================================
// GENERIC TYPED CACHE HELPERS
// ============================================================

func SetTyped[T any](ctx context.Context,cache *RedisCache,key string,value T,ttl time.Duration) error {

	item := CacheItem[T]{
		Data:      value,
		Version:   1,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(ttl),
	}

	return cache.Set(ctx, key, item, ttl)
}

func GetTyped[T any](ctx context.Context,cache *RedisCache,key string) (*T, error) {

	var item CacheItem[T]

	if err := cache.Get(ctx, key, &item); err != nil {
		return nil, err
	}

	return &item.Data, nil
}

// ============================================================
// CACHE STAMPEDE PROTECTION
// ============================================================

func GetOrSet[T any](
	ctx context.Context,
	cache *RedisCache,
	key string,
	ttl time.Duration,
	fetcher func() (*T, error),
) (*T, error) {

	data, err := GetTyped[T](ctx, cache, key)
	if err == nil {
		return data, nil
	}

	result, err, _ := cache.sf.Do(key, func() (interface{}, error) {

		data, err := GetTyped[T](ctx, cache, key)
		if err == nil {
			return data, nil
		}

		fresh, err := fetcher()
		if err != nil {
			return nil, err
		}

		if err := SetTyped(ctx, cache, key, *fresh, ttl); err != nil {
			cache.log.Error().
				Err(err).
				Str("key", key).
				Msg("cache set failed")
		}

		return fresh, nil
	})

	if err != nil {
		return nil, err
	}

	typed, ok := result.(*T)
	if !ok {
		return nil, errors.New("type assertion failed")
	}

	return typed, nil
}

// ============================================================
// DISTRIBUTED LOCKING
// ============================================================

func (c *RedisCache) AcquireLock(ctx context.Context,key string,expiration time.Duration) (*DistributedLock, error) {

	tokenBytes := make([]byte, 16)

	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}

	token := hex.EncodeToString(tokenBytes)

	lockKey := c.buildKey("lock:" + key)

	ok, err := c.client.SetNX(ctx,lockKey,token, expiration).Result()

	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, ErrLockNotAcquired
	}

	return &DistributedLock{
		cache:      c,
		key:        lockKey,
		token:      token,
		expiration: expiration,
	}, nil
}

func (l *DistributedLock) Release(ctx context.Context) error {

	script := `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end
`

	result, err := l.cache.client.Eval(ctx,script,[]string{l.key},l.token).Int()

	if err != nil {
		return err
	}

	if result == 0 {
		return ErrLockNotOwned
	}

	return nil
}

// ============================================================
// RATE LIMITING
// ============================================================

func NewRateLimiter(cache *RedisCache) *RateLimiter {
	return &RateLimiter{
		cache: cache,
	}
}

func (r *RateLimiter) Allow(ctx context.Context,key string,limit int,window time.Duration) (bool, error) {

	key = r.cache.buildKey("ratelimit:" + key)

	pipe := r.cache.client.TxPipeline()

	count := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	return count.Val() <= int64(limit), nil
}

// ============================================================
// IDEMPOTENCY
// ============================================================

func (c *RedisCache) StoreIdempotencyKey(ctx context.Context,key string,ttl time.Duration) error {

	key = c.buildKey("idempotency:" + key)

	ok, err := c.client.SetNX(ctx,key,"1",ttl).Result()

	if err != nil {
		return err
	}

	if !ok {
		return errors.New("duplicate request")
	}

	return nil
}

// ============================================================
// FRAUD DETECTION
// ============================================================

func (c *RedisCache) TrackFailedPayments(ctx context.Context,tenantID string,clientID string) error {

	key := fmt.Sprintf("fraud:%s:%s:failed_payments",tenantID,clientID)

	key = c.buildKey(key)

	return c.client.Incr(ctx, key).Err()
}

func (c *RedisCache) IsFraudRisk(
	ctx context.Context,
	tenantID string,
	clientID string,
) (bool, error) {

	key := fmt.Sprintf(
		"fraud:%s:%s:failed_payments",
		tenantID,
		clientID,
	)

	key = c.buildKey(key)

	count, err := c.client.Get(ctx, key).Int()

	if err != nil && !errors.Is(err, redis.Nil) {
		return false, err
	}

	return count >= 5, nil
}

// ============================================================
// SAFE INVALIDATION
// ============================================================

func (c *RedisCache) InvalidateTenantModule(
	ctx context.Context,
	tenantID string,
	module string,
) error {

	pattern := fmt.Sprintf(
		"%s:%s:%s:*",
		c.config.Environment,
		tenantID,
		module,
	)

	return c.DeleteByPattern(ctx, pattern)
}

// ============================================================
// HEALTH CHECKS
// ============================================================

func (c *RedisCache) Health(
	ctx context.Context,
) (*HealthStatus, error) {

	start := time.Now()

	err := c.client.Ping(ctx).Err()

	latency := time.Since(start)

	if err != nil {
		return &HealthStatus{
			Status:  "DOWN",
			Latency: latency,
		}, err
	}

	stats := c.client.PoolStats()

	c.log.Debug().
		Int32("hits", int32(stats.Hits)).
		Int32("misses", int32(stats.Misses)).
		Uint32("timeouts", stats.Timeouts).
		Msg("redis pool stats")

	return &HealthStatus{
		Status:  "UP",
		Latency: latency,
	}, nil
}

// ============================================================
// CLEAN SHUTDOWN
// ============================================================

func (c *RedisCache) Close() error {
	c.log.Info().Msg("closing redis cache connection")
	return c.client.Close()
}