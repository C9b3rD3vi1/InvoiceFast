package models

import (
	"time"
)

// KRAInvoice represents a KRA-compliant invoice
type KRAInvoice struct {
	ID             string       `json:"id" db:"id"`
	InvoiceNumber  string       `json:"invoice_number" db:"invoice_number"`
	InvoiceDate    time.Time    `json:"invoice_date" db:"invoice_date"`
	UserID         string       `json:"user_id" db:"user_id"`
	Taxpayer       KRATaxpayer  `json:"taxpayer"`
	Buyer          KRABuyer     `json:"buyer"`
	Items          []KRAItem    `json:"items"`
	TotalAmount    float64      `json:"total_amount" db:"total_amount"`
	TaxAmount      float64      `json:"tax_amount" db:"tax_amount"`
	Discount       float64      `json:"discount" db:"discount"`
	ControlNumber  string       `json:"control_number" db:"control_number"`
	QRCode         string       `json:"qr_code" db:"qr_code"`
	Status         string       `json:"status" db:"status"` // pending, registered, cancelled
	RegisteredAt   *time.Time   `json:"registered_at" db:"registered_at"`
	CreatedAt      time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at" db:"updated_at"`
}

// KRATaxpayer represents the taxpayer (seller)
type KRATaxpayer struct {
	TIN         string `json:"tin" db:"tin"`
	BranchCode  string `json:"branch_code" db:"branch_code"`
	CompanyName string `json:"company_name" db:"company_name"`
	Address     string `json:"address" db:"address"`
	Phone       string `json:"phone" db:"phone"`
	Email       string `json:"email" db:"email"`
}

// KRABuyer represents the buyer/customer
type KRABuyer struct {
	TIN     string `json:"tin" db:"tin"`
	Name    string `json:"name" db:"name"`
	Address string `json:"address" db:"address"`
	Phone   string `json:"phone" db:"phone"`
	Email   string `json:"email" db:"email"`
}

// KRAItem represents a line item
type KRAItem struct {
	ID             string  `json:"id" db:"id"`
	InvoiceID      string  `json:"invoice_id" db:"invoice_id"`
	ItemCode       string  `json:"item_code" db:"item_code"`
	Description   string  `json:"description" db:"description"`
	ItemDescription string `json:"item_description" db:"item_description"`
	Quantity      float64 `json:"quantity" db:"quantity"`
	UnitOfMeasure string  `json:"unit_of_measure" db:"unit_of_measure"`
	UnitPrice     float64 `json:"unit_price" db:"unit_price"`
	Discount      float64 `json:"discount" db:"discount"`
	DiscountRate  float64 `json:"discount_rate" db:"discount_rate"`
	TaxRate       float64 `json:"tax_rate" db:"tax_rate"`
	TaxAmount     float64 `json:"tax_amount" db:"tax_amount"`
	TotalAmount   float64 `json:"total_amount" db:"total_amount"`
}

// KRAConfig represents KRA configuration
type KRAConfig struct {
	ID            string `json:"id" db:"id"`
	UserID        string `json:"user_id" db:"user_id"`
	Enabled       bool   `json:"enabled" db:"enabled"`
	TIN           string `json:"tin" db:"tin"`
	BranchCode    string `json:"branch_code" db:"branch_code"`
	Mode          string `json:"mode" db:"mode"` // sandbox, production
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// KRASummary represents tax summary for reporting
type KRASummary struct {
	Period       string  `json:"period"`
	TotalSales   float64 `json:"total_sales"`
	TotalVAT     float64 `json:"total_vat"`
	TotalExempt  float64 `json:"total_exempt"`
	InvoiceCount int     `json:"invoice_count"`
}
