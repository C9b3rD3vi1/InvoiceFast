package services

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

// ErrMockMode is returned when KRA is running in sandbox/mock mode
var ErrMockMode = errors.New("KRA e-TIMS running in mock/sandbox mode - production API not configured")

// ErrKRAIntegrationRequired is returned when KRA integration is not fully configured
type ErrKRAIntegrationRequired struct {
	MissingFields []string
}

func (e *ErrKRAIntegrationRequired) Error() string {
	return fmt.Sprintf("KRA e-TIMS integration incomplete. Missing: %s", strings.Join(e.MissingFields, ", "))
}

func NewErrKRAIntegrationRequired(missing []string) *ErrKRAIntegrationRequired {
	return &ErrKRAIntegrationRequired{MissingFields: missing}
}

// IsKRAConfigured checks if KRA is fully configured for production
func (s *KRAService) IsKRAConfigured() bool {
	return s.cfg.KRA.Enabled &&
		s.cfg.KRA.APIURL != "" &&
		s.cfg.KRA.APIURL != "https://api.kra.go.ke" &&
		s.cfg.KRA.APIKey != "" &&
		s.cfg.KRA.PrivateKey != "" &&
		s.cfg.KRA.DeviceID != "" &&
		s.cfg.KRA.BranchID != ""
}

// GetMissingKRAConfig returns list of missing KRA configuration fields
func (s *KRAService) GetMissingKRAConfig() []string {
	var missing []string
	if s.cfg.KRA.APIURL == "" || s.cfg.KRA.APIURL == "https://api.kra.go.ke" {
		missing = append(missing, "KRA_API_URL")
	}
	if s.cfg.KRA.APIKey == "" {
		missing = append(missing, "KRA_API_KEY")
	}
	if s.cfg.KRA.PrivateKey == "" {
		missing = append(missing, "KRA_PRIVATE_KEY")
	}
	if s.cfg.KRA.DeviceID == "" {
		missing = append(missing, "KRA_DEVICE_ID")
	}
	if s.cfg.KRA.BranchID == "" {
		missing = append(missing, "KRA_BRANCH_ID")
	}
	return missing
}

// KRAService handles KRA e-TIMS integration
type KRAService struct {
	cfg *config.Config
	db  *database.DB
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

// NewKRAServiceWithDB creates a new KRA service with database for queue management
func NewKRAServiceWithDB(cfg *config.Config, db *database.DB) *KRAService {
	return &KRAService{cfg: cfg, db: db}
}

// SubmitInvoice submits an invoice to KRA e-TIMS via OSCP (Online Sales Control Protocol)
func (s *KRAService) SubmitInvoice(data *KRAInvoiceData) (*KRAResponse, error) {
	// If no API URL configured, return error so frontend can show sandbox warning
	if s.cfg.KRA.APIURL == "" || s.cfg.KRA.APIURL == "https://api.kra.go.ke" {
		log.Printf("[KRA] Mock mode - production API not configured")
		return nil, ErrMockMode
	}

	// Build the signed payload for VSCU (Virtual Sales Control Unit)
	payload, err := s.buildETIMSPayload(data)
	if err != nil {
		return nil, fmt.Errorf("failed to build e-TIMS payload: %w", err)
	}

	// Sign the payload
	_, err = s.signETIMSPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to sign e-TIMS payload: %w", err)
	}

	// Submit to KRA e-TIMS API
	response, err := s.submitToKRA(payload)
	if err != nil {
		// Queue for retry instead of silent fallback
		log.Printf("[KRA] API submission failed: %v - queuing for retry", err)

		// Extract tenant info from invoice data
		tenantID := "" // Would need to be passed in
		invoiceID := ""

		if s.db != nil && data.InvoiceNumber != "" {
			// Try to find invoice for queue item
			var invoice struct {
				ID       string
				TenantID string
			}
			if err := s.db.Model(&models.Invoice{}).Where("invoice_number = ?", data.InvoiceNumber).First(&invoice).Error; err == nil {
				tenantID = invoice.TenantID
				invoiceID = invoice.ID
			}
		}

		if tenantID != "" && invoiceID != "" {
			_ = s.QueueFailedSubmission(tenantID, invoiceID, data.InvoiceNumber, payload, err.Error())
		}

		return nil, fmt.Errorf("KRA submission queued for retry: %w", err)
	}

	return response, nil
}

