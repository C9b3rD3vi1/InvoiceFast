package main

import (
	"fmt"
	"invoicefast/internal/database"
	"invoicefast/internal/models"
	"invoicefast/internal/services"
	"invoicefast/internal/utils"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HandlePaymentRequest initiates a payment for an invoice via M-Pesa
func HandlePaymentRequest(c *gin.Context, db *database.DB, invoiceService *services.InvoiceService, intasendService *services.IntasendService) {
	invoiceID := c.Param("id")

	var req struct {
		Method string `json:"method"`
		Phone  string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, "Invalid request", err.Error())
		return
	}

	// Get user from context
	userID, exists := c.Get("user_id")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, utils.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	// Default to M-Pesa
	if req.Method == "" {
		req.Method = "mpesa"
	}

	// Get invoice
	invoice, err := invoiceService.GetInvoiceByID(invoiceID, userID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusNotFound, utils.ErrCodeNotFound, "Invoice not found")
		return
	}

	// Check if already paid
	if invoice.Status == "paid" {
		utils.RespondWithError(c, http.StatusConflict, utils.ErrCodeConflict, "Invoice already paid")
		return
	}

	// Use provided phone or fall back to client phone
	phone := req.Phone
	if phone == "" {
		phone = invoice.Client.Phone
	}

	// Initiate STK push via Intasend
	if intasendService != nil && phone != "" {
		result, err := intasendService.InitiateSTKPush(services.InitiatePaymentRequest{
			Amount:        invoice.Total,
			Currency:      invoice.Currency,
			PhoneNumber:   phone,
			APIRef:       invoice.InvoiceNumber,
			InvoiceNumber: invoice.InvoiceNumber,
		})
		if err != nil {
			log.Printf("STK push failed: %v", err)
		}

		if result != nil {
			utils.RespondWithSuccess(c, gin.H{
				"message":     "Payment request sent to your phone",
				"checkout_id": result.ID,
				"invoice_id":  invoiceID,
				"amount":      invoice.Total,
				"currency":    invoice.Currency,
				"status":      "pending",
			})
			return
		}
	}

	// Fallback if no Intasend configured
	utils.RespondWithSuccess(c, gin.H{
		"message":    "Payment initiated",
		"invoice_id": invoiceID,
		"amount":     invoice.Total,
		"currency":   invoice.Currency,
		"status":     "pending",
	})
}

// HandleIntasendWebhook processes callbacks from Intasend
func HandleIntasendWebhook(c *gin.Context, db *database.DB, invoiceService *services.InvoiceService, intasendService *services.IntasendService) {
	var payload struct {
		Event         string `json:"event"`
		CheckoutID    string `json:"checkout_id"`
		InvoiceNumber string `json:"invoice_number"`
		State         string `json:"state"`
		Amount        string `json:"amount"`
		Currency      string `json:"currency"`
		CustomerEmail string `json:"customer_email"`
		CustomerPhone string `json:"customer_phone"`
		Reference     string `json:"reference"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Printf("Webhook binding error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	log.Printf("Received Intasend webhook: event=%s, checkout=%s, state=%s",
		payload.Event, payload.CheckoutID, payload.State)

	if payload.InvoiceNumber == "" {
		log.Printf("No invoice number in webhook payload")
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	// Find invoice
	var invoice models.Invoice
	err := db.Preload("Client").Preload("Items").Preload("Payments").
		First(&invoice, "invoice_number = ?", payload.InvoiceNumber).Error

	if err != nil {
		log.Printf("Invoice not found for webhook: %s, error: %v", payload.InvoiceNumber, err)
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	// Handle different event types
	switch payload.Event {
	case "payment_reversed", "chargeback":
		invoice.Status = "sent"
		db.Save(&invoice)

	case "payment_successful", "invoice_payment_signed":
		amount := 0.0
		if payload.Amount != "" {
			var parsed float64
			_, err := fmt.Sscanf(payload.Amount, "%f", &parsed)
			if err == nil {
				amount = parsed
			}
		}

		if amount == 0 {
			amount = invoice.Total
		}

		payment := models.Payment{
			ID:          fmt.Sprintf("pay-%d", time.Now().UnixNano()),
			InvoiceID:   invoice.ID,
			UserID:      invoice.UserID,
			Amount:      amount,
			Currency:    invoice.Currency,
			Method:      models.PaymentMethodMpesa,
			Status:      models.PaymentStatusCompleted,
			Reference:   payload.Reference,
		}
		payment.CompletedAt.Valid = true
		payment.CompletedAt.Time = time.Now()

		db.Create(&payment)

		invoice.Status = "paid"
		db.Save(&invoice)

		log.Printf("Payment recorded for invoice %s: %f", invoice.InvoiceNumber, amount)

	default:
		log.Printf("Unhandled webhook event: %s", payload.Event)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}
