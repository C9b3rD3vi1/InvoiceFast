package contextutil

import (
	"context"
	"errors"
)

type contextKey string

const (
	TenantIDKey contextKey = "tenant_id"
	UserIDKey   contextKey = "user_id"
)

var (
	ErrNoTenantID = errors.New("no tenant ID in context")
	ErrNoUserID   = errors.New("no user ID in context")
)

type TenantContext struct {
	TenantID string
	UserID   string
}

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

func WithTenantAndUser(ctx context.Context, tenantID, userID string) context.Context {
	return context.WithValue(
		context.WithValue(ctx, TenantIDKey, tenantID),
		UserIDKey,
		userID,
	)
}

func GetTenantID(ctx context.Context) (string, error) {
	val := ctx.Value(TenantIDKey)
	if val == nil {
		return "", ErrNoTenantID
	}
	tenantID, ok := val.(string)
	if !ok || tenantID == "" {
		return "", ErrNoTenantID
	}
	return tenantID, nil
}

func GetUserID(ctx context.Context) (string, error) {
	val := ctx.Value(UserIDKey)
	if val == nil {
		return "", ErrNoUserID
	}
	userID, ok := val.(string)
	if !ok || userID == "" {
		return "", ErrNoUserID
	}
	return userID, nil
}

func GetTenantContext(ctx context.Context) (TenantContext, error) {
	tenantID, err := GetTenantID(ctx)
	if err != nil {
		return TenantContext{}, err
	}
	userID, err := GetUserID(ctx)
	if err != nil {
		return TenantContext{}, err
	}
	return TenantContext{TenantID: tenantID, UserID: userID}, nil
}
