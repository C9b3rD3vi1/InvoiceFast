package services

import (
	"bytes"
	"fmt"
	"html/template"
	"math/rand"
	"strings"
	"time"

	"invoicefast/internal/models"
)

// PDFService handles PDF generation for invoices
type PDFService struct{}

// InvoicePDFData contains all data needed for PDF rendering
type InvoicePDFData struct {
	InvoiceNumber string
	Reference     string
	IssueDate     string
	DueDate       string
	Status        string
	Currency      string

	CompanyName    string
	CompanyAddress string
	CompanyEmail   string
	CompanyPhone   string
	KRAPIN         string
	LogoURL        string
	BrandColor     string

	ClientName    string
	ClientEmail   string
	ClientPhone   string
	ClientAddress string
	ClientKRAPIN  string

	Items      []InvoicePDFItem
	Subtotal   float64
	TaxRate    float64
	TaxAmount  float64
	Discount   float64
	Total      float64
	PaidAmount float64
	BalanceDue float64

	Notes               string
	Terms               string
	PaymentLink         string
	MpesaBusinessNumber string
}

type InvoicePDFItem struct {
	Description string
	Quantity    float64
	Unit        string
	UnitPrice   float64
	Total       float64
}

// GenerateInvoicePDF generates a PDF-ready HTML for an invoice
func (s *PDFService) GenerateInvoiceHTML(invoice *models.Invoice, user *models.User) (string, error) {
	// Prepare items
	items := make([]InvoicePDFItem, len(invoice.Items))
	for i, item := range invoice.Items {
		items[i] = InvoicePDFItem{
			Description: item.Description,
			Quantity:    item.Quantity,
			Unit:        item.Unit,
			UnitPrice:   item.UnitPrice,
			Total:       item.Total,
		}
	}

	// Determine dates
	issueDate := invoice.CreatedAt.Format("02 Jan 2006")
	dueDate := invoice.DueDate.Format("02 Jan 2006")

	// Format totals
	balanceDue := invoice.Total - invoice.PaidAmount
	if balanceDue < 0 {
		balanceDue = 0
	}

	data := InvoicePDFData{
		InvoiceNumber: invoice.InvoiceNumber,
		Reference:     invoice.Reference,
		IssueDate:     issueDate,
		DueDate:       dueDate,
		Status:        string(invoice.Status),
		Currency:      invoice.Currency,

		CompanyName:    user.CompanyName,
		CompanyAddress: user.CompanyName + " Address", // Could add to user model
		CompanyEmail:   user.Email,
		CompanyPhone:   user.Phone,
		KRAPIN:         user.KRAPIN,
		LogoURL:        invoice.LogoURL,
		BrandColor:     invoice.BrandColor,

		ClientName:    invoice.Client.Name,
		ClientEmail:   invoice.Client.Email,
		ClientPhone:   invoice.Client.Phone,
		ClientAddress: invoice.Client.Address,
		ClientKRAPIN:  invoice.Client.KRAPIN,

		Items:      items,
		Subtotal:   invoice.Subtotal,
		TaxRate:    invoice.TaxRate,
		TaxAmount:  invoice.TaxAmount,
		Discount:   invoice.Discount,
		Total:      invoice.Total,
		PaidAmount: invoice.PaidAmount,
		BalanceDue: balanceDue,

		Notes:               invoice.Notes,
		Terms:               invoice.Terms,
		PaymentLink:         invoice.PaymentLink,
		MpesaBusinessNumber: "123456", // Configurable
	}

	// Generate QR code content (for KRA compliance)
	qrContent := fmt.Sprintf("INV:%s|AMT:%.2f|DATE:%s|TIN:%s",
		invoice.InvoiceNumber, invoice.Total, issueDate, user.KRAPIN)
	_ = qrContent

	// Render template
	return renderInvoiceTemplate(data)
}

