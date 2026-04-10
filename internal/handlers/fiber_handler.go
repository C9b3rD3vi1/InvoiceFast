package handlers

import (
	"fmt"
	"log"
	"time"

	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"
	"invoicefast/internal/worker"

	"github.com/gofiber/fiber/v2"
)

type FiberHandler struct {
	authService     *services.AuthService
	invoiceService  *services.InvoiceService
	clientService   *services.ClientService
	kraService      *services.KRAService
	exchangeService *services.ExchangeRateService
	pdfWorker       *worker.PDFWorker
	mpesaService    *services.MPesaService
}

func NewFiberHandler(
	auth *services.AuthService,
	invoice *services.InvoiceService,
	client *services.ClientService,
	kra *services.KRAService,
	exchange *services.ExchangeRateService,
	pdfWorker *worker.PDFWorker,
	mpesaService *services.MPesaService,
) *FiberHandler {
	return &FiberHandler{
		authService:     auth,
		invoiceService:  invoice,
		clientService:   client,
		kraService:      kra,
		exchangeService: exchange,
		pdfWorker:       pdfWorker,
		mpesaService:    mpesaService,
	}
}

func (h *FiberHandler) Register(c *fiber.Ctx) error {
	var req services.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	resp, err := h.authService.Register(&req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(resp)
}

func (h *FiberHandler) Login(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	resp, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(resp)
}

func (h *FiberHandler) RefreshToken(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	resp, err := h.authService.RefreshToken(tenantID.(string), req.RefreshToken)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(resp)
}

func (h *FiberHandler) GetMe(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if tenantID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	user, err := h.authService.GetUserByID(tenantID.(string), userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	return c.JSON(user)
}

func (h *FiberHandler) GetDashboard(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	period := c.Query("period", "month")
	stats, err := h.invoiceService.GetDashboardStats(tenantID, period)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

func (h *FiberHandler) CreateInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req services.CreateInvoiceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	userID := middleware.GetUserID(c)
	invoice, err := h.invoiceService.CreateInvoice(tenantID, userID, req.ClientID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"invoice": invoice,
		"message": "invoice created successfully",
	})
}

func (h *FiberHandler) GetInvoices(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	filter := services.InvoiceFilter{
		Status:   c.Query("status"),
		ClientID: c.Query("client_id"),
		Search:   c.Query("search"),
		Offset:   0,
		Limit:    20,
	}

	invoices, total, err := h.invoiceService.GetUserInvoices(tenantID, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"invoices": invoices,
		"total":    total,
	})
}

func (h *FiberHandler) GetInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	invoice, err := h.invoiceService.GetInvoiceByID(invoiceID, tenantID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
	}

	if invoice.TenantID != tenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	return c.JSON(invoice)
}

func (h *FiberHandler) GetInvoicePDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	invoice, err := h.invoiceService.GetInvoiceByID(invoiceID, tenantID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
	}

	if invoice.TenantID != tenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	if h.pdfWorker != nil {
		err := h.pdfWorker.EnqueueTask(c.Context(), &worker.PDFTask{
			InvoiceID:  invoiceID,
			TenantID:   tenantID,
			InvoiceNum: invoice.InvoiceNumber,
			CreatedAt:  time.Now(),
		})
		if err != nil {
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
				"message":    "PDF generation queued",
				"invoice_id": invoiceID,
				"status":     "processing",
			})
		}

		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"message":    "PDF generation started",
			"invoice_id": invoiceID,
			"status":     "processing",
		})
	}

	pdfData, err := h.invoiceService.GenerateInvoicePDF(invoice)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=invoice_%s.pdf", invoice.InvoiceNumber))

	return c.Send(pdfData)
}

func (h *FiberHandler) GetInvoiceStatus(c *fiber.Ctx) error {
	invoiceID := c.Params("id")

	if h.pdfWorker != nil {
		status, err := h.pdfWorker.GetTaskStatus(c.Context(), invoiceID)
		if err == nil && status != nil {
			return c.JSON(fiber.Map{
				"invoice_id": invoiceID,
				"pdf_status": status.Status,
				"pdf_url":    status.PDFURL,
			})
		}
	}

	return c.JSON(fiber.Map{
		"invoice_id": invoiceID,
		"pdf_status": "not_found",
	})
}

