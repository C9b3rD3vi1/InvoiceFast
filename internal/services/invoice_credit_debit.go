package services

import (
	"fmt"
	"time"

	"invoicefast/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreateCreditNote creates a credit note from an original invoice
func (s *InvoiceService) CreateCreditNote(tenantID, userID, originalInvoiceID string, kraPayloadItems []CreateCreditNoteItem) (*models.Invoice, error) {
	original, err := s.GetInvoiceByID(tenantID, originalInvoiceID)
	if err != nil {
		return nil, fmt.Errorf("original invoice not found: %w", err)
	}

	var creditItems []models.InvoiceItem
	var subtotal float64
	for i, item := range kraPayloadItems {
		lineTotal := item.Quantity * item.UnitPrice
		subtotal += lineTotal
		creditItems = append(creditItems, models.InvoiceItem{
			ID:          uuid.New().String(),
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   models.ToCents(item.UnitPrice),
			Unit:        item.Unit,
			Total:       models.ToCents(lineTotal),
			SortOrder:   i,
		})
	}

	taxRate := original.TaxRate
	taxAmount := subtotal * (taxRate / 100)
	discount := original.Discount
	if discount.LessThan(0) {
		discount = 0
	}
	total := subtotal + taxAmount - discount.Float64()

	// Validate buyer type inheritance - must match original for KRA compliance
	buyerType := original.BuyerClassification
	if buyerType == "" {
		buyerType = string(models.BuyerClassificationB2C)
	}

	// Validate original was submitted to KRA
	if original.KRAICN == "" {
		return nil, fmt.Errorf("cannot create credit note: original invoice not submitted to KRA")
	}

	creditNote := &models.Invoice{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		UserID:            userID,
		ClientID:          original.ClientID,
		InvoiceNumber:     generateCreditNoteNumber(userID),
		Reference:         "Credit for " + original.InvoiceNumber,
		Currency:          original.Currency,
		InvoiceType:       "credit_note",
		OriginalInvoiceID: originalInvoiceID,
		OriginalICN:       original.KRAICN,
		BuyerClassification: buyerType,
		Subtotal:          models.ToCents(subtotal),
		TaxRate:           taxRate,
		TaxAmount:         models.ToCents(taxAmount),
		Discount:          discount,
		Total:             models.ToCents(-total),
		Status:            models.InvoiceStatusCreditNote,
		DueDate:           time.Now().AddDate(0, 0, 30),
		Notes:             "Credit note for: " + original.InvoiceNumber,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(creditNote).Error; err != nil {
			return fmt.Errorf("failed to create credit note: %w", err)
		}
		for i := range creditItems {
			creditItems[i].InvoiceID = creditNote.ID
		}
		if err := tx.Create(&creditItems).Error; err != nil {
			return fmt.Errorf("failed to create credit note kraPayloadItems: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	creditNote.Items = creditItems
	return creditNote, nil
}

type CreateCreditNoteItem struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Unit        string  `json:"unit"`
}

type CreateDebitNoteItem struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Unit        string  `json:"unit"`
}

// CreateDebitNote creates a debit note from an original invoice
// Debit notes are used when additional charges need to be billed (e.g., extra services)
func (s *InvoiceService) CreateDebitNote(tenantID, userID, originalInvoiceID string, kraPayloadItems []CreateDebitNoteItem) (*models.Invoice, error) {
	original, err := s.GetInvoiceByID(tenantID, originalInvoiceID)
	if err != nil {
		return nil, fmt.Errorf("original invoice not found: %w", err)
	}

	var debitItems []models.InvoiceItem
	var subtotal float64
	for i, item := range kraPayloadItems {
		lineTotal := item.Quantity * item.UnitPrice
		subtotal += lineTotal
		debitItems = append(debitItems, models.InvoiceItem{
			ID:          uuid.New().String(),
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   models.ToCents(item.UnitPrice),
			Unit:        item.Unit,
			Total:       models.ToCents(lineTotal),
			SortOrder:   i,
		})
	}

	taxRate := original.TaxRate
	taxAmount := subtotal * (taxRate / 100)
	discount := original.Discount
	if discount.LessThan(0) {
		discount = 0
	}
	total := subtotal + taxAmount - discount.Float64()

	// Validate buyer type inheritance - must match original for KRA compliance
	buyerType := original.BuyerClassification
	if buyerType == "" {
		buyerType = string(models.BuyerClassificationB2C)
	}

	// Validate original was submitted to KRA
	if original.KRAICN == "" {
		return nil, fmt.Errorf("cannot create debit note: original invoice not submitted to KRA")
	}

	debitNote := &models.Invoice{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		UserID:            userID,
		ClientID:          original.ClientID,
		InvoiceNumber:     generateDebitNoteNumber(userID),
		Reference:         "Debit for " + original.InvoiceNumber,
		Currency:          original.Currency,
		InvoiceType:       "debit_note",
		OriginalInvoiceID: originalInvoiceID,
		OriginalICN:       original.KRAICN,
		BuyerClassification: buyerType,
		Subtotal:          models.ToCents(subtotal),
		TaxRate:           taxRate,
		TaxAmount:         models.ToCents(taxAmount),
		Discount:          discount,
		Total:             models.ToCents(total),
		Status:            models.InvoiceStatusSent,
		DueDate:           time.Now().AddDate(0, 0, 30),
		Notes:             "Debit note for: " + original.InvoiceNumber,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(debitNote).Error; err != nil {
			return fmt.Errorf("failed to create debit note: %w", err)
		}
		for i := range debitItems {
			debitItems[i].InvoiceID = debitNote.ID
		}
		if err := tx.Create(&debitItems).Error; err != nil {
			return fmt.Errorf("failed to create debit note kraPayloadItems: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	debitNote.Items = debitItems
	return debitNote, nil
}