func renderInvoiceTemplate(data InvoicePDFData) (string, error) {
	const templateStr = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Invoice {{.InvoiceNumber}}</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { 
            font-family: 'Helvetica Neue', Arial, sans-serif; 
            font-size: 14px; 
            line-height: 1.5; 
            color: #333;
            background: #fff;
        }
        .invoice-container {
            max-width: 800px;
            margin: 0 auto;
            padding: 40px;
        }
        .header {
            display: flex;
            justify-content: space-between;
            align-items: flex-start;
            margin-bottom: 40px;
            padding-bottom: 20px;
            border-bottom: 3px solid {{.BrandColor}};
        }
        .company-info { flex: 1; }
        .company-name { 
            font-size: 24px; 
            font-weight: 700; 
            color: {{.BrandColor}};
            margin-bottom: 8px;
        }
        .company-details { 
            font-size: 12px; 
            color: #666;
        }
        .invoice-details { 
            text-align: right; 
        }
        .invoice-number {
            font-size: 24px;
            font-weight: 700;
            color: {{.BrandColor}};
        }
        .invoice-meta {
            font-size: 12px;
            color: #666;
            margin-top: 8px;
        }
        .status-badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
            margin-top: 8px;
        }
        .status-draft { background: #f3f4f6; color: #6b7280; }
        .status-sent { background: #dbeafe; color: #2563eb; }
        .status-paid { background: #dcfce7; color: #16a34a; }
        .status-overdue { background: #fee2e2; color: #dc2626; }

        .parties {
            display: flex;
            justify-content: space-between;
            margin-bottom: 40px;
        }
        .party { flex: 1; }
        .party-title {
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
            color: #666;
            margin-bottom: 8px;
        }
        .party-name { font-weight: 600; margin-bottom: 4px; }

        table {
            width: 100%;
            border-collapse: collapse;
            margin-bottom: 30px;
        }
        th {
            background: {{.BrandColor}};
            color: white;
            padding: 12px;
            text-align: left;
            font-size: 12px;
            font-weight: 600;
            text-transform: uppercase;
        }
        th:nth-child(2),
        th:nth-child(3),
        th:nth-child(4) { text-align: right; }
        
        td {
            padding: 12px;
            border-bottom: 1px solid #e5e7eb;
        }
        td:nth-child(2),
        td:nth-child(3),
        td:nth-child(4) { text-align: right; }

        .totals {
            margin-left: auto;
            width: 300px;
        }
        .totals-row {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            font-size: 13px;
        }
        .totals-row.total {
            font-size: 18px;
            font-weight: 700;
            border-top: 2px solid {{.BrandColor}};
            margin-top: 8px;
            padding-top: 12px;
        }
        .totals-row.paid {
            color: #16a34a;
        }

        .notes {
            margin-top: 40px;
            padding-top: 20px;
            border-top: 1px solid #e5e7eb;
        }
        .notes-title {
            font-size: 12px;
            font-weight: 600;
            text-transform: uppercase;
            color: #666;
            margin-bottom: 8px;
        }

        .payment-box {
            background: #f9fafb;
            border: 1px solid #e5e7eb;
            border-radius: 8px;
            padding: 20px;
            margin-top: 30px;
        }
        .payment-title {
            font-size: 14px;
            font-weight: 600;
            margin-bottom: 12px;
        }
        .payment-instructions {
            font-size: 12px;
            color: #666;
            margin-bottom: 12px;
        }
        .pay-button {
            display: inline-block;
            background: {{.BrandColor}};
            color: white;
            padding: 12px 24px;
            text-decoration: none;
            border-radius: 6px;
            font-weight: 600;
        }

        .footer {
            margin-top: 60px;
            padding-top: 20px;
            border-top: 1px solid #e5e7eb;
            text-align: center;
            font-size: 11px;
            color: #999;
        }

        .qr-code {
            width: 80px;
            height: 80px;
            background: #f3f4f6;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 10px;
            color: #666;
        }

        @media print {
            body { -webkit-print-color-adjust: exact; }
            .invoice-container { padding: 0; }
        }
    </style>
</head>
<body>
    <div class="invoice-container">
        <!-- Header -->
        <div class="header">
            <div class="company-info">
                {{if .LogoURL}}
                <img src="{{.LogoURL}}" alt="Logo" style="max-height: 50px; margin-bottom: 12px;">
                {{end}}
                <div class="company-name">{{.CompanyName}}</div>
                <div class="company-details">
                    {{.CompanyAddress}}<br>
                    {{.CompanyEmail}}<br>
                    {{.CompanyPhone}}<br>
                    KRA PIN: {{.KRAPIN}}
                </div>
            </div>
            <div class="invoice-details">
                <div class="invoice-number">INVOICE</div>
                <div class="invoice-meta">{{.InvoiceNumber}}</div>
                {{if .Reference}}<div class="invoice-meta">Ref: {{.Reference}}</div>{{end}}
                <div class="invoice-meta">Date: {{.IssueDate}}</div>
                <div class="invoice-meta">Due: {{.DueDate}}</div>
                <span class="status-badge status-{{.Status}}">{{.Status}}</span>
            </div>
        </div>

        <!-- Parties -->
        <div class="parties">
            <div class="party">
                <div class="party-title">From</div>
                <div class="party-name">{{.CompanyName}}</div>
                <div>{{.CompanyAddress}}</div>
                <div>{{.CompanyEmail}}</div>
                <div>{{.CompanyPhone}}</div>
            </div>
            <div class="party">
                <div class="party-title">Bill To</div>
                <div class="party-name">{{.ClientName}}</div>
                <div>{{.ClientAddress}}</div>
                <div>{{.ClientEmail}}</div>
                <div>{{.ClientPhone}}</div>
                {{if .ClientKRAPIN}}<div>KRA PIN: {{.ClientKRAPIN}}</div>{{end}}
            </div>
        </div>

        <!-- Items Table -->
        <table>
            <thead>
                <tr>
                    <th>Description</th>
                    <th>Qty</th>
                    <th>Unit Price</th>
                    <th>Total</th>
                </tr>
            </thead>
            <tbody>
                {{range .Items}}
                <tr>
                    <td>{{.Description}}</td>
                    <td>{{.Quantity}} {{.Unit}}</td>
                    <td>{{$.Currency}} {{printf "%.2f" .UnitPrice}}</td>
                    <td>{{$.Currency}} {{printf "%.2f" .Total}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>

        <!-- Totals -->
        <div class="totals">
            <div class="totals-row">
                <span>Subtotal</span>
                <span>{{.Currency}} {{printf "%.2f" .Subtotal}}</span>
            </div>
            {{if .TaxRate}}
            <div class="totals-row">
                <span>Tax ({{.TaxRate}}%)</span>
                <span>{{.Currency}} {{printf "%.2f" .TaxAmount}}</span>
            </div>
            {{end}}
            {{if .Discount}}
            <div class="totals-row">
                <span>Discount</span>
                <span>-{{.Currency}} {{printf "%.2f" .Discount}}</span>
            </div>
            {{end}}
            <div class="totals-row total">
                <span>Total</span>
                <span>{{.Currency}} {{printf "%.2f" .Total}}</span>
            </div>
            {{if .PaidAmount}}
            <div class="totals-row paid">
                <span>Paid</span>
                <span>-{{.Currency}} {{printf "%.2f" .PaidAmount}}</span>
            </div>
            <div class="totals-row total">
                <span>Balance Due</span>
                <span>{{.Currency}} {{printf "%.2f" .BalanceDue}}</span>
            </div>
            {{end}}
        </div>

        <!-- Notes -->
        {{if .Notes}}
        <div class="notes">
            <div class="notes-title">Notes</div>
            <div>{{.Notes}}</div>
        </div>
        {{end}}

        {{if .Terms}}
        <div class="notes">
            <div class="notes-title">Terms & Conditions</div>
            <div>{{.Terms}}</div>
        </div>
        {{end}}

        <!-- Payment -->
        {{if .PaymentLink}}
        <div class="payment-box">
            <div class="payment-title">Payment Instructions</div>
            <div class="payment-instructions">
                Pay via M-Pesa using the button below or directly to Business No. <strong>{{.MpesaBusinessNumber}}</strong><br>
                Account No.: <strong>{{.InvoiceNumber}}</strong>
            </div>
            <a href="{{.PaymentLink}}" class="pay-button">Pay Now</a>
        </div>
        {{end}}

        <!-- Footer -->
        <div class="footer">
            <p>Thank you for your business!</p>
            <p>Powered by InvoiceFast</p>
        </div>
    </div>
</body>
</html>`

	tmpl, err := template.New("invoice").Funcs(template.FuncMap{
		"printf": func(format string, args ...interface{}) string {
			return fmt.Sprintf(format, args...)
		},
	}).Parse(templateStr)

	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GenerateReceiptPDF generates a receipt for a payment
func (s *PDFService) GenerateReceiptHTML(invoice *models.Invoice, payment *models.Payment, user *models.User) (string, error) {
	receiptNumber := generateReceiptNumber()
	receiptDate := time.Now().Format("02 Jan 2006")

	data := map[string]interface{}{
		"ReceiptNumber": receiptNumber,
		"ReceiptDate":   receiptDate,
		"InvoiceNumber": invoice.InvoiceNumber,
		"CompanyName":   user.CompanyName,
		"CompanyEmail":  user.Email,
		"CompanyPhone":  user.Phone,
		"KRAPIN":        user.KRAPIN,
		"ClientName":    invoice.Client.Name,
		"Amount":        payment.Amount,
		"Currency":      payment.Currency,
		"Method":        payment.Method,
		"Reference":     payment.Reference,
		"TotalInvoice":  invoice.Total,
		"BalanceBefore": invoice.Total,
		"BalanceAfter":  0.0,
	}

	const receiptTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Receipt {{.ReceiptNumber}}</title>
    <style>
        body { font-family: Arial, sans-serif; padding: 40px; }
        .receipt { max-width: 400px; margin: 0 auto; }
        .header { text-align: center; margin-bottom: 30px; }
        .company { font-size: 20px; font-weight: bold; color: #2563eb; }
        .title { font-size: 24px; font-weight: bold; margin: 20px 0; }
        .receipt-number { color: #666; font-size: 14px; }
        .details { margin: 20px 0; }
        .row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px dashed #ccc; }
        .row.total { font-weight: bold; font-size: 18px; border-top: 2px solid #2563eb; border-bottom: none; }
        .footer { text-align: center; margin-top: 30px; font-size: 12px; color: #999; }
    </style>
</head>
<body>
    <div class="receipt">
        <div class="header">
            <div class="company">{{.CompanyName}}</div>
            <div>{{.CompanyEmail}} | {{.CompanyPhone}}</div>
            <div>KRA PIN: {{.KRAPIN}}</div>
        </div>
        <div class="title">RECEIPT</div>
        <div class="receipt-number">No: {{.ReceiptNumber}}</div>
        <div class="receipt-number">Date: {{.ReceiptDate}}</div>
        
        <div class="details">
            <div class="row"><span>Invoice</span><span>{{.InvoiceNumber}}</span></div>
            <div class="row"><span>Client</span><span>{{.ClientName}}</span></div>
            <div class="row"><span>Payment Method</span><span>{{.Method}}</span></div>
            {{if .Reference}}<div class="row"><span>Reference</span><span>{{.Reference}}</span></div>{{end}}
            <div class="row total"><span>Amount Paid</span><span>{{.Currency}} {{printf "%.2f" .Amount}}</span></div>
        </div>
        
        <div class="footer">
            <p>Thank you for your payment!</p>
            <p>Generated by InvoiceFast</p>
        </div>
    </div>
</body>
</html>`

	tmpl, err := template.New("receipt").Funcs(template.FuncMap{
		"printf": func(format string, args ...interface{}) string {
			return fmt.Sprintf(format, args...)
		},
	}).Parse(receiptTemplate)

	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func generateReceiptNumber() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return "RCP-" + string(b) + "-" + time.Now().Format("060102")
}

// QRCodeData generates data for QR code (for KRA compliance)
type QRCodeData struct {
	InvoiceNumber string
	Date          string
	Time          string
	Amount        float64
	Currency      string
	SellerTIN     string
	BuyerTIN      string
	VATAmount     float64
	VATRate       float64
}

func (s *QRCodeData) String() string {
	return strings.Join([]string{
		"1", // Version
		s.InvoiceNumber,
		s.Date,
		s.Time,
		fmt.Sprintf("%.2f", s.Amount),
		s.Currency,
		s.SellerTIN,
		s.BuyerTIN,
		fmt.Sprintf("%.2f", s.VATAmount),
		fmt.Sprintf("%.0f", s.VATRate),
	}, "|")
}
