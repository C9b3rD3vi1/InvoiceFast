package pdf

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
)

// PDFGenerator handles PDF generation for invoices and receipts
type PDFGenerator struct {
	templatePath string
	outputPath   string
	templates    map[string]*template.Template
	mu           sync.RWMutex
}

// NewPDFGenerator creates a new PDF generator
func NewPDFGenerator(templatePath, outputPath string) *PDFGenerator {
	gen := &PDFGenerator{
		templatePath: templatePath,
		outputPath:   outputPath,
		templates:    make(map[string]*template.Template),
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		log.Printf("Warning: could not create output directory: %v", err)
	}

	// Load templates
	gen.loadTemplates()

	return gen
}

// InvoiceData represents data for invoice PDF
type InvoiceData struct {
	// Company info
	CompanyName    string
	CompanyEmail   string
	CompanyPhone   string
	CompanyAddress string
	CompanyLogo    string
	CompanyKRA     string
	BrandColor     string

	// Client info
	ClientName    string
	ClientEmail   string
	ClientPhone   string
	ClientAddress string
	ClientKRA     string

	// Invoice details
	InvoiceNumber string
	InvoiceDate   time.Time
	DueDate       time.Time
	Currency      string

	// Line items
	Items []InvoiceLineItem

	// Totals
	Subtotal  float64
	TaxRate   float64
	TaxAmount float64
	Discount  float64
	Total     float64

	// Payment info
	PaymentLink string
	PaymentQR   string

	// Additional
	Notes      string
	Terms      string
	Status     string
	PaidAmount float64
	Balance    float64

	// KRA Compliance
	KRACompliant  bool
	ControlNumber string
	QRCodeData    string

	// Receipt info (for receipts)
	ReceiptNumber string
	PaymentMethod string
	PaymentRef    string
	PaymentDate   time.Time
}

// InvoiceLineItem represents a single invoice line item
type InvoiceLineItem struct {
	Description string
	Quantity    float64
	Unit        string
	UnitPrice   float64
	TaxRate     float64
	Total       float64
}

// PDFOutput represents generated PDF
type PDFOutput struct {
	Content     []byte
	ContentType string
	Filename    string
	Checksum    string
}

// GenerateInvoicePDF generates a PDF for an invoice
func (p *PDFGenerator) GenerateInvoicePDF(data *InvoiceData) (*PDFOutput, error) {
	// Generate QR code for payment
	if data.PaymentLink != "" {
		qrCode, err := p.generateQRCode(data.PaymentLink)
		if err != nil {
			log.Printf("Warning: could not generate QR code: %v", err)
		} else {
			data.PaymentQR = qrCode
		}
	}

	// Generate KRA compliance QR
	if data.KRACompliant && data.ControlNumber != "" {
		kraQR, err := p.generateKRAQRCode(data)
		if err != nil {
			log.Printf("Warning: could not generate KRA QR: %v", err)
		} else {
			data.QRCodeData = kraQR
		}
	}

	// Render HTML template
	html, err := p.renderInvoiceHTML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to render HTML: %w", err)
	}

	// Convert HTML to PDF
	pdf, err := p.htmlToPDF(html)
	if err != nil {
		// Fallback: return HTML if PDF conversion fails
		log.Printf("Warning: PDF conversion failed, returning HTML: %v", err)
		return &PDFOutput{
			Content:     []byte(html),
			ContentType: "text/html",
			Filename:    fmt.Sprintf("invoice-%s.html", data.InvoiceNumber),
			Checksum:    hashContent([]byte(html)),
		}, nil
	}

	return &PDFOutput{
		Content:     pdf,
		ContentType: "application/pdf",
		Filename:    fmt.Sprintf("invoice-%s.pdf", data.InvoiceNumber),
		Checksum:    hashContent(pdf),
	}, nil
}

