package worker

import (
	"context"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/models"
	"invoicefast/internal/services"
)

type BillingWorker struct {
	db         *database.DB
	subService *services.SubscriptionService
	billingSvc *services.BillingService
}

func NewBillingWorker(db *database.DB, subSvc *services.SubscriptionService, billingSvc *services.BillingService) *BillingWorker {
	return &BillingWorker{
		db:         db,
		subService: subSvc,
		billingSvc: billingSvc,
	}
}

func (w *BillingWorker) ProcessSubscriptionRenewals() error {
	var subs []models.Subscription
	now := time.Now()

	if err := w.db.Where("status = ? AND renews_at <= ?", "active", now).
		Find(&subs).Error; err != nil {
		logger.Get().Error(context.Background(), "Error finding renewals", "error", err)
		return err
	}

	for _, sub := range subs {
		logger.Get().Info(context.Background(), "Processing renewal for tenant", "tenant_id", sub.TenantID)

		if err := w.subService.ProcessRenewalPayment(sub.TenantID); err != nil {
			logger.Get().Error(context.Background(), "Renewal failed for tenant", "tenant_id", sub.TenantID, "error", err)
			continue
		}

		logger.Get().Info(context.Background(), "Renewal processed for tenant", "tenant_id", sub.TenantID)
	}

	logger.Get().Info(context.Background(), "Processed renewals", "count", len(subs))
	return nil
}

func (w *BillingWorker) RetryFailedBilling() error {
	var subs []models.Subscription
	now := time.Now()

	if err := w.db.Where("status = ? AND retry_count < ? AND retry_count > 0", "suspended", 3).
		Find(&subs).Error; err != nil {
		logger.Get().Error(context.Background(), "Error finding failed payments", "error", err)
		return err
	}

	for _, sub := range subs {
		if sub.LastPaymentAt != nil {
			nextRetry := sub.LastPaymentAt.Add(time.Duration(sub.RetryCount*24) * time.Hour)
			if now.Before(nextRetry) {
				continue
			}
		}

		logger.Get().Info(context.Background(), "Retrying payment for tenant", "tenant_id", sub.TenantID, "attempt", sub.RetryCount+1)

		if err := w.subService.ProcessRenewalPayment(sub.TenantID); err != nil {
			logger.Get().Error(context.Background(), "Retry failed for tenant", "tenant_id", sub.TenantID, "error", err)
		}
	}

	logger.Get().Info(context.Background(), "Processed retry attempts", "count", len(subs))
	return nil
}

func (w *BillingWorker) ProcessTrialExpiry() error {
	var subs []models.Subscription
	now := time.Now()

	if err := w.db.Where("status = ? AND trial_ends_at <= ?", "trialing", now).
		Find(&subs).Error; err != nil {
		logger.Get().Error(context.Background(), "Error finding expired trials", "error", err)
		return err
	}

	for _, sub := range subs {
		logger.Get().Info(context.Background(), "Processing expired trial for tenant", "tenant_id", sub.TenantID)

		if err := w.subService.SuspendSubscription(sub.TenantID, "Trial expired"); err != nil {
			logger.Get().Error(context.Background(), "Failed to suspend subscription", "tenant_id", sub.TenantID, "error", err)
			continue
		}

		logger.Get().Info(context.Background(), "Trial expired, tenant suspended", "tenant_id", sub.TenantID)
	}

	logger.Get().Info(context.Background(), "Processed expired trials", "count", len(subs))
	return nil
}

func (w *BillingWorker) CancelExpiredSubscriptions() error {
	var subs []models.Subscription
	now := time.Now()

	if err := w.db.Where("status = ? AND expires_at <= ?", "canceled", now).
		Find(&subs).Error; err != nil {
		logger.Get().Error(context.Background(), "Error finding expired subscriptions", "error", err)
		return err
	}

	for _, sub := range subs {
		logger.Get().Info(context.Background(), "Cleaning up expired subscription for tenant", "tenant_id", sub.TenantID)
	}

	logger.Get().Info(context.Background(), "Processed expired subscriptions", "count", len(subs))
	return nil
}

func (w *BillingWorker) RunAllJobs() {
	logger.Get().Info(context.Background(), "Starting billing jobs")

	w.ProcessTrialExpiry()
	w.ProcessSubscriptionRenewals()
	w.RetryFailedBilling()
	w.CancelExpiredSubscriptions()

	logger.Get().Info(context.Background(), "Billing jobs completed")
}
