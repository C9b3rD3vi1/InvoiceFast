package handlers

import (
	"database/sql"
	"fmt"
	"time"

	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// PublicHandler handles public-facing routes (landing, portal, auth)
type PublicHandler struct {
	invoiceService  *services.InvoiceService
	authService     *services.AuthService
	paymentService  *services.PaymentService
	intasendService *services.IntasendService
}

// NewPublicHandler creates a new PublicHandler
func NewPublicHandler(
	invoice *services.InvoiceService,
	auth *services.AuthService,
	payment *services.PaymentService,
	intasend *services.IntasendService,
) *PublicHandler {
	return &PublicHandler{
		invoiceService:  invoice,
		authService:     auth,
		paymentService:  payment,
		intasendService: intasend,
	}
}

// ServeLanding serves the landing page
func (h *PublicHandler) ServeLanding(c *fiber.Ctx) error {
	return c.Render("landing", fiber.Map{
		"Title":       "Home",
		"Description": "Professional invoicing with M-Pesa integration and KRA e-TIMS compliance",
	})
}

// ServePortal serves the client payment portal
func (h *PublicHandler) ServePortal(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Render("portal", fiber.Map{
			"Title":       "Invoice Not Found",
			"Description": "Invalid payment link",
			"Error":       "Invalid payment link. Please check the URL and try again.",
		})
	}

	// Fetch invoice by magic token
	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		// Token expired or invalid
		return c.Render("portal", fiber.Map{
			"Title":       "Invoice Unavailable",
			"Description": "This payment link is expired or invalid",
			"Error":       "This payment link has expired or is invalid. Please contact the sender for a new link.",
		})
	}

	// Mark invoice as viewed (if not already)
	if invoice.Status == models.InvoiceStatusSent {
		invoice.Status = models.InvoiceStatusViewed
		invoice.ViewedAt = sql.NullTime{Time: time.Now(), Valid: true}
		// Note: Status update handled by service layer in production
	}

	return c.Render("portal", fiber.Map{
		"Title":       fmt.Sprintf("Invoice %s", invoice.InvoiceNumber),
		"Description": fmt.Sprintf("View and pay invoice %s", invoice.InvoiceNumber),
		"Invoice":     invoice,
	})
}

// ServeSuccess serves the payment success page
func (h *PublicHandler) ServeSuccess(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Redirect("/")
	}

	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil || invoice.Status != models.InvoiceStatusPaid {
		return c.Redirect("/")
	}

	return c.Render("success", fiber.Map{
		"Title":       "Payment Successful",
		"Description": "Your payment has been confirmed",
		"Invoice":     invoice,
	})
}

// ServeLogin serves the login page
func (h *PublicHandler) ServeLogin(c *fiber.Ctx) error {
	return c.Render("auth/login", fiber.Map{
		"Title":       "Sign In",
		"Description": "Sign in to your InvoiceFast account",
	})
}

// ServeRegister serves the registration page
func (h *PublicHandler) ServeRegister(c *fiber.Ctx) error {
	plan := c.Query("plan", "free")
	return c.Render("auth/register", fiber.Map{
		"Title":       "Create Account",
		"Description": "Create your free InvoiceFast account",
		"Plan":        plan,
	})
}

// HandleLogin handles the login form submission via HTMX
func (h *PublicHandler) HandleLogin(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email and password are required",
		})
	}

	// Attempt login
	resp, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid email or password",
		})
	}

	// Set cookies
	cookieMaxAge := 3600 // 1 hour
	if req.Remember {
		cookieMaxAge = 30 * 24 * 3600 // 30 days
	}

	c.Cookie(&fiber.Cookie{
		Name:     "access_token",
		Value:    resp.AccessToken,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Strict",
		MaxAge:   cookieMaxAge,
	})

	c.Cookie(&fiber.Cookie{
		Name:     "refresh_token",
		Value:    resp.RefreshToken,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Strict",
		MaxAge:   30 * 24 * 3600,
	})

	return c.JSON(resp)
}

// HandleRegister handles the registration form submission via HTMX
func (h *PublicHandler) HandleRegister(c *fiber.Ctx) error {
	var req services.RegisterRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	// Validate required fields
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email, password, and name are required",
		})
	}

	// Validate password length
	if len(req.Password) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Password must be at least 8 characters",
		})
	}

	// Attempt registration
	resp, err := h.authService.Register(&req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Set cookies
	c.Cookie(&fiber.Cookie{
		Name:     "access_token",
		Value:    resp.AccessToken,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Strict",
		MaxAge:   3600,
	})

	c.Cookie(&fiber.Cookie{
		Name:     "refresh_token",
		Value:    resp.RefreshToken,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Strict",
		MaxAge:   30 * 24 * 3600,
	})

	return c.JSON(resp)
}

