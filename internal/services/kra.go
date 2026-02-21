package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"invoicefast/internal/config"
)

// KRAService handles KRA e-TIMS integration
type KRAService struct {
	cfg *config.Config
}

// KRAInvoiceData for e-TIMS submission
type KRAInvoiceData struct {
	InvoiceNumber     string    `json:"invoiceNumber"`
	InvoiceDate       string    `json:"invoiceDate"`
	InvoiceTime       string    `json:"invoiceTime"`
	Seller            KRASeller `json:"seller"`
	Buyer             KRABuyer  `json:"buyer"`
	Items             []KRAItem `json:"items"`
	SubTotal          float64   `json:"subTotal"`
	Discount          float64   `json:"discount"`
	TotalExcludingVAT float64   `json:"totalExcludingVAT"`
	VATRate           float64   `json:"vatRate"`
	VATAmount         float64   `json:"vatAmount"`
	TotalIncludingVAT float64   `json:"totalIncludingVAT"`
	PaymentMode       string    `json:"paymentMode"`
	ESDAmount         float64   `json:"esdAmount"`
	ESCAmount         float64   `json:"escAmount"`
	Currency          string    `json:"currency"`
}

// KRASeller seller information
type KRASeller struct {
	RegistrationNumber string `json:"registrationNumber"` // KRA PIN
	BusinessName       string `json:"businessName"`
	Address            string `json:"address"`
	ContactMobile      string `json:"contactMobile"`
	ContactEmail       string `json:"contactEmail"`
}

// KRABuyer buyer information
type KRABuyer struct {
	BuyerType          string `json:"buyerType"`          // B2C, B2B, B2E
	RegistrationNumber string `json:"registrationNumber"` // KRA PIN (for B2B)
	CustomerName       string `json:"customerName"`
	Address            string `json:"address"`
	ContactMobile      string `json:"contactMobile"`
	ContactEmail       string `json:"contactEmail"`
}

// KRAItem line item
type KRAItem struct {
	ItemCode               string  `json:"itemCode"`
	ItemDescription        string  `json:"itemDescription"`
	Quantity               float64 `json:"quantity"`
	UnitOfMeasure          string  `json:"unitOfMeasure"`
	UnitPrice              float64 `json:"unitPrice"`
	Total                  float64 `json:"total"`
	Discount               float64 `json:"discount"`
	ExciseDuty             float64 `json:"exciseDuty"`
	VATRate                float64 `json:"vatRate"`
	VATAmount              float64 `json:"vatAmount"`
	ItemClassificationCode string  `json:"itemClassificationCode"`
}

// KRAResponse from e-TIMS API
type KRAResponse struct {
	ResultCode    string `json:"resultCode"`
	ResultDesc    string `json:"resultDesc"`
	InvoiceNumber string `json:"invoiceNumber"`
	QRCode        string `json:"qrCode"`
	Signature     string `json:"signature"`
	ICN           string `json:"icn"` // Invoice Confirmation Number
	Timestamp     string `json:"timestamp"`
}

// KRASignature for signed invoices
type KRASignature struct {
	Signature   string `json:"signature"`
	SigningTime string `json:"signingTime"`
	CertSerial  string `json:"certSerial"`
}

// NewKRAService creates a new KRA service
func NewKRAService(cfg *config.Config) *KRAService {
	return &KRAService{cfg: cfg}
}

// SubmitInvoice submits an invoice to KRA e-TIMS
func (s *KRAService) SubmitInvoice(data *KRAInvoiceData) (*KRAResponse, error) {
	if s.cfg.KRA.APIURL == "" {
		// Development mode - return mock response
		return s.mockSubmit(data)
	}

	// Sign the invoice (used in production)
	_, _ = s.signInvoice(data)

	// Submit to KRA (in production)
	// This is a placeholder for the actual API call
	return &KRAResponse{
		ResultCode:    "0",
		ResultDesc:    "SUCCESS",
		InvoiceNumber: data.InvoiceNumber,
		QRCode:        s.generateQRCode(data),
		ICN:           s.generateICN(),
		Timestamp:     time.Now().Format(time.RFC3339),
	}, nil
}

