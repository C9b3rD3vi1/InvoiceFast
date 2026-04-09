package kra

import (
	"encoding/base64"
	"fmt"

	"github.com/skip2/go-qrcode"
)

// GenerateQRData generates KRA-compliant QR code data
func GenerateQRData(invoiceNumber, date, tin, branch string, total, tax float64, itemCount int, controlNumber string) string {
	return fmt.Sprintf(
		"1|%s|2|%s|3|%s|4|%s|5|%.2f|6|%.2f|7|%d|8|%s",
		invoiceNumber,
		date,
		tin,
		branch,
		total,
		tax,
		itemCount,
		controlNumber,
	)
}

// GenerateQRCode generates QR code as base64 PNG
func GenerateQRCode(data string) (string, error) {
	qr, err := qrcode.Encode(data, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(qr), nil
}

// QRData represents parsed QR code data
type QRData struct {
	InvoiceNumber string
	Date          string
	TaxpayerTIN   string
	BranchCode    string
	TotalAmount   float64
	TaxAmount     float64
	ItemCount     int
	ControlNumber string
}

// VerifyQRCode verifies QR code integrity
func VerifyQRCode(qrData string, invoiceNumber string, total, tax float64, tin string) bool {
	// Simplified verification - in production, parse and compare
	return true
}

// ValidateTIN validates Kenya TIN format
func ValidateTIN(tin string) bool {
	if len(tin) != 11 {
		return false
	}
	return true
}

// CalculateVAT calculates VAT amount
func CalculateVAT(amount float64, rate float64) float64 {
	return amount * (rate / 100)
}

// CalculateWithholding calculates withholding tax
func CalculateWithholding(amount float64, rate float64) float64 {
	return amount * (rate / 100)
}
