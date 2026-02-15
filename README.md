
---

## InvoiceFast


```markdown
# InvoiceFast

**Professional Invoicing with M-Pesa Integration for African Businesses**

[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Africa-green.svg)](https://invoice.simuxtech.com)

InvoiceFast helps African freelancers and agencies get paid faster with professional invoices, instant M-Pesa payment collection, and automatic receipt generation. Built for the unique needs of the African market.

## 🎯 Problem Statement

African freelancers and SMEs struggle with:
- **Late Payments**: Average 45-day payment terms, often stretching to 90+ days
- **Payment Friction**: International clients find African payment methods confusing
- **Manual Follow-ups**: Chasing payments wastes 8+ hours per week
- **Tax Compliance**: KRA requirements for e-invoicing and record-keeping
- **Currency Issues**: Managing USD invoices vs. local KES payments

## ✨ Features

### Smart Invoicing
- **Professional Templates**: 10+ designs, customizable colors and logos
- **Multi-currency**: USD, KES, TZS, UGX, NGN with auto-conversion
- **Recurring Invoices**: Weekly, monthly, quarterly automation
- **Partial Payments**: Track deposits and installment payments

### M-Pesa Integration
- **STK Push**: Customer receives payment prompt directly on phone
- **Automatic Confirmation**: No "please confirm payment" back-and-forth
- **Split Payments**: Multiple M-Pesa payments against single invoice
- **Payment Plans**: Automated installment reminders and collection

### Client Management
- **Client Portal**: Branded space for clients to view/pay invoices
- **Credit Limits**: Automatic hold on new invoices for overdue accounts
- **Payment History**: Complete transaction timeline per client
- **Bulk Actions**: Send reminders to all overdue clients

### Automation
- **Payment Reminders**: Scheduled WhatsApp, SMS, and email sequences
- **Late Fees**: Automatic calculation and addition to overdue invoices
- **Thank You Messages**: Auto-sent upon payment confirmation
- **Receipt Generation**: KRA-compliant e-receipts with QR codes

## 🚀 Quick Start

### Prerequisites
- Go 1.21+
- SQLite 3
- Flutterwave or M-Pesa Daraja API credentials

### Installation

```bash
git clone https://github.com/C9b3rD3vi1/invoicefast.git
cd invoicefast

cp .env.example .env
# Edit .env with your API keys

go run cmd/migrate/main.go
go run cmd/server/main.go


### Docker Compose

```yaml
version: '3.8'
services:
  invoicefast:
    build: .
    ports:
      - "8082:8082"
    volumes:
      - ./data:/app/data
      - ./templates:/app/templates
    env_file: .env
    restart: unless-stopped
    
    
### First Invoice Creation

```bash
curl -X POST https://invoice.simuxtech.com/api/v1/invoices \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "ABC Company Ltd",
    "client_email": "accounts@abc.co.ke",
    "client_phone": "254712345678",
    "items": [
      {"description": "Web Development", "quantity": 1, "unit_price": 50000}
    ],
    "due_date": "2026-03-15",
    "currency": "KES"
  }'
  
  
  ### Pricing
  
  | Plan           | Price      | Invoices  | Features                                    |
  | -------------- | ---------- | --------- | ------------------------------------------- |
  | **Free**       | \$0        | 5/month   | Basic templates, email delivery             |
  | **Pro**        | \$12/month | Unlimited | WhatsApp integration, recurring, API access |
  | **Agency**     | \$39/month | Unlimited | Team (5 users), white-label, custom domain  |
  | **Enterprise** | \$99/month | Unlimited | Unlimited team, dedicated support, SLA      |



### Architecture

┌─────────────┐     ┌──────────────┐     ┌─────────────────┐
│   Client    │────▶│  InvoiceFast │────▶│   SQLite (WAL)  │
│   Browser   │     │   Go Server  │     │   Per-Tenant    │
└─────────────┘     └──────────────┘     └─────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│   M-Pesa     │   │  WhatsApp    │   │   Email      │
│   STK Push   │   │  Business    │   │  Service     │
└──────────────┘   └──────────────┘   └──────────────┘



### 🔧 Integrations
Payment Gateways
M-Pesa: STK Push, C2B, B2C (via Daraja API)
Flutterwave: Cards, bank transfer, mobile money (multi-country)
Stripe: International card payments (USD/EUR/GBP)
PayPal: Global coverage

## Accounting
QuickBooks: Bi-directional sync
Sage: Export/import
Wave: Free accounting integration
Custom CSV: Universal format

## Productivity
Google Calendar: Invoice due dates
Slack: Payment notifications
Zapier: 5000+ app connections


### 🎨 Customization
Template System
InvoiceFast uses Go's html/template with custom functions:

```HTML
<!-- Custom template example -->
<div class="invoice-header" style="background: {{.BrandColor}}">
  <img src="{{.LogoURL}}" alt="{{.CompanyName}}">
  <h1>INVOICE #{{.InvoiceNumber}}</h1>
</div>

<table class="items">
  {{range .Items}}
  <tr>
    <td>{{.Description}}</td>
    <td>{{.Quantity}} x {{.UnitPrice}}</td>
    <td>{{.Total}}</td>
  </tr>
  {{end}}
</table>
Branding Options
Logo upload (PNG, JPG, SVG)
Primary and secondary colors
Custom footer text
Font selection (Google Fonts)
Background patterns


### 📊 Analytics
Dashboard Metrics
Outstanding Revenue: Total unpaid invoices
Average Payment Time: Days from invoice to payment
Collection Rate: % of invoices paid on time
Revenue Trends: Monthly/weekly visualization
Client Value: Lifetime value per client

### Reports
Aging Report: Overdue invoices by period (30, 60, 90 days)
Tax Summary: VAT/output tax calculations
Revenue by Client: Top contributors
Payment Method Mix: M-Pesa vs. Card vs. Bank


###  Security & Compliance
KRA Compliance (Kenya)
e-TIMs Integration: Automatic invoice registration (upcoming)
QR Code Receipts: Scannable tax-compliant receipts
Audit Trail: Immutable invoice history
Data Retention: 7-year record keeping
Data Protection
GDPR Compliant: Right to deletion, data export
Encryption: AES-256 for sensitive data
Access Logs: Who viewed/modified what and when
2FA: Optional for all accounts


🛠️ Development

```bash

# Install dependencies
go mod download

# Run development server with hot reload
air

# Run tests
go test ./... -v

# Generate test invoices
go run cmd/seed/main.go --invoices=100

# Build for production
go build -o invoicefast cmd/server/main.go


### 📚 API Documentation
Full docs at https://docs.invoice.simuxtech.com


### Webhook Events

```JSON
Copy
{
  "event": "invoice.paid",
  "data": {
    "invoice_id": "inv_123456",
    "amount_paid": 50000,
    "payment_method": "mpesa",
    "mpesa_receipt": "QJ7H9K2L",
    "paid_at": "2026-02-15T14:30:00Z"
  }
}

### 🤝 Support
Help Center: help.invoice.simuxtech.com
WhatsApp: +254 712 345 679
Email: support@invoice.simuxtech.com
Community: community.simuxtech.com


### 🏢 Enterprise Features
Custom Domain: invoices.yourcompany.com
SSO: SAML 2.0, Google Workspace, Microsoft 365
Dedicated Instance: Isolated VPS deployment
Custom Integrations: ERP, CRM, inventory systems
SLA: 99.99% uptime guarantee
Contact: enterprise@simuxtech.com


### 📜 License
MIT License - see LICENSE
Built with 💼 in Nairobi | Powering African Business Payments