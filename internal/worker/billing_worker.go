package worker

import (
	"context"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/models"
	"invoicefast/internal/services"
)

const workerPageSize = 100

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	now := time.Now()
	offset := 0

	for {
		var subs []models.Subscription
		if err := w.db.WithContext(ctx).
			Where("status = ? AND current_period_end <= ?", "active", now).
			Limit(workerPageSize).Offset(offset).
			Find(&subs).Error; err != nil {
			logger.Get().Error(ctx, "Error finding renewals", "error", err)
			return err
		}

		if len(subs) == 0 {
			break
		}

		for _, sub := range subs {
			logger.Get().Info(ctx, "Processing renewal for tenant", "tenant_id", sub.TenantID)

			if err := w.subService.ProcessRenewalPayment(sub.TenantID); err != nil {
				logger.Get().Error(ctx, "Renewal failed for tenant", "tenant_id", sub.TenantID, "error", err)
				continue
			}

			logger.Get().Info(ctx, "Renewal processed for tenant", "tenant_id", sub.TenantID)
		}

		offset += len(subs)
	}

	logger.Get().Info(ctx, "Processed renewals")
	return nil
}

func (w *BillingWorker) RetryFailedBilling() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	now := time.Now()
	offset := 0

	for {
		var subs []models.Subscription
		if err := w.db.WithContext(ctx).
			Where("status = ? AND retry_count < ? AND retry_count > 0", "past_due", 3).
			Limit(workerPageSize).Offset(offset).
			Find(&subs).Error; err != nil {
			logger.Get().Error(ctx, "Error finding failed payments", "error", err)
			return err
		}

		if len(subs) == 0 {
			break
		}

		for _, sub := range subs {
			if sub.LastPaymentAt != nil {
				nextRetry := sub.LastPaymentAt.Add(time.Duration(sub.RetryCount*24) * time.Hour)
				if now.Before(nextRetry) {
					continue
				}
			}

			logger.Get().Info(ctx, "Retrying payment for tenant", "tenant_id", sub.TenantID, "attempt", sub.RetryCount+1)

			if err := w.subService.ProcessRenewalPayment(sub.TenantID); err != nil {
				logger.Get().Error(ctx, "Retry failed for tenant", "tenant_id", sub.TenantID, "error", err)
			}
		}

		offset += len(subs)
	}

	logger.Get().Info(ctx, "Processed retry attempts")
	return nil
}

func (w *BillingWorker) ProcessTrialExpiry() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	now := time.Now()
	offset := 0

	for {
		var subs []models.Subscription
		if err := w.db.WithContext(ctx).
			Where("status = ? AND trial_ends_at <= ?", "trialing", now).
			Limit(workerPageSize).Offset(offset).
			Find(&subs).Error; err != nil {
			logger.Get().Error(ctx, "Error finding expired trials", "error", err)
			return err
		}

		if len(subs) == 0 {
			break
		}

		for _, sub := range subs {
			logger.Get().Info(ctx, "Processing expired trial for tenant", "tenant_id", sub.TenantID)

			if err := w.subService.SuspendSubscription(sub.TenantID, "Trial expired"); err != nil {
				logger.Get().Error(ctx, "Failed to suspend subscription", "tenant_id", sub.TenantID, "error", err)
				continue
			}

			logger.Get().Info(ctx, "Trial expired, tenant suspended", "tenant_id", sub.TenantID)
		}

		offset += len(subs)
	}

	logger.Get().Info(ctx, "Processed expired trials")
	return nil
}

func (w *BillingWorker) CancelExpiredSubscriptions() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	now := time.Now()
	offset := 0

	for {
		var subs []models.Subscription
		if err := w.db.WithContext(ctx).
			Where("status = ? AND expires_at <= ?", "canceled", now).
			Limit(workerPageSize).Offset(offset).
			Find(&subs).Error; err != nil {
			logger.Get().Error(ctx, "Error finding expired subscriptions", "error", err)
			return err
		}

		if len(subs) == 0 {
			break
		}

		for _, sub := range subs {
			logger.Get().Info(ctx, "Cleaning up expired subscription for tenant", "tenant_id", sub.TenantID)

			if err := w.db.WithContext(ctx).Model(&sub).Update("status", "expired").Error; err != nil {
				logger.Get().Error(ctx, "Failed to mark subscription as expired", "tenant_id", sub.TenantID, "error", err)
				continue
			}
		}

		offset += len(subs)
	}

	logger.Get().Info(ctx, "Processed expired subscriptions")
	return nil
}

func (w *BillingWorker) RunAllJobs() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	logger.Get().Info(ctx, "Starting billing jobs")

	w.ProcessTrialExpiry()
	w.ProcessSubscriptionRenewals()
	w.RetryFailedBilling()
	w.CancelExpiredSubscriptions()

	logger.Get().Info(ctx, "Billing jobs completed")
}