// GenerateReceiptPDF generates a PDF receipt for a payment
func (p *PDFGenerator) GenerateReceiptPDF(data *InvoiceData) (*PDFOutput, error) {
	// Generate QR code for receipt
	receiptQRData := fmt.Sprintf("RECEIPT:%s|INV:%s|AMT:%.2f|DATE:%s",
		data.ReceiptNumber,
		data.InvoiceNumber,
		data.Total,
		data.PaymentDate.Format("20060102"))

	qrCode, err := p.generateQRCode(receiptQRData)
	if err != nil {
		log.Printf("Warning: could not generate receipt QR: %v", err)
	} else {
		data.QRCodeData = qrCode
	}

	html, err := p.renderReceiptHTML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to render receipt HTML: %w", err)
	}

	pdf, err := p.htmlToPDF(html)
	if err != nil {
		return &PDFOutput{
			Content:     []byte(html),
			ContentType: "text/html",
			Filename:    fmt.Sprintf("receipt-%s.html", data.ReceiptNumber),
			Checksum:    hashContent([]byte(html)),
		}, nil
	}

	return &PDFOutput{
		Content:     pdf,
		ContentType: "application/pdf",
		Filename:    fmt.Sprintf("receipt-%s.pdf", data.ReceiptNumber),
		Checksum:    hashContent(pdf),
	}, nil
}

// loadTemplates loads HTML templates
func (p *PDFGenerator) loadTemplates() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Define template functions
	funcMap := template.FuncMap{
		"dateFormat":   formatDate,
		"currency":     formatCurrency,
		"numberFormat": formatNumber,
		"add":          func(a, b float64) float64 { return a + b },
		"multiply":     func(a, b float64) float64 { return a * b },
		"round":        func(a float64, p int) float64 { return round(a, p) },
		"upper":        strings.ToUpper,
		"lower":        strings.ToLower,
		"safe":         func(s string) template.HTML { return template.HTML(s) },
	}

	// Load invoice template
	tmpl, err := template.New("invoice").Funcs(funcMap).Parse(defaultInvoiceTemplate)
	if err != nil {
		log.Printf("Warning: could not parse invoice template: %v", err)
	} else {
		p.templates["invoice"] = tmpl
	}

	// Load receipt template
	tmpl, err = template.New("receipt").Funcs(funcMap).Parse(defaultReceiptTemplate)
	if err != nil {
		log.Printf("Warning: could not parse receipt template: %v", err)
	} else {
		p.templates["receipt"] = tmpl
	}
}

// htmlToPDF converts HTML to PDF using available methods
func (p *PDFGenerator) htmlToPDF(html string) ([]byte, error) {
	// Try wkhtmltopdf first (if available)
	if pdf, err := p.wkhtmltopdf(html); err == nil {
		return pdf, nil
	}

	// Try chromedp or chrome headless (if available)
	if pdf, err := p.chromeHeadless(html); err == nil {
		return pdf, nil
	}

	// Fallback: Return HTML
	return nil, fmt.Errorf("no PDF converter available")
}

// HtmlToPDF converts HTML string to PDF and returns it as PDFOutput
func (p *PDFGenerator) HtmlToPDF(html string, filename string) (*PDFOutput, error) {
	pdfBytes, err := p.htmlToPDF(html)
	if err != nil {
		return nil, err
	}

	return &PDFOutput{
		Content:     pdfBytes,
		ContentType: "application/pdf",
		Filename:    filename + ".pdf",
		Checksum:    hashContent(pdfBytes),
	}, nil
}

