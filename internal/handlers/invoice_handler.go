package handlers

import (
	"fmt"

	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// InvoiceHandler handles invoice API endpoints
type InvoiceHandler struct {
	invoiceService    *services.InvoiceService
	kraService        *services.KRAService
	mpesaService      *services.MPesaService
	subService        *services.SubscriptionService
	attachmentService *services.AttachmentService
}

// NewInvoiceHandler creates InvoiceHandler
func NewInvoiceHandler(invoiceSvc *services.InvoiceService, kraSvc *services.KRAService, mpesaSvc *services.MPesaService, subSvc *services.SubscriptionService, attachmentSvc *services.AttachmentService) *InvoiceHandler {
	return &InvoiceHandler{
		invoiceService:    invoiceSvc,
		kraService:        kraSvc,
		mpesaService:      mpesaSvc,
		subService:        subSvc,
		attachmentService: attachmentSvc,
	}
}

// CreateInvoice - create new invoice
func (h *InvoiceHandler) CreateInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	if h.subService != nil {
		allowed, reason, _ := h.subService.CheckLimits(tenantID, "invoices", 1)
		if !allowed {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error":   "Invoice limit exceeded",
				"reason":  reason,
				"upgrade": "/billing/upgrade",
			})
		}
	}

	var req services.CreateInvoiceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request", "details": err.Error()})
	}

	userID := middleware.GetUserID(c)
	invoice, err := h.invoiceService.CreateInvoice(tenantID, userID, req.ClientID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if h.subService != nil {
		h.subService.IncrementUsage(tenantID, "invoices", 1)
	}

	return c.Status(fiber.StatusCreated).JSON(invoice)
}

// GetInvoices - list invoices with pagination and filtering
func (h *InvoiceHandler) GetInvoices(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	// Parse pagination
	page := c.Query("page", "1")
	limit := c.Query("limit", "20")
	offset := 0
	_, err := fmt.Sscanf(page, "%d", &offset)
	if err != nil || offset < 1 {
		offset = 1
	}
	lim := 20
	fmt.Sscanf(limit, "%d", &lim)
	if lim < 1 || lim > 100 {
		lim = 20
	}
	offset = (offset - 1) * lim

	filter := services.InvoiceFilter{
		Status:   c.Query("status"),
		ClientID: c.Query("client_id"),
		Search:   c.Query("search"),
		Offset:   offset,
		Limit:    lim,
	}

	invoices, total, err := h.invoiceService.GetUserInvoices(tenantID, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Calculate pagination info
	totalPages := (int(total) + lim - 1) / lim
	currentPage := offset/lim + 1

	return c.JSON(fiber.Map{
		"invoices":    invoices,
		"total":       total,
		"page":        currentPage,
		"total_pages": totalPages,
		"per_page":    lim,
	})
}

// GetInvoice - get single invoice
func (h *InvoiceHandler) GetInvoice(c *fiber.Ctx) error {
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

// UpdateInvoice - update invoice
func (h *InvoiceHandler) UpdateInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	var req services.UpdateInvoiceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	invoice, err := h.invoiceService.UpdateInvoice(invoiceID, tenantID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(invoice)
}

// SendInvoice - send invoice to client
func (h *InvoiceHandler) SendInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	invoice, err := h.invoiceService.SendInvoice(invoiceID, tenantID, "")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(invoice)
}

// CancelInvoice - cancel invoice
func (h *InvoiceHandler) CancelInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	userID := middleware.GetUserID(c)
	err := h.invoiceService.CancelInvoice(tenantID, invoiceID, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if h.subService != nil {
		h.subService.IncrementUsage(tenantID, "invoices", -1)
	}

	return c.JSON(fiber.Map{"message": "Invoice cancelled"})
}

// GetInvoiceByToken - get invoice by magic token (public)
func (h *InvoiceHandler) GetInvoiceByToken(c *fiber.Ctx) error {
	token := c.Params("token")
	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
	}

	return c.JSON(invoice)
}

