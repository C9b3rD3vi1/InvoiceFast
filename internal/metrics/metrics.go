package metrics

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

var (
	// HTTP request metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"method", "endpoint"},
	)

	HTTPRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed",
		},
	)

	// Invoice metrics
	InvoicesCreatedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "invoices_created_total",
			Help: "Total number of invoices created",
		},
	)

	InvoicesSentTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "invoices_sent_total",
			Help: "Total number of invoices sent",
		},
	)

	InvoicesPaidTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "invoices_paid_total",
			Help: "Total number of invoices paid",
		},
	)

	InvoiceGenerationDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "invoice_generation_duration_seconds",
			Help:    "Time taken to generate invoice PDF",
			Buckets: []float64{.5, 1, 2, 3, 5, 10},
		},
	)

	InvoicesByStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "invoices_by_status",
			Help: "Number of invoices by status",
		},
		[]string{"status"},
	)

	// Payment metrics
	PaymentsInitiatedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "payments_initiated_total",
			Help: "Total number of payment requests initiated",
		},
	)

	PaymentsCompletedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "payments_completed_total",
			Help: "Total number of successful payments",
		},
	)

	PaymentsFailedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "payments_failed_total",
			Help: "Total number of failed payments",
		},
	)

	PaymentAmountTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payment_amount_total",
			Help: "Total payment amount processed",
		},
		[]string{"currency", "status"},
	)

	STKPushDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "stk_push_duration_seconds",
			Help:    "Time taken for STK Push to complete",
			Buckets: []float64{1, 2, 5, 10, 30, 60},
		},
	)

	// KRA metrics
	KRASubmissionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "kra_submissions_total",
			Help: "Total number of KRA submissions",
		},
	)

	KRASubmissionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "kra_submission_duration_seconds",
			Help:    "Time taken for KRA submission",
			Buckets: []float64{1, 2, 5, 10, 30, 60},
		},
	)

	KRAFailedSubmissions = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "kra_failed_submissions_total",
			Help: "Total number of failed KRA submissions",
		},
	)

	// Database metrics
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "Database query duration in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"operation", "table"},
	)

	DBConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "db_connections_active",
			Help: "Number of active database connections",
		},
	)

	DBConnectionsIdle = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "db_connections_idle",
			Help: "Number of idle database connections",
		},
	)

	// Email metrics
	EmailsSentTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "emails_sent_total",
			Help: "Total number of emails sent",
		},
	)

	EmailsFailedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "emails_failed_total",
			Help: "Total number of failed email sends",
		},
	)

	EmailSendDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "email_send_duration_seconds",
			Help:    "Time taken to send email",
			Buckets: []float64{.1, .5, 1, 2, 5, 10},
		},
	)

	// SMS/WhatsApp metrics
	SMSSentTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "sms_sent_total",
			Help: "Total number of SMS sent",
		},
	)

	SMSFailedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "sms_failed_total",
			Help: "Total number of failed SMS",
		},
	)

	WhatsAppSentTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "whatsapp_sent_total",
			Help: "Total number of WhatsApp messages sent",
		},
	)

	WhatsAppFailedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "whatsapp_failed_total",
			Help: "Total number of failed WhatsApp messages",
		},
	)

	// Subscription/Billing metrics
	ActiveSubscriptions = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_subscriptions",
			Help: "Number of active subscriptions by plan",
		},
		[]string{"plan"},
	)

	SubscriptionChurnTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "subscription_churn_total",
			Help: "Total number of subscription cancellations",
		},
	)

	RevenueTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "revenue_total",
			Help: "Total revenue by currency",
		},
		[]string{"currency", "plan"},
	)

	// System metrics
	ActiveUsers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_users",
			Help: "Number of currently active users",
		},
	)

	ActiveClients = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_clients",
			Help: "Number of active clients",
		},
	)

	// Error metrics
	ErrorTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "errors_total",
			Help: "Total number of errors",
		},
		[]string{"type", "endpoint"},
	)
)

// ============================================================
// Middleware for HTTP metrics
// ============================================================

// PrometheusMiddleware returns Fiber middleware for Prometheus metrics
func PrometheusMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Increment in-flight requests
		HTTPRequestsInFlight.Inc()
		defer HTTPRequestsInFlight.Dec()

		err := c.Next()

		// Record metrics
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Response().StatusCode())
		endpoint := c.Route().Path
		if endpoint == "" {
			endpoint = "unknown"
		}

		// Total requests
		HTTPRequestsTotal.WithLabelValues(c.Method(), endpoint, status).Inc()

		// Request duration
		HTTPRequestDuration.WithLabelValues(c.Method(), endpoint).Observe(duration)

		// Record errors
		if c.Response().StatusCode() >= 500 {
			ErrorTotal.WithLabelValues("5xx", endpoint).Inc()
		} else if c.Response().StatusCode() >= 400 {
			ErrorTotal.WithLabelValues("4xx", endpoint).Inc()
		}

		return err
	}
}