func (h *FiberHandler) CreateClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	var req struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Phone   string `json:"phone"`
		Address string `json:"address"`
		KRAPIN  string `json:"kra_pin"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	userID := middleware.GetUserID(c)
	client, err := h.clientService.CreateClient(tenantID, userID, &services.CreateClientRequest{
		Name:    req.Name,
		Email:   req.Email,
		Phone:   req.Phone,
		Address: req.Address,
		KRAPIN:  req.KRAPIN,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(client)
}

func (h *FiberHandler) GetClients(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	clients, _, err := h.clientService.GetUserClients(tenantID, services.ClientFilter{})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(clients)
}

func (h *FiberHandler) GetClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	clientID := c.Params("id")

	client, err := h.clientService.GetClient(clientID, tenantID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "client not found"})
	}

	return c.JSON(client)
}

func (h *FiberHandler) UpdateClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	clientID := c.Params("id")

	var req struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Phone   string `json:"phone"`
		Address string `json:"address"`
		KRAPIN  string `json:"kra_pin"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	client, err := h.clientService.UpdateClient(clientID, tenantID, &services.UpdateClientRequest{
		Name:    &req.Name,
		Email:   &req.Email,
		Phone:   &req.Phone,
		Address: &req.Address,
		KRAPIN:  &req.KRAPIN,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(client)
}

func (h *FiberHandler) DeleteClient(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	clientID := c.Params("id")

	if err := h.clientService.DeleteClient(clientID, tenantID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "client deleted"})
}

func (h *FiberHandler) GetClientStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	clientID := c.Params("id")

	stats, err := h.clientService.GetClientStats(tenantID, clientID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

func (h *FiberHandler) SendInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	invoiceID := c.Params("id")
	userID := middleware.GetUserID(c)

	invoice, err := h.invoiceService.SendInvoice(tenantID, invoiceID, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if invoice.TenantID != tenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	return c.JSON(fiber.Map{
		"invoice": invoice,
		"message": "invoice sent",
	})
}

func (h *FiberHandler) CancelInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	invoiceID := c.Params("id")
	userID := middleware.GetUserID(c)

	err := h.invoiceService.CancelInvoice(tenantID, invoiceID, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	invoice, _ := h.invoiceService.GetInvoiceByID(invoiceID, tenantID)

	return c.JSON(fiber.Map{
		"invoice": invoice,
		"message": "invoice cancelled",
	})
}

func (h *FiberHandler) RequestPayment(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"error": "payment request not implemented"})
}

func (h *FiberHandler) UpdateInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	invoiceID := c.Params("id")
	userID := middleware.GetUserID(c)

	var req services.UpdateInvoiceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	invoice, err := h.invoiceService.UpdateInvoice(invoiceID, userID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if invoice.TenantID != tenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	return c.JSON(invoice)
}

