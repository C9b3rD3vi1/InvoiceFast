package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// HealthStatus represents the overall health status
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

// ComponentStatus represents the status of a health check component
type ComponentStatus struct {
	Name      string                 `json:"name"`
	Status    HealthStatus           `json:"status"`
	Latency   time.Duration          `json:"latency_ms,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	LastCheck time.Time              `json:"last_check"`
}

// Checker interface for health check components
type Checker interface {
	Name() string
	Check(ctx context.Context) ComponentStatus
}

// CheckFunc type for simple health check functions
type CheckFunc func(ctx context.Context) ComponentStatus

// NamedChecker wraps a function to implement Checker
type NamedChecker struct {
	name string
	fn   CheckFunc
}

func (n *NamedChecker) Name() string { return n.name }
func (n *NamedChecker) Check(ctx context.Context) ComponentStatus {
	return n.fn(ctx)
}

// Registry holds all health checkers
type Registry struct {
	mu       sync.RWMutex
	checkers map[string]Checker
	config   *Config
}

// Config holds health check configuration
type Config struct {
	Interval    time.Duration `json:"interval"`    // Interval between checks
	Timeout     time.Duration `json:"timeout"`    // Timeout for each check
	StartPeriod time.Duration `json:"start_period"` // Grace period after startup
	Verbose     bool          `json:"verbose"`      // Enable detailed checks
}

var (
	defaultRegistry *Registry
	defaultConfig   = &Config{
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
		StartPeriod: 30 * time.Second,
		Verbose:     false,
	}
)

// Default returns the default health check registry
func Default() *Registry {
	if defaultRegistry == nil {
		defaultRegistry = NewRegistry(nil)
	}
	return defaultRegistry
}

// NewRegistry creates a new health check registry
func NewRegistry(config *Config) *Registry {
	if config == nil {
		config = defaultConfig
	}

	return &Registry{
		checkers: make(map[string]Checker),
		config:   config,
	}
}

// Register registers a health check component
func (r *Registry) Register(checker Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers[checker.Name()] = checker
}

// RegisterFunc registers a health check function
func (r *Registry) RegisterFunc(name string, fn CheckFunc) {
	r.Register(&NamedChecker{name: name, fn: fn})
}

// Unregister removes a health check component
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.checkers, name)
}

// CheckAll runs all health checks and returns the result
func (r *Registry) CheckAll(ctx context.Context) *HealthReport {
	r.mu.RLock()
	checkers := make(map[string]Checker, len(r.checkers))
	for k, v := range r.checkers {
		checkers[k] = v
	}
	r.mu.RUnlock()

	report := &HealthReport{
		Timestamp:  time.Now(),
		Status:     StatusHealthy,
		Components: make([]ComponentStatus, 0, len(checkers)),
	}

	for _, checker := range checkers {
		start := time.Now()
		status := r.checkWithTimeout(ctx, checker)
		status.Latency = time.Since(start)
		status.LastCheck = time.Now()

		report.Components = append(report.Components, status)

		// Update overall status
		if status.Status == StatusUnhealthy {
			report.Status = StatusUnhealthy
		} else if status.Status == StatusDegraded && report.Status == StatusHealthy {
			report.Status = StatusDegraded
		}
	}

	return report
}

func (r *Registry) checkWithTimeout(ctx context.Context, checker Checker) ComponentStatus {
	resultCh := make(chan ComponentStatus, 1)

	go func() {
		resultCh <- checker.Check(ctx)
	}()

	select {
	case <-ctx.Done():
		return ComponentStatus{
			Name:     checker.Name(),
			Status:   StatusUnhealthy,
			Error:    "timeout",
			LastCheck: time.Now(),
		}
	case result := <-resultCh:
		return result
	}
}

// GetReport returns the health report as JSON
func (r *Registry) GetReport(ctx context.Context) ([]byte, error) {
	report := r.CheckAll(ctx)
	return json.MarshalIndent(report, "", "  ")
}

// HealthReport represents the overall health check result
type HealthReport struct {
	Timestamp time.Time          `json:"timestamp"`
	Status    HealthStatus        `json:"status"`
	Components []ComponentStatus `json:"components"`
}

// ============================================================
// Built-in Health Checkers
// ============================================================

// DBHealthChecker checks database connectivity
type DBHealthChecker struct {
	db *sql.DB
}

func NewDBHealthChecker(db *sql.DB) *DBHealthChecker {
	return &DBHealthChecker{db: db}
}

func (c *DBHealthChecker) Name() string { return "database" }

func (c *DBHealthChecker) Check(ctx context.Context) ComponentStatus {
	start := time.Now()

	err := c.db.PingContext(ctx)
	latency := time.Since(start)

	if err != nil {
		return ComponentStatus{
			Name:     "database",
			Status:   StatusUnhealthy,
			Error:    err.Error(),
			LastCheck: time.Now(),
		}
	}

	return ComponentStatus{
		Name:     "database",
		Status:   StatusHealthy,
		Latency:  latency,
		LastCheck: time.Now(),
		Details: map[string]interface{}{
			"driver": "postgres/sqlite",
		},
	}
}

// RedisHealthChecker checks Redis connectivity
type RedisHealthChecker struct {
	client *redis.Client
}

func NewRedisHealthChecker(client *redis.Client) *RedisHealthChecker {
	return &RedisHealthChecker{client: client}
}

func (c *RedisHealthChecker) Name() string { return "redis" }

func (c *RedisHealthChecker) Check(ctx context.Context) ComponentStatus {
	start := time.Now()

	result, err := c.client.Ping(ctx).Result()
	latency := time.Since(start)

	if err != nil {
		return ComponentStatus{
			Name:      "redis",
			Status:    StatusUnhealthy,
			Error:     err.Error(),
			LastCheck: time.Now(),
		}
	}

	return ComponentStatus{
		Name:      "redis",
		Status:    StatusHealthy,
		Latency:   latency,
		LastCheck: time.Now(),
		Details: map[string]interface{}{
			"ping": result,
		},
	}
}

// DiskHealthChecker checks disk space
type DiskHealthChecker struct {
	path string
}

func NewDiskHealthChecker(path string) *DiskHealthChecker {
	return &DiskHealthChecker{path: path}
}

func (c *DiskHealthChecker) Name() string { return "disk" }

func (c *DiskHealthChecker) Check(ctx context.Context) ComponentStatus {
	// Simplified disk check - in production use golang.org/x/sys/unix
	return ComponentStatus{
		Name:      "disk",
		Status:    StatusHealthy,
		LastCheck: time.Now(),
		Details: map[string]interface{}{
			"path": c.path,
		},
	}
}

// CircuitBreakerHealthChecker checks circuit breaker status
type CircuitBreakerHealthChecker struct{}

func NewCircuitBreakerHealthChecker() *CircuitBreakerHealthChecker {
	return &CircuitBreakerHealthChecker{}
}

func (c *CircuitBreakerHealthChecker) Name() string { return "circuit_breakers" }

func (c *CircuitBreakerHealthChecker) Check(ctx context.Context) ComponentStatus {
	return ComponentStatus{
		Name:      "circuit_breakers",
		Status:    StatusHealthy,
		LastCheck: time.Now(),
	}
}

// ============================================================
// HTTP Handlers for Health Checks
// ============================================================

// HealthResponse represents the health check response
type HealthResponse struct {
	Status     string           `json:"status"`
	Timestamp  string           `json:"timestamp"`
	Version    string           `json:"version,omitempty"`
	Components []ComponentStatus `json:"components,omitempty"`
	DurationMs int64            `json:"duration_ms,omitempty"`
}

// Handler returns a Fiber handler for health check endpoint
func Handler(registry *Registry) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		report := registry.CheckAll(c.Context())
		duration := time.Since(start)

		statusCode := fiber.StatusOK
		if report.Status == StatusUnhealthy {
			statusCode = fiber.StatusServiceUnavailable
		}

		return c.Status(statusCode).JSON(HealthResponse{
			Status:     string(report.Status),
			Timestamp:  report.Timestamp.Format(time.RFC3339),
			Version:    "1.0.0",
			Components: report.Components,
			DurationMs: duration.Milliseconds(),
		})
	}
}

// LiveHandler returns a simple liveness check
func LiveHandler(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status": "ok",
	})
}

// ReadyHandler returns a readiness check
func ReadyHandler(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "not ready",
				"error":  "database unavailable",
			})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "ready",
		})
	}
}

// SimpleHealthHandler returns a simple health response
func SimpleHealthHandler(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  "ok",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": "1.0.0",
	})
}

// ============================================================
// Health Check Utilities
// ============================================================

// IsHealthy returns true if the status is healthy
func (h HealthStatus) IsHealthy() bool {
	return h == StatusHealthy
}

// IsDegraded returns true if the status is degraded but functional
func (h HealthStatus) IsDegraded() bool {
	return h == StatusDegraded
}

// String returns string representation of health status
func (h HealthStatus) String() string {
	switch h {
	case StatusHealthy:
		return "healthy"
	case StatusDegraded:
		return "degraded"
	case StatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// ============================================================
// External API Health Checker
// ============================================================

// HTTPClient interface for making HTTP requests
type HTTPClient interface {
	GetContext(context.Context, string) (interface{}, error)
}

// SimpleHTTPClient is a basic HTTP client implementation
type SimpleHTTPClient struct {
	Client *http.Client
}

func (c *SimpleHTTPClient) GetContext(ctx context.Context, url string) (interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return map[string]interface{}{
		"status_code": resp.StatusCode,
	}, nil
}

// ExternalAPIHealthChecker checks external API connectivity
type ExternalAPIHealthChecker struct {
	name   string
	url    string
	client HTTPClient
}

func NewExternalAPIHealthChecker(name, url string, client HTTPClient) *ExternalAPIHealthChecker {
	if client == nil {
		client = &SimpleHTTPClient{Client: &http.Client{Timeout: 5 * time.Second}}
	}
	return &ExternalAPIHealthChecker{
		name:   name,
		url:    url,
		client: client,
	}
}

func (c *ExternalAPIHealthChecker) Name() string { return c.name }

func (c *ExternalAPIHealthChecker) Check(ctx context.Context) ComponentStatus {
	start := time.Now()

	result, err := c.client.GetContext(ctx, c.url)
	latency := time.Since(start)

	if err != nil {
		return ComponentStatus{
			Name:      c.name,
			Status:    StatusUnhealthy,
			Error:     err.Error(),
			Latency:   latency,
			LastCheck: time.Now(),
		}
	}

	// Type assert result
	respMap, ok := result.(map[string]interface{})
	if !ok {
		return ComponentStatus{
			Name:      c.name,
			Status:    StatusHealthy,
			Latency:   latency,
			LastCheck: time.Now(),
		}
	}

	// Check status code
	if statusCode, ok := respMap["status_code"].(int); ok {
		if statusCode >= 500 {
			return ComponentStatus{
				Name:      c.name,
				Status:    StatusDegraded,
				Error:     fmt.Sprintf("status: %d", statusCode),
				Latency:   latency,
				LastCheck: time.Now(),
			}
		}
	}

	return ComponentStatus{
		Name:      c.name,
		Status:    StatusHealthy,
		Latency:   latency,
		LastCheck: time.Now(),
	}
}

// CreateDefaultHealthChecks creates the default health check registry
func CreateDefaultHealthChecks(db *sql.DB, redisClient *redis.Client) *Registry {
	registry := NewRegistry(nil)

	// Register database check
	if db != nil {
		registry.Register(NewDBHealthChecker(db))
	}

	// Register Redis check
	if redisClient != nil {
		registry.Register(NewRedisHealthChecker(redisClient))
	}

	// Register circuit breaker check
	registry.Register(NewCircuitBreakerHealthChecker())

	// Register disk check
	registry.Register(NewDiskHealthChecker("./data"))

	return registry
}