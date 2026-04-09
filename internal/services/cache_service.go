package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"invoicefast/internal/cache"
	"invoicefast/internal/models"
)

// CachedService provides caching decorators for services
type CachedService struct {
	cache  *cache.RedisCache
	prefix string
}

// NewCachedService creates a new cached service wrapper
func NewCachedService(cache *cache.RedisCache) *CachedService {
	return &CachedService{
		cache:  cache,
		prefix: "invoicefast",
	}
}

// ==================== USER CACHING ====================

// CacheUser caches a user
func (cs *CachedService) CacheUser(ctx context.Context, user *models.User) error {
	if cs.cache == nil {
		return nil
	}

	key := fmt.Sprintf("user:%s", user.ID)
	return cs.cache.Set(ctx, key, user, cache.DurationMedium)
}

// GetCachedUser gets a cached user
func (cs *CachedService) GetCachedUser(ctx context.Context, userID string) (*models.User, error) {
	if cs.cache == nil {
		return nil, fmt.Errorf("cache not available")
	}

	key := fmt.Sprintf("user:%s", userID)
	var user models.User
	if err := cs.cache.Get(ctx, key, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// InvalidateUser removes a user from cache
func (cs *CachedService) InvalidateUser(ctx context.Context, userID string) error {
	if cs.cache == nil {
		return nil
	}
	return cs.cache.Delete(ctx, fmt.Sprintf("user:%s", userID))
}

// ==================== INVOICE CACHING ====================

// CacheInvoice caches an invoice
func (cs *CachedService) CacheInvoice(ctx context.Context, invoice *models.Invoice) error {
	if cs.cache == nil {
		return nil
	}

	key := fmt.Sprintf("invoice:%s", invoice.ID)
	data, err := json.Marshal(invoice)
	if err != nil {
		return err
	}
	return cs.cache.SetString(ctx, key, string(data), cache.DurationMedium)
}

// GetCachedInvoice gets a cached invoice
func (cs *CachedService) GetCachedInvoice(ctx context.Context, invoiceID string) (*models.Invoice, error) {
	if cs.cache == nil {
		return nil, fmt.Errorf("cache not available")
	}

	key := fmt.Sprintf("invoice:%s", invoiceID)
	data, err := cs.cache.GetString(ctx, key)
	if err != nil {
		return nil, err
	}

	var invoice models.Invoice
	if err := json.Unmarshal([]byte(data), &invoice); err != nil {
		return nil, err
	}
	return &invoice, nil
}

// InvalidateInvoice removes an invoice from cache
func (cs *CachedService) InvalidateInvoice(ctx context.Context, invoiceID string) error {
	if cs.cache == nil {
		return nil
	}

	keys := []string{
		fmt.Sprintf("invoice:%s", invoiceID),
		fmt.Sprintf("invoice:%s:items", invoiceID),
	}
	return cs.cache.Delete(ctx, keys...)
}

// CacheInvoiceList caches a list of invoices for a user
func (cs *CachedService) CacheInvoiceList(ctx context.Context, userID string, filter string, invoices []models.Invoice) error {
	if cs.cache == nil {
		return nil
	}

	key := fmt.Sprintf("invoices:%s:%s", userID, filter)
	return cs.cache.Set(ctx, key, invoices, cache.DurationShort)
}

// GetCachedInvoiceList gets a cached invoice list
func (cs *CachedService) GetCachedInvoiceList(ctx context.Context, userID, filter string) ([]models.Invoice, error) {
	if cs.cache == nil {
		return nil, fmt.Errorf("cache not available")
	}

	key := fmt.Sprintf("invoices:%s:%s", userID, filter)
	var invoices []models.Invoice
	if err := cs.cache.Get(ctx, key, &invoices); err != nil {
		return nil, err
	}
	return invoices, nil
}

// InvalidateInvoiceLists removes all cached invoice lists for a user
func (cs *CachedService) InvalidateInvoiceLists(ctx context.Context, userID string) error {
	if cs.cache == nil {
		return nil
	}
	return cs.cache.DeleteByPattern(ctx, fmt.Sprintf("invoices:%s:*", userID))
}

// ==================== CLIENT CACHING ====================

// CacheClient caches a client
func (cs *CachedService) CacheClient(ctx context.Context, client *models.Client) error {
	if cs.cache == nil {
		return nil
	}

	key := fmt.Sprintf("client:%s", client.ID)
	return cs.cache.Set(ctx, key, client, cache.DurationMedium)
}

// GetCachedClient gets a cached client
func (cs *CachedService) GetCachedClient(ctx context.Context, clientID string) (*models.Client, error) {
	if cs.cache == nil {
		return nil, fmt.Errorf("cache not available")
	}

	key := fmt.Sprintf("client:%s", clientID)
	var client models.Client
	if err := cs.cache.Get(ctx, key, &client); err != nil {
		return nil, err
	}
	return &client, nil
}

// InvalidateClient removes a client from cache
func (cs *CachedService) InvalidateClient(ctx context.Context, clientID string) error {
	if cs.cache == nil {
		return nil
	}
	return cs.cache.Delete(ctx, fmt.Sprintf("client:%s", clientID))
}

// ==================== DASHBOARD CACHING ====================

// CacheDashboardStats caches dashboard statistics
func (cs *CachedService) CacheDashboardStats(ctx context.Context, userID string, period string, stats *DashboardStats) error {
	if cs.cache == nil {
		return nil
	}

	key := fmt.Sprintf("dashboard:%s:%s", userID, period)
	return cs.cache.Set(ctx, key, stats, cache.DurationShort) // Short cache for stats
}

// GetCachedDashboardStats gets cached dashboard stats
func (cs *CachedService) GetCachedDashboardStats(ctx context.Context, userID, period string) (*DashboardStats, error) {
	if cs.cache == nil {
		return nil, fmt.Errorf("cache not available")
	}

	key := fmt.Sprintf("dashboard:%s:%s", userID, period)
	var stats DashboardStats
	if err := cs.cache.Get(ctx, key, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// InvalidateDashboard removes cached dashboard stats for a user
func (cs *CachedService) InvalidateDashboard(ctx context.Context, userID string) error {
	if cs.cache == nil {
		return nil
	}
	return cs.cache.DeleteByPattern(ctx, fmt.Sprintf("dashboard:%s:*", userID))
}

// ==================== API KEY CACHING ====================

// CacheAPIKey caches an API key validation
func (cs *CachedService) CacheAPIKey(ctx context.Context, keyHash string, userID string) error {
	if cs.cache == nil {
		return nil
	}

	key := fmt.Sprintf("apikey:%s", keyHash)
	return cs.cache.SetString(ctx, key, userID, cache.DurationLong)
}

// GetCachedAPIKeyUserID gets the cached user ID for an API key
func (cs *CachedService) GetCachedAPIKeyUserID(ctx context.Context, keyHash string) (string, error) {
	if cs.cache == nil {
		return "", fmt.Errorf("cache not available")
	}

	key := fmt.Sprintf("apikey:%s", keyHash)
	return cs.cache.GetString(ctx, key)
}

// InvalidateAPIKey removes an API key from cache
func (cs *CachedService) InvalidateAPIKey(ctx context.Context, keyHash string) error {
	if cs.cache == nil {
		return nil
	}
	return cs.cache.Delete(ctx, fmt.Sprintf("apikey:%s", keyHash))
}

// ==================== RATE LIMITING WITH CACHE ====================

// CheckRateLimit checks if a rate limit is exceeded
func (cs *CachedService) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	if cs.cache == nil {
		return true, 0, nil // Allow if cache not available
	}

	cacheKey := fmt.Sprintf("ratelimit:%s", key)
	count, err := cs.cache.Increment(ctx, cacheKey)
	if err != nil {
		return true, 0, err
	}

	// Set expiration on first increment
	if count == 1 {
		cs.cache.Expire(ctx, cacheKey, window)
	}

	remaining := limit - int(count)
	allowed := count <= int64(limit)

	return allowed, remaining, nil
}

// ==================== SESSION CACHING ====================

// CacheSession caches a session
func (cs *CachedService) CacheSession(ctx context.Context, sessionID string, userID string, expiry time.Duration) error {
	if cs.cache == nil {
		return nil
	}

	key := fmt.Sprintf("session:%s", sessionID)
	return cs.cache.SetString(ctx, key, userID, expiry)
}

// GetCachedSession gets a cached session
func (cs *CachedService) GetCachedSession(ctx context.Context, sessionID string) (string, error) {
	if cs.cache == nil {
		return "", fmt.Errorf("cache not available")
	}

	key := fmt.Sprintf("session:%s", sessionID)
	return cs.cache.GetString(ctx, key)
}

// InvalidateSession removes a session from cache
func (cs *CachedService) InvalidateSession(ctx context.Context, sessionID string) error {
	if cs.cache == nil {
		return nil
	}
	return cs.cache.Delete(ctx, fmt.Sprintf("session:%s", sessionID))
}

// InvalidateAllUserSessions removes all sessions for a user
func (cs *CachedService) InvalidateAllUserSessions(ctx context.Context, userID string) error {
	if cs.cache == nil {
		return nil
	}
	return cs.cache.DeleteByPattern(ctx, fmt.Sprintf("session:*:%s", userID))
}

// ==================== PUBLIC INVOICE (MAGIC LINK) CACHING ====================

// CachePublicInvoice caches a public invoice view
func (cs *CachedService) CachePublicInvoice(ctx context.Context, token string, data map[string]interface{}) error {
	if cs.cache == nil {
		return nil
	}

	key := fmt.Sprintf("public:%s", token)
	return cs.cache.Set(ctx, key, data, cache.DurationMedium)
}

// GetCachedPublicInvoice gets a cached public invoice
func (cs *CachedService) GetCachedPublicInvoice(ctx context.Context, token string) (map[string]interface{}, error) {
	if cs.cache == nil {
		return nil, fmt.Errorf("cache not available")
	}

	key := fmt.Sprintf("public:%s", token)
	var data map[string]interface{}
	if err := cs.cache.Get(ctx, key, &data); err != nil {
		return nil, err
	}
	return data, nil
}