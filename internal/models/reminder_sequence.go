package models

import (
	"time"
)

type ReminderSequence struct {
	ID            string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID      string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	Name          string    `json:"name" gorm:"not null"`
	Description   string    `json:"description"`
	IsActive      bool      `json:"is_active" gorm:"default:true"`
	TriggerType   string    `json:"trigger_type"`              // due_soon, overdue, payment_received, invoice_sent
	TriggerDays   int       `json:"trigger_days"`              // Days relative to trigger (negative = before, positive = after)
	Channels      string    `json:"channels" gorm:"type:text"` // Stored as JSON: ["email","whatsapp"]
	EmailTemplate string    `json:"email_template"`
	SMSContent    string    `json:"sms_content"`
	WhatsAppMsg   string    `json:"whatsapp_message"`
	IsDefault     bool      `json:"is_default" gorm:"default:false"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (ReminderSequence) TableName() string {
	return "reminder_sequences"
}

type ReminderSequenceLog struct {
	ID         string    `json:"id" gorm:"type:uuid;primaryKey"`
	SequenceID string    `json:"sequence_id" gorm:"type:uuid;index"`
	InvoiceID  string    `json:"invoice_id" gorm:"type:uuid;index"`
	TenantID   string    `json:"tenant_id" gorm:"type:uuid;index"`
	Channel    string    `json:"channel"` // email, whatsapp, sms
	Status     string    `json:"status"`  // sent, failed
	Error      string    `json:"error"`
	SentAt     time.Time `json:"sent_at"`
	CreatedAt  time.Time `json:"created_at"`
}

func (ReminderSequenceLog) TableName() string {
	return "reminder_sequence_logs"
}