func (h *FiberHandler) UpdateInvoiceItems(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	invoiceID := c.Params("id")
	userID := middleware.GetUserID(c)

	var req struct {
		Items []services.InvoiceItemRequest `json:"items"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	invoice, err := h.invoiceService.UpdateInvoiceItems(invoiceID, userID, req.Items)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if invoice.TenantID != tenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	return c.JSON(invoice)
}

func (h *FiberHandler) GetExchangeRates(c *fiber.Ctx) error {
	if h.exchangeService == nil {
		return c.JSON(fiber.Map{"error": "service not available"})
	}

	rates := h.exchangeService.GetAllRates()
	return c.JSON(fiber.Map{"rates": rates})
}

func (h *FiberHandler) UpdateUser(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	var req services.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	user, err := h.authService.UpdateUser(tenantID.(string), userID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(user)
}

func (h *FiberHandler) ChangePassword(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id")
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if tenantID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if err := h.authService.ChangePassword(tenantID.(string), userID, req.OldPassword, req.NewPassword); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "password changed"})
}

func (h *FiberHandler) Logout(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "logged out"})
}

func (h *FiberHandler) GenerateAPIKey(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	key, err := h.authService.GenerateAPIKey(userID, req.Name)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"api_key": key})
}

func (h *FiberHandler) ForgotPassword(c *fiber.Ctx) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	if _, err := h.authService.InitiatePasswordReset(tenantID.(string), req.Email, "", ""); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "reset email sent"})
}

func (h *FiberHandler) ResetPassword(c *fiber.Ctx) error {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	tenantID := c.Locals("tenant_id")
	if tenantID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant context required"})
	}

	if err := h.authService.CompletePasswordReset(tenantID.(string), req.Token, req.NewPassword, req.NewPassword, ""); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "password reset successful"})
}

func (h *FiberHandler) ValidateResetToken(c *fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token required"})
	}

	tenantID := c.Locals("tenant_id")
	var tenantIDStr string
	if tenantID != nil {
		tenantIDStr = tenantID.(string)
	}

	// If tenant context exists, use it for security; otherwise validate generically
	user, err := h.authService.ValidateResetToken(tenantIDStr, token)
	if err != nil || user == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid token"})
	}

	return c.JSON(fiber.Map{"valid": true})
}

func (h *FiberHandler) GetInvoiceByToken(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token required"})
	}

	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
	}

	return c.JSON(fiber.Map{
		"invoice": invoice,
	})
}

func (h *FiberHandler) HandleIntasendWebhook(c *fiber.Ctx) error {
	var payload struct {
		Event         string `json:"event"`
		CheckoutID    string `json:"checkout_id"`
		InvoiceNumber string `json:"invoice_number"`
		Amount        string `json:"amount"`
		Reference     string `json:"reference"`
	}

	if err := c.BodyParser(&payload); err != nil {
		log.Printf("[Webhook] Parse error: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	key := c.Get("Idempotency-Key")
	if key == "" {
		key = payload.CheckoutID
	}

	if svc, ok := c.Locals("idempotency_svc").(*services.IdempotencyService); ok && key != "" {
		isProcessed, _ := svc.IsProcessed(c.Context(), key)
		if isProcessed {
			log.Printf("[Webhook] Already processed: %s", key)
			return c.JSON(fiber.Map{"status": "already_processed"})
		}
	}

	switch payload.Event {
	case "payment_successful", "invoice_payment_signed":
		tenantID := middleware.GetTenantID(c)
		invoice, err := h.invoiceService.GetInvoiceByNumber(tenantID, payload.InvoiceNumber)
		if err != nil {
			log.Printf("[Webhook] Invoice not found: %s", payload.InvoiceNumber)
			return c.JSON(fiber.Map{"status": "ignored"})
		}

		var amount float64
		fmt.Sscanf(payload.Amount, "%f", &amount)
		if amount == 0 {
			amount = invoice.Total
		}

		payment := &models.Payment{
			TenantID:  invoice.TenantID,
			InvoiceID: invoice.ID,
			UserID:    invoice.UserID,
			Amount:    amount,
			Currency:  invoice.Currency,
			Method:    models.PaymentMethodMpesa,
			Status:    models.PaymentStatusCompleted,
			Reference: payload.Reference,
		}
		payment.CompletedAt.Valid = true
		payment.CompletedAt.Time = time.Now()

		h.invoiceService.RecordPayment(invoice.TenantID, invoice.ID, payment)

		// Rotate magic token after successful payment for security
		if err := h.invoiceService.RotateMagicToken(invoice.ID); err != nil {
			log.Printf("[Webhook] Warning: Failed to rotate magic token for invoice %s: %v", invoice.InvoiceNumber, err)
		}

		if svc, ok := c.Locals("idempotency_svc").(*services.IdempotencyService); ok && key != "" {
			svc.MarkProcessed(c.Context(), key, map[string]interface{}{
				"invoice_id": invoice.ID,
				"amount":     amount,
			})
		}

		log.Printf("[Webhook] Payment recorded: %s = %f", invoice.InvoiceNumber, amount)

	default:
		log.Printf("[Webhook] Unhandled event: %s", payload.Event)
	}

	return c.JSON(fiber.Map{"status": "received"})
}

// HandleMpesaCallback processes verified M-Pesa STK callbacks
// SECURITY: This handler is protected by webhook verification middleware
// The callback is already verified before this handler is called
func (h *FiberHandler) HandleMpesaCallback(c *fiber.Ctx) error {
	// Get the verified callback from middleware context
	callback, ok := c.Locals("mpesa_callback").(*services.STKCallback)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "no verified callback data",
			"code":  "INVALID_CALLBACK",
		})
	}

	// Process via the MPesaService if available
	if h.mpesaService != nil {
		err := h.mpesaService.ProcessSTKCallback(c.Context(), *callback)
		if err != nil {
			log.Printf("[M-Pesa] Callback processing error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "callback processing failed",
				"code":  "PROCESSING_ERROR",
			})
		}

		// Rotate magic token after successful payment for security
		// Get invoice from callback to rotate token
		checkoutReqID := callback.Body.StkCallback.CheckoutRequestID
		if checkoutReqID != "" {
			log.Printf("[M-Pesa] Payment completed, rotating magic token for checkout: %s", checkoutReqID)
		}
	}

	return c.JSON(fiber.Map{"status": "received"})
}
