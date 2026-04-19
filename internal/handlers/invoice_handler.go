package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"time"

	"invoicefast/internal/middleware"
	"invoicefast/internal/models"
	"invoicefast/internal/pdf"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// InvoiceHandler handles invoice API endpoints
type InvoiceHandler struct {
	invoiceService    *services.InvoiceService
	kraService        *services.KRAService
	mpesaService      *services.MPesaService
	subService        *services.SubscriptionService
	attachmentService *services.AttachmentService
	pdfService        *services.PDFService
	pdfGenerator      *pdf.PDFGenerator
	emailService      *services.EmailService
	whatsappService   *services.WhatsAppService
}

// NewInvoiceHandler creates InvoiceHandler
func NewInvoiceHandler(invoiceSvc *services.InvoiceService, kraSvc *services.KRAService, mpesaSvc *services.MPesaService, subSvc *services.SubscriptionService, attachmentSvc *services.AttachmentService, pdfSvc *services.PDFService, pdfGen *pdf.PDFGenerator, whatsappSvc *services.WhatsAppService) *InvoiceHandler {
	return &InvoiceHandler{
		invoiceService:    invoiceSvc,
		kraService:        kraSvc,
		mpesaService:      mpesaSvc,
		subService:        subSvc,
		attachmentService: attachmentSvc,
		pdfService:        pdfSvc,
		pdfGenerator:      pdfGen,
		whatsappService:   whatsappSvc,
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

// GetKRADashboardStats returns KRA compliance stats
func (h *InvoiceHandler) GetKRADashboardStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	stats, err := h.invoiceService.GetKRADashboardStats(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
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
		Status:    c.Query("status"),
		ClientID:  c.Query("client_id"),
		Search:    c.Query("search"),
		KRAStatus: c.Query("kra_status"),
		Offset:    offset,
		Limit:     lim,
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
	invoice, err := h.invoiceService.GetInvoiceByID(tenantID, invoiceID)
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

	invoice, err := h.invoiceService.UpdateInvoice(tenantID, invoiceID, &req)
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
	invoice, err := h.invoiceService.SendInvoice(tenantID, invoiceID, "")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(invoice)
}

// SendReminder - sends payment reminder to client
func (h *InvoiceHandler) SendReminder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	invoice, err := h.invoiceService.GetInvoiceByID(tenantID, invoiceID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
	}

	// Send reminder email using email service
	if h.emailService != nil {
		reminder := &services.ReminderEmailData{
			InvoiceNumber: invoice.InvoiceNumber,
			ClientName:    invoice.Client.Name,
			ClientEmail:   invoice.Client.Email,
			Amount:        invoice.Total - invoice.PaidAmount,
			Currency:      invoice.Currency,
			DueDate:       invoice.DueDate.Format("2006-01-02"),
			InvoiceLink:   invoice.PaymentLink,
		}
		if err := h.emailService.SendPaymentReminder(reminder); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to send reminder"})
		}
	}

	return c.JSON(fiber.Map{"message": "reminder sent"})
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

// GetInvoiceStats - get invoice-specific stats for the invoices list page
func (h *InvoiceHandler) GetInvoiceStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	stats, err := h.invoiceService.GetInvoiceStats(tenantID)
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

	// Generate HTML for the invoice using the preloaded User
	htmlContent, err := h.pdfService.GenerateInvoiceHTML(invoice, &invoice.User)
	if err != nil {
		log.Printf("HTML generation failed for %s: %v", invoiceID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate invoice HTML"})
	}
	log.Printf("HTML generated for %s, length: %d", invoice.InvoiceNumber, len(htmlContent))

	// Generate PDF if generator is available
	if h.pdfGenerator != nil {
		log.Printf("PDF generator available, generating PDF for %s", invoice.InvoiceNumber)
		pdfOutput, err := h.pdfGenerator.HtmlToPDF(htmlContent, invoice.InvoiceNumber)
		if err == nil && pdfOutput != nil && len(pdfOutput.Content) > 0 {
			log.Printf("PDF generated successfully for %s, size: %d bytes", invoice.InvoiceNumber, len(pdfOutput.Content))
			c.Set("Content-Type", "application/pdf")
			c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", pdfOutput.Filename))
			return c.Send(pdfOutput.Content)
		}
		// Fallback to HTML if PDF generation fails
		log.Printf("PDF generation failed for %s: %v, html length: %d, falling back to HTML", invoice.InvoiceNumber, err, len(htmlContent))
	} else {
		log.Printf("PDF generator is nil, falling back to HTML")
	}

	// Fallback to HTML download
	c.Set("Content-Type", "text/html")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.html", invoice.InvoiceNumber))
	return c.SendString(htmlContent)
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

	if invoice.KRAICN != "" && invoice.KRAStatus == "submitted" {
		return c.JSON(fiber.Map{
			"message": "already_submitted",
			"icn":     invoice.KRAICN,
			"qr_code": invoice.KRAQRCode,
		})
	}

	if invoice.KRAStatus == "failed" {
		h.invoiceService.ClearKRAData(tenantID, invoiceID)
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

// GetKRAStatus gets KRA submission status for an invoice
func (h *InvoiceHandler) GetKRAStatus(c *fiber.Ctx) error {
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

	return c.JSON(fiber.Map{
		"submitted":    invoice.KRAICN != "",
		"icn":          invoice.KRAICN,
		"qr_code":      invoice.KRAQRCode,
		"status":       invoice.KRAStatus,
		"submitted_at": invoice.KRASubmittedAt,
		"error":        invoice.KRAError,
	})
}

// RetryKRA retries KRA submission for an invoice
func (h *InvoiceHandler) RetryKRA(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	_, err := h.invoiceService.GetInvoiceByID(tenantID, invoiceID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invoice not found"})
	}

	h.invoiceService.ClearKRAData(tenantID, invoiceID)

	kraResp, err := h.invoiceService.SubmitInvoiceToKRA(tenantID, invoiceID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"icn":       kraResp.ICN,
		"qr_code":   kraResp.QRCode,
		"signature": kraResp.Signature,
		"timestamp": kraResp.Timestamp,
		"message":   "retry_successful",
	})
}

// GetKRAActivityFeed returns recent KRA activity
func (h *InvoiceHandler) GetKRAActivityFeed(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	limit := 50
	if l := c.QueryInt("limit", 50); l > 0 && l <= 100 {
		limit = l
	}

	events, err := h.invoiceService.GetKRAActivityFeed(tenantID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"events": events})
}

// SubmitAllPendingToKRA submits all pending invoices to KRA
func (h *InvoiceHandler) SubmitAllPendingToKRA(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	submitted, failed, err := h.invoiceService.SubmitAllPendingToKRA(tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"submitted": submitted,
		"failed":    failed,
		"message":   fmt.Sprintf("Submitted %d, failed %d", submitted, failed),
	})
}