// ============================================================
// Helper functions for recording metrics
// ============================================================

// RecordInvoiceCreated increments invoice created counter
func RecordInvoiceCreated() {
	InvoicesCreatedTotal.Inc()
}

// RecordInvoiceSent increments invoice sent counter
func RecordInvoiceSent() {
	InvoicesSentTotal.Inc()
}

// RecordInvoicePaid increments invoice paid counter
func RecordInvoicePaid() {
	InvoicesPaidTotal.Inc()
}

// RecordInvoiceGenerationDuration records PDF generation time
func RecordInvoiceGenerationDuration(duration float64) {
	InvoiceGenerationDuration.Observe(duration)
}

// RecordPaymentInitiated increments payment initiated counter
func RecordPaymentInitiated() {
	PaymentsInitiatedTotal.Inc()
}

// RecordPaymentCompleted records successful payment
func RecordPaymentCompleted(amount float64, currency string) {
	PaymentsCompletedTotal.Inc()
	PaymentAmountTotal.WithLabelValues(currency, "completed").Add(amount)
}

// RecordPaymentFailed increments failed payment counter
func RecordPaymentFailed() {
	PaymentsFailedTotal.Inc()
	PaymentAmountTotal.WithLabelValues("KES", "failed").Inc()
}

// RecordSTKPushDuration records STK Push duration
func RecordSTKPushDuration(duration float64) {
	STKPushDuration.Observe(duration)
}

// RecordKRASubmission records KRA submission
func RecordKRASubmission(duration float64, success bool) {
	KRASubmissionsTotal.Inc()
	KRASubmissionDuration.Observe(duration)
	if !success {
		KRAFailedSubmissions.Inc()
	}
}

// RecordDBQuery records database query duration
func RecordDBQuery(operation, table string, duration float64) {
	DBQueryDuration.WithLabelValues(operation, table).Observe(duration)
}

// RecordEmailSent records email sent
func RecordEmailSent() {
	EmailsSentTotal.Inc()
}

// RecordEmailFailed records email failure
func RecordEmailFailed() {
	EmailsFailedTotal.Inc()
}

// RecordEmailDuration records email send duration
func RecordEmailDuration(duration float64) {
	EmailSendDuration.Observe(duration)
}

// RecordSMSSent records SMS sent
func RecordSMSSent() {
	SMSSentTotal.Inc()
}

// RecordSMSFailed records SMS failure
func RecordSMSFailed() {
	SMSFailedTotal.Inc()
}

// RecordWhatsAppSent records WhatsApp sent
func RecordWhatsAppSent() {
	WhatsAppSentTotal.Inc()
}

// RecordWhatsAppFailed records WhatsApp failure
func RecordWhatsAppFailed() {
	WhatsAppFailedTotal.Inc()
}

// RecordActiveSubscriptions records subscription counts
func RecordActiveSubscriptions(plan string, count float64) {
	ActiveSubscriptions.WithLabelValues(plan).Set(count)
}

// RecordSubscriptionChurn records subscription cancellation
func RecordSubscriptionChurn() {
	SubscriptionChurnTotal.Inc()
}

// RecordRevenue records revenue
func RecordRevenue(currency, plan string, amount float64) {
	RevenueTotal.WithLabelValues(currency, plan).Add(amount)
}

// RecordError records an error occurrence
func RecordError(errType, endpoint string) {
	ErrorTotal.WithLabelValues(errType, endpoint).Inc()
}

// SetDBConnections sets database connection counts
func SetDBConnections(active, idle int) {
	DBConnectionsActive.Set(float64(active))
	DBConnectionsIdle.Set(float64(idle))
}

// SetActiveUsers sets the number of active users
func SetActiveUsers(count float64) {
	ActiveUsers.Set(count)
}

// SetActiveClients sets the number of active clients
func SetActiveClients(count float64) {
	ActiveClients.Set(count)
}

// UpdateInvoicesByStatus updates invoice counts by status
func UpdateInvoicesByStatus(status string, count float64) {
	InvoicesByStatus.WithLabelValues(status).Set(count)
}

// Handler returns a Fiber handler for the /metrics endpoint
func Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())(c.Context())
		return nil
	}
}