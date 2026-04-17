package models

import (
	"time"
)

type ExpenseCategory struct {
	ID          string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID    string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	ParentID    string    `json:"parent_id" gorm:"type:uuid"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (ExpenseCategory) TableName() string {
	return "expense_categories"
}

type Expense struct {
	ID              string     `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID        string     `json:"tenant_id" gorm:"type:uuid;index;not null"`
	CategoryID      string     `json:"category_id" gorm:"type:uuid;index"`
	Title           string     `json:"title" gorm:"not null"`
	Description     string     `json:"description"`
	Amount          float64    `json:"amount" gorm:"not null"`
	Currency        string     `json:"currency" gorm:"default:KES"`
	Date            time.Time  `json:"date" gorm:"index"`
	Status          string     `json:"status" gorm:"default:pending"` // pending, approved, rejected, paid
	PaymentMethod   string     `json:"payment_method"`                // cash, bank, mpesa, card
	Reference       string     `json:"reference"`
	Vendor          string     `json:"vendor"`
	TaxAmount       float64    `json:"tax_amount"`
	TaxRate         float64    `json:"tax_rate"`
	IsRecurring     bool       `json:"is_recurring"`
	RecurringPeriod string     `json:"recurring_period"` // weekly, monthly, yearly
	Notes           string     `json:"notes"`
	Attachments     int        `json:"attachments"`
	CreatedBy       string     `json:"created_by" gorm:"type:uuid;index"`
	ApprovedBy      string     `json:"approved_by" gorm:"type:uuid"`
	ApprovedAt      *time.Time `json:"approved_at"`
	PaidAt          *time.Time `json:"paid_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (Expense) TableName() string {
	return "expenses"
}

type ExpenseAttachment struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey"`
	ExpenseID string    `json:"expense_id" gorm:"type:uuid;index;not null"`
	TenantID  string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	FileName  string    `json:"file_name"`
	FileURL   string    `json:"file_url"`
	FileSize  int64     `json:"file_size"`
	FileType  string    `json:"file_type"`
	CreatedAt time.Time `json:"created_at"`
}

func (ExpenseAttachment) TableName() string {
	return "expense_attachments"
}
