package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2")

// ErrCircuitOpen is returned when the circuit is open
var ErrCircuitOpen = errors.New("circuit breaker is open")

// ErrCircuitHalfOpen is returned when attempting to call during half-open state
var ErrCircuitHalfOpen = errors.New("circuit breaker is half-open")

// State represents the state of the circuit breaker
type State int

const (
	StateClosed State = iota // Normal operation
	StateOpen               // Failing, reject all requests
	StateHalfOpen           // Testing if service recovered
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration
type Config struct {
	// Threshold for opening the circuit (default: 5)
	FailureThreshold int `json:"failure_threshold"`

	// Threshold for closing the circuit after recovery (default: 3)
	SuccessThreshold int `json:"success_threshold"`

	// How long to wait before attempting to recover (default: 30s)
	RecoveryTimeout time.Duration `json:"recovery_timeout"`

	// Callback when circuit opens
	OnOpen func(service string) `json:"-"`

	// Callback when circuit closes
	OnClose func(service string) `json:"-"`

	// Callback when circuit half-opens
	OnHalfOpen func(service string) `json:"-"`
}

// OptionFunc is a functional option for configuring the circuit breaker
type OptionFunc func(*Config)

// WithFailureThreshold sets the number of failures before opening
func WithFailureThreshold(n int) OptionFunc {
	return func(c *Config) {
		if n > 0 {
			c.FailureThreshold = n
		}
	}
}

// WithSuccessThreshold sets the number of successes needed to close
func WithSuccessThreshold(n int) OptionFunc {
	return func(c *Config) {
		if n > 0 {
			c.SuccessThreshold = n
		}
	}
}

// WithRecoveryTimeout sets the time to wait before attempting recovery
func WithRecoveryTimeout(d time.Duration) OptionFunc {
	return func(c *Config) {
		if d > 0 {
			c.RecoveryTimeout = d
		}
	}
}

// WithOnOpen sets the callback when circuit opens
func WithOnOpen(f func(string)) OptionFunc {
	return func(c *Config) {
		c.OnOpen = f
	}
}

// WithOnClose sets the callback when circuit closes
func WithOnClose(f func(string)) OptionFunc {
	return func(c *Config) {
		c.OnClose = f
	}
}

// CircuitBreaker represents a circuit breaker
type CircuitBreaker struct {
	name      string
	state     State
	mu        sync.RWMutex
	failures  int
	successes int
	lastFail  time.Time
	config    Config
}

// New creates a new circuit breaker with the given name and options
func New(name string, opts ...OptionFunc) *CircuitBreaker {
	config := Config{
		FailureThreshold: 5,
		SuccessThreshold: 3,
		RecoveryTimeout:  30 * time.Second,
	}

	for _, opt := range opts {
		opt(&config)
	}

	return &CircuitBreaker{
		name:   name,
		state:  StateClosed,
		config: config,
	}
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// IsAvailable returns true if the circuit breaker allows requests
func (cb *CircuitBreaker) IsAvailable() bool {
	state := cb.State()
	return state == StateClosed || state == StateHalfOpen
}

// Execute runs the given function through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	state := cb.State()

	// Check if circuit is open
	if state == StateOpen {
		// Check if recovery timeout has passed
		cb.mu.RLock()
		timeSinceFail := time.Since(cb.lastFail)
		cb.mu.RUnlock()

		if timeSinceFail >= cb.config.RecoveryTimeout {
			// Transition to half-open
			cb.mu.Lock()
			if cb.state == StateOpen {
				cb.state = StateHalfOpen
				cb.successes = 0
				if cb.config.OnHalfOpen != nil {
					cb.config.OnHalfOpen(cb.name)
				}
			}
			cb.mu.Unlock()
		} else {
			return ErrCircuitOpen
		}
	}

	// Execute the function
	err := fn(ctx)

	// Handle result
	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return err
}

// ExecuteWithResult runs the function and returns the result
func (cb *CircuitBreaker) ExecuteWithResult(ctx context.Context, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	state := cb.State()

	// Check if circuit is open
	if state == StateOpen {
		// Check if recovery timeout has passed
		cb.mu.RLock()
		timeSinceFail := time.Since(cb.lastFail)
		cb.mu.RUnlock()

		if timeSinceFail >= cb.config.RecoveryTimeout {
			// Transition to half-open
			cb.mu.Lock()
			if cb.state == StateOpen {
				cb.state = StateHalfOpen
				cb.successes = 0
				if cb.config.OnHalfOpen != nil {
					cb.config.OnHalfOpen(cb.name)
				}
			}
			cb.mu.Unlock()
		} else {
			return nil, ErrCircuitOpen
		}
	}

	// Execute the function
	result, err := fn(ctx)

	// Handle result
	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return result, err
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFail = time.Now()

	if cb.state == StateHalfOpen {
		// Failure during half-open, go back to open
		cb.state = StateOpen
		cb.successes = 0
	} else if cb.failures >= cb.config.FailureThreshold && cb.state == StateClosed {
		// Too many failures, open the circuit
		cb.state = StateOpen
		if cb.config.OnOpen != nil {
			cb.config.OnOpen(cb.name)
		}
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successes++
	cb.failures = 0 // Reset failures on success

	if cb.state == StateHalfOpen && cb.successes >= cb.config.SuccessThreshold {
		// Enough successes, close the circuit
		cb.state = StateClosed
		if cb.config.OnClose != nil {
			cb.config.OnClose(cb.name)
		}
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
	cb.lastFail = time.Time{}
}

// GetMetrics returns current metrics
func (cb *CircuitBreaker) GetMetrics() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return map[string]interface{}{
		"name":         cb.name,
		"state":        cb.state.String(),
		"failures":     cb.failures,
		"successes":    cb.successes,
		"last_fail":    cb.lastFail,
		"threshold":     cb.config.FailureThreshold,
	}
}

// ============================================================
// Circuit Breaker Registry
// ============================================================

// Registry manages multiple circuit breakers
type Registry struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

var (
	defaultRegistry *Registry
	registryOnce    sync.Once
)

// Default returns the default registry
func Default() *Registry {
	registryOnce.Do(func() {
		defaultRegistry = NewRegistry()
	})
	return defaultRegistry
}

// NewRegistry creates a new registry
func NewRegistry() *Registry {
	return &Registry{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// GetOrCreate gets an existing circuit breaker or creates a new one
func (r *Registry) GetOrCreate(name string, opts ...OptionFunc) *CircuitBreaker {
	r.mu.RLock()
	if cb, exists := r.breakers[name]; exists {
		r.mu.RUnlock()
		return cb
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, exists := r.breakers[name]; exists {
		return cb
	}

	cb := New(name, opts...)
	r.breakers[name] = cb

	return cb
}

// Get returns an existing circuit breaker
func (r *Registry) Get(name string) (*CircuitBreaker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cb, exists := r.breakers[name]
	return cb, exists
}

// Remove removes a circuit breaker
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.breakers, name)
}

// GetAll returns all circuit breakers
func (r *Registry) GetAll() map[string]*CircuitBreaker {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*CircuitBreaker, len(r.breakers))
	for k, v := range r.breakers {
		result[k] = v
	}
	return result
}

// GetAllMetrics returns metrics for all circuit breakers
func (r *Registry) GetAllMetrics() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metrics := make([]map[string]interface{}, 0, len(r.breakers))
	for _, cb := range r.breakers {
		metrics = append(metrics, cb.GetMetrics())
	}
	return metrics
}

// ============================================================
// Convenience functions
// ============================================================

// MpesaCircuit returns the M-Pesa circuit breaker
func MpesaCircuit(opts ...OptionFunc) *CircuitBreaker {
	return Default().GetOrCreate("mpesa", opts...)
}

// KRACircuit returns the KRA circuit breaker
func KRACircuit(opts ...OptionFunc) *CircuitBreaker {
	return Default().GetOrCreate("kra", opts...)
}

// EmailCircuit returns the email circuit breaker
func EmailCircuit(opts ...OptionFunc) *CircuitBreaker {
	return Default().GetOrCreate("email", opts...)
}

// WhatsAppCircuit returns the WhatsApp circuit breaker
func WhatsAppCircuit(opts ...OptionFunc) *CircuitBreaker {
	return Default().GetOrCreate("whatsapp", opts...)
}

// StripeCircuit returns the Stripe circuit breaker
func StripeCircuit(opts ...OptionFunc) *CircuitBreaker {
	return Default().GetOrCreate("stripe", opts...)
}

// SMSCircuit returns the SMS circuit breaker
func SMSCircuit(opts ...OptionFunc) *CircuitBreaker {
	return Default().GetOrCreate("sms", opts...)
}

// ============================================================
// Circuit Breaker HTTP Middleware for Fiber
// ============================================================

// Middleware returns a Fiber middleware that checks circuit state
func Middleware(name string) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		circuits := Default()
		circuit, exists := circuits.Get(name)

		if !exists || circuit.IsAvailable() {
			return c.Next()
		}

		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":       "Service temporarily unavailable",
			"code":        "CIRCUIT_OPEN",
			"service":     name,
			"retry_after": "30s",
		})
	}
}

