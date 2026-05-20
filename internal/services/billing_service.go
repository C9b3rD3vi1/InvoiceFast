package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"invoicefast/internal/logger"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

	stripe "github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/webhook"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrNoSubscription = errors.New("no active subscription")
var ErrInvalidTransition = errors.New("invalid subscription state transition")
var ErrPlanNotFound = errors.New("plan not found")
var ErrWebhookUnauthorized = errors.New("webhook signature verification failed")

const (
	StatusActive    = "active"
	StatusTrialing  = "trialing"
	StatusPastDue  = "past_due"
	StatusCanceled = "canceled"
	StatusSuspended = "suspended"
	StatusExpired  = "expired"
)

type BillingService struct {
	db        *database.DB
	planSvc   *PlanService
	subSvc   *SubscriptionService
	stripeSvc *StripeService
	notifySvc *NotificationService
	cfg      *config.Config
}

func NewBillingService(db *database.DB, planSvc *PlanService, subSvc *SubscriptionService, stripeSvc *StripeService, notifySvc *NotificationService, cfg *config.Config) *BillingService {
	if stripeSvc != nil && cfg != nil && cfg.Stripe.SecretKey != "" {
		stripe.Key = cfg.Stripe.SecretKey
	}
	return &BillingService{
		db:        db,
		planSvc:   planSvc,
		subSvc:    subSvc,
		stripeSvc: stripeSvc,
		notifySvc: notifySvc,
		cfg:      cfg,
	}
}

func (s *BillingService) GetSubscription(tenantID string) (*models.Subscription, error) {
	return s.subSvc.GetSubscription(tenantID)
}

func (s *BillingService) GetActiveSubscription(tenantID string) (*models.Subscription, error) {
	return s.subSvc.GetActiveSubscription(tenantID)
}

type SubscriptionResponse struct {
	Subscription *models.Subscription    `json:"subscription"`
	Plan         *models.SubscriptionPlan `json:"plan"`
	Features     map[string]bool       `json:"features"`
	Limits       map[string]int       `json:"limits"`
	Usage        *models.UsageTracking `json:"usage"`
}

func (s *BillingService) GetSubscriptionWithDetails(tenantID string) (*SubscriptionResponse, error) {
	sub, err := s.subSvc.GetSubscription(tenantID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoSubscription
		}
		return nil, err
	}

	plan, err := s.planSvc.GetPlan(sub.PlanID)
	if err != nil {
		return nil, err
	}

	usage, err := s.subSvc.GetUsage(tenantID)
	if err != nil {
		usage = &models.UsageTracking{}
	}

	return &SubscriptionResponse{
		Subscription: sub,
		Plan:         plan,
		Features:     sliceToMap(plan.GetFeatures()),
		Limits:       plan.GetLimits(),
		Usage:        usage,
	}, nil
}

func sliceToMap(slice []string) map[string]bool {
	m := make(map[string]bool)
	for _, s := range slice {
		m[s] = true
	}
	return m
}

func (s *BillingService) HandleStripeWebhook(payload []byte, signature string) error {
	if s.cfg == nil || s.cfg.Stripe.WebhookSecret == "" {
		return errors.New("stripe webhook not configured")
	}

	event, err := webhook.ConstructEvent(payload, signature, s.cfg.Stripe.WebhookSecret)
	if err != nil {
		logger.Get().Error(context.Background(), "Stripe webhook signature verification failed", "error", err)
		return ErrWebhookUnauthorized
	}

	logger.Get().Info(context.Background(), "Stripe webhook received", "event_type", event.Type)
	return nil
}

func (s *BillingService) RecordBillingPayment(tx *models.SubscriptionTransaction) error {
	now := time.Now()
	if tx.ID == "" {
		tx.ID = uuid.New().String()
	}
	if tx.CreatedAt.IsZero() {
		tx.CreatedAt = now
	}
	if tx.UpdatedAt.IsZero() {
		tx.UpdatedAt = now
	}
	
	return s.db.Create(tx).Error
}

func (s *BillingService) CancelSubscription(tenantID string) error {
	return s.subSvc.CancelSubscription(tenantID)
}

func (s *BillingService) ReactivateSubscription(tenantID string) error {
	return s.subSvc.ReactivateSubscription(tenantID)
}

func (s *BillingService) ChangePlan(tenantID, newPlanID, billingCycle string) error {
	sub, err := s.subSvc.GetSubscription(tenantID)
	if err != nil {
		return err
	}

	newPlan, err := s.planSvc.GetPlan(newPlanID)
	if err != nil {
		return ErrPlanNotFound
	}

	oldPlan, err := s.planSvc.GetPlan(sub.PlanID)
	if err != nil {
		return err
	}

	if newPlan.Tier <= oldPlan.Tier {
		logger.Get().Info(context.Background(), "Downgrade plan", "from", oldPlan.Name, "to", newPlan.Name)
	} else {
		logger.Get().Info(context.Background(), "Upgrade plan", "from", oldPlan.Name, "to", newPlan.Name)
	}

	// Update subscription with new plan
	sub.PlanID = newPlanID
	sub.BillingCycle = billingCycle
	
	// Recalculate amount based on billing cycle
	var amount int64
	if billingCycle == "yearly" {
		amount = s.planSvc.GetYearlyPriceKES(newPlan)
	} else {
		amount = s.planSvc.GetMonthlyPriceKES(newPlan)
	}
	sub.Amount = amount
	sub.Currency = "KES"
	
	return s.db.Save(sub).Error
}

