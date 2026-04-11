package handlers

import(
	"fmt"
	"time"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"
	"invoicefast/internal/worker"
	
	"github.com/gofiber/fiber/v2"
	
)

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