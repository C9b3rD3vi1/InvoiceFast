package services

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/models"
	"invoicefast/internal/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type NotificationService struct {
	db          *database.DB
	emailSvc   *EmailService
	smsSvc     *SMSService
	waSvc      *WhatsAppService
	cfg        *config.Config
	queueChan  chan *QueuedNotification
	workerChan chan struct{}
}

type QueuedNotification struct {
	ID         string
	TenantID   string
	UserID    string
	EventType string
	Channel   string
	Recipient string
	Subject   string
	Body      string
	Variables map[string]string
	TemplateID string
	Reference string
	Priority  int
}

type NotifEvent struct {
	EventType string
	TenantID  string
	UserID   string
	Data     map[string]interface{}
}

const (
	ChannelEmail     = "email"
	ChannelSMS    = "sms"
	ChannelWA     = "whatsapp"

	EventInvoiceCreated = "invoice.created"
	EventInvoiceSent  = "invoice.sent"
	EventInvoiceViewed = "invoice.viewed"
	EventInvoicePaid = "invoice.paid"
	EventInvoiceOverdue = "invoice.overdue"
	EventCreditNote  = "credit_note.issued"
	EventDebitNote  = "debit_note.issued"

	EventPaymentReceived = "payment.received"
	EventPaymentFailed = "payment.failed"
	EventPaymentPartial = "payment.partial"
	EventRefundIssued = "refund.issued"

	EventKRASubmitted = "kra.submitted"
	EventKRAAccepted = "kra.accepted"
	EventKRARejected = "kra.rejected"

	EventPasswordReset = "password.reset"
	EventLoginAlert = "login.alert"
	EventSubExpiring = "subscription.expiring"
	EventSubCreated  = "subscription.created"
	EventSubRenewed  = "subscription.renewed"

	EventPaymentMatched   = "payment.matched"
	EventPaymentUnmatched = "payment.unmatched"

	EventInvoiceDueSoon = "invoice.due_soon"
	EventHighValueTransaction = "high_value.transaction"

	EventFraudAlert     = "fraud.alert"
	EventFailedPayment = "payment.attempts"
)

func NewNotificationService(db *database.DB, email *EmailService, sms *SMSService, wa *WhatsAppService, cfg *config.Config) *NotificationService {
	return &NotificationService{
		db:         db,
		emailSvc:  email,
		smsSvc:   sms,
		waSvc:   wa,
		cfg:     cfg,
		queueChan: make(chan *QueuedNotification, 1000),
		workerChan: make(chan struct{}),
	}
}

// getTenantNotificationSettings reads tenant-level notification defaults from Tenant.Settings JSON.
// Used as fallback when no per-user NotificationPreference exists.
func (s *NotificationService) getTenantNotificationSettings(tenantID string) *NotificationSettings {
	var tenant models.Tenant
	if err := s.db.First(&tenant, "id = ?", tenantID).Error; err != nil {
		return nil
	}
	if tenant.Settings == "" {
		return nil
	}
	var settings TenantSettings
	if err := json.Unmarshal([]byte(tenant.Settings), &settings); err != nil {
		return nil
	}
	return settings.Notifications
}

