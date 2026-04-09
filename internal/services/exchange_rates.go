package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

type ExchangeRateService struct {
	db          *database.DB
	apiURL      string
	mu          sync.RWMutex
	lastUpdated time.Time
	cachedRates map[string]float64
}

func NewExchangeRateService(db *database.DB) *ExchangeRateService {
	return &ExchangeRateService{
		db:     db,
		apiURL: "https://api.centralbank.go.ke/exchange-rates",
		cachedRates: map[string]float64{
			"KES/USD": 0.0091,
			"KES/EUR": 0.0083,
			"KES/GBP": 0.0072,
			"KES/TZS": 23.5,
			"KES/UGX": 34.2,
			"KES/NGN": 13.5,
		},
	}
}

type CBKRateResponse struct {
	Code string  `json:"code"`
	Rate float64 `json:"rate"`
	Date string  `json:"date"`
}

func (s *ExchangeRateService) GetRate(from, to string) (float64, error) {
	if from == to {
		return 1.0, nil
	}

	s.mu.RLock()
	rate, ok := s.cachedRates[fmt.Sprintf("%s/%s", from, to)]
	s.mu.RUnlock()

	if ok {
		return rate, nil
	}

	return 0, fmt.Errorf("exchange rate not found for %s to %s", from, to)
}

func (s *ExchangeRateService) Convert(amount float64, from, to string) (float64, error) {
	if from == to {
		return amount, nil
	}

	rate, err := s.GetRate(from, to)
	if err != nil {
		return 0, err
	}

	return amount * rate, nil
}

func (s *ExchangeRateService) GetKESEquivalent(amount float64, currency string) (float64, error) {
	if currency == "KES" {
		return amount, nil
	}

	rate, err := s.GetRate(currency, "KES")
	if err != nil {
		return 0, err
	}

	return amount / rate, nil
}

func (s *ExchangeRateService) FetchRatesFromCBK() error {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", s.apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("CBK API returned status %d", resp.StatusCode)
	}

	var rates []CBKRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&rates); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	s.mu.Lock()
	for _, r := range rates {
		switch r.Code {
		case "USD":
			s.cachedRates["KES/USD"] = 1.0 / r.Rate
		case "EUR":
			s.cachedRates["KES/EUR"] = 1.0 / r.Rate
		case "GBP":
			s.cachedRates["KES/GBP"] = 1.0 / r.Rate
		}
	}
	s.lastUpdated = time.Now()
	s.mu.Unlock()

	log.Printf("[ExchangeRates] Updated rates from CBK")
	return nil
}

func (s *ExchangeRateService) StartCronJob() {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			if err := s.FetchRatesFromCBK(); err != nil {
				log.Printf("[ExchangeRates] Failed to fetch rates: %v", err)
			}
		}
	}()
}

func (s *ExchangeRateService) StoreRateInDB(currency string, rate float64) error {
	rateRecord := models.ExchangeRate{
		Currency:     currency,
		Rate:         rate,
		BaseCurrency: "KES",
		ValidFrom:    time.Now(),
	}

	result := s.db.Where("currency = ?", currency).Assign(models.ExchangeRate{
		Currency:  currency,
		Rate:      rate,
		ValidFrom: time.Now(),
	}).FirstOrCreate(&rateRecord)

	return result.Error
}

func (s *ExchangeRateService) GetAllRates() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rates := make(map[string]float64)
	for k, v := range s.cachedRates {
		rates[k] = v
	}
	return rates
}
