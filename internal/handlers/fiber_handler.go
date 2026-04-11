package handlers

import (
	"invoicefast/internal/middleware"
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
	auditService    *services.AuditService
}

func NewFiberHandler(
	auth *services.AuthService,
	invoice *services.InvoiceService,
	client *services.ClientService,
	kra *services.KRAService,
	exchange *services.ExchangeRateService,
	pdfWorker *worker.PDFWorker,
	mpesaService *services.MPesaService,
	auditService *services.AuditService,
) *FiberHandler {
	return &FiberHandler{
		authService:     auth,
		invoiceService:  invoice,
		clientService:   client,
		kraService:      kra,
		exchangeService: exchange,
		pdfWorker:       pdfWorker,
		mpesaService:    mpesaService,
		auditService:    auditService,
	}
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





