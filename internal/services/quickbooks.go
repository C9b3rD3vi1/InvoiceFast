package services

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

type QuickBooksService struct {
	cfg    *config.Config
	db     *database.DB
	client *http.Client
}

func NewQuickBooksService(cfg *config.Config, db *database.DB) *QuickBooksService {
	return &QuickBooksService{
		cfg:    cfg,
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type QuickBooksOAuthState struct {
	TenantID    string `json:"tenant_id"`
	RedirectURI string `json:"redirect_uri"`
	CreatedAt   int64  `json:"created_at"`
}

type QuickBooksTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	RealmID      string `json:"realm_id"`
}

type QuickBooksConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	Environment  string `json:"environment"` // sandbox, production
}

func (s *QuickBooksService) IsEnabled() bool {
	return s.cfg.QuickBooks.Enabled && s.cfg.QuickBooks.ClientID != ""
}

func (s *QuickBooksService) GetAuthorizationURL(tenantID, redirectURI string) (string, error) {
	if !s.IsEnabled() {
		return "", fmt.Errorf("QuickBooks integration is not configured")
	}

	state := QuickBooksOAuthState{
		TenantID:    tenantID,
		RedirectURI: redirectURI,
		CreatedAt:   time.Now().Unix(),
	}

	stateJSON, _ := json.Marshal(state)
	stateBase64 := base64.URLEncoding.EncodeToString(stateJSON)

	baseURL := "https://appcenter.intuit.com/connect/oauth2"
	if s.cfg.QuickBooks.Environment == "production" {
		baseURL = "https://appcenter.intuit.com/connect/oauth2"
	}

	params := url.Values{}
	params.Set("client_id", s.cfg.QuickBooks.ClientID)
	params.Set("response_type", "code")
	params.Set("scope", "com.intuit.quickbooks.accounting")
	params.Set("redirect_uri", s.cfg.QuickBooks.RedirectURI)
	params.Set("state", stateBase64)

	return baseURL + "?" + params.Encode(), nil
}

func (s *QuickBooksService) HandleOAuthCallback(code, state string) (*QuickBooksTokens, string, error) {
	stateJSON, err := base64.URLEncoding.DecodeString(state)
	if err != nil {
		return nil, "", fmt.Errorf("invalid state parameter")
	}

	var oauthState QuickBooksOAuthState
	if err := json.Unmarshal(stateJSON, &oauthState); err != nil {
		return nil, "", fmt.Errorf("invalid state format")
	}

	// Exchange code for tokens
	tokenURL := "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer"
	if s.cfg.QuickBooks.Environment == "production" {
		tokenURL = "https://oauth.platform.intintuit.com/oauth2/v1/tokens/bearer"
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", s.cfg.QuickBooks.RedirectURI)
	data.Set("client_id", s.cfg.QuickBooks.ClientID)
	data.Set("client_secret", s.cfg.QuickBooks.ClientSecret)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("token exchange failed: %s", string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, "", err
	}

	tokens := &QuickBooksTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Unix() + int64(tokenResp.ExpiresIn),
		RealmID:      s.cfg.QuickBooks.RealmID,
	}

	return tokens, oauthState.TenantID, nil
}

func (s *QuickBooksService) RefreshAccessToken(refreshToken string) (*QuickBooksTokens, error) {
	tokenURL := "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer"

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", s.cfg.QuickBooks.ClientID)
	data.Set("client_secret", s.cfg.QuickBooks.ClientSecret)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token refresh failed")
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &QuickBooksTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Unix() + int64(tokenResp.ExpiresIn),
	}, nil
}

func (s *QuickBooksService) SaveIntegration(tenantID string, tokens *QuickBooksTokens) error {
	integration := &models.Integration{
		ID:          generateUUID(),
		TenantID:    tenantID,
		Provider:    "quickbooks",
		Name:        "QuickBooks Online",
		Description: "Accounting software integration",
		IsActive:    true,
		IsConfigured: true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	configJSON, _ := json.Marshal(map[string]interface{}{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_at":    tokens.ExpiresAt,
		"realm_id":      tokens.RealmID,
	})
	integration.Config = string(configJSON)

	return s.db.Create(integration).Error
}

func (s *QuickBooksService) GetIntegration(tenantID string) (*models.Integration, error) {
	var integration models.Integration
	err := s.db.Where("tenant_id = ? AND provider = ?", tenantID, "quickbooks").First(&integration).Error
	if err != nil {
		return nil, err
	}
	return &integration, nil
}

func (s *QuickBooksService) Disconnect(tenantID string) error {
	return s.db.Where("tenant_id = ? AND provider = ?", tenantID, "quickbooks").Delete(&models.Integration{}).Error
}

func (s *QuickBooksService) TestConnection(tenantID string) (bool, error) {
	integration, err := s.GetIntegration(tenantID)
	if err != nil {
		return false, err
	}

	var config map[string]interface{}
	json.Unmarshal([]byte(integration.Config), &config)

	accessToken, ok := config["access_token"].(string)
	if !ok || accessToken == "" {
		return false, fmt.Errorf("no access token found")
	}

	realmID, _ := config["realm_id"].(string)
	if realmID == "" {
		realmID = s.cfg.QuickBooks.RealmID
	}

	// Test API call to QuickBooks
	apiURL := fmt.Sprintf("https://sandbox-quickbooks.api.intuit.com/v3/company/%s/companyinfo/%s", realmID, realmID)
	if s.cfg.QuickBooks.Environment == "production" {
		apiURL = fmt.Sprintf("https://quickbooks.api.intuit.com/v3/company/%s/companyinfo/%s", realmID, realmID)
	}

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200, nil
}

func (s *QuickBooksService) SyncInvoices(tenantID string) (int, error) {
	integration, err := s.GetIntegration(tenantID)
	if err != nil {
		return 0, err
	}

	var config map[string]interface{}
	json.Unmarshal([]byte(integration.Config), &config)

	accessToken, _ := config["access_token"].(string)
	realmID, _ := config["realm_id"].(string)

	if realmID == "" {
		realmID = s.cfg.QuickBooks.RealmID
	}

	// Get unpaid invoices from our system
	var invoices []models.Invoice
	s.db.Where("tenant_id = ? AND status IN ?", tenantID, []string{"sent", "viewed", "partially_paid", "overdue"}).Find(&invoices)

	syncedCount := 0
	for _, inv := range invoices {
		if err := s.createQuickBooksInvoice(accessToken, realmID, &inv); err != nil {
			log.Printf("Failed to sync invoice %s to QuickBooks: %v", inv.InvoiceNumber, err)
			continue
		}
		syncedCount++
	}

	return syncedCount, nil
}

func (s *QuickBooksService) createQuickBooksInvoice(accessToken, realmID string, inv *models.Invoice) error {
	apiURL := fmt.Sprintf("https://sandbox-quickbooks.api.intuit.com/v3/company/%s/invoice", realmID)
	if s.cfg.QuickBooks.Environment == "production" {
		apiURL = fmt.Sprintf("https://quickbooks.api.intuit.com/v3/company/%s/invoice", realmID)
	}

	payload := map[string]interface{}{
		"DocNumber":           inv.InvoiceNumber,
		"TxnDate":            inv.CreatedAt.Format("2006-01-02"),
		"TotalAmt":           inv.Total,
		"Balance":            inv.BalanceDue,
		"CustomerRef":        map[string]interface{}{"value": inv.ClientID},
		"Line":               []interface{}{},
	}

	payloadJSON, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(string(payloadJSON)))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("QuickBooks API error: %s", string(body))
	}

	return nil
}