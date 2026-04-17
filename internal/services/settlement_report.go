package services

import (
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

type SettlementReport struct {
	Date             string            `json:"date"`
	TotalAmount      float64           `json:"total_amount"`
	Currency         string            `json:"currency"`
	TransactionCount int               `json:"transaction_count"`
	SuccessCount     int               `json:"success_count"`
	FailedCount      int               `json:"failed_count"`
	Transactions     []SettlementItem  `json:"transactions"`
	Summary          SettlementSummary `json:"summary"`
}

type SettlementItem struct {
	Time        time.Time `json:"time"`
	Reference   string    `json:"reference"`
	Amount      float64   `json:"amount"`
	Status      string    `json:"status"`
	PhoneNumber string    `json:"phone_number"`
	InvoiceID   string    `json:"invoice_id,omitempty"`
}

type SettlementSummary struct {
	TotalDebited  float64 `json:"total_debited"`
	TotalPending  float64 `json:"total_pending"`
	AverageAmount float64 `json:"average_amount"`
	MinAmount     float64 `json:"min_amount"`
	MaxAmount     float64 `json:"max_amount"`
}

type MPaySettlementService struct {
	db *database.DB
}

func NewMPaySettlementService(db *database.DB) *MPaySettlementService {
	return &MPaySettlementService{db: db}
}

func (s *MPaySettlementService) GenerateDailySettlement(date time.Time) (*SettlementReport, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.AddDate(0, 0, 1)

	var payments []models.Payment
	err := s.db.Where("created_at BETWEEN ? AND ? AND method = ?", startOfDay, endOfDay, "mpesa").
		Order("created_at ASC").
		Find(&payments).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get payments: %w", err)
	}

	report := &SettlementReport{
		Date:         date.Format("2006-01-02"),
		Currency:     "KES",
		Transactions: make([]SettlementItem, 0, len(payments)),
	}

	var totalDebited, totalPending float64
	successCount := 0
	failedCount := 0

	for _, p := range payments {
		item := SettlementItem{
			Time:        p.CreatedAt,
			Reference:   p.Reference,
			Amount:      p.Amount,
			Status:      string(p.Status),
			PhoneNumber: p.PhoneNumber,
			InvoiceID:   p.InvoiceID,
		}
		report.Transactions = append(report.Transactions, item)

		if p.Status == models.PaymentStatusCompleted {
			totalDebited += p.Amount
			successCount++
		} else if p.Status == models.PaymentStatusPending {
			totalPending += p.Amount
			failedCount++
		}
	}

	report.TotalAmount = totalDebited + totalPending
	report.TransactionCount = len(payments)
	report.SuccessCount = successCount
	report.FailedCount = failedCount

	avgAmount := 0.0
	if len(payments) > 0 {
		avgAmount = report.TotalAmount / float64(len(payments))
	}

	minAmount := 0.0
	maxAmount := 0.0
	if len(payments) > 0 {
		minAmount = payments[0].Amount
		maxAmount = payments[0].Amount
		for _, p := range payments {
			if p.Amount < minAmount {
				minAmount = p.Amount
			}
			if p.Amount > maxAmount {
				maxAmount = p.Amount
			}
		}
	}

	report.Summary = SettlementSummary{
		TotalDebited:  totalDebited,
		TotalPending:  totalPending,
		AverageAmount: avgAmount,
		MinAmount:     minAmount,
		MaxAmount:     maxAmount,
	}

	return report, nil
}

func (s *MPaySettlementService) GetSettlementForDateRange(tenantID string, startDate, endDate time.Time) ([]SettlementReport, error) {
	reports := make([]SettlementReport, 0)

	for d := startDate; d.Before(endDate); d = d.AddDate(0, 0, 1) {
		report, err := s.GenerateDailySettlement(d)
		if err != nil {
			continue
		}
		if report.TransactionCount > 0 {
			reports = append(reports, *report)
		}
	}

	return reports, nil
}

func (s *MPaySettlementService) ExportSettlementCSV(date time.Time) (string, error) {
	report, err := s.GenerateDailySettlement(date)
	if err != nil {
		return "", err
	}

	csv := "Time,Reference,Amount,Status,Phone Number,Invoice ID\n"
	for _, t := range report.Transactions {
		csv += fmt.Sprintf("%s,%s,%.2f,%s,%s,%s\n",
			t.Time.Format("15:04:05"),
			t.Reference,
			t.Amount,
			t.Status,
			t.PhoneNumber,
			t.InvoiceID,
		)
	}

	csv += fmt.Sprintf("\nSummary\n")
	csv += fmt.Sprintf("Total Amount,%.2f\n", report.TotalAmount)
	csv += fmt.Sprintf("Total Debited,%.2f\n", report.Summary.TotalDebited)
	csv += fmt.Sprintf("Total Pending,%.2f\n", report.Summary.TotalPending)
	csv += fmt.Sprintf("Success Count,%d\n", report.SuccessCount)
	csv += fmt.Sprintf("Failed Count,%d\n", report.FailedCount)

	return csv, nil
}