// createInAppNotification creates a bell-icon Notification record for the UI.
func (s *NotificationService) createInAppNotification(tenantID, userID, eventType, recipient, body string) {
	category, icon := mapEventToInAppCategory(eventType)
	notification := models.Notification{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		Title:     eventType,
		Message:   truncateForNotification(body, 255),
		Category:  category,
		Icon:      icon,
		Actor:     "system",
		Read:      false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.db.Create(&notification)
}

func mapEventToInAppCategory(eventType string) (string, string) {
	switch {
	case strings.HasPrefix(eventType, "invoice.") || strings.HasPrefix(eventType, "credit_note") || strings.HasPrefix(eventType, "debit_note"):
		return "Invoices", "file-text"
	case strings.HasPrefix(eventType, "payment.") || strings.HasPrefix(eventType, "refund."):
		return "Payments", "credit-card"
	case strings.HasPrefix(eventType, "subscription."):
		return "System", "settings"
	case strings.HasPrefix(eventType, "kra."):
		return "System", "bell"
	case strings.HasPrefix(eventType, "password.") || strings.HasPrefix(eventType, "login."):
		return "System", "bell"
	case strings.HasPrefix(eventType, "fraud.") || strings.HasPrefix(eventType, "high_value."):
		return "System", "bell"
	default:
		return "System", "bell"
	}
}

func truncateForNotification(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func (s *NotificationService) Start() {
	// Process in-memory channel (legacy path)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Get().Error(context.Background(), "panic recovered", "category", "panic", "recover", r)
			}
		}()
		for item := range s.queueChan {
			req := &NotificationRequest{
				TenantID:   item.TenantID,
				UserID:     item.UserID,
				EventType:  item.EventType,
				Recipient:  item.Recipient,
				Subject:    item.Subject,
				Body:       item.Body,
				Variables:  item.Variables,
				Reference:  item.Reference,
			}
			s.sendAsync(item.Channel, req, item.ID)
		}
	}()

	// Poll DB queue for pending/retry items
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Get().Error(context.Background(), "panic recovered", "category", "panic", "recover", r)
			}
		}()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.processQueue()
			case <-s.workerChan:
				return
			}
		}
	}()
}

func (s *NotificationService) Stop() {
	close(s.workerChan)
	close(s.queueChan)
}

// processQueue picks pending queue items from the DB and sends them.
func (s *NotificationService) processQueue() {
	var items []models.NotificationQueueItem
	err := s.db.Where("status = ? AND (scheduled_at IS NULL OR scheduled_at <= ?)", "pending", time.Now()).
		Order("priority DESC, created_at ASC").Limit(50).Find(&items).Error
	if err != nil || len(items) == 0 {
		return
	}

	for _, item := range items {
		// Mark as processing
		s.db.Model(&item).Update("status", "processing")

		vars := make(map[string]string)
		if item.Variables != "" {
			json.Unmarshal([]byte(item.Variables), &vars)
		}

		extID, errMsg, provider := "", "", ""
		switch item.Channel {
		case ChannelEmail:
			extID, errMsg, provider = s.sendEmail(item.Recipient, item.Subject, item.Body, vars)
		case ChannelSMS:
			extID, errMsg, provider = s.sendSMS(item.Recipient, item.Body, vars)
		case ChannelWA:
			extID, errMsg, provider = s.sendWhatsApp(item.Recipient, item.Body, vars)
		}

		if errMsg != "" {
			item.RetryCount++
			if item.RetryCount >= item.MaxRetries {
				item.Status = "dead_letter"
			} else {
				item.Status = "failed"
			}
			item.ErrorMsg = errMsg
		} else {
			item.Status = "sent"
			item.ExternalID = extID
			item.SentAt = time.Now()
		}

		// Create delivery log for queue-processed notifications
		req := &NotificationRequest{
			TenantID:   item.TenantID,
			UserID:     item.UserID,
			EventType:  item.EventType,
			Recipient:  item.Recipient,
			Subject:    item.Subject,
			Body:       item.Body,
			Reference:  item.Reference,
		}
		s.logDelivery(req, item.Channel, provider, extID, errMsg)
		item.UpdatedAt = time.Now()
		s.db.Save(&item)
	}
}

func (s *NotificationService) Init() error {
	return s.db.AutoMigrate(
		&models.NotificationQueueItem{},
		&models.NotificationTemplate{},
		&models.NotificationPreference{},
	)
}

func (s *NotificationService) Send(ctx context.Context, req *NotificationRequest) error {
	prefs, _ := s.GetPreferences(ctx, req.TenantID, req.UserID)

	// Fallback: if no per-user preferences exist, use tenant-level defaults
	if len(prefs) == 0 {
		tenantPrefs := s.getTenantNotificationSettings(req.TenantID)
		if tenantPrefs != nil {
			req.Channels = tenantPrefsToChannels(tenantPrefs, req.EventType)
		} else {
			req.Channels = []string{ChannelEmail}
		}
	}

	// Create in-app notification for the user regardless of channel success
	s.createInAppNotification(req.TenantID, req.UserID, req.EventType, req.Recipient, req.Body)

	for _, ch := range req.Channels {
		channelEnabled := true
		for _, p := range prefs {
			if p.EventType == req.EventType {
				switch ch {
				case ChannelEmail:
					channelEnabled = p.ChannelEmail
				case ChannelSMS:
					channelEnabled = p.ChannelSMS
				case ChannelWA:
					channelEnabled = p.ChannelWhatsApp
				}
				break
			}
		}

		if !channelEnabled {
			continue
		}

		if s.isQuietHours(req.TenantID, req.UserID) {
			continue
		}

		// Persist to queue table for audit trail and retry capability
		itemID := s.enqueue(req, ch)

		// Send immediately via goroutine
		go s.sendAsync(ch, req, itemID)
	}
	return nil
}

