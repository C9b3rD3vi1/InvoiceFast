package services

import (
	"encoding/json"
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ReminderSequenceService struct {
	db           *database.DB
	emailService *EmailService
}

func NewReminderSequenceService(db *database.DB, email *EmailService) *ReminderSequenceService {
	return &ReminderSequenceService{
		db:           db,
		emailService: email,
	}
}

func (s *ReminderSequenceService) CreateSequence(tenantID string, req *CreateSequenceRequest) (*models.ReminderSequence, error) {
	channelsJSON, _ := json.Marshal(req.Channels)
	sequence := &models.ReminderSequence{
		ID:            uuid.New().String(),
		TenantID:      tenantID,
		Name:          req.Name,
		Description:   req.Description,
		IsActive:      true,
		TriggerType:   req.TriggerType,
		TriggerDays:   req.TriggerDays,
		Channels:      string(channelsJSON),
		EmailTemplate: req.EmailTemplate,
		SMSContent:    req.SMSContent,
		WhatsAppMsg:   req.WhatsAppMsg,
		IsDefault:     req.IsDefault,
	}

	if err := s.db.Create(sequence).Error; err != nil {
		return nil, fmt.Errorf("failed to create sequence: %w", err)
	}

	return sequence, nil
}

func (s *ReminderSequenceService) UpdateSequence(sequenceID string, req *UpdateSequenceRequest) (*models.ReminderSequence, error) {
	var sequence models.ReminderSequence
	if err := s.db.First(&sequence, "id = ?", sequenceID).Error; err != nil {
		return nil, fmt.Errorf("sequence not found: %w", err)
	}

	if req.Name != nil {
		sequence.Name = *req.Name
	}
	if req.Description != nil {
		sequence.Description = *req.Description
	}
	if req.IsActive != nil {
		sequence.IsActive = *req.IsActive
	}
	if req.TriggerType != nil {
		sequence.TriggerType = *req.TriggerType
	}
	if req.TriggerDays != nil {
		sequence.TriggerDays = *req.TriggerDays
	}
	if req.Channels != nil {
		channelsJSON, _ := json.Marshal(req.Channels)
		sequence.Channels = string(channelsJSON)
	}
	if req.EmailTemplate != nil {
		sequence.EmailTemplate = *req.EmailTemplate
	}
	if req.SMSContent != nil {
		sequence.SMSContent = *req.SMSContent
	}
	if req.WhatsAppMsg != nil {
		sequence.WhatsAppMsg = *req.WhatsAppMsg
	}

	if err := s.db.Save(&sequence).Error; err != nil {
		return nil, fmt.Errorf("failed to update sequence: %w", err)
	}

	return &sequence, nil
}

func (s *ReminderSequenceService) GetSequences(tenantID string) ([]models.ReminderSequence, error) {
	var sequences []models.ReminderSequence
	err := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&sequences).Error
	return sequences, err
}

func (s *ReminderSequenceService) GetActiveSequences(tenantID string) ([]models.ReminderSequence, error) {
	var sequences []models.ReminderSequence
	err := s.db.Where("tenant_id = ? AND is_active = ?", tenantID, true).Find(&sequences).Error
	return sequences, err
}

func (s *ReminderSequenceService) DeleteSequence(sequenceID string) error {
	result := s.db.Where("id = ?", sequenceID).Delete(&models.ReminderSequence{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete sequence: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *ReminderSequenceService) ProcessSequences() error {
	sequences, err := s.GetActiveSequences("")
	if err != nil {
		return err
	}

	for _, seq := range sequences {
		s.processSequence(&seq)
	}

	return nil
}

func (s *ReminderSequenceService) processSequence(seq *models.ReminderSequence) {
	var invoices []models.Invoice

	now := time.Now()
	triggerDate := now.AddDate(0, 0, seq.TriggerDays)

	switch seq.TriggerType {
	case "due_soon":
		s.db.Where("tenant_id = ? AND status IN ? AND due_date BETWEEN ? AND ?",
			seq.TenantID, []string{"sent", "viewed"},
			triggerDate, triggerDate.AddDate(0, 0, 1)).Find(&invoices)
	case "overdue":
		s.db.Where("tenant_id = ? AND status = ? AND due_date < ?",
			seq.TenantID, "overdue", triggerDate).Find(&invoices)
	}

	for _, inv := range invoices {
		s.sendSequenceMessage(seq, &inv)
	}
}

func (s *ReminderSequenceService) sendSequenceMessage(seq *models.ReminderSequence, invoice *models.Invoice) {
	var client models.Client
	s.db.First(&client, "id = ?", invoice.ClientID)

	var channels []string
	json.Unmarshal([]byte(seq.Channels), &channels)

	for _, channel := range channels {
		var err error
		switch channel {
		case "email":
			if s.emailService != nil && client.Email != "" {
				err = s.emailService.Send(EmailRequest{
					To:      []string{client.Email},
					Subject: fmt.Sprintf("Reminder: Invoice %s", invoice.InvoiceNumber),
					Body:    seq.EmailTemplate,
					IsHTML:  true,
				})
			}
		case "sms":
			// Would integrate with SMS service
		case "whatsapp":
			// Would integrate with WhatsApp service
		}

		log := models.ReminderSequenceLog{
			ID:         uuid.New().String(),
			SequenceID: seq.ID,
			InvoiceID:  invoice.ID,
			TenantID:   invoice.TenantID,
			Channel:    channel,
			Status:     "sent",
			SentAt:     time.Now(),
		}
		if err != nil {
			log.Status = "failed"
			log.Error = err.Error()
		}
		s.db.Create(&log)
	}
}

type CreateSequenceRequest struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	TriggerType   string   `json:"trigger_type"` // due_soon, overdue
	TriggerDays   int      `json:"trigger_days"`
	Channels      []string `json:"channels"` // email, whatsapp, sms
	EmailTemplate string   `json:"email_template"`
	SMSContent    string   `json:"sms_content"`
	WhatsAppMsg   string   `json:"whatsapp_message"`
	IsDefault     bool     `json:"is_default"`
}

type UpdateSequenceRequest struct {
	Name          *string  `json:"name"`
	Description   *string  `json:"description"`
	IsActive      *bool    `json:"is_active"`
	TriggerType   *string  `json:"trigger_type"`
	TriggerDays   *int     `json:"trigger_days"`
	Channels      []string `json:"channels"`
	EmailTemplate *string  `json:"email_template"`
	SMSContent    *string  `json:"sms_content"`
	WhatsAppMsg   *string  `json:"whatsapp_message"`
}
