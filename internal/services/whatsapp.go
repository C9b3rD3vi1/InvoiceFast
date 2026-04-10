package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
)

type WhatsAppService struct {
	cfg        *config.Config
	db         *database.DB
	httpClient *http.Client
}

func NewWhatsAppService(cfg *config.Config) *WhatsAppService {
	return &WhatsAppService{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *WhatsAppService) IsConfigured() bool {
	return s.cfg.WhatsApp.Enabled &&
		s.cfg.WhatsApp.AccessToken != "" &&
		s.cfg.WhatsApp.PhoneNumberID != ""
}

func (s *WhatsAppService) SendInvoice(phone, invoiceNumber, amount, companyName, link string) error {
	if !s.IsConfigured() {
		fmt.Printf("[WhatsApp Mock] Invoice %s to %s: %s %s\n", invoiceNumber, phone, amount, companyName)
		return nil
	}

	msg := fmt.Sprintf("New Invoice from %s\n\nInvoice: %s\nAmount: %s\nPay: %s", companyName, invoiceNumber, amount, link)
	return s.SendText(phone, msg)
}

func (s *WhatsAppService) SendText(phone, message string) error {
	if !s.IsConfigured() {
		fmt.Printf("[WhatsApp Mock] To: %s, Message: %s\n", phone, message)
		return nil
	}

	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", s.cfg.WhatsApp.PhoneNumberID)

	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                phone,
		"type":              "text",
		"text":              map[string]string{"body": message},
	}

	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+s.cfg.WhatsApp.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("WhatsApp API error: %d", resp.StatusCode)
	}

	return nil
}