// tenantPrefsToChannels converts tenant-level NotificationSettings to a channel list
// for a given event type, based on which channel has the event enabled.
func tenantPrefsToChannels(settings *NotificationSettings, eventType string) []string {
	var channels []string
	for _, ev := range settings.Email {
		if ev.Key == eventType && ev.Enabled {
			channels = append(channels, ChannelEmail)
			break
		}
	}
	for _, ev := range settings.SMS {
		if ev.Key == eventType && ev.Enabled {
			channels = append(channels, ChannelSMS)
			break
		}
	}
	for _, ev := range settings.WhatsApp {
		if ev.Key == eventType && ev.Enabled {
			channels = append(channels, ChannelWA)
			break
		}
	}
	if len(channels) == 0 {
		channels = []string{ChannelEmail}
	}
	return channels
}

// enqueue persists a notification to the queue table and returns the item ID.
func (s *NotificationService) enqueue(req *NotificationRequest, channel string) string {
	varsJSON, _ := json.Marshal(req.Variables)

	item := models.NotificationQueueItem{
		ID:        uuid.New().String(),
		TenantID:  req.TenantID,
		UserID:    req.UserID,
		EventType: req.EventType,
		Channel:   channel,
		Recipient: req.Recipient,
		Subject:   req.Subject,
		Body:      req.Body,
		Variables: string(varsJSON),
		Reference: req.Reference,
		Status:    "pending",
	}

	if err := s.db.Create(&item).Error; err != nil {
		logger.Get().Error(context.Background(), "failed to enqueue notification", "category", "notification", "error", err)
	}
	return item.ID
}

func (s *NotificationService) sendAsync(channel string, req *NotificationRequest, queueItemID string) {
	vars := req.Variables
	if vars == nil {
		vars = make(map[string]string)
	}

	// Look up tenant template for this event + channel; override subject/body if found
	subject, body := req.Subject, req.Body
	if tmpl := s.GetTemplateByEvent(req.TenantID, req.EventType, channel); tmpl != nil {
		if tmpl.Subject != "" {
			subject = tmpl.Subject
		}
		if tmpl.Body != "" {
			body = tmpl.Body
		}
	}

	var errMsg string
	var extID string
	var provider string

	switch channel {
	case ChannelEmail:
		extID, errMsg, provider = s.sendEmail(req.Recipient, subject, body, vars)
	case ChannelSMS:
		extID, errMsg, provider = s.sendSMS(req.Recipient, body, vars)
	case ChannelWA:
		extID, errMsg, provider = s.sendWhatsApp(req.Recipient, body, vars)
	}

	s.logDelivery(req, channel, provider, extID, errMsg)

	// Update queue item status if we have one
	if queueItemID != "" {
		updates := map[string]interface{}{
			"updated_at": time.Now(),
		}
		if errMsg != "" {
			updates["status"] = "failed"
			updates["error_msg"] = errMsg
		} else {
			updates["status"] = "sent"
			updates["external_id"] = extID
			updates["sent_at"] = time.Now()
		}
		s.db.Model(&models.NotificationQueueItem{}).Where("id = ?", queueItemID).Updates(updates)
	}
}

func (s *NotificationService) sendEmail(to, subject, body string, vars map[string]string) (string, string, string) {
	body = s.processTemplate(body, vars)
	subject = s.processTemplate(subject, vars)

	if s.emailSvc == nil {
		return "", "email service not configured", ""
	}

	infoName, infoEmail := s.emailSvc.sender("info")
	err := s.emailSvc.Send(EmailRequest{
		FromName:  infoName,
		FromEmail: infoEmail,
		To:        []string{to},
		Subject:   subject,
		Body:      body,
		IsHTML:    true,
	})

	if err != nil {
		return "", err.Error(), ""
	}
	return "sent", "", "smtp"
}

