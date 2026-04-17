package handlers

import (
	"time"

	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

type SettlementHandler struct {
	service *services.MPaySettlementService
}

func NewSettlementHandler(svc *services.MPaySettlementService) *SettlementHandler {
	return &SettlementHandler{service: svc}
}

func (h *SettlementHandler) GetDailySettlement(c *fiber.Ctx) error {
	dateStr := c.Query("date")
	var date time.Time
	var err error

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid date format"})
		}
	} else {
		date = time.Now()
	}

	report, err := h.service.GenerateDailySettlement(date)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(report)
}

func (h *SettlementHandler) ExportSettlement(c *fiber.Ctx) error {
	dateStr := c.Query("date")
	var date time.Time
	var err error

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid date format"})
		}
	} else {
		date = time.Now()
	}

	csv, err := h.service.ExportSettlementCSV(date)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", "attachment; filename=settlement-"+date.Format("2006-01-02")+".csv")
	return c.SendString(csv)
}
