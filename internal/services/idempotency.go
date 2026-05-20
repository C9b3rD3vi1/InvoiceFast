package services

import (
	"context"
	"time"
	"errors"

	"invoicefast/internal/cache"
	"invoicefast/internal/logger"
)

type IdempotencyService struct {
	cache *cache.RedisCache
}

func NewIdempotencyService(redisCache *cache.RedisCache) *IdempotencyService {
	return &IdempotencyService{cache: redisCache}
}

func (s *IdempotencyService) CheckAndLock(ctx context.Context,key string) (bool, func(), error) {

	lock, err := s.cache.Lock(ctx, key, 5*time.Minute)

	if err != nil {

		if errors.Is(err, cache.ErrLockNotAcquired) {
			return false, nil, nil
		}

		return false, nil, err
	}

	if lock == nil {
		return false, nil, nil
	}

	return true, func() { lock.Release(ctx); }, nil
}


func (s *IdempotencyService) IsProcessed(ctx context.Context, key string) (bool, error) {
	key = "idempotent:" + key
	return s.cache.Exists(ctx, key)
}

func (s *IdempotencyService) MarkProcessed(ctx context.Context, key string, data interface{}) error {
	key = "idempotent:" + key
	return s.cache.Set(ctx, key, data, 24*time.Hour)
}

func (s *IdempotencyService) HandlePaymentCallback(ctx context.Context,checkoutID string,payload map[string]interface{},) (bool, error) {

	lockKey := "payment:lock:" + checkoutID

	// acquire distributed lock
	lock, err := s.cache.Lock(ctx,lockKey,5*time.Minute)

	if err != nil {

		// another process already processing
		if errors.Is(err, cache.ErrLockNotAcquired) {

			logger.Get().Warn(ctx, "Lock already held - returning 200 to stop retries", "checkout_id", checkoutID)

			return true, nil
		}

		logger.Get().Error(ctx,"Error acquiring lock","checkout_id",checkoutID,"error",err)

		return false, err
	}

	// ALWAYS release lock
	defer func() {

		if lock != nil {

			if err := lock.Release(ctx); err != nil {

				logger.Get().Error(ctx,"Failed to release payment lock","checkout_id",checkoutID,"error",err)
			}
		}
	}()

	processedKey := "payment:processed:" + checkoutID

	exists, err := s.cache.Exists(ctx, processedKey)
	if err != nil {

		logger.Get().Error(ctx,"Failed checking processed payment","checkout_id",checkoutID,"error",err)

		return false, err
	}

	// already processed
	if exists {

		logger.Get().Info(ctx,"Payment already processed","checkout_id",checkoutID)

		return true, nil
	}

	// mark processed
	if err := s.cache.Set(ctx, processedKey,payload,24*time.Hour); err != nil {

		logger.Get().Warn(ctx,"Failed to mark payment as processed", "checkout_id",checkoutID,"error",err)
	}

	logger.Get().Info(ctx,"New payment - processing callback", "checkout_id",checkoutID)

	return false, nil
}