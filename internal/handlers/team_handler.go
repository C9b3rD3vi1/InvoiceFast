package handlers

import (
	"fmt"
	"strings"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type TeamHandler struct {
	db           *database.DB
	authService  *services.AuthService
	emailService *services.EmailService
}

func NewTeamHandler(db *database.DB, authSvc *services.AuthService, emailSvc *services.EmailService) *TeamHandler {
	return &TeamHandler{
		db:           db,
		authService:  authSvc,
		emailService: emailSvc,
	}
}

type InviteRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type TeamMember struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	Status   string    `json:"status"`
	JoinedAt time.Time `json:"joined_at"`
}

func (h *TeamHandler) GetTeamMembers(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var users []models.User
	if err := h.db.Where("tenant_id = ?", tenantID).Find(&users).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	members := make([]TeamMember, len(users))
	for i, u := range users {
		members[i] = TeamMember{
			ID:       u.ID,
			Name:     u.Name,
			Email:    u.Email,
			Role:     u.Role,
			Status:   "active",
			JoinedAt: u.CreatedAt,
		}
	}

	return c.JSON(fiber.Map{"members": members})
}

func (h *TeamHandler) InviteMember(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	userID := middleware.GetUserID(c)
	requesterRole := c.Locals("user_role")
	if requesterRole != "admin" && requesterRole != "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admins can invite team members"})
	}

	var req InviteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email is required"})
	}
	if !isValidEmail(req.Email) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid email format"})
	}

	validRoles := map[string]bool{"admin": true, "finance": true, "staff": true, "viewer": true}
	if req.Role == "" {
		req.Role = "staff"
	}
	if !validRoles[req.Role] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid role"})
	}

	var existing models.User
	if err := h.db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "User already exists with this email"})
	}

	inviteToken := uuid.New().String()
	invite := &models.TeamInvite{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		InvitedBy: userID,
		Email:     req.Email,
		Name:      req.Name,
		Role:      req.Role,
		Token:     inviteToken,
		Status:    "pending",
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}

	if err := h.db.Create(invite).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create invitation"})
	}

	if h.emailService != nil {
		go func() {
			inviteLink := fmt.Sprintf("/register?invite=%s", inviteToken)
			_ = h.emailService.SendTeamInvite(req.Email, req.Name, "Your Company", inviteLink)
		}()
	}

	return c.JSON(fiber.Map{
		"status": "invited",
		"email":  req.Email,
		"invite": inviteToken,
	})
}

func (h *TeamHandler) RemoveMember(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	memberID := c.Params("id")
	if memberID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "member ID required"})
	}

	requesterID := middleware.GetUserID(c)
	if memberID == requesterID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot remove yourself"})
	}

	var member models.User
	if err := h.db.Where("id = ? AND tenant_id = ?", memberID, tenantID).First(&member).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Member not found"})
	}

	if member.Role == "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Cannot remove owner"})
	}

	if err := h.db.Delete(&member).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to remove member"})
	}

	return c.JSON(fiber.Map{"status": "removed"})
}

func (h *TeamHandler) UpdateMemberRole(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	memberID := c.Params("id")

	var req struct {
		Role string `json:"role"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	validRoles := map[string]bool{"admin": true, "finance": true, "staff": true, "viewer": true}
	if !validRoles[req.Role] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid role"})
	}

	var member models.User
	if err := h.db.Where("id = ? AND tenant_id = ?", memberID, tenantID).First(&member).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Member not found"})
	}

	member.Role = req.Role
	if err := h.db.Save(&member).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update role"})
	}

	return c.JSON(fiber.Map{"status": "updated", "role": req.Role})
}

func (h *TeamHandler) GetInvitations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var invites []models.TeamInvite
	if err := h.db.Where("tenant_id = ? AND status = 'pending'", tenantID).Find(&invites).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"invitations": invites})
}

func (h *TeamHandler) CancelInvitation(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	inviteID := c.Params("id")

	var invite models.TeamInvite
	if err := h.db.Where("id = ? AND tenant_id = ?", inviteID, tenantID).First(&invite).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Invitation not found"})
	}

	invite.Status = "cancelled"
	if err := h.db.Save(&invite).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to cancel invitation"})
	}

	return c.JSON(fiber.Map{"status": "cancelled"})
}

func isValidEmail(email string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	return len(parts[0]) > 0 && len(parts[1]) > 0
}