// GetInvoiceByToken returns invoice data by magic token (API endpoint)
func (h *PublicHandler) GetInvoiceByToken(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Token required",
		})
	}

	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Invoice not found",
		})
	}

	// Return only safe, public-facing data
	return c.JSON(fiber.Map{
		"invoice": fiber.Map{
			"id":             invoice.ID,
			"invoice_number": invoice.InvoiceNumber,
			"status":         invoice.Status,
			"total":          invoice.Total,
			"paid_amount":    invoice.PaidAmount,
			"currency":       invoice.Currency,
			"kes_equivalent": invoice.KESEquivalent,
			"due_date":       invoice.DueDate,
			"created_at":     invoice.CreatedAt,
			"client_name":    invoice.Client.Name,
			"company_name":   invoice.User.CompanyName,
			"logo_url":       invoice.LogoURL,
			"items":          invoice.Items,
			"kra_icn":        invoice.KRAICN,
			"kra_qr_code":    invoice.KRAQRCode,
			"magic_token":    invoice.MagicToken,
		},
	})
}

// InitiateSTKPush initiates an M-Pesa STK push for payment
func (h *PublicHandler) InitiateSTKPush(c *fiber.Ctx) error {
	var req struct {
		InvoiceToken string  `json:"invoice_token"`
		Phone        string  `json:"phone"`
		Amount       float64 `json:"amount"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	// Validate input
	if req.InvoiceToken == "" || req.Phone == "" || req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment details",
		})
	}

	// Fetch invoice
	invoice, err := h.invoiceService.GetInvoiceByMagicToken(req.InvoiceToken)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Invoice not found",
		})
	}

	// Check if already paid
	if invoice.Status == models.InvoiceStatusPaid {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invoice already paid",
		})
	}

	// Calculate remaining amount
	remainingAmount := invoice.Total - invoice.PaidAmount
	if remainingAmount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invoice already paid in full",
		})
	}

	// Use the provided amount or remaining amount
	paymentAmount := req.Amount
	if paymentAmount > remainingAmount {
		paymentAmount = remainingAmount
	}

	// Generate payment reference
	reference := fmt.Sprintf("PAY-%s", uuid.New().String()[:8])

	// Create payment record with PENDING status
	payment := &models.Payment{
		ID:          reference,
		TenantID:    invoice.TenantID,
		InvoiceID:   invoice.ID,
		UserID:      invoice.UserID,
		Amount:      paymentAmount,
		Currency:    invoice.Currency,
		Method:      models.PaymentMethodMpesa,
		Status:      models.PaymentStatusPending,
		PhoneNumber: req.Phone,
		Reference:   reference,
	}

	if err := h.invoiceService.RecordPayment(invoice.TenantID, invoice.ID, payment); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to initiate payment",
		})
	}

	// If Intasend service is configured, trigger STK Push
	if h.intasendService != nil {
		intasendReq := services.InitiatePaymentRequest{
			Amount:        paymentAmount,
			Currency:      invoice.Currency,
			PhoneNumber:   req.Phone,
			APIRef:        reference,
			CallbackURL:   fmt.Sprintf("%s/api/v1/webhooks/payment", c.BaseURL()),
			CustomerEmail: invoice.Client.Email,
			CustomerName:  invoice.Client.Name,
			InvoiceNumber: invoice.InvoiceNumber,
		}

		resp, err := h.intasendService.InitiateSTKPush(intasendReq)
		if err != nil {
			fmt.Printf("[STK Push] Failed to initiate: %v\n", err)
			return c.JSON(fiber.Map{
				"status":     "pending",
				"payment_id": reference,
				"amount":     paymentAmount,
				"phone":      req.Phone,
				"message":    "Payment recorded. STK push may be delayed.",
			})
		}

		return c.JSON(fiber.Map{
			"status":      "stk_push_sent",
			"payment_id":  reference,
			"intasend_id": resp.ID,
			"amount":      paymentAmount,
			"phone":       req.Phone,
			"message":     "STK push sent. Please check your phone and enter your M-Pesa PIN.",
		})
	}

	return c.JSON(fiber.Map{
		"status":     "pending",
		"payment_id": reference,
		"amount":     paymentAmount,
		"phone":      req.Phone,
		"message":    "Payment initiated. STK push service not configured.",
	})
}

// CheckPaymentStatus checks the status of a payment (for HTMX polling)
func (h *PublicHandler) CheckPaymentStatus(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Token required",
		})
	}

	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Invoice not found",
		})
	}

	// Return status fragment for HTMX swap
	if invoice.Status == models.InvoiceStatusPaid {
		return c.Render("partials/payment_success", fiber.Map{
			"Invoice": invoice,
		})
	}

	return c.Render("partials/payment_pending", fiber.Map{
		"Invoice": invoice,
	})
}

// GetPricing returns pricing based on currency (for HTMX toggle)
func (h *PublicHandler) GetPricing(c *fiber.Ctx) error {
	currency := c.Query("currency", "KES")

	prices := map[string]map[string]interface{}{
		"KES": {
			"free":   0,
			"pro":    999,
			"agency": 2499,
		},
		"USD": {
			"free":   0,
			"pro":    9.99,
			"agency": 24.99,
		},
	}

	selectedPrices, ok := prices[currency]
	if !ok {
		selectedPrices = prices["KES"]
		currency = "KES"
	}

	symbol := "KES "
	if currency == "USD" {
		symbol = "$"
	}

	// Format prices
	freePrice := fmt.Sprintf("%s%.0f", symbol, selectedPrices["free"])
	proPrice := fmt.Sprintf("%s%.2f", symbol, selectedPrices["pro"])
	agencyPrice := fmt.Sprintf("%s%.2f", symbol, selectedPrices["agency"])

	if currency == "KES" {
		proPrice = fmt.Sprintf("%s%.0f", symbol, selectedPrices["pro"])
		agencyPrice = fmt.Sprintf("%s%.0f", symbol, selectedPrices["agency"])
	}

	html := fmt.Sprintf(`
<div class="grid grid-cols-1 md:grid-cols-3 gap-8">
    <!-- Free Plan -->
    <div class="card p-8 hover:shadow-xl transition-shadow">
        <h3 class="text-xl font-bold text-slate-900 mb-2">Free</h3>
        <p class="text-slate-600 mb-6">For solo entrepreneurs</p>
        <div class="mb-6">
            <span class="text-4xl font-bold text-slate-900">%s</span>
            <span class="text-slate-600">/month</span>
        </div>
        <ul class="space-y-3 mb-8">
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                5 invoices/month
            </li>
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                Basic templates
            </li>
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                Email support
            </li>
        </ul>
        <a href="/register.html?plan=free" class="btn-outline w-full block text-center">Get Started</a>
    </div>

    <!-- Pro Plan -->
    <div class="card p-8 border-2 border-trust relative hover:shadow-xl transition-shadow">
        <div class="absolute -top-4 left-1/2 -translate-x-1/2 bg-trust text-white px-4 py-1 rounded-full text-sm font-medium">
            Most Popular
        </div>
        <h3 class="text-xl font-bold text-slate-900 mb-2">Pro</h3>
        <p class="text-slate-600 mb-6">For growing businesses</p>
        <div class="mb-6">
            <span class="text-4xl font-bold text-slate-900">%s</span>
            <span class="text-slate-600">/month</span>
        </div>
        <ul class="space-y-3 mb-8">
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                Unlimited invoices
            </li>
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                M-Pesa integration
            </li>
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                KRA e-TIMS compliance
            </li>
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                Custom branding
            </li>
        </ul>
        <a href="/register.html?plan=pro" class="btn-trust w-full block text-center">Start Free Trial</a>
    </div>

    <!-- Agency Plan -->
    <div class="card p-8 hover:shadow-xl transition-shadow">
        <h3 class="text-xl font-bold text-slate-900 mb-2">Agency</h3>
        <p class="text-slate-600 mb-6">For agencies & accountants</p>
        <div class="mb-6">
            <span class="text-4xl font-bold text-slate-900">%s</span>
            <span class="text-slate-600">/month</span>
        </div>
        <ul class="space-y-3 mb-8">
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                Everything in Pro
            </li>
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                Multi-client management
            </li>
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                API access
            </li>
            <li class="flex items-center gap-2 text-slate-600">
                <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                Priority support
            </li>
        </ul>
        <a href="/register.html?plan=agency" class="btn-outline w-full block text-center">Contact Sales</a>
    </div>
</div>
`, freePrice, proPrice, agencyPrice)

	return c.SendString(html)
}

// GetInvoicePDF serves the invoice PDF
func (h *PublicHandler) GetInvoicePDF(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Token required",
		})
	}

	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Invoice not found",
		})
	}

	// Generate PDF (placeholder - use actual PDF generation service)
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("inline; filename=invoice_%s.pdf", invoice.InvoiceNumber))

	return c.SendString("PDF content would be generated here")
}

// GetInvoiceReceipt serves the payment receipt PDF
func (h *PublicHandler) GetInvoiceReceipt(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Token required",
		})
	}

	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Invoice not found",
		})
	}

	if invoice.Status != models.InvoiceStatusPaid {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invoice not paid",
		})
	}

	// Generate receipt PDF
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=receipt_%s.pdf", invoice.InvoiceNumber))

	return c.SendString("Receipt PDF content would be generated here")
}

// SecurityHeaders middleware adds security headers
func SecurityHeaders(next fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		return next(c)
	}
}

// RateLimit middleware for public routes
func RateLimit(next fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Simple rate limiting - integrate with actual rate limiter
		return next(c)
	}
}
