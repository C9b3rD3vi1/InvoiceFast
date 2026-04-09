package services

import (
	"context"
	"log"
	"time"

	"invoicefast/internal/cache"
)

type IdempotencyService struct {
	cache *cache.RedisCache
}

func NewIdempotencyService(redisCache *cache.RedisCache) *IdempotencyService {
	return &IdempotencyService{cache: redisCache}
}

func (s *IdempotencyService) CheckAndLock(ctx context.Context, key string) (bool, error) {
	acquired, err := s.cache.Lock(ctx, key, 5*time.Minute)
	if err != nil {
		return false, err
	}

	return acquired, nil
}

func (s *IdempotencyService) IsProcessed(ctx context.Context, key string) (bool, error) {
	key = "idempotent:" + key
	return s.cache.Exists(ctx, key)
}

func (s *IdempotencyService) MarkProcessed(ctx context.Context, key string, data interface{}) error {
	key = "idempotent:" + key
	return s.cache.Set(ctx, key, data, 24*time.Hour)
}

func (s *IdempotencyService) Unlock(ctx context.Context, key string) error {
	lockKey := "idempotency:" + key
	return s.cache.Unlock(ctx, lockKey)
}

func (s *IdempotencyService) HandlePaymentCallback(ctx context.Context, checkoutID string, payload map[string]interface{}) (bool, error) {
	lockKey := "payment:lock:" + checkoutID

	acquired, err := s.cache.Lock(ctx, lockKey, 5*time.Minute)
	if err != nil {
		log.Printf("[Idempotency] Error acquiring lock for %s: %v", checkoutID, err)
		return false, err
	}

	if !acquired {
		log.Printf("[Idempotency] Lock already held for %s - returning 200 to stop retries", checkoutID)
		return true, nil
	}

	processedKey := "payment:processed:" + checkoutID
	exists, err := s.cache.Exists(ctx, processedKey)
	if err == nil && exists {
		log.Printf("[Idempotency] Payment %s already processed - releasing lock and returning 200", checkoutID)
		_ = s.cache.Unlock(ctx, lockKey)
		return true, nil
	}

	if err := s.cache.Set(ctx, processedKey, payload, 24*time.Hour); err != nil {
		log.Printf("[Idempotency] Warning: failed to mark payment %s as processed: %v", checkoutID, err)
	}

	log.Printf("[Idempotency] New payment %s - processing callback", checkoutID)
	return false, nil
}
