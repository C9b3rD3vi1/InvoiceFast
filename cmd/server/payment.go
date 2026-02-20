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

	// Get user from context (set by auth middleware)
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
	if phone == "" && invoice.Client != nil {
		phone = invoice.Client.Phone
	}

	// Initiate STK push via Intasend
	if intasendService != nil && phone != "" {
		result, err := intasendService.InitiateSTKPush(phone, invoice.Total, invoice.InvoiceNumber)
		if err != nil {
			log.Printf("STK push failed: %v", err)
			// Continue - may work offline
		}

		if result != nil {
			utils.RespondWithSuccess(c, gin.H{
				"message":     "Payment request sent to your phone",
				"checkout_id": result.CheckoutID,
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

	// Find invoice by invoice_number
	if payload.InvoiceNumber == "" {
		log.Printf("No invoice number in webhook payload")
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	// Try to find invoice by number (public access for webhook)
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
		// Payment was reversed
		invoice.Status = "sent"
		invoiceService.UpdateInvoice(invoice.ID, invoice)

	case "payment_successful", "invoice_payment_signed":
		// Payment successful - record it
		amount := 0.0
		if payload.Amount != "" {
			// Parse amount - remove any currency symbols
			var parsed float64
			_, err := fmt.Sscanf(payload.Amount, "%f", &parsed)
			if err == nil {
				amount = parsed
			}
		}

		if amount == 0 {
			amount = invoice.Total
		}

		payment := services.Payment{
			InvoiceID:   invoice.ID,
			Amount:      amount,
			Method:      "mpesa",
			Status:      "completed",
			Reference:   payload.Reference,
			CompletedAt: time.Now(),
		}

		invoiceService.RecordPayment(invoice.ID, payment)

		// Update invoice status
		invoice.Status = "paid"
		invoiceService.UpdateInvoice(invoice.ID, invoice)

		log.Printf("Payment recorded for invoice %s: %f", invoice.InvoiceNumber, amount)

	default:
		log.Printf("Unhandled webhook event: %s", payload.Event)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}
