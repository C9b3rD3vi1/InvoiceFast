package handlers

import (
	"fmt"
	"log"
	"time"

	"invoicefast/internal/models"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

// PublicHandler handles public API endpoints
type PublicHandler struct {
	invoiceService       *services.InvoiceService
	authService          *services.AuthService
	paymentService       *services.PaymentService
	mpesaService         *services.MPesaService
	intasendService      *services.IntasendService
	emailTrackingService *services.EmailTrackingService
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

func NewPublicHandlerWithTracking(
	invoice *services.InvoiceService,
	auth *services.AuthService,
	payment *services.PaymentService,
	mpesa *services.MPesaService,
	intasend *services.IntasendService,
	emailTracking *services.EmailTrackingService,
) *PublicHandler {
	return &PublicHandler{
		invoiceService:       invoice,
		authService:          auth,
		paymentService:       payment,
		mpesaService:         mpesa,
		intasendService:      intasend,
		emailTrackingService: emailTracking,
	}
}

// ServeLanding serves the landing page
func (h *PublicHandler) ServeLanding(c *fiber.Ctx) error {
	return c.SendFile("./views/pages/landing.html")
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

// HandleContact - contact form JSON (non-auth public)
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

// HandleLogin processes login form submission
func (h *PublicHandler) HandleLogin(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email and password required"})
	}

	return c.JSON(fiber.Map{"message": "Use /api/v1/auth/login endpoint"})
}

// HandleRegister processes registration form submission
func (h *PublicHandler) HandleRegister(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "Use /api/v1/auth/register endpoint"})
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

// TrackOpen - tracking pixel for email open
func (h *PublicHandler) TrackOpen(c *fiber.Ctx) error {
	if h.emailTrackingService == nil {
		return c.Status(fiber.StatusNotImplemented).SendString("")
	}

	trackingID := c.Params("trackingId")
	if trackingID == "" {
		return c.Status(fiber.StatusBadRequest).SendString("")
	}

	userAgent := c.Get("User-Agent")
	ipAddress := c.IP()

	if err := h.emailTrackingService.TrackOpen(trackingID, userAgent, ipAddress); err != nil {
		log.Printf("Email open tracking error: %v", err)
	}

	// Return 1x1 transparent GIF
	c.Set("Content-Type", "image/gif")
	c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	return c.SendString("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\x00\x00\x00!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
}

// TrackClick - redirect to original URL after tracking click
func (h *PublicHandler) TrackClick(c *fiber.Ctx) error {
	if h.emailTrackingService == nil {
		return c.Status(fiber.StatusNotImplemented).Redirect("/")
	}

	linkID := c.Params("linkId")
	trackingID := c.Params("trackingId")

	if linkID == "" || trackingID == "" {
		return c.Status(fiber.StatusBadRequest).Redirect("/")
	}

	originalURL, err := h.emailTrackingService.TrackClick(linkID, trackingID)
	if err != nil {
		log.Printf("Email click tracking error: %v", err)
		return c.Status(fiber.StatusNotFound).Redirect("/")
	}

	return c.Redirect(originalURL, fiber.StatusFound)
}
