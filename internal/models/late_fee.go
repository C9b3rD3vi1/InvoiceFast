package models

import (
	"time"
)

type LateFeeConfig struct {
	ID              string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID        string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	IsEnabled       bool      `json:"is_enabled" gorm:"default:false"`
	FeeType         string    `json:"fee_type"`                           // percentage, fixed
	FeeAmount       float64   `json:"fee_amount"`                         // e.g., 5% or 500 KES
	GracePeriodDays int       `json:"grace_period_days" gorm:"default:0"` // Days after due date before late fee applies
	MaxLateFees     float64   `json:"max_late_fees" gorm:"default:0"`     // Cap on total late fees per invoice
	ApplyOnTax      bool      `json:"apply_on_tax" gorm:"default:true"`   // Apply on top of tax
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (LateFeeConfig) TableName() string {
	return "late_fee_configs"
}

type LateFeeInvoice struct {
	ID        string     `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID  string     `json:"tenant_id" gorm:"type:uuid;index"`
	InvoiceID string     `json:"invoice_id" gorm:"type:uuid;index"`
	FeeAmount float64    `json:"fee_amount" gorm:"not null"`
	FeeType   string     `json:"fee_type"`
	Reason    string     `json:"reason"`
	AppliedAt time.Time  `json:"applied_at"`
	Waived    bool       `json:"waived" gorm:"default:false"`
	WaivedAt  *time.Time `json:"waived_at"`
	WaivedBy  string     `json:"waived_by"`
	CreatedAt time.Time  `json:"created_at"`
}

func (LateFeeInvoice) TableName() string {
	return "late_fee_invoices"
}