// GetDashboardStats - get dashboard stats
func (h *InvoiceHandler) GetDashboardStats(c *fiber.Ctx) error {
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

// HandleIntasendWebhook processes Intasend webhook callbacks
func (h *InvoiceHandler) HandleIntasendWebhook(c *fiber.Ctx) error {
	var payload struct {
		Event         string `json:"event"`
		CheckoutID    string `json:"checkout_id"`
		InvoiceNumber string `json:"invoice_number"`
		Amount        string `json:"amount"`
		Reference     string `json:"reference"`
	}

	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	key := c.Get("Idempotency-Key")
	if key == "" {
		key = payload.CheckoutID
	}

	if svc, ok := c.Locals("idempotency_svc").(*services.IdempotencyService); ok && key != "" {
		isProcessed, _ := svc.IsProcessed(c.Context(), key)
		if isProcessed {
			return c.JSON(fiber.Map{"status": "already_processed"})
		}
	}

	tenantID := middleware.GetTenantID(c)
	invoice, err := h.invoiceService.GetInvoiceByNumber(tenantID, payload.InvoiceNumber)
	if err != nil {
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

	h.invoiceService.RecordPayment(invoice.TenantID, invoice.ID, payment)

	if svc, ok := c.Locals("idempotency_svc").(*services.IdempotencyService); ok && key != "" {
		svc.MarkProcessed(c.Context(), key, map[string]interface{}{
			"invoice_id": invoice.ID,
			"amount":     amount,
		})
	}

	return c.JSON(fiber.Map{"status": "received"})
}

// HandleMpesaCallback processes M-Pesa STK callbacks
func (h *InvoiceHandler) HandleMpesaCallback(c *fiber.Ctx) error {
	callback, ok := c.Locals("mpesa_callback").(*services.STKCallback)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "no verified callback data",
			"code":  "INVALID_CALLBACK",
		})
	}

	if h.mpesaService != nil {
		err := h.mpesaService.ProcessSTKCallback(c.Context(), *callback)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "callback processing failed",
				"code":  "PROCESSING_ERROR",
			})
		}
	}

	return c.JSON(fiber.Map{"status": "received"})
}

// GetInvoicePDF gets the PDF for an invoice
func (h *InvoiceHandler) GetInvoicePDF(c *fiber.Ctx) error {
	token := c.Params("token")

	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
	}

	return c.JSON(fiber.Map{"invoice_id": invoice.ID, "invoice_number": invoice.InvoiceNumber})
}

// SubmitToKRA submits an invoice to KRA eTIMS
func (h *InvoiceHandler) SubmitToKRA(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	invoice, err := h.invoiceService.GetInvoiceByID(tenantID, invoiceID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
	}

	if invoice.KRAICN != "" {
		return c.JSON(fiber.Map{
			"message": "already_submitted",
			"icn":     invoice.KRAICN,
			"qr_code": invoice.KRAQRCode,
		})
	}

	kraResp, err := h.invoiceService.SubmitInvoiceToKRA(tenantID, invoiceID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"icn":       kraResp.ICN,
		"qr_code":   kraResp.QRCode,
		"signature": kraResp.Signature,
		"timestamp": kraResp.Timestamp,
	})
}

// CreateCreditNote creates a credit note for an invoice
func (h *InvoiceHandler) CreateCreditNote(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	var req struct {
		Items []services.CreateCreditNoteItem `json:"items"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if len(req.Items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "at least one item required"})
	}

	userID := middleware.GetUserID(c)
	creditNote, err := h.invoiceService.CreateCreditNote(tenantID, userID, invoiceID, req.Items)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(creditNote)
}

// CreateInvoiceAttachment handles file upload for an invoice
func (h *InvoiceHandler) CreateInvoiceAttachment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	userID := middleware.GetUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "user ID required"})
	}

	invoiceID := c.Params("id")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	// Verify invoice exists and belongs to tenant/user
	_, err := h.invoiceService.GetInvoiceByID(tenantID, invoiceID)
	if err != nil {
		if err.Error() == "invoice not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no file uploaded"})
	}

	// Upload attachment
	attachment, err := h.attachmentService.UploadFile(tenantID, invoiceID, file, c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(attachment)
}

// GetInvoiceAttachments retrieves all attachments for an invoice
func (h *InvoiceHandler) GetInvoiceAttachments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	// Verify invoice exists and belongs to tenant
	_, err := h.invoiceService.GetInvoiceByID(tenantID, invoiceID)
	if err != nil {
		if err.Error() == "invoice not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	attachments, err := h.attachmentService.GetAttachments(tenantID, invoiceID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(attachments)
}

// DeleteInvoiceAttachment removes an attachment from an invoice
func (h *InvoiceHandler) DeleteInvoiceAttachment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	attachmentID := c.Params("id")
	if attachmentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "attachment ID required"})
	}

	// Get attachment to verify ownership and get invoice ID
	attachment, err := h.attachmentService.GetAttachmentByID(tenantID, attachmentID)
	if err != nil {
		if err.Error() == "attachment not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "attachment not found"})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Verify invoice belongs to tenant
	if _, err := h.invoiceService.GetInvoiceByID(tenantID, attachment.InvoiceID); err != nil {
		if err.Error() == "invoice not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Delete attachment
	if err := h.attachmentService.DeleteAttachment(tenantID, attachmentID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
