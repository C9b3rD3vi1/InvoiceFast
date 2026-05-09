package services

import (
	"fmt"
	"invoicefast/internal/database"
	"invoicefast/internal/models"
	"log"
	"time"

	"github.com/google/uuid"
)

type PlanService struct {
	db *database.DB
}

func NewPlanService(db *database.DB) *PlanService {
	return &PlanService{db: db}
}

func (s *PlanService) GetPlan(idOrSlug string) (*models.SubscriptionPlan, error) {
	var plan models.SubscriptionPlan
	err := s.db.Where("id = ? OR slug = ?", idOrSlug, idOrSlug).First(&plan).Error
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s *PlanService) GetAllPlans() ([]models.SubscriptionPlan, error) {
	var plans []models.SubscriptionPlan
	err := s.db.Where("is_active = ?", true).Order("sort_order ASC").Find(&plans).Error
	return plans, err
}

func (s *PlanService) CreatePlan(plan *models.SubscriptionPlan) error {
	if plan.ID == "" {
		plan.ID = uuid.New().String()
	}
	return s.db.Create(plan).Error
}

func (s *PlanService) UpdatePlan(id string, updates map[string]interface{}) error {
	return s.db.Model(&models.SubscriptionPlan{}).Where("id = ?", id).Updates(updates).Error
}

func (s *PlanService) GetExchangeRate() float64 {
	return 150.0
}

func (s *PlanService) GetMonthlyPriceKES(plan *models.SubscriptionPlan) int64 {
	rate := s.GetExchangeRate()
	return int64(float64(plan.MonthlyPriceUSD) * rate)
}

func (s *PlanService) GetYearlyPriceKES(plan *models.SubscriptionPlan) int64 {
	rate := s.GetExchangeRate()
	return int64(float64(plan.YearlyPriceUSD) * rate)
}

func (s *PlanService) SeedDefaultPlans() error {
	plans := []models.SubscriptionPlan{
		{
			ID:              uuid.New().String(),
			Name:            "Starter",
			Slug:            "starter",
			Description:     "Perfect for small businesses. Includes unlimited invoices, clients, payment tracking, reports, PDF export, and email reminders.",
			MonthlyPriceUSD: 1000,
			YearlyPriceUSD:  10000,
			FeaturesJSON:    `["invoices","clients","payments","reports","pdf_export","email_reminders"]`,
			LimitsJSON:      `{"invoices":-1,"clients":-1,"users":1,"storage":1073741824}`,
			IsActive:        true,
			SortOrder:       1,
			TrialDays:       14,
		},
		{
			ID:              uuid.New().String(),
			Name:            "Growth",
			Slug:            "growth",
			Description:     "For growing businesses. Includes all Starter features plus team members, advanced analytics, branding customization, and priority support.",
			MonthlyPriceUSD: 2500,
			YearlyPriceUSD:  25000,
			FeaturesJSON:    `["invoices","clients","payments","reports","pdf_export","email_reminders","team_members","advanced_analytics","branding","priority_support"]`,
			LimitsJSON:      `{"invoices":-1,"clients":-1,"users":5,"storage":5368709120}`,
			IsActive:        true,
			SortOrder:       2,
			TrialDays:       14,
		},
		{
			ID:              uuid.New().String(),
			Name:            "Business",
			Slug:            "business",
			Description:     "Full-featured business solution. Includes all Growth features plus automation tools, API access, bulk actions, and workflow automation.",
			MonthlyPriceUSD: 5000,
			YearlyPriceUSD:  50000,
			FeaturesJSON:    `["invoices","clients","payments","reports","pdf_export","email_reminders","team_members","advanced_analytics","branding","priority_support","automation","api_access","bulk_actions","workflow_automation"]`,
			LimitsJSON:      `{"invoices":-1,"clients":-1,"users":-1,"storage":107374182400}`,
			IsActive:        true,
			SortOrder:       3,
			TrialDays:       14,
		},
		{
			ID:              uuid.New().String(),
			Name:            "Enterprise",
			Slug:            "enterprise",
			Description:     "Contact us for custom pricing. Unlimited everything with dedicated support, SLA, custom integrations, and white-label options.",
			MonthlyPriceUSD: 0,
			YearlyPriceUSD:  0,
			FeaturesJSON:    `["invoices","clients","payments","reports","pdf_export","email_reminders","team_members","advanced_analytics","branding","priority_support","automation","api_access","bulk_actions","workflow_automation","dedicated_support","sla","custom_integrations","whitelabel"]`,
			LimitsJSON:      `{"invoices":-1,"clients":-1,"users":-1,"storage":-1}`,
			IsActive:        true,
			SortOrder:       4,
			TrialDays:       14,
		},
	}

	for _, plan := range plans {
		var existing models.SubscriptionPlan
		if err := s.db.First(&existing, "slug = ?", plan.Slug).Error; err != nil {
			if err.Error() == "record not found" {
				s.db.Create(&plan)
			}
		}
	}

	return nil
}

func (s *PlanService) MigrateUsersWithoutSubscription() error {
	var tenants []models.Tenant
	if err := s.db.Find(&tenants).Error; err != nil {
		return err
	}

	starterPlan, err := s.GetPlanBySlug("starter")
	if err != nil {
		return fmt.Errorf("starter plan not found: %w", err)
	}

	now := time.Now()
	trialEnd := now.AddDate(0, 0, 14)
	migrated := 0

	for _, tenant := range tenants {
		var existingSub models.Subscription
		err := s.db.Where("tenant_id = ? AND status IN ?", tenant.ID, []string{"trialing", "active", "past_due", "canceled"}).First(&existingSub).Error
		if err == nil {
			continue
		}

		subscription := &models.Subscription{
			ID:                 uuid.New().String(),
			TenantID:           tenant.ID,
			PlanID:             starterPlan.ID,
			Status:             "trialing",
			BillingCycle:       "monthly",
			Amount:             starterPlan.MonthlyPriceUSD,
			Currency:           "USD",
			TrialStart:         &now,
			TrialEnd:           &trialEnd,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   trialEnd,
			CreatedAt:          now,
			UpdatedAt:          now,
		}

		if err := s.db.Create(subscription).Error; err != nil {
			log.Printf("[Migrate] Failed to create trial for tenant %s: %v", tenant.ID, err)
			continue
		}

		var usage models.UsageTracking
		if err := s.db.First(&usage, "tenant_id = ?", tenant.ID).Error; err != nil {
			usage = models.UsageTracking{
				ID:           uuid.New().String(),
				TenantID:     tenant.ID,
				InvoicesUsed: 0,
				ClientsUsed:  0,
				UsersUsed:    1,
				UpdatedAt:    now,
			}
			s.db.Create(&usage)
		}

		migrated++
		log.Printf("[Migrate] Assigned trial subscription to tenant: %s", tenant.ID)
	}

	log.Printf("[Migrate] Total users migrated to trial: %d", migrated)
	return nil
}

func (s *PlanService) GetPlanBySlug(slug string) (*models.SubscriptionPlan, error) {
	var plan models.SubscriptionPlan
	if err := s.db.Where("slug = ?", slug).First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}
