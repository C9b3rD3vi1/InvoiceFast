package services

import (
	"fmt"
	"log"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

// ReminderService handles automated payment reminders
type ReminderService struct {
	db           *database.DB
	emailService *EmailService
	waService    *WhatsAppService
}

// ReminderConfig for configuring reminder schedules
type ReminderConfig struct {
	// Days after invoice due date to send reminders
	DaysBeforeDue   int // e.g., 3 (send reminder 3 days before due)
	DaysAfterDue    int // e.g., 1, 7, 14, 30
	EnableEmail     bool
	EnableWhatsApp  bool
	EnableSMS       bool
	LateFeePercent  float64
	LateFeeCap      float64
	GracePeriodDays int
}

var defaultReminderConfig = ReminderConfig{
	DaysBeforeDue:   3,
	DaysAfterDue:    []int{1, 7, 14, 30}[0], // Just do 1 day for now
	EnableEmail:     true,
	EnableWhatsApp:  true,
	EnableSMS:       false,
	LateFeePercent:  0,
	LateFeeCap:      5000,
	GracePeriodDays: 3,
}

// NewReminderService creates a new reminder service
func NewReminderService(db *database.DB, email *EmailService, wa *WhatsAppService) *ReminderService {
	return &ReminderService{
		db:           db,
		emailService: email,
		waService:    wa,
	}
}

// RunReminders checks and sends due reminders
func (s *ReminderService) RunReminders() error {
	log.Println("🔔 Running scheduled reminders...")

	// Get all sent invoices that are overdue or due soon
	var invoices []models.Invoice
	now := time.Now().UTC()

	// Find invoices due in X days
	upcomingDue := now.AddDate(0, 0, defaultReminderConfig.DaysBeforeDue)
	s.db.Where("status IN ? AND due_date <= ?",
		[]string{string(models.InvoiceStatusSent), string(models.InvoiceStatusViewed)},
		upcomingDue,
	).Find(&invoices)

	// Send "due soon" reminders
	for _, inv := range invoices {
		if err := s.sendDueSoonReminder(&inv); err != nil {
			log.Printf("Error sending reminder for %s: %v", inv.InvoiceNumber, err)
		}
	}

	// Find overdue invoices
	s.db.Where("status IN ? AND due_date < ?",
		[]string{string(models.InvoiceStatusSent), string(models.InvoiceStatusViewed)},
		now,
	).Find(&invoices)

	// Send overdue reminders
	for _, inv := range invoices {
		daysOverdue := int(now.Sub(inv.DueDate).Hours() / 24)

		// Only remind at specific intervals
		if daysOverdue == 1 || daysOverdue == 7 || daysOverdue == 14 || daysOverdue == 30 {
			if err := s.sendOverdueReminder(&inv, daysOverdue); err != nil {
				log.Printf("Error sending overdue reminder for %s: %v", inv.InvoiceNumber, err)
			}
		}

		// Apply late fee after grace period
		if daysOverdue > defaultReminderConfig.GracePeriodDays && defaultReminderConfig.LateFeePercent > 0 {
			if err := s.applyLateFee(&inv, daysOverdue); err != nil {
				log.Printf("Error applying late fee for %s: %v", inv.InvoiceNumber, err)
			}
		}
	}

	// Mark heavily overdue as "at risk"
	s.db.Model(&models.Invoice{}).
		Where("status IN ? AND due_date < ?",
			[]string{string(models.InvoiceStatusSent), string(models.InvoiceStatusViewed)},
			now.AddDate(0, 0, -60),
		).Update("status", models.InvoiceStatusOverdue)

	log.Println("✅ Reminders completed")
	return nil
}

func (s *ReminderService) sendDueSoonReminder(invoice *models.Invoice) error {
	// Check if reminder already sent today
	var existing models.Reminder
	err := s.db.Where("invoice_id = ? AND type = ? AND created_at > ?",
		invoice.ID, "due_soon", time.Now().UTC().AddDate(0, 0, -1),
	).First(&existing).Error

	if err == nil {
		return nil // Already sent
	}

	log.Printf("📧 Sending due soon reminder for invoice %s", invoice.InvoiceNumber)

	// Load client
	var client models.Client
	s.db.First(&client, invoice.ClientID)

	// Load user
	var user models.User
	s.db.First(&user, invoice.UserID)

	// Send email
	if defaultReminderConfig.EnableEmail && client.Email != "" {
		emailData := &ReminderEmailData{
			CompanyName:   user.CompanyName,
			ClientName:    client.Name,
			ClientEmail:   client.Email,
			InvoiceNumber: invoice.InvoiceNumber,
			Amount:        invoice.Total,
			Currency:      invoice.Currency,
			DueDate:       FormatDate(invoice.DueDate),
			DaysOverdue:   0,
		}
		s.emailService.SendPaymentReminder(emailData)
	}

	// Send WhatsApp
	if defaultReminderConfig.EnableWhatsApp && client.Phone != "" {
		waMsg := fmt.Sprintf("⏰ Invoice %s is due on %s\nAmount: %s %.2f\n\nPlease arrange payment.",
			invoice.InvoiceNumber,
			FormatDate(invoice.DueDate),
			invoice.Currency,
			invoice.Total,
		)
		log.Printf("📱 [WOULD SEND WHATSAPP]: %s", waMsg)
	}

	// Log reminder
	s.logReminder(invoice.UserID, invoice.ID, "due_soon")

	return nil
}

func (s *ReminderService) sendOverdueReminder(invoice *models.Invoice, daysOverdue int) error {
	// Check if reminder already sent for this interval
	reminderType := fmt.Sprintf("overdue_%d", daysOverdue)
	var existing models.Reminder
	err := s.db.Where("invoice_id = ? AND type = ? AND created_at > ?",
		invoice.ID, reminderType, time.Now().UTC().AddDate(0, 0, -2),
	).First(&existing).Error

	if err == nil {
		return nil // Already sent
	}

	log.Printf("📧 Sending overdue reminder for invoice %s (day %d)", invoice.InvoiceNumber, daysOverdue)

	// Load client
	var client models.Client
	s.db.First(&client, invoice.ClientID)

	// Load user
	var user models.User
	s.db.First(&user, invoice.UserID)

	balanceDue := invoice.Total - invoice.PaidAmount

	// Send email
	if defaultReminderConfig.EnableEmail && client.Email != "" {
		emailData := &ReminderEmailData{
			CompanyName:   user.CompanyName,
			ClientName:    client.Name,
			ClientEmail:   client.Email,
			InvoiceNumber: invoice.InvoiceNumber,
			Amount:        balanceDue,
			Currency:      invoice.Currency,
			DueDate:       FormatDate(invoice.DueDate),
			DaysOverdue:   daysOverdue,
		}
		s.emailService.SendPaymentReminder(emailData)
	}

	// Send WhatsApp
	if defaultReminderConfig.EnableWhatsApp && client.Phone != "" {
		msg := fmt.Sprintf("⚠️ Payment Overdue: Invoice %s\nDays Overdue: %d\nAmount: %s %.2f\n\nPlease pay immediately to avoid late fees.",
			invoice.InvoiceNumber,
			daysOverdue,
			invoice.Currency,
			balanceDue,
		)
		log.Printf("📱 [WOULD SEND WHATSAPP]: %s", msg)
	}

	// Log reminder
	s.logReminder(invoice.UserID, invoice.ID, reminderType)

	return nil
}

func (s *ReminderService) applyLateFee(invoice *models.Invoice, daysOverdue int) error {
	// Only apply once
	if invoice.TaxRate > 0 && invoice.TaxRate < 100 {
		return nil // Already has late fee (tax_rate used as late fee indicator)
	}

	// Calculate late fee
	lateFee := (invoice.Total - invoice.PaidAmount) * (defaultReminderConfig.LateFeePercent / 100)

	// Cap the late fee
	if lateFee > defaultReminderConfig.LateFeeCap {
		lateFee = defaultReminderConfig.LateFeeCap
	}

	if lateFee <= 0 {
		return nil
	}

	log.Printf("💰 Applying late fee of %.2f to invoice %s", lateFee, invoice.InvoiceNumber)

	// Update invoice
	invoice.TaxRate = defaultReminderConfig.LateFeePercent // Reuse field for late fee indicator
	invoice.TaxAmount = lateFee
	invoice.Total = invoice.Subtotal + lateFee - invoice.Discount

	// Note: in production, add actual late fee line item
	s.db.Save(invoice)

	return nil
}

func (s *ReminderService) logReminder(userID, invoiceID, reminderType string) {
	reminder := &models.Reminder{
		ID:          fmt.Sprintf("rem-%d", time.Now().Unix()),
		UserID:      userID,
		InvoiceID:   invoiceID,
		Type:        reminderType,
		Status:      "sent",
		ScheduledAt: time.Now().UTC(),
	}
	s.db.Create(reminder)
}

// ReminderSchedule stores reminder configuration
type ReminderSchedule struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	InvoiceID     string    `json:"invoice_id"`
	ReminderType  string    `json:"reminder_type"` // due_soon, overdue_1, overdue_7, etc.
	ScheduledFor  time.Time `json:"scheduled_for"`
	Status        string    `json:"status"` // pending, sent, failed
	SentAt        time.Time `json:"sent_at"`
	FailureReason string    `json:"failure_reason,omitempty"`
}

