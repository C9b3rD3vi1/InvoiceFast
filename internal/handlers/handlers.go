package handlers

import (
	"errors"
	"net/http"
	"strings"

	"invoicefast/internal/services"
	"invoicefast/internal/utils"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	auth    *services.AuthService
	invoice *services.InvoiceService
	client  *services.ClientService
}

func NewHandler(auth *services.AuthService, invoice *services.InvoiceService, client *services.ClientService) *Handler {
	return &Handler{
		auth:    auth,
		invoice: invoice,
		client:  client,
	}
}

// Health check
func (h *Handler) Health(c *gin.Context) {
	utils.RespondWithSuccess(c, gin.H{
		"status": "ok",
		"time":   "2025-02-20T00:00:00Z",
	})
}

// ==================== AUTH HANDLERS ====================

// Register creates a new user
func (h *Handler) Register(c *gin.Context) {
	var req services.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid request body", err.Error())
		return
	}

	resp, err := h.auth.Register(&req)
	if err != nil {
		// Check specific error types
		if errors.Is(err, services.ErrEmailExists) {
			utils.RespondWithError(c, http.StatusConflict, utils.ErrCodeConflict, "Email already registered")
			return
		}
		if errors.Is(err, services.ErrWeakPassword) {
			utils.RespondWithValidationError(c, "Password too weak", err.Error())
			return
		}
		utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeBadRequest, err.Error())
		return
	}

	utils.RespondWithCreated(c, resp)
}

// Login authenticates a user
func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid credentials", err.Error())
		return
	}

	resp, err := h.auth.Login(req.Email, req.Password)
	if err != nil {
		utils.RespondWithError(c, http.StatusUnauthorized, utils.ErrCodeUnauthorized, "Invalid email or password")
		return
	}

	utils.RespondWithSuccess(c, resp)
}

// RefreshToken refreshes access token
func (h *Handler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid request", err.Error())
		return
	}

	resp, err := h.auth.RefreshToken(req.RefreshToken)
	if err != nil {
		utils.RespondWithError(c, http.StatusUnauthorized, utils.ErrCodeUnauthorized, "Invalid or expired refresh token")
		return
	}

	utils.RespondWithSuccess(c, resp)
}

// GetMe returns current user
func (h *Handler) GetMe(c *gin.Context) {
	userID := c.GetString("user_id")
	user, err := h.auth.GetUserByID(userID)
	if err != nil {
		utils.RespondWithError(c, http.StatusNotFound, utils.ErrCodeNotFound, "User not found")
		return
	}

	utils.RespondWithSuccess(c, user)
}

// UpdateUser updates user profile
func (h *Handler) UpdateUser(c *gin.Context) {
	userID := c.GetString("user_id")

	var req services.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid request", err.Error())
		return
	}

	user, err := h.auth.UpdateUser(userID, &req)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeBadRequest, err.Error())
		return
	}

	utils.RespondWithSuccess(c, user)
}

// ChangePassword changes user password
func (h *Handler) ChangePassword(c *gin.Context) {
	userID := c.GetString("user_id")

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid password", err.Error())
		return
	}

	if err := h.auth.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, services.ErrWrongPassword) {
			utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeBadRequest, "Current password is incorrect")
			return
		}
		utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeBadRequest, err.Error())
		return
	}

	utils.RespondWithSuccess(c, gin.H{"message": "Password changed successfully"})
}

// Logout invalidates refresh token
func (h *Handler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	c.ShouldBindJSON(&req)

	if req.RefreshToken != "" {
		h.auth.Logout(req.RefreshToken)
	}

	utils.RespondWithSuccess(c, gin.H{"message": "Logged out successfully"})
}

// GenerateAPIKey creates an API key
func (h *Handler) GenerateAPIKey(c *gin.Context) {
	userID := c.GetString("user_id")

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid request", err.Error())
		return
	}

	key, err := h.auth.GenerateAPIKey(userID, req.Name)
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, utils.ErrCodeInternalError, "Failed to generate API key")
		return
	}

	utils.RespondWithCreated(c, gin.H{"api_key": key})
}

// ==================== CLIENT HANDLERS ====================

