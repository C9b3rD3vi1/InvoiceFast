package handlers

import (
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"
	"log"

	"github.com/gofiber/fiber/v2"
)

type BillingHandler struct {
	subService  *services.SubscriptionService
	planService *services.PlanService
	billingSvc  *services.BillingService
}

func NewBillingHandler(subSvc *services.SubscriptionService, planSvc *services.PlanService, billingSvc *services.BillingService) *BillingHandler {
	return &BillingHandler{
		subService:  subSvc,
		planService: planSvc,
		billingSvc:  billingSvc,
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

	return c.JSON(fiber.Map{
		"subscription": sub,
		"plan":         plan,
		"usage":        usage,
	})
}

func (h *BillingHandler) GetPlans(c *fiber.Ctx) error {
	plans, err := h.planService.GetAllPlans()
	if err != nil {
		return c.JSON(fiber.Map{"error": "failed to fetch plans"})
	}

	return c.JSON(fiber.Map{"plans": plans})
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

	sub, err := h.subService.CreateSubscription(tenantID, req.PlanID)
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

	txs, count := h.billingSvc.GetBillingHistory(tenantID, page, limit)

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
	}

	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	if payload.CheckoutRequestID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "CheckoutRequestID required"})
	}

	status := "success"
	if payload.ResultCode != "0" {
		status = "failed"
	}

	if err := h.billingSvc.ProcessMpesaCallback(payload.CheckoutRequestID, status); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *BillingHandler) HandleIntasendWebhook(c *fiber.Ctx) error {
	var payload struct {
		Event     string `json:"event"`
		InvoiceID string `json:"invoice_id"`
		Status    string `json:"status"`
		TenantID  string `json:"tenant_id"`
	}

	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	if payload.Event == "payment.success" && payload.TenantID != "" {
		sub, err := h.subService.GetSubscription(payload.TenantID)
		if err == nil && sub != nil {
			if err := h.subService.ReactivateSubscription(payload.TenantID); err != nil {
				log.Printf("[BILLING] Failed to reactivate subscription: %v", err)
			}
		}
	}

	return c.JSON(fiber.Map{"success": true})
}