// wkhtmltopdf converts HTML to PDF using wkhtmltopdf
func (p *PDFGenerator) wkhtmltopdf(html string) ([]byte, error) {
	// Check if wkhtmltopdf is installed
	if _, err := exec.LookPath("wkhtmltopdf"); err != nil {
		return nil, fmt.Errorf("wkhtmltopdf not found: %w", err)
	}

	// Create temp files
	tmpHTML := filepath.Join(os.TempDir(), fmt.Sprintf("invoice-%d.html", time.Now().UnixNano()))
	tmpPDF := filepath.Join(os.TempDir(), fmt.Sprintf("invoice-%d.pdf", time.Now().UnixNano()))

	defer os.Remove(tmpHTML)
	defer os.Remove(tmpPDF)

	// Write HTML to temp file
	if err := os.WriteFile(tmpHTML, []byte(html), 0644); err != nil {
		return nil, fmt.Errorf("could not write temp HTML: %w", err)
	}

	// Run wkhtmltopdf
	cmd := exec.Command("wkhtmltopdf",
		"--enable-local-file-access",
		"--page-size", "A4",
		"--margin-top", "15mm",
		"--margin-bottom", "15mm",
		"--margin-left", "15mm",
		"--margin-right", "15mm",
		"--encoding", "UTF-8",
		"--no-stop-slow-scripts",
		"--javascript-delay", "1000",
		tmpHTML, tmpPDF,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("wkhtmltopdf failed: %v\n%s", err, stderr.String())
	}

	// Read generated PDF
	pdf, err := os.ReadFile(tmpPDF)
	if err != nil {
		return nil, fmt.Errorf("could not read generated PDF: %w", err)
	}

	return pdf, nil
}

// chromeHeadless converts HTML to PDF using Chrome headless
func (p *PDFGenerator) chromeHeadless(html string) ([]byte, error) {
	// Check if chrome/chromium is installed
	chromePath := ""
	for _, path := range []string{"google-chrome", "chromium", "chromium-browser", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"} {
		if _, err := exec.LookPath(path); err == nil {
			chromePath = path
			break
		}
	}

	if chromePath == "" {
		return nil, fmt.Errorf("chrome/chromium not found")
	}

	// Create temp files
	tmpHTML := filepath.Join(os.TempDir(), fmt.Sprintf("invoice-%d.html", time.Now().UnixNano()))
	tmpPDF := filepath.Join(os.TempDir(), fmt.Sprintf("invoice-%d.pdf", time.Now().UnixNano()))
	defer os.Remove(tmpHTML)
	defer os.Remove(tmpPDF)

	if err := os.WriteFile(tmpHTML, []byte(html), 0644); err != nil {
		return nil, fmt.Errorf("could not write temp HTML: %w", err)
	}

	// Run Chrome headless with file output
	cmd := exec.Command(chromePath,
		"--headless",
		"--disable-gpu",
		"--no-sandbox",
		"--disable-software-rasterizer",
		"--print-to-pdf="+tmpPDF,
		tmpHTML,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("chrome headless failed: %v\n%s", err, stderr.String())
	}

	// Read the generated PDF
	pdfBytes, err := os.ReadFile(tmpPDF)
	if err != nil {
		return nil, fmt.Errorf("could not read generated PDF: %w", err)
	}

	// Check if PDF is valid
	if len(pdfBytes) < 100 {
		return nil, fmt.Errorf("generated PDF is empty or too small")
	}

	return pdfBytes, nil
}