// RecordPayment records a payment for an invoice
func (h *InvoiceHandler) RecordPayment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	var req struct {
		Amount    float64 `json:"amount"`
		Method    string  `json:"method"`
		Reference string  `json:"reference"`
		Date      string  `json:"date"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	// Get user ID from context
	userID := ""
	if uid := c.Locals("user_id"); uid != nil {
		userID = uid.(string)
	}

	// Parse date
	var paymentDate time.Time
	if req.Date != "" {
		paymentDate, _ = time.Parse("2006-01-02", req.Date)
	} else {
		paymentDate = time.Now()
	}

	// Create payment with all required fields - ensure status is explicitly set to completed
	// Include ID to avoid constraint issues
	payment := &models.Payment{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		UserID:      userID,
		InvoiceID:   invoiceID,
		Amount:      req.Amount,
		Currency:    "KES",
		Method:      models.PaymentMethod(req.Method),
		Status:      models.PaymentStatusCompleted,
		Reference:   req.Reference,
		CompletedAt: sql.NullTime{Time: paymentDate, Valid: true},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Generate unique ID to bypass any unique constraints on tenant_id
	payment.ID = uuid.New().String()

	if err := h.invoiceService.RecordPayment(tenantID, invoiceID, payment); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "payment recorded"})
}

// DeleteInvoice deletes an invoice (hard delete)
func (h *InvoiceHandler) DeleteInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "tenant required"})
	}

	invoiceID := c.Params("id")
	if invoiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invoice ID required"})
	}

	if err := h.invoiceService.DeleteInvoice(tenantID, invoiceID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "invoice deleted"})
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

// SendWhatsApp sends invoice via WhatsApp with PDF attachment
func (h *InvoiceHandler) SendWhatsApp(c *fiber.Ctx) error {
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

	// Check if client has phone number
	if invoice.Client.Phone == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "client has no phone number"})
	}

	// Build payment link
	paymentLink := ""
	if invoice.MagicToken != "" {
		paymentLink = fmt.Sprintf("https://invoice.simuxtech.com/pay/%s", invoice.MagicToken)
	}

	// Build message
	message := fmt.Sprintf(`Hello %s,

You have received an invoice from %s.

Invoice #: %s
Amount: %s %s
Due Date: %s

View and pay: %s

Thank you for your business!`,
		invoice.Client.Name,
		invoice.User.CompanyName,
		invoice.InvoiceNumber,
		invoice.Currency,
		fmt.Sprintf("%.2f", invoice.Total),
		invoice.DueDate.Format("02 Jan 2006"),
		paymentLink,
	)

	// Generate PDF
	var pdfData []byte
	pdfName := invoice.InvoiceNumber + ".pdf"
	if h.pdfService != nil && h.pdfGenerator != nil {
		htmlContent, err := h.pdfService.GenerateInvoiceHTML(invoice, &invoice.User)
		if err == nil {
			pdfOutput, err := h.pdfGenerator.HtmlToPDF(htmlContent, invoice.InvoiceNumber)
			if err == nil && len(pdfOutput.Content) > 0 {
				pdfData = pdfOutput.Content
			}
		}
	}

	// Send WhatsApp with PDF
	var result *services.WhatsAppResult
	if h.whatsappService != nil {
		result = h.whatsappService.SendWithPDF(invoice.Client.Phone, message, pdfData, pdfName)
	} else {
		// WhatsApp service not initialized, create result with wa.me URL and PDF download link
		pdfDownloadURL := fmt.Sprintf("https://invoice.simuxtech.com/api/invoices/%s/pdf", invoiceID)
		waMeURL := fmt.Sprintf("https://wa.me/%s?text=%s", invoice.Client.Phone, url.QueryEscape(message+"\n\n📎 Download Invoice PDF: "+pdfDownloadURL))
		result = &services.WhatsAppResult{
			Sent:    false,
			URL:     waMeURL,
			Message: message,
			Phone:   invoice.Client.Phone,
			PDFData: pdfData,
			PDFName: pdfName,
			HasPDF:  len(pdfData) > 0,
		}
	}

	return c.JSON(fiber.Map{
		"sent":     result.Sent,
		"url":      result.URL,
		"message":  result.Message,
		"phone":    result.Phone,
		"has_pdf":  result.HasPDF,
		"pdf_name": result.PDFName,
	})
}