// ============================================================
// Health Check Integration
// ============================================================

// HealthChecker checks circuit breaker health
func (cb *CircuitBreaker) HealthChecker() map[string]interface{} {
	state := cb.State()

	return map[string]interface{}{
		"name":     cb.name,
		"healthy": state == StateClosed,
		"state":    state.String(),
		"failures": cb.failures,
	}
}

// HealthCheck returns health status for all circuit breakers
func (r *Registry) HealthCheck() map[string]interface{} {
	breakers := r.GetAll()

	healthy := true
	status := make(map[string]interface{})

	for name, cb := range breakers {
		status[name] = cb.HealthChecker()
		if cb.State() != StateClosed {
			healthy = false
		}
	}

	return map[string]interface{}{
		"healthy": healthy,
		"circuits": status,
	}
}

// GetCircuitBreakerHealth returns health status for all services
func GetCircuitBreakerHealth() map[string]interface{} {
	return Default().HealthCheck()
}

// Format circuit breaker status for display
func FormatStatus() string {
	metrics := Default().GetAllMetrics()
	if len(metrics) == 0 {
		return "No circuit breakers configured"
	}

	var status string
	for _, m := range metrics {
		status += fmt.Sprintf("%s: %s (failures: %v)\n",
			m["name"], m["state"], m["failures"])
	}
	return status
}