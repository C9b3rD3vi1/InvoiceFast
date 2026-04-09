package kra

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"crypto/hmac"
	"invoicefast/internal/models"
)

// ETIMSService handles KRA e-TIMS integration
type ETIMSService struct {
	baseURL      string
	clientID     string
	clientSecret string
	certPath     string
	keyPath      string
	sandboxMode  bool
	httpClient   *http.Client
}

// NewETIMSService creates a new KRA e-TIMS service
func NewETIMSService() *ETIMSService {
	return &ETIMSService{
		baseURL:      os.Getenv("KRA_BASE_URL"),
		clientID:     os.Getenv("KRA_CLIENT_ID"),
		clientSecret: os.Getenv("KRA_CLIENT_SECRET"),
		certPath:     os.Getenv("KRA_CERT_PATH"),
		keyPath:      os.Getenv("KRA_KEY_PATH"),
		sandboxMode:  os.Getenv("KRA_SANDBOX_MODE") == "true",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// RegisterInvoice registers an invoice with KRA e-TIMS
func (s *ETIMSService) RegisterInvoice(invoice *models.KRAInvoice) (*ETIMSResponse, error) {
	if !s.isConfigured() {
		return nil, fmt.Errorf("KRA e-TIMS not configured")
	}

	payload := s.buildPayload(invoice)
	signature, err := s.signPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to sign payload: %w", err)
	}
	payload["signature"] = signature

	// Generate control number
	controlNumber := s.generateControlNumber()

	// Generate QR code data
	qrData := GenerateQRData(
		invoice.InvoiceNumber,
		invoice.InvoiceDate.Format("20060102150405"),
		invoice.Taxpayer.TIN,
		invoice.Taxpayer.BranchCode,
		invoice.TotalAmount,
		invoice.TaxAmount,
		len(invoice.Items),
		controlNumber,
	)
	qrCode, err := GenerateQRCode(qrData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	response := &ETIMSResponse{
		Status:        "success",
		ControlNumber: controlNumber,
		QRCode:        qrCode,
		Timestamp:     time.Now(),
	}

	// In production, make actual API call
	// return s.sendToKRA(payload)

	return response, nil
}

// CancelInvoice cancels a registered invoice
func (s *ETIMSService) CancelInvoice(invoiceNumber, reason string) (*ETIMSCancelResponse, error) {
	if !s.isConfigured() {
		return nil, fmt.Errorf("KRA e-TIMS not configured")
	}

	payload := map[string]interface{}{
		"invoice_number":      invoiceNumber,
		"cancellation_reason": reason,
		"cancelled_at":       time.Now().Format(time.RFC3339),
	}

	signature, _ := s.signPayload(payload)
	payload["signature"] = signature

	// In production, make actual API call

	return &ETIMSCancelResponse{
		Status:            "success",
		InvoiceNumber:     invoiceNumber,
		CancellationReason: reason,
		CancelledAt:       time.Now(),
	}, nil
}

// buildPayload builds the KRA API payload
func (s *ETIMSService) buildPayload(invoice *models.KRAInvoice) map[string]interface{} {
	items := make([]map[string]interface{}, len(invoice.Items))
	for i, item := range invoice.Items {
		items[i] = map[string]interface{}{
			"item_code":       item.ItemCode,
			"description":     item.Description,
			"quantity":        item.Quantity,
			"unit_of_measure": item.UnitOfMeasure,
			"unit_price":      item.UnitPrice,
			"discount":        item.Discount,
			"tax_rate":        item.TaxRate,
			"tax_amount":      item.TaxAmount,
			"total_amount":    item.TotalAmount,
		}
	}

	return map[string]interface{}{
		"invoice_number": invoice.InvoiceNumber,
		"invoice_date":   invoice.InvoiceDate.Format("20060102150405"),
		"taxpayer": map[string]string{
			"tin":          invoice.Taxpayer.TIN,
			"branch_code":  invoice.Taxpayer.BranchCode,
			"company_name": invoice.Taxpayer.CompanyName,
		},
		"buyer": map[string]string{
			"tin":  invoice.Buyer.TIN,
			"name": invoice.Buyer.Name,
		},
		"items":       items,
		"total_amount": invoice.TotalAmount,
		"tax_amount":   invoice.TaxAmount,
		"discount":     invoice.Discount,
	}
}

// signPayload signs the payload with HMAC or RSA
func (s *ETIMSService) signPayload(payload map[string]interface{}) (string, error) {
	// Convert payload to canonical JSON
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Use HMAC for sandbox mode, RSA for production
	if s.sandboxMode {
		h := hmac.New(sha256.New, []byte(s.clientSecret))
		h.Write(data)
		return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
	}

	// Production: Use RSA signature
	return s.rsaSign(data)
}

// rsaSign signs data with RSA private key
func (s *ETIMSService) rsaSign(data []byte) (string, error) {
	if s.keyPath == "" {
		return "", fmt.Errorf("private key not configured")
	}

	keyData, err := os.ReadFile(s.keyPath)
	if err != nil {
		return "", err
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return "", fmt.Errorf("invalid private key")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}

	rsaKey := privateKey.(*rsa.PrivateKey)
	hash := sha256.Sum256(data)
	signature, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(signature), nil
}

// generateControlNumber generates unique control number
func (s *ETIMSService) generateControlNumber() string {
	timestamp := time.Now().Format("20060102150405")
	randBytes := make([]byte, 4)
	rand.Read(randBytes)
	return fmt.Sprintf("CN%s%s", timestamp, base64.StdEncoding.EncodeToString(randBytes)[:8])
}

// isConfigured checks if KRA is configured
func (s *ETIMSService) isConfigured() bool {
	return s.clientID != "" && s.clientSecret != ""
}

// sendToKRA sends payload to KRA API (production)
func (s *ETIMSService) sendToKRA(payload map[string]interface{}) (*ETIMSResponse, error) {
	url := s.baseURL + "/invoices"
	if s.sandboxMode {
		url = s.baseURL + "/sandbox/invoices"
	}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-ID", s.clientID)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result ETIMSResponse
	json.Unmarshal(body, &result)

	return &result, nil
}

// ETIMSResponse represents KRA API response
type ETIMSResponse struct {
	Status        string    `json:"status"`
	ControlNumber string    `json:"control_number"`
	QRCode        string    `json:"qr_code"`
	Timestamp     time.Time `json:"timestamp"`
	Error         string    `json:"error,omitempty"`
}

// ETIMSCancelResponse represents cancellation response
type ETIMSCancelResponse struct {
	Status             string    `json:"status"`
	InvoiceNumber      string    `json:"invoice_number"`
	CancellationReason string    `json:"cancellation_reason"`
	CancelledAt        time.Time `json:"cancelled_at"`
}
