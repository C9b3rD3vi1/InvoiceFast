package models

import (
	"database/sql"
	"time"
)

type EmailTracking struct {
	ID         string       `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID   string       `json:"tenant_id" gorm:"type:uuid;index"`
	InvoiceID  string       `json:"invoice_id" gorm:"type:uuid;index"`
	Recipient  string       `json:"recipient" gorm:"index"`
	Subject    string       `json:"subject"`
	EmailType  string       `json:"email_type"` // invoice, reminder, receipt
	SentAt     time.Time    `json:"sent_at"`
	OpenedAt   sql.NullTime `json:"opened_at"`
	ClickedAt  sql.NullTime `json:"clicked_at"`
	OpenCount  int          `json:"open_count" gorm:"default:0"`
	ClickCount int          `json:"click_count" gorm:"default:0"`
	UserAgent  string       `json:"user_agent"`
	IPAddress  string       `json:"ip_address"`
	CreatedAt  time.Time    `json:"created_at"`
}

func (EmailTracking) TableName() string {
	return "email_tracking"
}

type EmailTrackingLink struct {
	ID            string    `json:"id" gorm:"type:uuid;primaryKey"`
	TrackingID    string    `json:"tracking_id" gorm:"type:uuid;index"`
	OriginalURL   string    `json:"original_url" gorm:"index"`
	RedirectCount int       `json:"redirect_count" gorm:"default:0"`
	CreatedAt     time.Time `json:"created_at"`
}

func (EmailTrackingLink) TableName() string {
	return "email_tracking_links"
}
