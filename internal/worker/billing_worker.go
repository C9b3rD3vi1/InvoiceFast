package worker

import (
	"invoicefast/internal/database"
	"invoicefast/internal/models"
	"invoicefast/internal/services"
	"log"
	"time"
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
		log.Printf("[BillingWorker] Error finding renewals: %v", err)
		return err
	}

	for _, sub := range subs {
		log.Printf("[BillingWorker] Processing renewal for tenant: %s", sub.TenantID)

		if err := w.subService.ProcessRenewalPayment(sub.TenantID); err != nil {
			log.Printf("[BillingWorker] Renewal failed for %s: %v", sub.TenantID, err)
			continue
		}

		log.Printf("[BillingWorker] Renewal processed for tenant: %s", sub.TenantID)
	}

	log.Printf("[BillingWorker] Processed %d renewals", len(subs))
	return nil
}

func (w *BillingWorker) RetryFailedBilling() error {
	var subs []models.Subscription
	now := time.Now()

	if err := w.db.Where("status = ? AND retry_count < ? AND retry_count > 0", "suspended", 3).
		Find(&subs).Error; err != nil {
		log.Printf("[BillingWorker] Error finding failed payments: %v", err)
		return err
	}

	for _, sub := range subs {
		if sub.LastPaymentAt != nil {
			nextRetry := sub.LastPaymentAt.Add(time.Duration(sub.RetryCount*24) * time.Hour)
			if now.Before(nextRetry) {
				continue
			}
		}

		log.Printf("[BillingWorker] Retrying payment for tenant: %s (attempt %d)", sub.TenantID, sub.RetryCount+1)

		if err := w.subService.ProcessRenewalPayment(sub.TenantID); err != nil {
			log.Printf("[BillingWorker] Retry failed for %s: %v", sub.TenantID, err)
		}
	}

	log.Printf("[BillingWorker] Processed %d retry attempts", len(subs))
	return nil
}

func (w *BillingWorker) ProcessTrialExpiry() error {
	var subs []models.Subscription
	now := time.Now()

	if err := w.db.Where("status = ? AND trial_ends_at <= ?", "trialing", now).
		Find(&subs).Error; err != nil {
		log.Printf("[BillingWorker] Error finding expired trials: %v", err)
		return err
	}

	for _, sub := range subs {
		log.Printf("[BillingWorker] Processing expired trial for tenant: %s", sub.TenantID)

		if err := w.subService.SuspendSubscription(sub.TenantID, "Trial expired"); err != nil {
			log.Printf("[BillingWorker] Failed to suspend %s: %v", sub.TenantID, err)
			continue
		}

		log.Printf("[BillingWorker] Trial expired, tenant suspended: %s", sub.TenantID)
	}

	log.Printf("[BillingWorker] Processed %d expired trials", len(subs))
	return nil
}

func (w *BillingWorker) CancelExpiredSubscriptions() error {
	var subs []models.Subscription
	now := time.Now()

	if err := w.db.Where("status = ? AND expires_at <= ?", "canceled", now).
		Find(&subs).Error; err != nil {
		log.Printf("[BillingWorker] Error finding expired subscriptions: %v", err)
		return err
	}

	for _, sub := range subs {
		log.Printf("[BillingWorker] Cleaning up expired subscription for tenant: %s", sub.TenantID)
	}

	log.Printf("[BillingWorker] Processed %d expired subscriptions", len(subs))
	return nil
}

func (w *BillingWorker) RunAllJobs() {
	log.Println("[BillingWorker] Starting billing jobs...")

	w.ProcessTrialExpiry()
	w.ProcessSubscriptionRenewals()
	w.RetryFailedBilling()
	w.CancelExpiredSubscriptions()

	log.Println("[BillingWorker] Billing jobs completed")
}
