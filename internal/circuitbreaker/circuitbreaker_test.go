package circuitbreaker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerInitialState(t *testing.T) {
	cb := New("test")
	if cb.State() != StateClosed {
		t.Errorf("expected closed, got %s", cb.State())
	}
}

func TestCircuitBreakerOpensAfterFailures(t *testing.T) {
	cb := New("test", WithFailureThreshold(3), WithRecoveryTimeout(10*time.Minute))

	for i := 0; i < 3; i++ {
		err := cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("fail")
		})
		if err == nil {
			t.Fatal("expected error")
		}
	}

	if cb.State() != StateOpen {
		t.Errorf("expected open, got %s", cb.State())
	}
}

func TestCircuitBreakerRejectsWhenOpen(t *testing.T) {
	cb := New("test", WithFailureThreshold(1), WithRecoveryTimeout(10*time.Minute))

	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})

	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreakerClosesAfterSuccess(t *testing.T) {
	cb := New("test", WithFailureThreshold(1), WithSuccessThreshold(2), WithRecoveryTimeout(1*time.Millisecond))

	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})

	time.Sleep(2 * time.Millisecond)

	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return nil
		})
	}

	if cb.State() != StateClosed {
		t.Errorf("expected closed, got %s", cb.State())
	}
}

func TestExecuteWithResult(t *testing.T) {
	cb := New("test")

	result, err := cb.ExecuteWithResult(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "success", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.(string) != "success" {
		t.Errorf("expected 'success', got %v", result)
	}
}

func TestRegistryGetOrCreate(t *testing.T) {
	r := NewRegistry()
	cb1 := r.GetOrCreate("svc1")
	cb2 := r.GetOrCreate("svc1")
	if cb1 != cb2 {
		t.Error("expected same instance")
	}
}

func TestIsAvailable(t *testing.T) {
	cb := New("test", WithFailureThreshold(1), WithRecoveryTimeout(10*time.Minute))

	if !cb.IsAvailable() {
		t.Error("expected available initially")
	}

	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})

	if cb.IsAvailable() {
		t.Error("expected unavailable after failure")
	}
}

func TestConvenienceFunctions(t *testing.T) {
	mpesa := MpesaCircuit()
	kra := KRACircuit()
	email := EmailCircuit()
	whatsapp := WhatsAppCircuit()
	stripe := StripeCircuit()
	sms := SMSCircuit()

	if mpesa == nil || kra == nil || email == nil || whatsapp == nil || stripe == nil || sms == nil {
		t.Error("convenience functions should return non-nil breakers")
	}

	if MpesaCircuit() != mpesa {
		t.Error("expected same M-Pesa instance")
	}
}