// ScheduleReminder creates a future reminder
func (s *ReminderService) ScheduleReminder(invoiceID, reminderType string, sendAt time.Time) error {
	schedule := &ReminderSchedule{
		ID:           fmt.Sprintf("sch-%d", time.Now().UnixNano()),
		InvoiceID:    invoiceID,
		ReminderType: reminderType,
		ScheduledFor: sendAt,
		Status:       "pending",
	}

	// In production, store in database
	log.Printf("📅 Scheduled reminder for invoice %s at %s", invoiceID, sendAt)
	return nil
}

// CancelReminder cancels a scheduled reminder
func (s *ReminderService) CancelReminder(invoiceID, reminderType string) error {
	// In production, update database
	log.Printf("❌ Cancelled reminder for invoice %s (%s)", invoiceID, reminderType)
	return nil
}

// GetReminderHistory returns reminder history for an invoice
func (s *ReminderService) GetReminderHistory(invoiceID string) ([]models.Reminder, error) {
	var reminders []models.Reminder
	err := s.db.Where("invoice_id = ?", invoiceID).
		Order("created_at DESC").
		Find(&reminders).Error

	return reminders, err
}

// PauseReminders pauses all reminders for a client
func (s *ReminderService) PauseReminders(clientID string) error {
	// In production, update client record
	log.Printf("⏸️ Paused reminders for client %s", clientID)
	return nil
}

// ResumeReminders resumes reminders for a client
func (s *ReminderService) ResumeReminders(clientID string) error {
	// In production, update client record
	log.Printf("▶️ Resumed reminders for client %s", clientID)
	return nil
}
