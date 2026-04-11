package handlers

import (
	"fmt"
	"time"

	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// PublicHandler handles public API endpoints
type PublicHandler struct {
	invoiceService  *services.InvoiceService
	authService     *services.AuthService
	paymentService  *services.PaymentService
	mpesaService    *services.MPesaService
	intasendService *services.IntasendService
}

// NewPublicHandler creates a new PublicHandler
func NewPublicHandler(
	invoice *services.InvoiceService,
	auth *services.AuthService,
	payment *services.PaymentService,
	mpesa *services.MPesaService,
	intasend *services.IntasendService,
) *PublicHandler {
	return &PublicHandler{
		invoiceService:  invoice,
		authService:     auth,
		paymentService:  payment,
		mpesaService:    mpesa,
		intasendService: intasend,
	}
}

// ServeLanding - DEPRECATED: use routes
func (h *PublicHandler) ServeLanding(c *fiber.Ctx) error {
	return c.Redirect("/")
}

// ServePortal - get public invoice by magic token
func (h *PublicHandler) ServePortal(c *fiber.Ctx) error {
	token := c.Params("token")
	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Invoice not found"})
	}

	return c.JSON(fiber.Map{
		"id":            invoice.ID,
		"invoiceNumber": invoice.InvoiceNumber,
		"total":         invoice.Total,
		"currency":      invoice.Currency,
		"status":        invoice.Status,
		"dueDate":       invoice.DueDate,
	})
}

// ServeSuccess - payment success JSON
func (h *PublicHandler) ServeSuccess(c *fiber.Ctx) error {
	token := c.Params("token")
	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil || invoice.Status != models.InvoiceStatusPaid {
		return c.JSON(fiber.Map{"message": "Payment pending"})
	}
	return c.JSON(fiber.Map{"message": "Payment successful"})
}

// Deprecated: use routes
func (h *PublicHandler) ServeLogin(c *fiber.Ctx) error    { return c.Redirect("/login") }
func (h *PublicHandler) ServeRegister(c *fiber.Ctx) error { return c.Redirect("/register") }
func (h *PublicHandler) ServeContact(c *fiber.Ctx) error  { return c.Redirect("/contact") }

// HandleContact - contact form JSON
func (h *PublicHandler) HandleContact(c *fiber.Ctx) error {
	var req struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Message string `json:"message"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	if req.Name == "" || req.Email == "" || req.Message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "All fields required"})
	}
	return c.JSON(fiber.Map{"message": "Thank you! We'll contact you soon.", "ticket": "TKT-" + fmt.Sprint(time.Now().Unix())})
}

// HandleLogin - login JSON API
func (h *PublicHandler) HandleLogin(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	resp, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
	}
	return c.JSON(fiber.Map{"access_token": resp.AccessToken, "refresh_token": resp.RefreshToken})
}

// HandleRegister - register JSON API
func (h *PublicHandler) HandleRegister(c *fiber.Ctx) error {
	var req services.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	resp, err := h.authService.Register(&req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"access_token": resp.AccessToken, "refresh_token": resp.RefreshToken})
}

// GetInvoiceByToken - public invoice by magic token
func (h *PublicHandler) GetInvoiceByToken(c *fiber.Ctx) error {
	token := c.Params("token")
	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Not found"})
	}
	return c.JSON(invoice)
}

// GetInvoicePDF - get invoice PDF
func (h *PublicHandler) GetInvoicePDF(c *fiber.Ctx) error {
	token := c.Params("token")
	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Not found"})
	}
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", invoice.InvoiceNumber))
	return c.SendString("PDF generation not implemented")
}

// GetInvoiceReceipt - get receipt PDF
func (h *PublicHandler) GetInvoiceReceipt(c *fiber.Ctx) error {
	return h.GetInvoicePDF(c)
}

// InitiateSTKPush - initiate payment
func (h *PublicHandler) InitiateSTKPush(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "Use payment portal"})
}

// CheckPaymentStatus - check payment status
func (h *PublicHandler) CheckPaymentStatus(c *fiber.Ctx) error {
	token := c.Params("token")
	invoice, err := h.invoiceService.GetInvoiceByMagicToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Not found"})
	}
	return c.JSON(fiber.Map{"status": invoice.Status, "paidAmount": invoice.PaidAmount})
}

// GetPricing - get pricing info
func (h *PublicHandler) GetPricing(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"plans": []fiber.Map{
			{"name": "Free", "price": 0, "features": []string{"50 invoices/month", "Email support"}},
			{"name": "Pro", "price": 2900, "features": []string{"Unlimited invoices", "Priority support", "KRA integration"}},
		},
	})
}
