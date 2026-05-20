package handlers

import (
	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type BillingHandler struct {
	subService        *services.SubscriptionService
	planService       *services.PlanService
	billingSvc        *services.BillingService
	stripeSvc         *services.StripeService
	intasendSvc       *services.IntasendService
	exchangeRateSvc   *services.ExchangeRateService
	db                *database.DB
}

func NewBillingHandler(subSvc *services.SubscriptionService, planSvc *services.PlanService, billingSvc *services.BillingService, stripeSvc *services.StripeService, intasendSvc *services.IntasendService, exchangeRateSvc *services.ExchangeRateService, db *database.DB) *BillingHandler {
	return &BillingHandler{
		subService:      subSvc,
		planService:     planSvc,
		billingSvc:      billingSvc,
		stripeSvc:       stripeSvc,
		intasendSvc:     intasendSvc,
		exchangeRateSvc: exchangeRateSvc,
		db:              db,
	}
}

func (h *BillingHandler) GetSubscription(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	sub, plan, err := h.subService.GetSubscriptionWithPlan(tenantID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "subscription not found"})
	}

	usage, _ := h.subService.GetUsage(tenantID)
	
	// Get tenant currency
	tenant := models.Tenant{}
	currency := "KES" // Default
	if err := h.db.First(&tenant, "id = ?", tenantID).Error; err == nil {
		currency = tenant.Currency
		if currency == "" {
			currency = "KES"
		}
	}

	return c.JSON(fiber.Map{
		"subscription": sub,
		"plan":         plan,
		"usage":        usage,
		"currency":     currency,
	})
}

func (h *BillingHandler) GetPlans(c *fiber.Ctx) error {
	plans, err := h.planService.GetAllPlans()
	if err != nil {
		return c.JSON(fiber.Map{"error": "failed to fetch plans"})
	}

	type PlanResponse struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Slug            string `json:"slug"`
		Description     string `json:"description"`
		MonthlyPriceKES int64  `json:"monthly_price_kes"`
		YearlyPriceKES  int64  `json:"yearly_price_kes"`
		CustomPrice     bool   `json:"custom_price"`
		FeaturesJSON    string `json:"features_json"`
		LimitsJSON      string `json:"limits_json"`
		Popular         bool   `json:"popular"`
		SortOrder       int    `json:"sort_order"`
	}

	response := make([]PlanResponse, len(plans))
	for i, plan := range plans {
		monthly := h.planService.GetMonthlyPriceKES(&plan)
		yearly := h.planService.GetYearlyPriceKES(&plan)
		response[i] = PlanResponse{
			ID:              plan.ID,
			Name:            plan.Name,
			Slug:            plan.Slug,
			Description:     plan.Description,
			MonthlyPriceKES: monthly,
			YearlyPriceKES:  yearly,
			CustomPrice:     monthly <= 0 && yearly <= 0,
			FeaturesJSON:    plan.FeaturesJSON,
			LimitsJSON:      plan.LimitsJSON,
			Popular:         plan.SortOrder == 3,
			SortOrder:       plan.SortOrder,
		}
	}

	return c.JSON(fiber.Map{
		"plans": response,
		"rates": h.planService.GetAllExchangeRates(),
	})
}

func (h *BillingHandler) GetExchangeRates(c *fiber.Ctx) error {
	rates := h.exchangeRateSvc.GetAllRates()
	
	normalized := make(map[string]float64)
	// Convert to USD-based rates (USD = 1)
	for key, rate := range rates {
		parts := []string{}
		if idx := strings.Index(key, "/"); idx != -1 {
			parts = []string{key[:idx], key[idx+1:]}
		}
		if len(parts) == 2 {
			if parts[1] == "USD" {
				normalized[parts[0]] = rate
			} else if parts[0] == "USD" {
				normalized[parts[1]] = 1 / rate
			} else {
				normalized[key] = rate
			}
		}
	}
	
	// Add base rates
	normalized["USD"] = 1.0
	
	return c.JSON(fiber.Map{"rates": normalized})
}

