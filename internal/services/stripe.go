package services

import (
	"errors"
	"fmt"
	"time"

	"invoicefast/internal/database"
	"invoicefast/internal/models"

	stripe "github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/customer"
	"github.com/stripe/stripe-go/v72/paymentintent"
	"github.com/stripe/stripe-go/v72/refund"
	"github.com/stripe/stripe-go/v72/webhook"
)

type StripeService struct {
	db        *database.DB
	secretKey string
	publicKey string
	whSecret string
}

type CreatePaymentIntentRequest struct {
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	InvoiceID   string  `json:"invoice_id"`
	Description string  `json:"description"`
	Email       string  `json:"email"`
}

type PaymentIntentResponse struct {
	ClientSecret string  `json:"client_secret"`
	PaymentID    string  `json:"payment_id"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
}

func NewStripeService(db *database.DB, secretKey, publicKey, whSecret string) *StripeService {
	stripe.Key = secretKey
	return &StripeService{
		db:        db,
		secretKey: secretKey,
		publicKey: publicKey,
		whSecret: whSecret,
	}
}

func (s *StripeService) CreatePaymentIntent(req *CreatePaymentIntentRequest) (*PaymentIntentResponse, error) {
	if s.secretKey == "" {
		return nil, errors.New("stripe not configured - secret key required")
	}

	amountCents := int64(req.Amount * 100)

	desc := fmt.Sprintf("Invoice %s", req.InvoiceID)
	if req.Description != "" {
		desc = req.Description
	}

	params := &stripe.PaymentIntentParams{
		Amount:                  stripe.Int64(amountCents),
		Currency:              stripe.String(req.Currency),
		Description:          stripe.String(desc),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
	}
	if req.Email != "" {
		params.ReceiptEmail = stripe.String(req.Email)
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment intent: %w", err)
	}

	return &PaymentIntentResponse{
		ClientSecret: pi.ClientSecret,
		PaymentID:    pi.ID,
		Amount:      req.Amount,
		Currency:    req.Currency,
	}, nil
}

func (s *StripeService) HandleWebhook(payload []byte, signature string) error {
	if s.secretKey == "" {
		return errors.New("stripe not configured")
	}

	event, err := webhook.ConstructEvent(payload, signature, s.whSecret)
	if err != nil {
		return fmt.Errorf("webhook signature verification failed: %w", err)
	}

	switch event.Type {
	case "payment_intent.succeeded":
		return s.handlePaymentSuccess(event.Data.Object)
	case "payment_intent.payment_failed":
		return s.handlePaymentFailure(event.Data.Object)
	case "charge.refunded":
		return s.handleRefund(event.Data.Object)
	}

	return nil
}

func invoiceIDFromDescription(desc string) string {
	if len(desc) > 8 && desc[:7] == "Invoice" {
		return desc[8:]
	}
	return desc
}

func (s *StripeService) handlePaymentSuccess(data interface{}) error {
	pi, ok := data.(*stripe.PaymentIntent)
	if !ok {
		return errors.New("invalid payment intent data")
	}

	invoiceID := invoiceIDFromDescription(pi.Description)
	if invoiceID == "" {
		return errors.New("no invoice reference in payment intent")
	}

	var invoice models.Invoice
	if err := s.db.Scopes(database.TenantFilter("")).First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return fmt.Errorf("invoice not found: %w", err)
	}

	amount := models.Money(pi.Amount)

	chargeID := ""
	if pi.Charges != nil && len(pi.Charges.Data) > 0 {
		chargeID = pi.Charges.Data[0].ID
	}

	payment := &models.Payment{
		ID:             pi.ID,
		TenantID:       invoice.TenantID,
		UserID:         invoice.UserID,
		InvoiceID:      invoiceID,
		Amount:         amount,
		Currency:       string(pi.Currency),
		Method:         models.PaymentMethodCard,
		Status:         models.PaymentStatusCompleted,
		Reference:      pi.ID,
		StripeChargeID: chargeID,
	}
	now := time.Now()
	payment.CompletedAt = &now

	if err := s.db.Create(payment).Error; err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}

	invoice.PaidAmount = invoice.PaidAmount.Add(amount)
	if invoice.PaidAmount.GreaterThan(invoice.Total) || invoice.PaidAmount.Equals(invoice.Total) {
		invoice.Status = models.InvoiceStatusPaid
		invoice.PaidAt = &now
	}

	return s.db.Save(&invoice).Error
}

func (s *StripeService) handlePaymentFailure(data interface{}) error {
	pi, ok := data.(*stripe.PaymentIntent)
	if !ok {
		return errors.New("invalid payment intent data")
	}

	invoiceID := invoiceIDFromDescription(pi.Description)
	if invoiceID == "" {
		return nil
	}

	lastError := ""
	if pi.LastPaymentError != nil {
		lastError = pi.LastPaymentError.Error()
	}

	return s.db.Model(&models.Invoice{}).Where("id = ?", invoiceID).
		Updates(map[string]interface{}{
			"last_payment_error": lastError,
		}).Error
}

func (s *StripeService) handleRefund(data interface{}) error {
	ch, ok := data.(*stripe.Charge)
	if !ok {
		return errors.New("invalid charge data")
	}

	if ch.Invoice == nil && ch.PaymentIntent == nil {
		return nil
	}

	var payment models.Payment
	// Try matching by StripeChargeID first, then by Reference (payment intent ID)
	if err := s.db.Where("stripe_charge_id = ?", ch.ID).First(&payment).Error; err != nil {
		if ch.PaymentIntent != nil {
			if err := s.db.Where("reference = ?", ch.PaymentIntent).First(&payment).Error; err != nil {
				return nil
			}
		} else {
			return nil
		}
	}

	payment.Status = models.PaymentStatusRefunded
	return s.db.Save(&payment).Error
}

func (s *StripeService) CreateCheckoutSession(invoice *models.Invoice, successURL, cancelURL string) (string, error) {
	if s.secretKey == "" {
		return "", errors.New("stripe not configured")
	}

	amountCents := int64(invoice.Total)

	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					UnitAmount: stripe.Int64(amountCents),
					Currency:  stripe.String(invoice.Currency),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String(fmt.Sprintf("Invoice %s", invoice.InvoiceNumber)),
					},
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String("payment"),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
	}

	sess, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("failed to create checkout session: %w", err)
	}

	return sess.URL, nil
}

func (s *StripeService) CreateBillingSession(planName string, amount int64, customerEmail, successURL, cancelURL string) (string, error) {
	if s.secretKey == "" {
		return "", errors.New("stripe not configured")
	}

	amountCents := amount * 100

	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					UnitAmount: stripe.Int64(amountCents),
					Currency:  stripe.String("usd"),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("InvoiceFast - " + planName + " Plan"),
					},
					Recurring: &stripe.CheckoutSessionLineItemPriceDataRecurringParams{
						Interval: stripe.String("month"),
					},
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:                stripe.String("subscription"),
		SuccessURL:          stripe.String(successURL),
		CancelURL:           stripe.String(cancelURL),
		CustomerEmail:      stripe.String(customerEmail),
		BillingAddressCollection: stripe.String("required"),
	}

	sess, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("failed to create billing session: %w", err)
	}

	return sess.URL, nil
}

func (s *StripeService) CreateCustomer(email, name string) (string, error) {
	if s.secretKey == "" {
		return "", errors.New("stripe not configured")
	}

	params := &stripe.CustomerParams{
		Email: stripe.String(email),
		Name: stripe.String(name),
	}

	c, err := customer.New(params)
	if err != nil {
		return "", fmt.Errorf("failed to create customer: %w", err)
	}

	return c.ID, nil
}

func (s *StripeService) Refund(paymentID string, amount float64) error {
	if s.secretKey == "" {
		return errors.New("stripe not configured")
	}

	amountCents := int64(amount * 100)

	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(paymentID),
		Amount:       stripe.Int64(amountCents),
	}

	_, err := refund.New(params)
	if err != nil {
		return fmt.Errorf("failed to process refund: %w", err)
	}

	return nil
}

func (s *StripeService) GetPublicKey() string {
	return s.publicKey
}

func (s *StripeService) IsEnabled() bool {
	return s.secretKey != "" && s.publicKey != ""
}