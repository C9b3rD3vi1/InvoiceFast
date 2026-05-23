package middleware

import (
	"testing"
	"time"
)

func TestCSRFStoreGetSet(t *testing.T) {
	store := newCSRFStore()
	token := &csrfToken{
		Value:     "test-token",
		ExpiresAt: time.Now().Add(time.Hour),
		UserID:    "user-1",
	}

	store.Set("user-1", token)
	got := store.Get("test-token")
	if got == nil {
		t.Fatal("expected token, got nil")
	}
	if got.Value != "test-token" {
		t.Errorf("expected 'test-token', got %s", got.Value)
	}
	if got.UserID != "user-1" {
		t.Errorf("expected 'user-1', got %s", got.UserID)
	}
}

func TestCSRFStoreGetMissing(t *testing.T) {
	store := newCSRFStore()
	got := store.Get("nonexistent")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestCSRFStoreExpiredToken(t *testing.T) {
	store := newCSRFStore()
	store.Set("user-1", &csrfToken{
		Value:     "expired-val",
		ExpiresAt: time.Now().Add(-time.Hour),
		UserID:    "user-1",
	})
	got := store.Get("expired-val")
	if got == nil {
		t.Fatal("expected token (expiry checked by handler, not store)")
	}
	if !got.ExpiresAt.Before(time.Now()) {
		t.Error("expected expired token")
	}
}

func TestCSRFStoreConcurrent(t *testing.T) {
	store := newCSRFStore()
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			store.Set("user-1", &csrfToken{Value: "val"})
			store.Get("nonexistent")
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestDefaultCSRFConfig(t *testing.T) {
	cfg := DefaultCSRFConfig()
	if cfg.CookieName != "csrf_token" {
		t.Errorf("expected 'csrf_token', got %s", cfg.CookieName)
	}
	if cfg.Expiration != 24*time.Hour {
		t.Errorf("expected 24h expiration, got %v", cfg.Expiration)
	}
	if cfg.SingleToken {
		t.Error("expected SingleToken to be false")
	}
}