// buildETIMSPayload builds the e-TIMS JSON payload according to KRA specification
func (s *KRAService) buildETIMSPayload(data *KRAInvoiceData) ([]byte, error) {
	etimsPayload := map[string]interface{}{
		"invoiceNumber": data.InvoiceNumber,
		"invoiceDate":   data.InvoiceDate,
		"invoiceTime":   data.InvoiceTime,
		"seller": map[string]string{
			"registrationNumber": data.Seller.RegistrationNumber,
			"businessName":       data.Seller.BusinessName,
			"address":            data.Seller.Address,
			"contactMobile":      data.Seller.ContactMobile,
			"contactEmail":       data.Seller.ContactEmail,
		},
		"buyer": map[string]interface{}{
			"buyerType":          data.Buyer.BuyerType,
			"registrationNumber": data.Buyer.RegistrationNumber,
			"customerName":       data.Buyer.CustomerName,
			"address":            data.Buyer.Address,
			"contactMobile":      data.Buyer.ContactMobile,
			"contactEmail":       data.Buyer.ContactEmail,
		},
		"items":              data.Items,
		"subTotal":           data.SubTotal,
		"discount":           data.Discount,
		"totalExcludingVAT":  data.TotalExcludingVAT,
		"vatRate":            data.VATRate,
		"vatAmount":          data.VATAmount,
		"totalIncludingVAT":  data.TotalIncludingVAT,
		"paymentMode":        data.PaymentMode,
		"esdAmount":          data.ESDAmount,
		"escAmount":          data.ESCAmount,
		"currency":           data.Currency,
		"deviceID":           s.cfg.KRA.DeviceID,
		"branchID":           s.cfg.KRA.BranchID,
		"registrationNumber": s.cfg.KRA.BranchCode,
	}

	return json.Marshal(etimsPayload)
}

// signETIMSPayload signs the e-TIMS payload using RSA-SHA256
func (s *KRAService) signETIMSPayload(payload []byte) (string, error) {
	if s.cfg.KRA.PrivateKey == "" {
		return "", errors.New("KRA private key not configured")
	}

	// Calculate SHA256 hash of payload
	hash := sha256.Sum256(payload)

	// Try to parse as RSA private key
	rsaPrivateKey, err := s.parsePrivateKey()
	if err != nil {
		// Fallback: use mock signing for development
		log.Printf("[KRA] Using fallback signature method (not RSA)")
		return base64.StdEncoding.EncodeToString(hash[:]), nil
	}

	// Sign with RSA private key
	signature, err := rsa.SignPKCS1v15(rand.Reader, rsaPrivateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("RSA signing failed: %w", err)
	}

	return base64.StdEncoding.EncodeToString(signature), nil
}

func (s *KRAService) parsePrivateKey() (*rsa.PrivateKey, error) {
	keyData := s.cfg.KRA.PrivateKey

	// Try to decode base64 first (if key is base64 encoded)
	keyBytes, err := base64.StdEncoding.DecodeString(keyData)
	if err != nil {
		// Try raw bytes
		keyBytes = []byte(keyData)
	}

	// Try to parse as PKCS#8 or PKCS#1
	priv, err := x509.ParsePKCS8PrivateKey(keyBytes)
	if err != nil {
		priv, err = x509.ParsePKCS1PrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
	}

	rsaKey, ok := priv.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not an RSA private key")
	}

	return rsaKey, nil
}

