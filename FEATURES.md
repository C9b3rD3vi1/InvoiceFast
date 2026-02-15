
### FEATURES.md

```markdown
# InvoiceFast Feature Specification

## Version 1.0.0 (MVP) - Month 5 Target

### Core Invoicing

#### Invoice Creation
- **Quick Create**: Single-page form with smart defaults
- **Item Library**: Save frequently used products/services
- **Auto-numbering**: Customizable invoice number patterns (INV-2026-0001)
- **Notes & Terms**: Default terms per client, customizable per invoice
- **Attachments**: PDF, image, document attachments (up to 10MB)

#### Multi-currency Support
- **Supported Currencies**: KES, USD, EUR, GBP, TZS, UGX, NGN
- **Exchange Rates**: Daily auto-update from Central Bank of Kenya
- **Manual Override**: Custom exchange rates for specific invoices
- **Dual Display**: Show both invoice currency and KES equivalent

#### Invoice States
Draft →  Sent →  Viewed →   Partially Paid → Paid →  Overdue → Cancelled
  ↓        ↓        ↓            ↓           ↓         ↓
Editable  Locked  Reminder    Reminder     Receipt   Retention




### M-Pesa Integration

#### STK Push (Lipa na M-Pesa)
- **Trigger**: One-click "Request Payment" from invoice view
- **Customer Experience**: 
  1. Receives USSD prompt on phone
  2. Enters M-Pesa PIN
  3. Instant confirmation
- **Fallback**: If STK fails, show paybill number and account number
- **Retry Logic**: 3 automatic retries with exponential backoff

#### Payment Matching
- **Automatic**: Match by phone number + amount + time window
- **Manual**: Admin interface to match unallocated payments
- **Partial Payments**: Track multiple payments against single invoice
- **Overpayments**: Handle excess payment (credit note or refund)

#### M-Pesa Reconciliation
- **Daily Settlement Report**: Match M-Pesa settlements to invoices
- **Discrepancy Alerts**: Flag unmatched payments within 1 hour
- **Bulk Reconciliation**: Process multiple payments simultaneously

### Client Management

#### Client Profiles
- **Basic Info**: Name, email, phone, address, KRA PIN
- **Payment Preferences**: Default currency, payment method
- **Credit Terms**: Custom due dates per client (Net 15, Net 30, etc.)
- **Internal Notes**: Private notes visible only to team

#### Client Portal
- **Branded Access**: `client.simuxtech.com/portal/:client_id`
- **Invoice History**: View all invoices and payment status
- **Download**: PDF downloads of invoices and receipts
- **Pay Online**: Direct payment without logging in (magic link)

### Automation

#### Reminder Sequences

**Standard Sequence (Pro Plan)**:
- Day 0: Invoice sent (email + WhatsApp)
- Day 3: Gentle reminder if not viewed
- Day 7: Payment due soon reminder
- Day 1 (overdue): First overdue notice
- Day 7 (overdue): Second overdue notice + late fee warning
- Day 14 (overdue): Final notice + account hold
- Day 30 (overdue): Collections escalation

**Custom Sequences**: User-defined triggers and messages

#### Late Fees
- **Configuration**: Percentage or flat fee, grace period
- **Automatic Application**: Added to invoice on schedule
- **Cap**: Maximum late fee amount (compliance)
- **Waiver**: One-click removal with reason logging

### Notifications

#### WhatsApp Business API
- **Invoice Delivery**: PDF + payment link via WhatsApp
- **Payment Reminders**: Scheduled messages
- **Payment Confirmation**: Instant thank you with receipt
- **Template Messages**: Pre-approved by Meta for reliability

#### Email System
- **Custom SMTP**: Use own email server or shared
- **Templates**: HTML and plain text versions
- **Attachments**: PDF invoice attached
- **Tracking**: Open rates, click tracking on payment links

#### SMS Fallback
- **Provider**: Africa's Talking or Twilio
- **Use Cases**: Critical reminders, 2FA codes
- **Cost**: Pass-through pricing + 10% margin

### Reporting & Analytics

#### Dashboard Widgets
- **Outstanding Revenue**: Total unpaid amount with aging breakdown
- **This Month vs Last Month**: Revenue comparison chart
- **Recent Activity**: Latest invoices, payments, client actions
- **Quick Actions**: Create invoice, send reminder, add client

#### Financial Reports
- **Income Statement**: Revenue by period
- **Aging Report**: Overdue invoices categorized by days
- **Client Statement**: Complete transaction history per client
- **Tax Report**: VAT/output tax summary

#### Export Options
- **PDF**: Professional formatted reports
- **Excel**: Data for further analysis
- **CSV**: Universal compatibility
- **QuickBooks**: Direct import format

## Version 1.1.0 - Month 8 Target

### Advanced Features

#### Recurring Invoices
- **Frequency**: Weekly, bi-weekly, monthly, quarterly, annually
- **End Conditions**: After X occurrences, specific date, never
- **Auto-send**: Email invoice on schedule
- **Auto-charge**: Attempt payment via saved M-Pesa token (with consent)

#### Expense Tracking
- **Receipt Capture**: Photo upload with OCR
- **Categorization**: Predefined or custom categories
- **Billable Expenses**: Mark up and invoice to clients
- **Supplier Management**: Track vendors and payment terms

#### Team Collaboration
- **User Roles**:
  - Admin: Full access
  - Manager: Create/edit invoices, view all reports
  - Sales: Create invoices, view own clients only
  - Accountant: View-only access to all data
- **Activity Log**: Who did what, when
- **Mentions**: @username notifications in notes

### Integrations

#### Accounting Software
- **QuickBooks Online**: OAuth2, bi-directional sync
- **Wave**: Free accounting, one-way export
- **Sage**: CSV export with mapping

#### Payment Gateways
- **Flutterwave**: Multi-country mobile money
- **Stripe**: International cards
- **PayPal**: Global coverage

#### Productivity
- **Google Calendar**: Due date reminders
- **Slack**: #payments channel notifications
- **Zapier**: Trigger workflows on invoice events

## Version 2.0.0 - Month 12 Target

### Enterprise Features

#### White-Label Solution
- **Custom Domain**: CNAME support for `invoices.yourdomain.com`
- **Branding**: Remove InvoiceFast branding, custom colors
- **Email Sending**: Send from your domain (SPF/DKIM setup)
- **Mobile App**: White-label Android/iOS apps

#### Advanced API
- **GraphQL Endpoint**: Flexible data querying
- **Webhooks**: Custom HTTP callbacks for all events
- **Bulk Operations**: Create 1000+ invoices via API
- **Sandbox Mode**: Test environment with fake payments

#### Compliance & Security
- **SSO/SAML**: Corporate authentication
- **Audit Trail**: Immutable logs of all actions
- **Data Residency**: Store data in specific regions
- **HIPAA/GDPR**: Healthcare and EU compliance modes

### Mobile Applications

#### iOS & Android Apps
- **Create Invoices**: On-the-go invoice generation
- **Photo Receipts**: Capture expense receipts
- **Push Notifications**: Payment alerts
- **Offline Mode**: Create invoices without internet
- **Biometric Auth**: Face ID / fingerprint login

## Technical Specifications

### Database Schema

```sql
-- Core tables
tenants (id, name, subdomain, plan, settings, created_at)
users (id, tenant_id, email, name, role, avatar_url, created_at)
clients (
  id, tenant_id, name, email, phone, address, kra_pin,
  payment_terms, currency, created_at
)
invoices (
  id, tenant_id, client_id, number, currency, subtotal, tax_total, total,
  status, due_date, sent_at, paid_at, paid_amount, notes, terms, created_at
)
invoice_items (
  id, invoice_id, description, quantity, unit_price, total, created_at
)
payments (
  id, invoice_id, amount, method, mpesa_receipt, reference,
  status, created_at
)
templates (id, tenant_id, name, html_content, is_default, created_at)

### API Rate Limits

| Plan       | Requests/Minute | Burst     |
| ---------- | --------------- | --------- |
| Free       | 60              | 10        |
| Pro        | 300             | 50        |
| Agency     | 1000            | 100       |
| Enterprise | Unlimited       | Unlimited |


### File Storage
Invoices: Generated PDFs cached for 30 days
Receipts: Permanent storage (compliance requirement)
Logos: 2MB max, PNG/JPG/SVG
Attachments: 10MB max per file, 100MB per tenant

### Performance Targets

| Metric             | Target      | Measurement            |
| ------------------ | ----------- | ---------------------- |
| Invoice generation | <2 seconds  | PDF creation time      |
| STK Push delivery  | <10 seconds | Time to customer phone |
| Page load time     | <1 second   | Dashboard initial load |
| API response (p95) | <200ms      | REST API endpoints     |
| Concurrent users   | 500+        | Per instance           |
