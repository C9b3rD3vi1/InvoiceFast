package services

import (
	"invoicefast/internal/database"
	"invoicefast/internal/models"

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

func (s *PlanService) SeedDefaultPlans() error {
	plans := []models.SubscriptionPlan{
		{
			ID:           uuid.New().String(),
			Name:         "Starter",
			Slug:         "starter",
			Description:  "Perfect for small businesses",
			MonthlyPrice: 2900,
			YearlyPrice:  29000,
			FeaturesJSON: `["invoices","clients","payments"]`,
			LimitsJSON:   `{"invoices":50,"clients":25,"users":1}`,
			IsActive:     true,
			SortOrder:    1,
		},
		{
			ID:           uuid.New().String(),
			Name:         "Professional",
			Slug:         "professional",
			Description:  "For growing businesses",
			MonthlyPrice: 7900,
			YearlyPrice:  79000,
			FeaturesJSON: `["invoices","clients","payments","reports","automations","api"]`,
			LimitsJSON:   `{"invoices":500,"clients":200,"users":5}`,
			IsActive:     true,
			SortOrder:    2,
		},
		{
			ID:           uuid.New().String(),
			Name:         "Enterprise",
			Slug:         "enterprise",
			Description:  "Unlimited everything",
			MonthlyPrice: 19900,
			YearlyPrice:  199000,
			FeaturesJSON: `["invoices","clients","payments","reports","automations","api","dedicated_support"]`,
			LimitsJSON:   `{"invoices":-1,"clients":-1,"users":-1}`,
			IsActive:     true,
			SortOrder:    3,
		},
	}

	for _, plan := range plans {
		var existing models.SubscriptionPlan
		if err := s.db.First(&existing, "slug = ?", plan.Slug).Error; err != nil {
			s.db.Create(&plan)
		}
	}

	return nil
}