// CreateClient creates a new client
func (h *Handler) CreateClient(c *gin.Context) {
	userID := c.GetString("user_id")

	var req services.CreateClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid client data", err.Error())
		return
	}

	client, err := h.client.CreateClient(userID, &req)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeBadRequest, err.Error())
		return
	}

	utils.RespondWithCreated(c, client)
}

// GetClients returns all clients for user
func (h *Handler) GetClients(c *gin.Context) {
	userID := c.GetString("user_id")

	page, limit, offset := utils.PaginationParams(c)

	filter := services.ClientFilter{
		Search: c.Query("search"),
		Offset: offset,
		Limit:  limit,
	}

	clients, total, err := h.client.GetUserClients(userID, filter)
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, utils.ErrCodeInternalError, "Failed to fetch clients")
		return
	}

	utils.PaginatedResponse(c, clients, total, page, limit)
}

// GetClient returns a single client
func (h *Handler) GetClient(c *gin.Context) {
	userID := c.GetString("user_id")
	clientID := c.Param("id")

	client, err := h.client.GetClient(clientID, userID)
	if err != nil {
		utils.RespondWithError(c, http.StatusNotFound, utils.ErrCodeNotFound, "Client not found")
		return
	}

	utils.RespondWithSuccess(c, client)
}

// UpdateClient updates a client
func (h *Handler) UpdateClient(c *gin.Context) {
	userID := c.GetString("user_id")
	clientID := c.Param("id")

	var req services.UpdateClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid data", err.Error())
		return
	}

	client, err := h.client.UpdateClient(clientID, userID, &req)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeBadRequest, err.Error())
		return
	}

	utils.RespondWithSuccess(c, client)
}

// DeleteClient deletes a client
func (h *Handler) DeleteClient(c *gin.Context) {
	userID := c.GetString("user_id")
	clientID := c.Param("id")

	if err := h.client.DeleteClient(clientID, userID); err != nil {
		// Check if client has invoices
		if strings.Contains(err.Error(), "cannot delete client with existing invoices") {
			utils.RespondWithError(c, http.StatusConflict, utils.ErrCodeConflict, "Cannot delete client with existing invoices")
			return
		}
		utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeBadRequest, err.Error())
		return
	}

	utils.RespondWithSuccess(c, gin.H{"message": "Client deleted successfully"})
}

// GetClientStats returns client statistics
func (h *Handler) GetClientStats(c *gin.Context) {
	userID := c.GetString("user_id")
	clientID := c.Param("id")

	stats, err := h.client.GetClientStats(clientID, userID)
	if err != nil {
		utils.RespondWithError(c, http.StatusNotFound, utils.ErrCodeNotFound, "Client not found")
		return
	}

	utils.RespondWithSuccess(c, stats)
}

// ==================== INVOICE HANDLERS ====================

// CreateInvoice creates a new invoice
func (h *Handler) CreateInvoice(c *gin.Context) {
	userID := c.GetString("user_id")

	var req services.CreateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid invoice data", err.Error())
		return
	}

	invoice, err := h.invoice.CreateInvoice(userID, req.ClientID, &req)
	if err != nil {
		// Handle specific errors
		switch {
		case errors.Is(err, services.ErrEmptyItems):
			utils.RespondWithValidationError(c, "Invoice must have at least one item", nil)
		case errors.Is(err, services.ErrInvalidQuantity):
			utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeValidationFailed, "Invalid quantity")
		default:
			if strings.Contains(err.Error(), "client not found") {
				utils.RespondWithError(c, http.StatusNotFound, utils.ErrCodeNotFound, "Client not found")
				return
			}
			utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeBadRequest, err.Error())
		}
		return
	}

	utils.RespondWithCreated(c, invoice)
}

// GetInvoices returns all invoices for user
func (h *Handler) GetInvoices(c *gin.Context) {
	userID := c.GetString("user_id")

	page, limit, offset := utils.PaginationParams(c)

	filter := services.InvoiceFilter{
		Status:   c.Query("status"),
		ClientID: c.Query("client_id"),
		Search:   c.Query("search"),
		Offset:   offset,
		Limit:    limit,
	}

	invoices, total, err := h.invoice.GetUserInvoices(userID, filter)
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, utils.ErrCodeInternalError, "Failed to fetch invoices")
		return
	}

	utils.PaginatedResponse(c, invoices, total, page, limit)
}