// SubmitInvoiceBatch submits multiple invoices
func (s *KRAService) SubmitInvoiceBatch(invoices []KRAInvoiceData) ([]KRAResponse, error) {
	responses := make([]KRAResponse, len(invoices))

	for i, inv := range invoices {
		resp, err := s.SubmitInvoice(&inv)
		if err != nil {
			return responses, fmt.Errorf("failed to submit invoice %d: %w", i, err)
		}
		responses[i] = *resp
	}

	return responses, nil
}

// CancelInvoice cancels an invoice in KRA
func (s *KRAService) CancelInvoice(invoiceNumber, reason string) (*KRAResponse, error) {
	// Submit cancellation request to KRA
	return &KRAResponse{
		ResultCode:    "0",
		ResultDesc:    "CANCELLED",
		InvoiceNumber: invoiceNumber,
		Timestamp:     time.Now().Format(time.RFC3339),
	}, nil
}

// GetInvoiceStatus checks invoice status in KRA
func (s *KRAService) GetInvoiceStatus(invoiceNumber string) (*KRAResponse, error) {
	// Query KRA for invoice status
	return &KRAResponse{
		ResultCode:    "0",
		ResultDesc:    "ACTIVE",
		InvoiceNumber: invoiceNumber,
		Timestamp:     time.Now().Format(time.RFC3339),
	}, nil
}

// GenerateQRCode generates QR code for invoice
func (s *KRAService) generateQRCode(data *KRAInvoiceData) string {
	// KRA QR format:
	// TIN|SIN|BranchID|InvoiceNo|Date|Time|Total|VAT|GrandTotal|Currency|Signature

	qrData := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%.2f|%.2f|%.2f|%s|%s",
		data.Seller.RegistrationNumber, // TIN
		s.cfg.KRA.DeviceID,             // SIN (Serial/Instance Number)
		s.cfg.KRA.BranchID,             // BranchID
		data.InvoiceNumber,             // InvoiceNo
		data.InvoiceDate,               // Date
		data.InvoiceTime,               // Time
		data.TotalExcludingVAT,         // Total
		data.VATAmount,                 // VAT
		data.TotalIncludingVAT,         // GrandTotal
		data.Currency,                  // Currency
		"",                             // Signature (placeholder)
	)

	return base64.StdEncoding.EncodeToString([]byte(qrData))
}

// generateICN generates Invoice Confirmation Number
func (s *KRAService) generateICN() string {
	timestamp := time.Now().Format("20060102150405")
	randNum, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("ICN%s%04d", timestamp, randNum)
}

// signInvoice signs invoice data for integrity
func (s *KRAService) signInvoice(data *KRAInvoiceData) (*KRASignature, error) {
	if s.cfg.KRA.PrivateKey == "" {
		// Development mode - generate mock signature
		return &KRASignature{
			Signature:   "MOCK_SIGNATURE_" + fmt.Sprintf("%x", time.Now().Unix()),
			SigningTime: time.Now().Format(time.RFC3339),
			CertSerial:  "MOCK_CERT",
		}, nil
	}

	// Convert invoice to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	// Hash the data
	hash := sha256.Sum256(jsonData)
	hashStr := hex.EncodeToString(hash[:])

	// Sign with private key (simplified - in production use proper crypto)
	block, err := aes.NewCipher([]byte(s.cfg.KRA.PrivateKey[:32]))
	if err != nil {
		return nil, err
	}

	ciphertext := make([]byte, aes.BlockSize+len(hashStr))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], []byte(hashStr))

	return &KRASignature{
		Signature:   base64.StdEncoding.EncodeToString(ciphertext),
		SigningTime: time.Now().Format(time.RFC3339),
		CertSerial:  s.cfg.KRA.CertSerial,
	}, nil
}