func (h *BillingHandler) CreateCheckoutSession(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	var req struct {
		PlanID       string `json:"plan_id"`
		PaymentMethod string `json:"payment_method"` // stripe, mpesa
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.PlanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "plan_id required"})
	}

	paymentMethod := req.PaymentMethod
	if paymentMethod == "" {
		paymentMethod = "stripe"
	}

	plan, err := h.planService.GetPlan(req.PlanID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "plan not found"})
	}

	userEmail := c.Locals("user_id")
	emailStr, _ := userEmail.(string)

	baseURL := c.BaseURL() + "/billing"
	successURL := baseURL + "?success=true"
	cancelURL := baseURL + "?canceled=true"

	amount := plan.MonthlyPriceUSD
	
	// FREE PLAN - activate immediately without payment
	if amount == 0 {
		sub, err := h.subService.CreateSubscription(tenantID, req.PlanID, "monthly")
		if err != nil {
			logger.Get().Error(c.UserContext(), "CreateSubscription failed", "component", "CreateCheckoutSession", "error", err)
			if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "duplicate key") {
				existing, _, getErr := h.subService.GetSubscriptionWithPlan(tenantID)
				if getErr == nil && existing != nil {
					logger.Get().Info(c.UserContext(), "Upgrading existing subscription", "component", "CreateCheckoutSession")
					existing.Status = "active"
					existing.PlanID = req.PlanID
					sub = existing
				} else {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Subscription already exists"})
				}
			} else {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
		}
		
		return c.JSON(fiber.Map{
			"subscription":  sub,
			"success":     true,
			"message":    "Free plan activated",
			"activated":  true,
		})
	}
	
	// PAID PLAN - requires payment first
	logger.Get().Info(c.UserContext(), "Creating checkout session", "component", "CreateCheckoutSession", "plan", plan.Name, "amount", amount, "stripe_svc_available", h.stripeSvc != nil, "payment_method", paymentMethod)
	
	if paymentMethod == "mpesa" && h.intasendSvc != nil && amount > 0 {
		baseURL := c.BaseURL() + "/billing"
		
		tx, err := h.intasendSvc.InitiatePayment(&services.InitiatePaymentRequest{
			Amount:         float64(amount),
			Currency:      "KES",
			PhoneNumber:  c.Query("phone", ""),
			CustomerEmail: emailStr,
			CallbackURL:   baseURL + "/callback",
			APIRef:       "subscription_" + req.PlanID,
		})
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		
		return c.JSON(fiber.Map{
			"checkout_id":     tx.ID,
			"checkout_url":  tx.URL,
			"payment_method": "mpesa",
			"message":       "STK push sent to your phone",
		})
	}

	if h.stripeSvc != nil && amount > 0 && emailStr != "" {
		sessionURL, err := h.stripeSvc.CreateBillingSession(plan.Name, amount, emailStr, successURL, cancelURL)
		if err != nil {
			logger.Get().Error(c.UserContext(), "Failed to create stripe billing session", "error", err)
			logger.Get().Info(c.UserContext(), "Creating direct subscription instead (fallback)")
		} else {
			return c.JSON(fiber.Map{"url": sessionURL})
		}
	}
	
	// NO PAYMENT PROVIDER CONFIGURED - this should not happen in production
	return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
		"error": "No payment provider configured. Please contact support.",
	})
}

func (h *BillingHandler) CreateSubscription(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	var req struct {
		PlanID       string `json:"plan_id"`
		BillingCycle string `json:"billing_cycle"`
		TrialDays    int    `json:"trial_days"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.PlanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "plan_id required"})
	}

	billingCycle := "monthly"
	if req.BillingCycle != "" {
		billingCycle = req.BillingCycle
	}

	sub, err := h.subService.CreateSubscription(tenantID, req.PlanID, billingCycle)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"subscription": sub})
}

func (h *BillingHandler) CancelSubscription(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	if err := h.subService.CancelSubscription(tenantID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *BillingHandler) ReactivateSubscription(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	if err := h.subService.ReactivateSubscription(tenantID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *BillingHandler) ChangePlan(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	var req struct {
		PlanID       string `json:"plan_id"`
		BillingCycle string `json:"billing_cycle"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.PlanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "plan_id required"})
	}

	sub, err := h.subService.UpgradePlan(tenantID, req.PlanID, req.BillingCycle)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"subscription": sub})
}

func (h *BillingHandler) GetBillingHistory(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	txs, count, _ := h.billingSvc.GetBillingHistory(tenantID, page, limit)

	return c.JSON(fiber.Map{
		"transactions": txs,
		"total":        count,
		"page":         page,
		"limit":        limit,
	})
}

