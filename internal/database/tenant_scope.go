package database

import (
	"context"

	"gorm.io/gorm"
)

type DBTX interface {
	Where(query interface{}, args ...interface{}) *gorm.DB
	Create(value interface{}) *gorm.DB
	First(dest interface{}, conditions ...interface{}) *gorm.DB
	Find(dest interface{}, conditions ...interface{}) *gorm.DB
	Update(column string, value interface{}) *gorm.DB
	Updates(values interface{}) *gorm.DB
	Delete(value interface{}, conditions ...interface{}) *gorm.DB
	Save(value interface{}) *gorm.DB
	Preload(column string, conditions ...interface{}) *gorm.DB
	Transaction(fc func(tx *gorm.DB) error) error
}

func TenantFilter(tenantID string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("tenant_id = ?", tenantID)
	}
}

func TenantScopeFromContext(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		tenantID := ctx.Value("tenant_id")
		if tenantID == nil {
			return db
		}
		if id, ok := tenantID.(string); ok && id != "" {
			return db.Where("tenant_id = ?", id)
		}
		return db
	}
}

func TenantIDFromContext(ctx context.Context) string {
	if val := ctx.Value("tenant_id"); val != nil {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
}

func UserIDFromContext(ctx context.Context) string {
	if val := ctx.Value("user_id"); val != nil {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
}

func ContextWithTenant(ctx context.Context, tenantID, userID string) context.Context {
	ctx = context.WithValue(ctx, "tenant_id", tenantID)
	ctx = context.WithValue(ctx, "user_id", userID)
	return ctx
}

func TenantIDFromString(tenantID string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if tenantID == "" {
			return db
		}
		return db.Where("tenant_id = ?", tenantID)
	}
}