func (s *NotificationService) sendSMS(to, body string, vars map[string]string) (string, string, string) {
	body = s.processTemplate(body, vars)

	if s.smsSvc == nil {
		return "", "SMS service not configured", ""
	}

	err := s.smsSvc.Send(to, body)
	if err != nil {
		return "", err.Error(), ""
	}
	return "sent", "", "africastalking"
}

func (s *NotificationService) sendWhatsApp(to, body string, vars map[string]string) (string, string, string) {
	body = s.processTemplate(body, vars)

	if s.waSvc == nil {
		return "", "WhatsApp service not configured", ""
	}

	result := s.waSvc.Send(to, body)
	if result.Sent {
		return "sent", "", "metas"
	}
	if result.URL != "" {
		// Fallback to wa.me link — return the URL as external ID so caller can use it
		return result.URL, "", "metas"
	}
	return "", "WhatsApp send failed", ""
}

func (s *NotificationService) processTemplate(template string, vars map[string]string) string {
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

func (s *NotificationService) logDelivery(req *NotificationRequest, channel, provider, extID, errMsg string) {
	status := "sent"
	if errMsg != "" {
		status = "failed"
	}

	entry := models.NotificationLog{
		ID:        uuid.New().String(),
		TenantID:  req.TenantID,
		UserID:   req.UserID,
		Type:     channel,
		Provider: provider,
		To:       req.Recipient,
		Status:   status,
		ExternalID: extID,
		ErrorMsg: errMsg,
		SentAt:   time.Now(),
	}

	if req.Reference != "" {
		if strings.HasPrefix(req.Reference, "INV-") {
			entry.InvoiceID = req.Reference
		}
	}

	s.db.Create(&entry)
}

func (s *NotificationService) GetPreferences(ctx context.Context, tenantID, userID string) ([]models.NotificationPreference, error) {
	var prefs []models.NotificationPreference
	err := s.db.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Find(&prefs).Error
	return prefs, err
}

func (s *NotificationService) SetPreference(ctx context.Context, pref *models.NotificationPreference) error {
	var existing models.NotificationPreference
	err := s.db.Where("tenant_id = ? AND user_id = ? AND event_type = ?",
		pref.TenantID, pref.UserID, pref.EventType).First(&existing).Error

	if err == nil {
		pref.ID = existing.ID
		pref.CreatedAt = existing.CreatedAt
		pref.UpdatedAt = time.Now()
		return s.db.Save(pref).Error
	}

	pref.ID = uuid.New().String()
	pref.CreatedAt = time.Now()
	pref.UpdatedAt = time.Now()
	return s.db.Create(pref).Error
}

func (s *NotificationService) GetTemplates(tenantID string) ([]models.NotificationTemplate, error) {
	var templates []models.NotificationTemplate
	err := s.db.Where("tenant_id = ?", tenantID).Find(&templates).Error
	return templates, err
}

func (s *NotificationService) CreateTemplate(t *models.NotificationTemplate) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	return s.db.Create(t).Error
}

func (s *NotificationService) UpdateTemplate(t *models.NotificationTemplate) error {
	t.UpdatedAt = time.Now()
	return s.db.Save(t).Error
}

func (s *NotificationService) DeleteTemplate(id, tenantID string) error {
	return s.db.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.NotificationTemplate{}).Error
}

func (s *NotificationService) GetDeliveryLogs(tenantID string) ([]models.NotificationLog, error) {
	var logs []models.NotificationLog
	err := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Limit(100).Find(&logs).Error
	return logs, err
}

func (s *NotificationService) RetryFailed(limit int) error {
	var items []models.NotificationQueueItem
	err := s.db.Where("status = 'failed' AND retry_count < max_retries").
		Order("created_at ASC").Limit(limit).Find(&items).Error

	if err != nil {
		return err
	}

	for _, item := range items {
		item.RetryCount++
		item.Status = "pending"
		item.UpdatedAt = time.Now()
		s.db.Save(&item)
	}
	return nil
}