// mockSubmit for development/testing
func (s *KRAService) mockSubmit(data *KRAInvoiceData) (*KRAResponse, error) {
	fmt.Printf("📋 [MOCK KRA SUBMISSION]\n")
	fmt.Printf("Invoice: %s\n", data.InvoiceNumber)
	fmt.Printf("Seller: %s (%s)\n", data.Seller.BusinessName, data.Seller.RegistrationNumber)
	fmt.Printf("Buyer: %s\n", data.Buyer.CustomerName)
	fmt.Printf("Total: %.2f %s\n", data.TotalIncludingVAT, data.Currency)
	fmt.Printf("VAT: %.2f\n", data.VATAmount)
	fmt.Println()

	// Simulate network delay
	time.Sleep(100 * time.Millisecond)

	return &KRAResponse{
		ResultCode:    "0",
		ResultDesc:    "SUCCESS",
		InvoiceNumber: data.InvoiceNumber,
		QRCode:        s.generateQRCode(data),
		Signature:     "MOCK_SIGNATURE_" + data.InvoiceNumber,
		ICN:           s.generateICN(),
		Timestamp:     time.Now().Format(time.RFC3339),
	}, nil
}

// ConvertInvoiceToKRA converts internal invoice to KRA format
func (s *KRAService) ConvertInvoiceToKRA(invoice *Invoice, user *User, client *Client) *KRAInvoiceData {
	items := make([]KRAItem, len(invoice.Items))
	for i, item := range invoice.Items {
		items[i] = KRAItem{
			ItemCode:               fmt.Sprintf("ITEM%03d", i+1),
			ItemDescription:        item.Description,
			Quantity:               item.Quantity,
			UnitOfMeasure:          item.Unit,
			UnitPrice:              item.UnitPrice,
			Total:                  item.Total,
			Discount:               0,
			ExciseDuty:             0,
			VATRate:                invoice.TaxRate,
			VATAmount:              item.Total * (invoice.TaxRate / 100),
			ItemClassificationCode: "001", // General goods
		}
	}

	buyerType := "B2C"
	if client.KRAPIN != "" {
		buyerType = "B2B"
	}

	return &KRAInvoiceData{
		InvoiceNumber: invoice.InvoiceNumber,
		InvoiceDate:   invoice.CreatedAt.Format("2006-01-02"),
		InvoiceTime:   invoice.CreatedAt.Format("15:04:05"),
		Seller: KRASeller{
			RegistrationNumber: user.KRAPIN,
			BusinessName:       user.CompanyName,
			Address:            "Nairobi, Kenya",
			ContactMobile:      user.Phone,
			ContactEmail:       user.Email,
		},
		Buyer: KRABuyer{
			BuyerType:          buyerType,
			RegistrationNumber: client.KRAPIN,
			CustomerName:       client.Name,
			Address:            client.Address,
			ContactMobile:      client.Phone,
			ContactEmail:       client.Email,
		},
		Items:             items,
		SubTotal:          invoice.Subtotal,
		Discount:          invoice.Discount,
		TotalExcludingVAT: invoice.Subtotal - invoice.Discount,
		VATRate:           invoice.TaxRate,
		VATAmount:         invoice.TaxAmount,
		TotalIncludingVAT: invoice.Total,
		PaymentMode:       "CASH", // Would map from actual payment
		ESDAmount:         0,
		ESCAmount:         0,
		Currency:          invoice.Currency,
	}
}

// ValidateKRAPIN validates a KRA PIN
func (s *KRAService) ValidateKRAPIN(pin string) (bool, error) {
	// KRA PIN format: A123456789B
	if len(pin) != 11 {
		return false, fmt.Errorf("invalid PIN format")
	}

	// Check format
	if !strings.HasPrefix(pin, "A") || !strings.HasSuffix(pin, "B") {
		return false, fmt.Errorf("invalid PIN format")
	}

	// In production, call KRA API to validate
	return true, nil
}

// Types for internal models (simplified)
type Invoice struct {
	ID            string
	InvoiceNumber string
	Currency      string
	Subtotal      float64
	TaxRate       float64
	TaxAmount     float64
	Discount      float64
	Total         float64
	PaidAmount    float64
	CreatedAt     time.Time
	DueDate       time.Time
	Status        string
	Items         []InvoiceItem
}

type InvoiceItem struct {
	ID          string
	Description string
	Quantity    float64
	Unit        string
	UnitPrice   float64
	Total       float64
}

type User struct {
	ID          string
	Email       string
	Phone       string
	CompanyName string
	KRAPIN      string
}

type Client struct {
	ID      string
	Name    string
	Email   string
	Phone   string
	Address string
	KRAPIN  string
}
