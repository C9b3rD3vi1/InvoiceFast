package services

import (
	"context"
	"time"

	"invoicefast/internal/cache"
	"invoicefast/internal/logger"
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
		logger.Get().Error(ctx, "Error acquiring lock", "checkout_id", checkoutID, "error", err)
		return false, err
	}

	if !acquired {
		logger.Get().Warn(ctx, "Lock already held - returning 200 to stop retries", "checkout_id", checkoutID)
		return true, nil
	}

	processedKey := "payment:processed:" + checkoutID
	exists, err := s.cache.Exists(ctx, processedKey)
	if err == nil && exists {
		logger.Get().Info(ctx, "Payment already processed - releasing lock and returning 200", "checkout_id", checkoutID)
		_ = s.cache.Unlock(ctx, lockKey)
		return true, nil
	}

	if err := s.cache.Set(ctx, processedKey, payload, 24*time.Hour); err != nil {
		logger.Get().Warn(ctx, "Failed to mark payment as processed", "checkout_id", checkoutID, "error", err)
	}

	logger.Get().Info(ctx, "New payment - processing callback", "checkout_id", checkoutID)
	return false, nil
}