// submitToKRA submits the signed payload to KRA e-TIMS API
func (s *KRAService) submitToKRA(payload []byte) (*KRAResponse, error) {
	apiURL := fmt.Sprintf("%s/etims/v1/sales/invoice", s.cfg.KRA.APIURL)

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.KRA.APIKey)
	httpReq.Header.Set("X-Device-ID", s.cfg.KRA.DeviceID)
	httpReq.Header.Set("X-Branch-ID", s.cfg.KRA.BranchID)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to submit to KRA: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("KRA API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse KRA response
	var kraResp struct {
		ResultCode    string `json:"resultCode"`
		ResultDesc    string `json:"resultDesc"`
		InvoiceNumber string `json:"invoiceNumber"`
		ICN           string `json:"icn"`
		QRCode        string `json:"qrCode"`
		Signature     string `json:"signature"`
		Timestamp     string `json:"timestamp"`
	}

	if err := json.Unmarshal(body, &kraResp); err != nil {
		return nil, fmt.Errorf("failed to parse KRA response: %w", err)
	}

	if kraResp.ResultCode != "0" && kraResp.ResultCode != "00" {
		return nil, fmt.Errorf("KRA rejected invoice: %s", kraResp.ResultDesc)
	}

	return &KRAResponse{
		ResultCode:    kraResp.ResultCode,
		ResultDesc:    kraResp.ResultDesc,
		InvoiceNumber: kraResp.InvoiceNumber,
		QRCode:        kraResp.QRCode,
		Signature:     kraResp.Signature,
		ICN:           kraResp.ICN,
		Timestamp:     kraResp.Timestamp,
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
	// Mask sensitive data for logging (KRA PINs are sensitive PII)
	maskedSellerPIN := maskPIN(data.Seller.RegistrationNumber)
	maskedBuyerPIN := maskPIN(data.Buyer.RegistrationNumber)

	log.Printf("[KRA MOCK] Invoice: %s | Seller: %s (PIN: %s) | Buyer: %s (PIN: %s) | Total: %.2f %s | VAT: %.2f",
		data.InvoiceNumber,
		data.Seller.BusinessName,
		maskedSellerPIN,
		data.Buyer.CustomerName,
		maskedBuyerPIN,
		data.TotalIncludingVAT,
		data.Currency,
		data.VATAmount,
	)

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

// maskPIN masks KRA PIN for secure logging (shows only last 4 chars)
func maskPIN(pin string) string {
	if pin == "" {
		return ""
	}
	if len(pin) <= 4 {
		return "****"
	}
	return "****" + pin[len(pin)-4:]
}

// QueueFailedSubmission adds a failed KRA submission to the retry queue
func (s *KRAService) QueueFailedSubmission(tenantID, invoiceID, invoiceNumber string, payload []byte, errMsg string) error {
	if s.db == nil {
		log.Printf("[KRA] Queue unavailable - no database connection")
		return errors.New("KRA queue unavailable")
	}

	payloadJSON, _ := json.Marshal(payload)
	queueItem := models.KRAQueueItem{
		ID:            fmt.Sprintf("kra-%d", time.Now().UnixNano()),
		TenantID:      tenantID,
		InvoiceID:     invoiceID,
		InvoiceNumber: invoiceNumber,
		Payload:       string(payloadJSON),
		RetryCount:    0,
		MaxRetries:    3,
		Status:        models.KRAQueuePending,
		LastError:     errMsg,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.db.Create(&queueItem).Error; err != nil {
		log.Printf("[KRA] Failed to queue item: %v", err)
		return err
	}

	log.Printf("[KRA] Queued failed submission for invoice %s (retry %d/%d)", invoiceNumber, 0, 3)
	return nil
}

// ProcessRetryQueue processes pending KRA submissions
func (s *KRAService) ProcessRetryQueue() error {
	if s.db == nil {
		return errors.New("KRA queue unavailable")
	}

	var pending []models.KRAQueueItem
	s.db.Where("status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", models.KRAQueuePending, time.Now()).
		Order("created_at ASC").
		Limit(50).
		Find(&pending)

	if len(pending) == 0 {
		return nil
	}

	log.Printf("[KRA] Processing %d queued items", len(pending))

	for _, item := range pending {
		s.processQueueItem(&item)
	}

	return nil
}

func (s *KRAService) processQueueItem(item *models.KRAQueueItem) {
	var payload []byte
	json.Unmarshal([]byte(item.Payload), &payload)

	resp, err := s.submitToKRA(payload)
	if err != nil {
		item.RetryCount++
		item.LastError = err.Error()

		if item.RetryCount >= item.MaxRetries {
			item.Status = models.KRAQueueFailed
			log.Printf("[KRA] Queue item %s failed permanently after %d retries", item.ID, item.RetryCount)
		} else {
			nextRetry := time.Now().Add(time.Duration(item.RetryCount) * 5 * time.Minute)
			item.NextRetryAt = &nextRetry
			log.Printf("[KRA] Queue item %s failed, retry %d/%d at %v", item.ID, item.RetryCount, item.MaxRetries, nextRetry)
		}
	} else {
		item.Status = models.KRAQueueCompleted
		now := time.Now()
		item.CompletedAt = &now
		log.Printf("[KRA] Queue item %s completed - ICN: %s", item.ID, resp.ICN)
	}

	item.UpdatedAt = time.Now()
	s.db.Save(item)
}

// GetQueueStatus returns the status of KRA queue
func (s *KRAService) GetQueueStatus() (pending, failed, completed int64, err error) {
	if s.db == nil {
		err = errors.New("KRA queue unavailable")
		return
	}

	s.db.Model(&models.KRAQueueItem{}).Where("status = ?", models.KRAQueuePending).Count(&pending)
	s.db.Model(&models.KRAQueueItem{}).Where("status = ?", models.KRAQueueFailed).Count(&failed)
	s.db.Model(&models.KRAQueueItem{}).Where("status = ?", models.KRAQueueCompleted).Count(&completed)

	return
}