// GetInvoice returns a single invoice
func (h *Handler) GetInvoice(c *gin.Context) {
	userID := c.GetString("user_id")
	invoiceID := c.Param("id")

	invoice, err := h.invoice.GetInvoiceByID(invoiceID, userID)
	if err != nil {
		utils.RespondWithError(c, http.StatusNotFound, utils.ErrCodeNotFound, "Invoice not found")
		return
	}

	utils.RespondWithSuccess(c, invoice)
}

// UpdateInvoice updates an invoice
func (h *Handler) UpdateInvoice(c *gin.Context) {
	userID := c.GetString("user_id")
	invoiceID := c.Param("id")

	var req services.UpdateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid data", err.Error())
		return
	}

	invoice, err := h.invoice.UpdateInvoice(invoiceID, userID, &req)
	if err != nil {
		handleInvoiceError(c, err)
		return
	}

	utils.RespondWithSuccess(c, invoice)
}

// UpdateInvoiceItems updates invoice items
func (h *Handler) UpdateInvoiceItems(c *gin.Context) {
	userID := c.GetString("user_id")
	invoiceID := c.Param("id")

	var req []services.InvoiceItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid items", err.Error())
		return
	}

	invoice, err := h.invoice.UpdateInvoiceItems(invoiceID, userID, req)
	if err != nil {
		handleInvoiceError(c, err)
		return
	}

	utils.RespondWithSuccess(c, invoice)
}

// SendInvoice marks invoice as sent
func (h *Handler) SendInvoice(c *gin.Context) {
	userID := c.GetString("user_id")
	invoiceID := c.Param("id")

	invoice, err := h.invoice.SendInvoice(invoiceID, userID)
	if err != nil {
		handleInvoiceError(c, err)
		return
	}

	utils.RespondWithSuccess(c, invoice)
}

// CancelInvoice cancels an invoice
func (h *Handler) CancelInvoice(c *gin.Context) {
	userID := c.GetString("user_id")
	invoiceID := c.Param("id")

	if err := h.invoice.CancelInvoice(invoiceID, userID); err != nil {
		handleInvoiceError(c, err)
		return
	}

	utils.RespondWithSuccess(c, gin.H{"message": "Invoice cancelled successfully"})
}

// GetInvoiceByToken returns invoice by magic token (public)
func (h *Handler) GetInvoiceByToken(c *gin.Context) {
	token := c.Param("token")

	invoice, err := h.invoice.GetInvoiceByMagicToken(token)
	if err != nil {
		utils.RespondWithError(c, http.StatusNotFound, utils.ErrCodeNotFound, "Invoice not found")
		return
	}

	utils.RespondWithSuccess(c, invoice)
}

// GetDashboard returns dashboard stats
func (h *Handler) GetDashboard(c *gin.Context) {
	userID := c.GetString("user_id")
	period := c.DefaultQuery("period", "month")

	stats, err := h.invoice.GetDashboardStats(userID, period)
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, utils.ErrCodeInternalError, "Failed to fetch dashboard data")
		return
	}

	utils.RespondWithSuccess(c, stats)
}

// handleInvoiceError handles invoice-specific errors
func handleInvoiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrInvoiceNotFound):
		utils.RespondWithError(c, http.StatusNotFound, utils.ErrCodeNotFound, "Invoice not found")
	case errors.Is(err, services.ErrCannotEditPaid):
		utils.RespondWithError(c, http.StatusConflict, utils.ErrCodeConflict, "Cannot edit paid invoice")
	case errors.Is(err, services.ErrCannotCancelPaid):
		utils.RespondWithError(c, http.StatusConflict, utils.ErrCodeConflict, "Cannot cancel paid invoice")
	case errors.Is(err, services.ErrAlreadySent):
		utils.RespondWithError(c, http.StatusConflict, utils.ErrCodeConflict, "Invoice already sent")
	case errors.Is(err, services.ErrEmptyItems):
		utils.RespondWithValidationError(c, "Invoice must have items", nil)
	case errors.Is(err, services.ErrInvalidQuantity):
		utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeValidationFailed, "Invalid quantity")
	default:
		utils.RespondWithError(c, http.StatusBadRequest, utils.ErrCodeBadRequest, err.Error())
	}
}