// GetTemplateByEvent finds the active template for a given tenant, event type, and channel.
func (s *NotificationService) GetTemplateByEvent(tenantID, eventType, channel string) *models.NotificationTemplate {
	var t models.NotificationTemplate
	err := s.db.Where("tenant_id = ? AND event_type = ? AND channel = ? AND is_active = ?",
		tenantID, eventType, channel, true).First(&t).Error
	if err != nil {
		return nil
	}
	return &t
}

func (s *NotificationService) isQuietHours(tenantID, userID string) bool {
	var prefs []models.NotificationPreference
	err := s.db.Where("tenant_id = ? AND user_id = ? AND is_enabled = ?", tenantID, userID, true).Find(&prefs).Error
	if err != nil || len(prefs) == 0 {
		return false
	}

	currentTime := time.Now().Format("15:04")
	for _, pref := range prefs {
		if pref.QuietHoursStart == "" || pref.QuietHoursEnd == "" {
			continue
		}
		if pref.QuietHoursStart > pref.QuietHoursEnd {
			if currentTime >= pref.QuietHoursStart || currentTime < pref.QuietHoursEnd {
				return true
			}
		} else {
			if currentTime >= pref.QuietHoursStart && currentTime < pref.QuietHoursEnd {
				return true
			}
		}
	}
	return false
}

type NotificationRequest struct {
	TenantID   string
	UserID     string
	EventType  string
	Channel    string
	Channels   []string
	Recipient  string
	Subject    string
	Body       string
	Variables  map[string]string
	Reference  string
}

type NotificationHandler struct {
	db  *database.DB
	svc *NotificationService
	cfg *config.Config
}

func NewNotificationHandler(db *database.DB, svc *NotificationService, cfg *config.Config) *NotificationHandler {
	return &NotificationHandler{db: db, svc: svc, cfg: cfg}
}

func (h *NotificationHandler) GetPreferences(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)
	userID := c.Locals("user_id").(string)

	prefs, err := h.svc.GetPreferences(context.Background(), tenantID, userID)
	if err != nil {
		return c.JSON(fiber.Map{"error": "Failed to fetch preferences"})
	}

	return c.JSON(fiber.Map{"preferences": prefs})
}