func (s *BillingService) ValidateFeature(tenantID, feature string) error {
	details, err := s.GetSubscriptionWithDetails(tenantID)
	if err != nil {
		return ErrNoSubscription
	}

	if details.Subscription.Status != StatusActive && details.Subscription.Status != StatusTrialing {
		return errors.New("subscription not active")
	}

	limits := details.Limits

	if maxInvoices, ok := limits["max_invoices_per_month"]; ok {
		if details.Usage.InvoicesUsed >= maxInvoices {
			return fmt.Errorf("invoice limit exceeded: %d/%d", details.Usage.InvoicesUsed, maxInvoices)
		}
	}

	if maxClients, ok := limits["max_clients"]; ok {
		if details.Usage.ClientsUsed >= maxClients {
			return fmt.Errorf("client limit exceeded: %d/%d", details.Usage.ClientsUsed, maxClients)
		}
	}

	if maxUsers, ok := limits["max_users"]; ok {
		if details.Usage.UsersUsed >= maxUsers {
			return fmt.Errorf("user limit exceeded: %d/%d", details.Usage.UsersUsed, maxUsers)
		}
	}

	return nil
}

func (s *BillingService) sendNotification(tenantID, eventType, message string) {
	if s.notifySvc == nil {
		return
	}

	s.notifySvc.Send(context.Background(), &NotificationRequest{
		TenantID:  tenantID,
		UserID:   tenantID,
		EventType: eventType,
		Channels: []string{ChannelEmail},
		Subject: "Billing Notification",
		Body:    message,
	})
}

func (s *BillingService) GetPaymentHistory(tenantID string, limit int) ([]models.SubscriptionTransaction, error) {
	var payments []models.SubscriptionTransaction
	query := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&payments).Error
	return payments, err
}

func (s *BillingService) GetBillingHistory(tenantID string, page, limit int) ([]models.SubscriptionTransaction, int64, int64) {
	var txs []models.SubscriptionTransaction
	var count int64
	
	offset := (page - 1) * limit
	s.db.Model(&models.SubscriptionTransaction{}).Where("tenant_id = ?", tenantID).Count(&count)
	
	s.db.Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&txs)

	_ = txs
	return txs, count, int64(offset)
}

func (s *BillingService) InitiateMpesaSubscription(tenantID, phone string, amount int64) (string, error) {
	return "", errors.New("M-Pesa subscription not implemented - use IntaSend")
}

type SavedPaymentMethod struct {
	ID        string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	IsDefault bool   `json:"is_default"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
}

func (s *BillingService) GetSavedPaymentMethods(tenantID string) []SavedPaymentMethod {
	sub, err := s.subSvc.GetSubscription(tenantID)
	if err != nil || sub == nil {
		return []SavedPaymentMethod{}
	}

	methods := []SavedPaymentMethod{}
	
	// Add payment method from subscription if available
	if sub.Provider != "" && sub.PaymentMethod != "" {
		method := SavedPaymentMethod{
			ID:        sub.ID,
			Type:      sub.PaymentMethod,
			IsDefault: true,
		}
		
		// Try to get last 4 digits from provider customer ID or metadata
		if sub.ProviderCustomerID != "" {
			if len(sub.ProviderCustomerID) >= 4 {
				method.Last4 = sub.ProviderCustomerID[len(sub.ProviderCustomerID)-4:]
			}
		}
		
		methods = append(methods, method)
	}
	
	return methods
}

func (s *BillingService) DeletePaymentMethod(tenantID, methodID string) error {
	return nil
}

func (s *BillingService) SetDefaultPaymentMethod(tenantID, methodID string) error {
	return s.db.Model(&models.Subscription{}).Where("tenant_id = ?", tenantID).Update("payment_method", methodID).Error
}

func (s *BillingService) UpdateSubscriptionPaymentMethod(tenantID, paymentMethod, provider string) error {
	sub, err := s.subSvc.GetSubscription(tenantID)
	if err != nil {
		return err
	}
	if sub == nil {
		return errors.New("no subscription found")
	}

	sub.PaymentMethod = paymentMethod
	if provider != "" {
		sub.Provider = provider
	}

	return s.subSvc.db.Save(sub).Error
}

func (s *BillingService) ProcessMpesaCallback(checkoutID, status string) error {
	logger.Get().Info(context.Background(), "M-Pesa callback received", "checkout_id", checkoutID, "status", status)
	// Note: M-Pesa callbacks should be handled via M-Pesa service directly
	// This is a placeholder for backward compatibility
	return nil
}