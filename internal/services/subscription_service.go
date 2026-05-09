package services

import (
	"context"
	"fmt"
	"invoicefast/internal/database"
	"invoicefast/internal/models"
	"time"

	"github.com/google/uuid"
)

type SubscriptionService struct {
	db          *database.DB
	planService *PlanService
	notifySvc  *NotificationService
}

func NewSubscriptionService(db *database.DB, planSvc *PlanService, notifySvc *NotificationService) *SubscriptionService {
	return &SubscriptionService{
		db:          db,
		planService: planSvc,
		notifySvc:  notifySvc,
	}
}

func (s *SubscriptionService) GetSubscription(tenantID string) (*models.Subscription, error) {
	var sub models.Subscription
	if err := s.db.Where("tenant_id = ?", tenantID).First(&sub).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *SubscriptionService) GetActiveSubscription(tenantID string) (*models.Subscription, error) {
	var sub models.Subscription
	if err := s.db.Where("tenant_id = ? AND status IN ?", tenantID, []string{"active", "trialing"}).
		First(&sub).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *SubscriptionService) CreateSubscriptionWithTrial(tenantID string) (*models.Subscription, error) {
	starterPlan, err := s.planService.GetPlan("starter")
	if err != nil {
		return nil, err
	}

	trialStart := time.Now()
	trialEnd := trialStart.AddDate(0, 0, 14)

	sub := &models.Subscription{
		ID:                 uuid.New().String(),
		TenantID:           tenantID,
		PlanID:             starterPlan.ID,
		Status:             "trialing",
		BillingCycle:       "monthly",
		Amount:             starterPlan.MonthlyPriceUSD,
		Currency:           "USD",
		TrialStart:         &trialStart,
		TrialEnd:           &trialEnd,
		CurrentPeriodStart: trialStart,
		CurrentPeriodEnd:   trialEnd,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := s.db.Create(sub).Error; err != nil {
		return nil, err
	}

	s.InitUsageTracking(tenantID)
	return sub, nil
}

func (s *SubscriptionService) CreateSubscription(tenantID, planID string, billingCycle string, opts ...func(*models.Subscription)) (*models.Subscription, error) {
	plan, err := s.planService.GetPlan(planID)
	if err != nil {
		return nil, err
	}

	var amount int64
	if billingCycle == "yearly" {
		amount = plan.YearlyPriceUSD
	} else {
		amount = plan.MonthlyPriceUSD
	}

	now := time.Now()
	sub := &models.Subscription{
		ID:                 uuid.New().String(),
		TenantID:           tenantID,
		PlanID:             planID,
		Status:             "active",
		BillingCycle:       billingCycle,
		Amount:             amount,
		Currency:           "USD",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0), // 1 month from now
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	for _, opt := range opts {
		opt(sub)
	}

	if err := s.db.Create(sub).Error; err != nil {
		return nil, err
	}

	s.InitUsageTracking(tenantID)
	s.sendSubscriptionNotification(tenantID, sub, EventSubCreated)
	return sub, nil
}

func (s *SubscriptionService) CancelSubscription(tenantID string) error {
	sub, err := s.GetSubscription(tenantID)
	if err != nil {
		return err
	}

	now := time.Now()
	sub.CanceledAt = &now
	sub.Status = "canceled"
	sub.UpdatedAt = now

	return s.db.Save(sub).Error
}

func (s *SubscriptionService) SuspendSubscription(tenantID, reason string) error {
	sub, err := s.GetSubscription(tenantID)
	if err != nil {
		return err
	}

	now := time.Now()
	sub.Status = "past_due"
	sub.PastDueSince = &now
	sub.PauseReason = reason
	sub.UpdatedAt = now

	return s.db.Save(sub).Error
}

func (s *SubscriptionService) ReactivateSubscription(tenantID string) error {
	sub, err := s.GetSubscription(tenantID)
	if err != nil {
		return err
	}

	sub.Status = "active"
	sub.PastDueSince = nil
	sub.PausedAt = nil
	sub.RetryCount = 0
	sub.UpdatedAt = time.Now()

	return s.db.Save(sub).Error
}

func (s *SubscriptionService) UpgradePlan(tenantID, newPlanID string, billingCycle string) (*models.Subscription, error) {
	sub, err := s.GetSubscription(tenantID)
	if err != nil {
		return nil, err
	}

	newPlan, err := s.planService.GetPlan(newPlanID)
	if err != nil {
		return nil, err
	}

	sub.PlanID = newPlanID
	sub.BillingCycle = billingCycle

	if billingCycle == "yearly" {
		sub.Amount = newPlan.YearlyPriceUSD
	} else {
		sub.Amount = newPlan.MonthlyPriceUSD
	}

	sub.UpdatedAt = time.Now()
	s.db.Save(sub)

	return sub, nil
}

func (s *SubscriptionService) DowngradePlan(tenantID, newPlanID string) (*models.Subscription, error) {
	sub, err := s.GetSubscription(tenantID)
	if err != nil {
		return nil, err
	}

	newPlan, err := s.planService.GetPlan(newPlanID)
	if err != nil {
		return nil, err
	}

	sub.PlanID = newPlanID
	sub.Amount = newPlan.MonthlyPriceUSD
	sub.UpdatedAt = time.Now()

	s.db.Save(sub)
	return sub, nil
}

func (s *SubscriptionService) ExtendTrial(tenantID string, days int) error {
	sub, err := s.GetSubscription(tenantID)
	if err != nil {
		return err
	}

	if sub.TrialEnd == nil {
		now := time.Now()
		sub.TrialEnd = &now
	}

	newEnd := sub.TrialEnd.Add(time.Duration(days) * 24 * time.Hour)
	sub.TrialEnd = &newEnd
	sub.UpdatedAt = time.Now()

	return s.db.Save(sub).Error
}

func (s *SubscriptionService) ProcessRenewalPayment(tenantID string) error {
	sub, err := s.GetActiveSubscription(tenantID)
	if err != nil {
		return err
	}

	sub.RetryCount++
	sub.LastPaymentError = "renewal_pending"
	sub.UpdatedAt = time.Now()

	if sub.RetryCount >= 3 {
		sub.Status = "suspended"
	}

	if err := s.db.Save(sub).Error; err != nil {
		return err
	}

	s.sendSubscriptionNotification(tenantID, sub, EventSubRenewed)
	return nil
}

func (s *SubscriptionService) RecordTransaction(sub *models.Subscription, txType, status, failureReason string) error {
	tx := models.SubscriptionTransaction{
		ID:             uuid.New().String(),
		SubscriptionID: sub.ID,
		TenantID:       sub.TenantID,
		PlanID:         sub.PlanID,
		Amount:         sub.Amount,
		Currency:       sub.Currency,
		Provider:       sub.Provider,
		Status:         status,
		Type:           txType,
		FailureReason:  failureReason,
		CreatedAt:      time.Now(),
	}

	if status == "completed" {
		now := time.Now()
		tx.PaidAt = &now
	}

	return s.db.Create(&tx).Error
}

func (s *SubscriptionService) GetTransactions(tenantID string, limit int) []models.SubscriptionTransaction {
	var txs []models.SubscriptionTransaction
	s.db.Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Find(&txs)
	return txs
}

func (s *SubscriptionService) InitUsageTracking(tenantID string) error {
	usage := models.UsageTracking{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		InvoicesUsed: 0,
		ClientsUsed:  0,
		UsersUsed:    1,
		UpdatedAt:    time.Now(),
	}
	return s.db.Create(&usage).Error
}

func (s *SubscriptionService) GetUsage(tenantID string) (*models.UsageTracking, error) {
	var usage models.UsageTracking
	if err := s.db.Where("tenant_id = ?", tenantID).First(&usage).Error; err != nil {
		if err.Error() == "record not found" {
			s.InitUsageTracking(tenantID)
			return s.GetUsage(tenantID)
		}
		return nil, err
	}
	return &usage, nil
}

func (s *SubscriptionService) IncrementUsage(tenantID, resource string, amount int) error {
	usage, err := s.GetUsage(tenantID)
	if err != nil {
		return err
	}

	switch resource {
	case "invoices":
		usage.InvoicesUsed += amount
	case "clients":
		usage.ClientsUsed += amount
	case "users":
		usage.UsersUsed += amount
	case "storage":
		usage.StorageUsed += int64(amount)
	}

	usage.UpdatedAt = time.Now()
	return s.db.Save(usage).Error
}

func (s *SubscriptionService) CheckLimits(tenantID, resource string, amount int) (bool, string, error) {
	sub, err := s.GetActiveSubscription(tenantID)
	if err != nil {
		return false, "no_active_subscription", nil
	}

	if sub.Status == "suspended" {
		return false, "subscription_suspended", nil
	}

	plan, err := s.planService.GetPlan(sub.PlanID)
	if err != nil {
		return false, "", err
	}

	usage, err := s.GetUsage(tenantID)
	if err != nil {
		return false, "", err
	}

	limits := plan.GetLimits()
	if limits == nil {
		return true, "", nil
	}

	switch resource {
	case "invoices":
		if limit, ok := limits["invoices"]; ok && limit > 0 {
			if usage.InvoicesUsed+amount > limit {
				return false, "invoice_limit_exceeded", nil
			}
		}
	case "clients":
		if limit, ok := limits["clients"]; ok && limit > 0 {
			if usage.ClientsUsed+amount > limit {
				return false, "client_limit_exceeded", nil
			}
		}
	case "users":
		if limit, ok := limits["users"]; ok && limit > 0 {
			if usage.UsersUsed+amount > limit {
				return false, "user_limit_exceeded", nil
			}
		}
	}

	return true, "", nil
}

func (s *SubscriptionService) HasFeature(tenantID, feature string) bool {
	sub, err := s.GetActiveSubscription(tenantID)
	if err != nil {
		return false
	}

	plan, err := s.planService.GetPlan(sub.PlanID)
	if err != nil {
		return false
	}

	return plan.HasFeature(feature)
}

func (s *SubscriptionService) GetSubscriptionWithPlan(tenantID string) (*models.Subscription, *models.SubscriptionPlan, error) {
	sub, err := s.GetSubscription(tenantID)
	if err != nil {
		return nil, nil, err
	}

	plan, err := s.planService.GetPlan(sub.PlanID)
	if err != nil {
		return sub, nil, err
	}

	return sub, plan, nil
}

func (s *SubscriptionService) sendSubscriptionNotification(tenantID string, sub *models.Subscription, eventType string) {
	if s.notifySvc == nil {
		return
	}

	plan, _ := s.planService.GetPlan(sub.PlanID)
	planName := "Unknown"
	if plan != nil {
		planName = plan.Name
	}

	amountStr := fmt.Sprintf("%s %d", sub.Currency, sub.Amount)
	body := ""
	subject := ""
	switch eventType {
	case EventSubCreated:
		subject = "Subscription Activated"
		body = fmt.Sprintf("Your subscription to the %s plan has been activated. Amount: %s", planName, amountStr)
	case EventSubRenewed:
		subject = "Subscription Renewed"
		body = fmt.Sprintf("Your subscription has been renewed. Plan: %s, Amount: %s", planName, amountStr)
	case EventSubExpiring:
		subject = "Subscription Expiring Soon"
		body = fmt.Sprintf("Your subscription will expire in %d days. Plan: %s", sub.DaysUntilRenewal(), planName)
	}

	s.notifySvc.Send(context.Background(), &NotificationRequest{
		TenantID:  tenantID,
		UserID:   tenantID,
		EventType: eventType,
		Channels: []string{ChannelEmail},
		Subject:  subject,
		Body:     body,
		Variables: map[string]string{
			"plan_name": planName,
			"amount":   amountStr,
			"status":   sub.Status,
		},
	})
}
