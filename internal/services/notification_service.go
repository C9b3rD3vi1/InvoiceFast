package services

import (
	"context"
	"strings"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"

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
	if len(prefs) == 0 {
		req.Channels = []string{ChannelEmail}
	}

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

		go s.sendAsync(ch, req)
	}
	return nil
}

func (s *NotificationService) sendAsync(channel string, req *NotificationRequest) {
	vars := req.Variables
	if vars == nil {
		vars = make(map[string]string)
	}

	var errMsg string
	var extID string

	switch channel {
	case ChannelEmail:
		extID, errMsg = s.sendEmail(req.Recipient, req.Subject, req.Body, vars)
	case ChannelSMS:
		extID, errMsg = s.sendSMS(req.Recipient, req.Body, vars)
	case ChannelWA:
		extID, errMsg = s.sendWhatsApp(req.Recipient, req.Body, vars)
	}

	s.logDelivery(req, channel, extID, errMsg)
}

func (s *NotificationService) sendEmail(to, subject, body string, vars map[string]string) (string, string) {
	body = s.processTemplate(body, vars)
	subject = s.processTemplate(subject, vars)

	if s.emailSvc == nil {
		return "", "email service not configured"
	}

	err := s.emailSvc.Send(EmailRequest{
		To:      []string{to},
		Subject: subject,
		Body:    body,
		IsHTML:  true,
	})

	if err != nil {
		return "", err.Error()
	}
	return "sent", ""
}

func (s *NotificationService) sendSMS(to, body string, vars map[string]string) (string, string) {
	body = s.processTemplate(body, vars)

	if s.smsSvc == nil {
		return "", "SMS service not configured"
	}

	err := s.smsSvc.Send(to, body)
	if err != nil {
		return "", err.Error()
	}
	return "sent", ""
}

func (s *NotificationService) sendWhatsApp(to, body string, vars map[string]string) (string, string) {
	body = s.processTemplate(body, vars)

	if s.waSvc == nil {
		return "", "WhatsApp service not configured"
	}

	result := s.waSvc.Send(to, body)
	if !result.Sent && result.URL == "" {
		return "", "WhatsApp send failed"
	}
	if result.URL != "" {
		return "", "WhatsApp not configured - use wa.me link"
	}
	return "sent", ""
}

func (s *NotificationService) processTemplate(template string, vars map[string]string) string {
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

func (s *NotificationService) logDelivery(req *NotificationRequest, channel, extID, errMsg string) {
	status := "sent"
	if errMsg != "" {
		status = "failed"
	}

	entry := models.NotificationLog{
		ID:        uuid.New().String(),
		TenantID:  req.TenantID,
		UserID:   req.UserID,
		Type:     channel,
		To:       req.Recipient,
		Status:   status,
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

func (s *NotificationService) isQuietHours(tenantID, userID string) bool {
	var pref models.NotificationPreference
	err := s.db.Where("tenant_id = ? AND user_id = ? AND is_enabled = ?", tenantID, userID, true).First(&pref).Error
	if err != nil {
		return false
	}

	if pref.QuietHoursStart == "" || pref.QuietHoursEnd == "" {
		return false
	}

	currentTime := time.Now().Format("15:04")
	if pref.QuietHoursStart > pref.QuietHoursEnd {
		return currentTime >= pref.QuietHoursStart || currentTime < pref.QuietHoursEnd
	}
	return currentTime >= pref.QuietHoursStart && currentTime < pref.QuietHoursEnd
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
	re := strings.NewReplacer("<br>", "\n", "<br>", "\n", "<p>", "", "</p>", "", "<span>", "", "</span>", "")
	return re.Replace(s)
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
			EventType: "high_value.transaction",
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