func (h *NotificationHandler) UpdatePreferences(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)
	userID := c.Locals("user_id").(string)

	var req struct {
		EventType         string `json:"event_type"`
		ChannelEmail     bool   `json:"channel_email"`
		ChannelSMS      bool   `json:"channel_sms"`
		ChannelWhatsApp bool   `json:"channel_whatsapp"`
		IsEnabled        bool   `json:"is_enabled"`
		QuietHoursStart  string `json:"quiet_hours_start"`
		QuietHoursEnd   string `json:"quiet_hours_end"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.JSON(fiber.Map{"error": "Invalid request"})
	}

	pref := &models.NotificationPreference{
		TenantID:         tenantID,
		UserID:          userID,
		EventType:       req.EventType,
		ChannelEmail:    req.ChannelEmail,
		ChannelSMS:      req.ChannelSMS,
		ChannelWhatsApp: req.ChannelWhatsApp,
		IsEnabled:       req.IsEnabled,
		QuietHoursStart:  req.QuietHoursStart,
		QuietHoursEnd:   req.QuietHoursEnd,
	}

	var existing models.NotificationPreference
	err := h.svc.db.Where("tenant_id = ? AND user_id = ? AND event_type = ?", tenantID, userID, req.EventType).
		First(&existing).Error

	if err == nil {
		pref.ID = existing.ID
		h.svc.db.Save(pref)
	} else {
		h.svc.SetPreference(context.Background(), pref)
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *NotificationHandler) GetDeliveryLogs(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)

	logs, err := h.svc.GetDeliveryLogs(tenantID)
	if err != nil {
		return c.JSON(fiber.Map{"error": "Failed to fetch logs"})
	}

	return c.JSON(fiber.Map{"logs": logs})
}

func (h *NotificationHandler) GetTemplates(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)

	templates, err := h.svc.GetTemplates(tenantID)
	if err != nil {
		return c.JSON(fiber.Map{"error": "Failed to fetch templates"})
	}

	return c.JSON(fiber.Map{"templates": templates})
}

func (h *NotificationHandler) CreateTemplate(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)

	var t models.NotificationTemplate
	if err := c.BodyParser(&t); err != nil {
		return c.JSON(fiber.Map{"error": "Invalid request"})
	}

	t.TenantID = tenantID

	if err := h.svc.CreateTemplate(&t); err != nil {
		return c.JSON(fiber.Map{"error": "Failed to create template"})
	}

	return c.JSON(fiber.Map{"template": t})
}

func (h *NotificationHandler) UpdateTemplate(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)
	templateID := c.Params("id")

	var t models.NotificationTemplate
	if err := c.BodyParser(&t); err != nil {
		return c.JSON(fiber.Map{"error": "Invalid request"})
	}

	t.ID = templateID
	t.TenantID = tenantID

	if err := h.svc.UpdateTemplate(&t); err != nil {
		return c.JSON(fiber.Map{"error": "Failed to update template"})
	}

	return c.JSON(fiber.Map{"template": t})
}

func (h *NotificationHandler) DeleteTemplate(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)
	templateID := c.Params("id")

	if err := h.svc.DeleteTemplate(templateID, tenantID); err != nil {
		return c.JSON(fiber.Map{"error": "Failed to delete template"})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *NotificationHandler) RetryNotification(c *fiber.Ctx) error {
	notificationID := c.Params("id")

	if err := h.svc.db.Model(&models.NotificationQueueItem{}).
		Where("id = ?", notificationID).
		Updates(map[string]interface{}{
			"status":      "pending",
			"retry_count": 0,
			"error_msg": "",
		}).Error; err != nil {
		return c.JSON(fiber.Map{"error": "Failed to retry"})
	}

	return c.JSON(fiber.Map{"success": true})
}

func stripHTML(s string) string {
	return utils.StripHTMLTags(s)
}

func (s *NotificationService) SendFraudAlert(tenantID, userID string, details map[string]string) error {
	body := "SECURITY ALERT: Suspicious activity detected"
	if msg, ok := details["message"]; ok {
		body = msg
	}

	admins, err := s.getTenantAdmins(tenantID)
	if err != nil || len(admins) == 0 {
		return nil
	}

	req := &NotificationRequest{
		TenantID:   tenantID,
		UserID:    userID,
		EventType: EventFraudAlert,
		Channels: []string{ChannelEmail, ChannelSMS},
		Body:      body,
		Variables: details,
	}

	for _, admin := range admins {
		req.UserID = admin.ID
		req.Recipient = admin.Email
		s.Send(context.Background(), req)
	}

	return nil
}

func (s *NotificationService) SendLoginAlert(tenantID, userID, email, ipAddress string) error {
	admins, err := s.getTenantAdmins(tenantID)
	if err != nil || len(admins) == 0 {
		return nil
	}

	details := map[string]string{
		"ip_address": ipAddress,
		"time":       time.Now().Format("2006-01-02 15:04:05"),
		"user":       email,
	}

	for _, admin := range admins {
		req := &NotificationRequest{
			TenantID:   tenantID,
			UserID:    admin.ID,
			EventType: EventLoginAlert,
			Channels:  []string{ChannelEmail},
			Body:     "New login from " + ipAddress + " at " + details["time"],
			Variables: details,
		}
		s.Send(context.Background(), req)
	}

	return nil
}

func (s *NotificationService) SendHighValueAlert(tenantID, userID, amount, reference string) error {
	admins, err := s.getTenantAdmins(tenantID)
	if err != nil || len(admins) == 0 {
		return nil
	}

	details := map[string]string{
		"amount":    amount,
		"reference": reference,
	}

	for _, admin := range admins {
		req := &NotificationRequest{
			TenantID:   tenantID,
			UserID:    admin.ID,
			EventType: EventHighValueTransaction,
			Channels:  []string{ChannelEmail, ChannelSMS},
			Body:      "High value transaction: " + amount + " Ref: " + reference,
			Variables: details,
		}
		s.Send(context.Background(), req)
	}

	return nil
}

func (s *NotificationService) getTenantAdmins(tenantID string) ([]models.User, error) {
	var users []models.User
	err := s.db.Where("tenant_id = ? AND role = ?", tenantID, "admin").Find(&users).Error
	return users, err
}