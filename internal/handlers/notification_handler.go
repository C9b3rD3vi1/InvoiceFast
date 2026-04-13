package handlers

import (
	"invoicefast/internal/database"
	"invoicefast/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type NotificationHandler struct {
	db *database.DB
}

func NewNotificationHandler(db *database.DB) *NotificationHandler {
	return &NotificationHandler{db: db}
}

func (h *NotificationHandler) GetNotifications(c *fiber.Ctx) error {
	tenantID := c.Locals("tenantID").(string)
	userID := c.Locals("userID").(string)

	var notifications []models.Notification
	if err := h.db.Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("created_at DESC").
		Limit(100).
		Find(&notifications).Error; err != nil {
		return c.JSON(fiber.Map{"error": "Failed to fetch notifications"})
	}

	return c.JSON(fiber.Map{
		"notifications": notifications,
		"total":         len(notifications),
		"unread":        len(FilterUnreadNotifications(notifications)),
	})
}

func (h *NotificationHandler) MarkAsRead(c *fiber.Ctx) error {
	tenantID := c.Locals("tenantID").(string)
	userID := c.Locals("userID").(string)
	notificationID := c.Params("id")

	if err := h.db.Model(&models.Notification{}).
		Where("id = ? AND tenant_id = ? AND user_id = ?", notificationID, tenantID, userID).
		Updates(map[string]interface{}{
			"read":       true,
			"updated_at": time.Now(),
		}).Error; err != nil {
		return c.JSON(fiber.Map{"error": "Failed to mark as read"})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *NotificationHandler) MarkAllAsRead(c *fiber.Ctx) error {
	tenantID := c.Locals("tenantID").(string)
	userID := c.Locals("userID").(string)

	if err := h.db.Model(&models.Notification{}).
		Where("tenant_id = ? AND user_id = ? AND read = ?", tenantID, userID, false).
		Updates(map[string]interface{}{
			"read":       true,
			"updated_at": time.Now(),
		}).Error; err != nil {
		return c.JSON(fiber.Map{"error": "Failed to mark all as read"})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *NotificationHandler) DeleteNotification(c *fiber.Ctx) error {
	tenantID := c.Locals("tenantID").(string)
	userID := c.Locals("userID").(string)
	notificationID := c.Params("id")

	if err := h.db.Where("id = ? AND tenant_id = ? AND user_id = ?", notificationID, tenantID, userID).
		Delete(&models.Notification{}).Error; err != nil {
		return c.JSON(fiber.Map{"error": "Failed to delete notification"})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *NotificationHandler) CreateNotification(tenantID, userID, title, message, category, icon, actor, link string, data string) error {
	notification := models.Notification{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		Title:     title,
		Message:   message,
		Category:  category,
		Icon:      icon,
		Actor:     actor,
		Read:      false,
		Link:      link,
		Data:      data,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.db.Create(&notification).Error; err != nil {
		return err
	}
	return nil
}

func FilterUnreadNotifications(notifications []models.Notification) []models.Notification {
	var unread []models.Notification
	for _, n := range notifications {
		if !n.Read {
			unread = append(unread, n)
		}
	}
	return unread
}