func (h *BillingHandler) InitiateMpesaPayment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	var req struct {
		Phone  string `json:"phone"`
		Amount int64  `json:"amount"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Phone == "" || req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "phone and amount required"})
	}

	txID, err := h.billingSvc.InitiateMpesaSubscription(tenantID, req.Phone, req.Amount)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"transaction_id": txID})
}

func (h *BillingHandler) GetPaymentMethods(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	methods := h.billingSvc.GetSavedPaymentMethods(tenantID)

	return c.JSON(fiber.Map{"payment_methods": methods})
}

func (h *BillingHandler) DeletePaymentMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	methodID := c.Params("id")
	if methodID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "payment method ID required"})
	}

	if err := h.billingSvc.DeletePaymentMethod(tenantID, methodID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

type UpdatePaymentMethodRequest struct {
	PaymentMethod string `json:"payment_method"`
	Provider      string `json:"provider"`
}

func (h *BillingHandler) UpdateSubscriptionPaymentMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	var req UpdatePaymentMethodRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.PaymentMethod == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "payment method is required"})
	}

	if err := h.billingSvc.UpdateSubscriptionPaymentMethod(tenantID, req.PaymentMethod, req.Provider); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Payment method updated"})
}

func (h *BillingHandler) SetDefaultPaymentMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	methodID := c.Params("id")
	if methodID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "payment method ID required"})
	}

	if err := h.billingSvc.SetDefaultPaymentMethod(tenantID, methodID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *BillingHandler) CheckLimits(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	resource := c.Params("resource")
	if resource == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "resource required"})
	}

	amount := c.QueryInt("amount", 1)

	allowed, reason, err := h.subService.CheckLimits(tenantID, resource, amount)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"allowed": allowed,
		"reason":  reason,
	})
}

func (h *BillingHandler) GetUsage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	usage, err := h.subService.GetUsage(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"usage": usage})
}

func (h *BillingHandler) HandleMpesaWebhook(c *fiber.Ctx) error {
	var payload struct {
		CheckoutRequestID string `json:"CheckoutRequestID"`
		ResultCode        string `json:"ResultCode"`
		Amount            string `json:"Amount"`
		PhoneNumber       string `json:"PhoneNumber"`
		TenantID          string `json:"tenant_id"`
	}

	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	if payload.CheckoutRequestID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "CheckoutRequestID required"})
	}

	// IDEMPOTENCY CHECK
	if svc, ok := c.Locals("idempotency_svc").(*services.IdempotencyService); ok {
		isProcessed, _ := svc.IsProcessed(c.Context(), "billing_mpesa:"+payload.CheckoutRequestID)
		if isProcessed {
			logger.Get().Info(c.UserContext(), "Already processed", "component", "BILLING", "checkout_request_id", payload.CheckoutRequestID)
			return c.JSON(fiber.Map{"success": true, "status": "already_processed"})
		}
	}

	status := "success"
	if payload.ResultCode != "0" {
		status = "failed"
	}

	if err := h.billingSvc.ProcessMpesaCallback(payload.CheckoutRequestID, status); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// MARK IDEMPOTENCY
	if svc, ok := c.Locals("idempotency_svc").(*services.IdempotencyService); ok {
		svc.MarkProcessed(c.Context(), "billing_mpesa:"+payload.CheckoutRequestID, map[string]interface{}{
			"tenant_id": payload.TenantID,
			"result":   status,
		})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *BillingHandler) HandleIntasendWebhook(c *fiber.Ctx) error {
	var payload struct {
		Event              string `json:"event"`
		Status             string `json:"status"`
		TenantID           string `json:"tenant_id"`
		Amount            int64  `json:"amount"`
		CheckoutRequestID string `json:"checkout_request_id"`
		CheckoutID        string `json:"checkout_id"`
		Message           string `json:"message"`
	}

	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	// IDEMPOTENCY CHECK
	iduKey := payload.CheckoutRequestID
	if iduKey == "" {
		iduKey = payload.CheckoutID
	}
	if iduKey != "" {
		if svc, ok := c.Locals("idempotency_svc").(*services.IdempotencyService); ok {
			isProcessed, _ := svc.IsProcessed(c.Context(), "billing:"+iduKey)
			if isProcessed {
				logger.Get().Info(c.UserContext(), "Already processed", "component", "BILLING", "idempotency_key", iduKey)
				return c.JSON(fiber.Map{"success": true, "status": "already_processed"})
			}
		}
	}

	// BILLING webhook - confirm payment and activate subscription
	if payload.Event == "checkout.complete" || payload.Status == "completed" {
		if payload.TenantID != "" {
			now := time.Now()
			tx := &models.SubscriptionTransaction{
				ID:                 uuid.New().String(),
				TenantID:           payload.TenantID,
				Provider:          "intasend",
				ProviderReference: payload.CheckoutRequestID,
				Status:            "completed",
				PaymentType:       "subscription",
				IdempotencyKey:    iduKey,
				PaidAt:            &now,
				CreatedAt:         now,
				UpdatedAt:         now,
			}
			
			if err := h.billingSvc.RecordBillingPayment(tx); err != nil {
				logger.Get().Error(c.UserContext(), "Failed to record payment", "component", "BILLING", "error", err)
			} else {
				sub, err := h.subService.GetActiveSubscription(payload.TenantID)
				if err != nil || sub == nil {
					logger.Get().Info(c.UserContext(), "Creating subscription for tenant", "component", "BILLING", "tenant_id", payload.TenantID)
					_, _ = h.subService.CreateSubscriptionWithTrial(payload.TenantID)
				} else {
					logger.Get().Info(c.UserContext(), "Subscription already active for tenant", "component", "BILLING", "tenant_id", payload.TenantID)
				}
			}
			
			// MARK IDEMPOTENCY
			if svc, ok := c.Locals("idempotency_svc").(*services.IdempotencyService); ok && iduKey != "" {
				svc.MarkProcessed(c.Context(), "billing:"+iduKey, map[string]interface{}{
					"tenant_id": payload.TenantID,
					"amount":   payload.Amount,
				})
			}
		}
	} else if payload.Event == "checkout.failed" || payload.Status == "failed" {
		logger.Get().Warn(c.UserContext(), "Payment failed", "component", "BILLING", "message", payload.Message)
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *BillingHandler) HandleStripeWebhook(c *fiber.Ctx) error {
	payload := c.BodyRaw()
	signature := c.Get("stripe-signature")

	if err := h.billingSvc.HandleStripeWebhook(payload, signature); err != nil {
		logger.Get().Error(c.UserContext(), "Stripe webhook handling failed", "error", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}
