package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"invoicefast/internal/models"
	"invoicefast/internal/pdf"
)

// GenerateInvoicePDF generates a PDF for an invoice
func (s *InvoiceService) GenerateInvoicePDF(invoice *models.Invoice) ([]byte, error) {
	if s.pdfGenerator == nil {
		return nil, fmt.Errorf("PDF generator not configured")
	}

	pdfData := s.invoiceToPDFData(invoice)

	result, err := s.pdfGenerator.GenerateInvoicePDF(pdfData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	return result.Content, nil
}

func (s *InvoiceService) invoiceToPDFData(invoice *models.Invoice) *pdf.InvoiceData {
	items := make([]pdf.InvoiceLineItem, len(invoice.Items))
	for i, item := range invoice.Items {
		items[i] = pdf.InvoiceLineItem{
			Description: item.Description,
			Quantity:    item.Quantity,
			Unit:        item.Unit,
			UnitPrice:   item.UnitPrice.Float64(),
			TaxRate:     item.TaxRate,
			Total:       item.Total.Float64(),
		}
	}

	paymentLink := ""
	if invoice.MagicToken != "" {
		paymentLink = fmt.Sprintf("%s/invoice/%s", s.BaseURL(), invoice.MagicToken)
	}

	kraCompliant := invoice.KRAICN != ""
	controlNumber := invoice.KRAICN

	paidAmount := invoice.PaidAmount.Float64()
	balance := invoice.BalanceDue.Float64()

	data := &pdf.InvoiceData{
		CompanyName:    invoice.User.CompanyName,
		CompanyEmail:   invoice.User.Email,
		CompanyPhone:   invoice.User.Phone,
		CompanyAddress: invoice.User.CompanyName,
		CompanyKRA:     invoice.User.KRAPIN,
		BrandColor:     invoice.BrandColor,
		ClientName:     invoice.Client.Name,
		ClientEmail:    invoice.Client.Email,
		ClientPhone:    invoice.Client.Phone,
		ClientAddress:  invoice.Client.Address,
		ClientKRA:      invoice.Client.KRAPIN,
		InvoiceNumber:  invoice.InvoiceNumber,
		InvoiceDate:    invoice.CreatedAt,
		DueDate:        invoice.DueDate,
		Currency:       invoice.Currency,
		Items:          items,
		Subtotal:       invoice.Subtotal.Float64(),
		TaxRate:        invoice.TaxRate,
		TaxAmount:      invoice.TaxAmount.Float64(),
		Discount:       invoice.Discount.Float64(),
		Total:          invoice.Total.Float64(),
		PaymentLink:    paymentLink,
		Notes:          invoice.Notes,
		Terms:          invoice.Terms,
		Status:         string(invoice.Status),
		PaidAmount:     paidAmount,
		Balance:        balance,
		KRACompliant:   kraCompliant,
		ControlNumber:  controlNumber,
	}

	if invoice.LogoURL != "" {
		data.CompanyLogo = invoice.LogoURL
	}

	return data
}

func generateInvoiceNumber(userID string) string {
	timestamp := time.Now().UTC().Format("20060102")
	randBytes := make([]byte, 2)
	rand.Read(randBytes)
	return fmt.Sprintf("INV-%s-%s", timestamp, hex.EncodeToString(randBytes))
}

func generateCreditNoteNumber(userID string) string {
	timestamp := time.Now().UTC().Format("20060102")
	randBytes := make([]byte, 2)
	rand.Read(randBytes)
	return fmt.Sprintf("CN-%s-%s", timestamp, hex.EncodeToString(randBytes))
}

func generateDebitNoteNumber(userID string) string {
	timestamp := time.Now().UTC().Format("20060102")
	randBytes := make([]byte, 2)
	rand.Read(randBytes)
	return fmt.Sprintf("DN-%s-%s", timestamp, hex.EncodeToString(randBytes))
}