// generateQRCode generates a base64 encoded QR code
func (p *PDFGenerator) generateQRCode(data string) (string, error) {
	png, err := qrcode.Encode(data, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("could not encode QR: %w", err)
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// generateKRAQRCode generates KRA compliance QR code
func (p *PDFGenerator) generateKRAQRCode(data *InvoiceData) (string, error) {
	// KRA QR format: https://etims.kra.go.ke/verify/{control_number}
	kraData := fmt.Sprintf("KRA|%s|%s|%s|%.2f|%d",
		data.ControlNumber,
		data.InvoiceNumber,
		data.CompanyKRA,
		data.Total,
		len(data.Items))

	return p.generateQRCode(kraData)
}

// renderInvoiceHTML renders the invoice HTML template
func (p *PDFGenerator) renderInvoiceHTML(data *InvoiceData) (string, error) {
	p.mu.RLock()
	tmpl, ok := p.templates["invoice"]
	p.mu.RUnlock()

	if !ok {
		return p.renderDefaultInvoiceHTML(data), nil
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}

	return buf.String(), nil
}

// renderReceiptHTML renders the receipt HTML template
func (p *PDFGenerator) renderReceiptHTML(data *InvoiceData) (string, error) {
	p.mu.RLock()
	tmpl, ok := p.templates["receipt"]
	p.mu.RUnlock()

	if !ok {
		return p.renderDefaultReceiptHTML(data), nil
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}

	return buf.String(), nil
}

// renderDefaultInvoiceHTML renders invoice using embedded template
func (p *PDFGenerator) renderDefaultInvoiceHTML(data *InvoiceData) string {
	brandColor := data.BrandColor
	if brandColor == "" {
		brandColor = "#2563eb"
	}

	var itemsHTML string
	for _, item := range data.Items {
		itemsHTML += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td class="amount">%.2f</td>
				<td class="amount">%s %.2f</td>
				<td class="amount">%s %.2f</td>
			</tr>`,
			item.Description,
			item.Quantity,
			data.Currency, item.UnitPrice,
			data.Currency, item.Total,
		)
	}

	statusBadge := ""
	switch data.Status {
	case "paid":
		statusBadge = `<span style="background: #22c55e; color: white; padding: 4px 12px; border-radius: 20px; font-size: 12px;">PAID</span>`
	case "overdue":
		statusBadge = `<span style="background: #ef4444; color: white; padding: 4px 12px; border-radius: 20px; font-size: 12px;">OVERDUE</span>`
	case "partial":
		statusBadge = `<span style="background: #f59e0b; color: white; padding: 4px 12px; border-radius: 20px; font-size: 12px;">PARTIAL</span>`
	}

	var paymentQRHTML string
	if data.PaymentQR != "" {
		paymentQRHTML = fmt.Sprintf(`<img src="%s" alt="Pay with QR" style="width: 150px;">`, data.PaymentQR)
	}

	var kraQRHTML string
	if data.QRCodeData != "" {
		kraQRHTML = fmt.Sprintf(`
			<div style="margin-top: 20px; text-align: center;">
				<img src="%s" alt="KRA QR Code" style="width: 120px;">
				<p style="font-size: 10px; color: #666;">KRA E-TIMS Compliant</p>
			</div>
		`, data.QRCodeData)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Invoice %s</title>
	<style>
		* { box-sizing: border-box; margin: 0; padding: 0; }
		body { font-family: 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; }
		.invoice { max-width: 800px; margin: 0 auto; padding: 40px 20px; }
		.header { display: flex; justify-content: space-between; align-items: flex-start; border-bottom: 3px solid %s; padding-bottom: 30px; margin-bottom: 30px; }
		.company { flex: 1; }
		.company-name { font-size: 28px; font-weight: bold; color: %s; margin-bottom: 10px; }
		.company-info { font-size: 13px; color: #666; }
		.invoice-info { text-align: right; }
		.invoice-number { font-size: 24px; font-weight: bold; color: %s; }
		.invoice-date { font-size: 13px; color: #666; margin-top: 5px; }
		.parties { display: flex; justify-content: space-between; margin-bottom: 40px; }
		.party { width: 45%%; }
		.party-title { font-size: 11px; text-transform: uppercase; color: #999; letter-spacing: 1px; margin-bottom: 10px; }
		.party-name { font-weight: bold; font-size: 16px; margin-bottom: 5px; }
		.party-details { font-size: 13px; color: #666; }
		table { width: 100%%; border-collapse: collapse; margin-bottom: 30px; }
		th { background: #f8f9fa; padding: 15px 12px; text-align: left; font-size: 11px; text-transform: uppercase; color: #666; letter-spacing: 0.5px; }
		td { padding: 15px 12px; border-bottom: 1px solid #eee; }
		.amount { text-align: right; }
		.totals { margin-left: auto; width: 300px; }
		.totals-row { display: flex; justify-content: space-between; padding: 10px 0; font-size: 14px; }
		.total-row { font-size: 20px; font-weight: bold; color: %s; border-top: 2px solid %s; margin-top: 10px; padding-top: 15px; }
		.footer { margin-top: 50px; text-align: center; color: #999; font-size: 11px; }
		.payment-qr { text-align: center; margin: 30px 0; }
		.kra-compliance { text-align: center; margin-top: 30px; padding: 15px; background: #f8f9fa; border-radius: 8px; }
		.status { position: absolute; top: 20px; right: 20px; }
		@media print {
			body { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
		}
	</style>
</head>
<body>
	<div class="invoice">
		<div class="header">
			<div class="company">
				<div class="company-name">%s</div>
				<div class="company-info">
					<div>Email: %s</div>
					<div>Phone: %s</div>
					<div>%s</div>
					%s
				</div>
			</div>
			<div class="invoice-info">
				%s
				<div class="invoice-number">INVOICE #%s</div>
				<div class="invoice-date">Date: %s</div>
				<div class="invoice-date">Due: %s</div>
			</div>
		</div>

		<div class="parties">
			<div class="party">
				<div class="party-title">Billed To</div>
				<div class="party-name">%s</div>
				<div class="party-details">
					<div>%s</div>
					<div>%s</div>
					<div>%s</div>
					%s
				</div>
			</div>
		</div>

		<table>
			<thead>
				<tr>
					<th>Description</th>
					<th class="amount">Quantity</th>
					<th class="amount">Unit Price</th>
					<th class="amount">Total</th>
				</tr>
			</thead>
			<tbody>
				%s
			</tbody>
		</table>

		<div class="totals">
			<div class="totals-row"><span>Subtotal</span><span>%s %.2f</span></div>
			%s
			%s
			<div class="totals-row total-row"><span>Total</span><span>%s %.2f</span></div>
			%s
		</div>

		%s

		%s

		%s

		<div class="footer">
			<div>%s</div>
			<div>This invoice was generated by InvoiceFast</div>
		</div>
	</div>
</body>
</html>`,
		data.InvoiceNumber,
		brandColor,
		brandColor,
		brandColor,
		brandColor,
		brandColor,
		brandColor,
		data.CompanyName,
		data.CompanyEmail,
		data.CompanyPhone,
		data.CompanyAddress,
		func() string {
			if data.CompanyKRA != "" {
				return fmt.Sprintf("<div>KRA PIN: %s</div>", data.CompanyKRA)
			}
			return ""
		}(),
		statusBadge,
		data.InvoiceNumber,
		data.InvoiceDate.Format("Jan 02, 2006"),
		data.DueDate.Format("Jan 02, 2006"),
		data.ClientName,
		data.ClientEmail,
		data.ClientPhone,
		data.ClientAddress,
		func() string {
			if data.ClientKRA != "" {
				return fmt.Sprintf("<div>KRA PIN: %s</div>", data.ClientKRA)
			}
			return ""
		}(),
		itemsHTML,
		data.Currency, data.Subtotal,
		func() string {
			if data.TaxRate > 0 {
				return fmt.Sprintf(`<div class="totals-row"><span>Tax (%.1f%%)</span><span>%s %.2f</span></div>`,
					data.TaxRate, data.Currency, data.TaxAmount)
			}
			return ""
		}(),
		func() string {
			if data.Discount > 0 {
				return fmt.Sprintf(`<div class="totals-row"><span>Discount</span><span>-%s %.2f</span></div>`,
					data.Currency, data.Discount)
			}
			return ""
		}(),
		data.Currency, data.Total,
		func() string {
			if data.PaidAmount > 0 {
				balance := data.Total - data.PaidAmount
				return fmt.Sprintf(`<div class="totals-row" style="color: #22c55e;"><span>Paid</span><span>-%s %.2f</span></div>
							<div class="totals-row" style="font-size: 16px; font-weight: bold;"><span>Balance Due</span><span>%s %.2f</span></div>`,
					data.Currency, data.PaidAmount,
					data.Currency, balance)
			}
			return ""
		}(),
		func() string {
			if paymentQRHTML != "" {
				return fmt.Sprintf(`<div class="payment-qr"><h3>Scan to Pay</h3>%s</div>`, paymentQRHTML)
			}
			return ""
		}(),
		kraQRHTML,
		func() string {
			if data.ControlNumber != "" {
				return fmt.Sprintf(`<div class="kra-compliance" style="background: #ecfdf5; border: 1px solid #10b981;">
					<p style="font-size: 11px; color: #10b981; font-weight: bold;">KRA eTIMS VERIFIED</p>
					<p style="font-size: 10px; color: #666;">ICN: %s</p>
				</div>`, data.ControlNumber)
			}
			return ""
		}(),
		data.Notes,
	)
}

// renderDefaultReceiptHTML renders receipt using embedded template
func (p *PDFGenerator) renderDefaultReceiptHTML(data *InvoiceData) string {
	brandColor := "#22c55e" // Green for receipts

	qrHTML := ""
	if data.QRCodeData != "" {
		qrHTML = fmt.Sprintf(`<img src="%s" alt="Receipt QR" style="width: 150px;">`, data.QRCodeData)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Receipt %s</title>
	<style>
		body { font-family: 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; max-width: 600px; margin: 0 auto; padding: 40px 20px; }
		.header { text-align: center; border-bottom: 2px solid %s; padding-bottom: 20px; margin-bottom: 20px; }
		.success { color: #22c55e; font-size: 48px; }
		h1 { color: %s; margin: 10px 0; }
		.receipt-box { background: #f8f9fa; border-radius: 12px; padding: 25px; margin: 20px 0; }
		.receipt-row { display: flex; justify-content: space-between; padding: 10px 0; border-bottom: 1px solid #eee; }
		.receipt-row:last-child { border-bottom: none; }
		.amount { font-size: 32px; font-weight: bold; color: %s; text-align: center; margin: 20px 0; }
		.footer { text-align: center; color: #666; font-size: 12px; margin-top: 30px; }
		.qr { text-align: center; margin: 20px 0; }
	</style>
</head>
<body>
	<div class="header">
		<div class="success">✓</div>
		<h1>Payment Received</h1>
		<p>Receipt #%s</p>
	</div>

	<div class="receipt-box">
		<div class="receipt-row"><span>Date:</span><span>%s</span></div>
		<div class="receipt-row"><span>Invoice:</span><span>%s</span></div>
		<div class="receipt-row"><span>From:</span><span>%s</span></div>
		<div class="receipt-row"><span>Payment Method:</span><span>%s</span></div>
		<div class="receipt-row"><span>Reference:</span><span>%s</span></div>
	</div>

	<div class="amount">%s %.2f</div>

	<div class="qr">%s</div>

	<div class="footer">
		<p>Thank you for your payment!</p>
		<p>Receipt generated by InvoiceFast</p>
	</div>
</body>
</html>`,
		data.ReceiptNumber, brandColor, brandColor, brandColor, data.ReceiptNumber,
		data.PaymentDate.Format("Jan 02, 2006 at 15:04"),
		data.InvoiceNumber, data.CompanyName, data.PaymentMethod, data.PaymentRef,
		data.Currency, data.Total, qrHTML,
	)
}

// Helper functions

func formatDate(t time.Time) string {
	return t.Format("Jan 02, 2006")
}

func formatCurrency(currency string) string {
	symbols := map[string]string{
		"KES": "KSh",
		"USD": "$",
		"EUR": "€",
		"GBP": "£",
		"TZS": "TSh",
		"UGX": "USh",
		"NGN": "₦",
		"ZAR": "R",
	}
	if symbol, ok := symbols[currency]; ok {
		return symbol
	}
	return currency
}

func formatNumber(n float64) string {
	return fmt.Sprintf("%.2f", n)
}

func round(n float64, precision int) float64 {
	factor := 1.0
	for i := 0; i < precision; i++ {
		factor *= 10
	}
	return float64(int(n*factor+0.5)) / factor
}

func hashContent(content []byte) string {
	// Simple hash for cache/validation
	return fmt.Sprintf("%x", len(content))
}

// Embed templates
var defaultInvoiceTemplate = `
{{/* Invoice template loaded dynamically */ -}}
`

var defaultReceiptTemplate = `
{{/* Receipt template loaded dynamically */ -}}
`

// Close cleans up resources
func (p *PDFGenerator) Close() error {
	// Clean up any resources
	return nil
}
