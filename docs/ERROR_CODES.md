# InvoiceFast API Error Codes

This document outlines all API error codes returned by the InvoiceFast backend.

## Format

All errors return JSON in the following format:
```json
{
  "error": "human readable message",
  "code": "ERROR_CODE"
}
```

---

## Authentication Errors (4xx)

| Code | HTTP Status | Description | Resolution |
|------|--------------|-------------|------------|
| `AUTH_INVALID_CREDENTIALS` | 401 | Invalid email or password | Check credentials and try again |
| `AUTH_TOKEN_EXPIRED` | 401 | JWT token has expired | Refresh the access token |
| `AUTH_TOKEN_INVALID` | 401 | Malformed or tampered token | Re-authenticate |
| `AUTH_FORBIDDEN` | 403 | Access denied to resource | Check user permissions |
| `AUTH_MISSING_TOKEN` | 401 | No authorization header | Include Bearer token |
| `AUTH_RATE_LIMITED` | 429 | Too many auth attempts | Wait 5 minutes before retry |

---

## Tenant Errors (4xx)

| Code | HTTP Status | Description | Resolution |
|------|--------------|-------------|------------|
| `TENANT_REQUIRED` | 403 | Tenant ID is required | Ensure tenant context is set |
| `TENANT_NOT_FOUND` | 404 | Tenant not found | Verify tenant ID |
| `TENANT_INACTIVE` | 403 | Tenant account is suspended | Contact support |

---

## Invoice Errors (4xx)

| Code | HTTP Status | Description | Resolution |
|------|--------------|-------------|------------|
| `INVOICE_NOT_FOUND` | 404 | Invoice not found | Verify invoice ID |
| `INVOICE_EMPTY_ITEMS` | 400 | Invoice must have at least one item | Add line items |
| `INVOICE_INVALID_QUANTITY` | 400 | Item quantity cannot be negative | Use positive numbers |
| `INVOICE_CANT_EDIT_PAID` | 400 | Cannot edit a paid invoice | Contact support |
| `INVOICE_CANT_CANCEL_PAID` | 400 | Cannot cancel a paid invoice | Refund first |
| `INVOICE_CANT_SEND_DRAFT` | 400 | Cannot send a draft invoice | Review and send |
| `INVOICE_ALREADY_SENT` | 400 | Invoice already sent | View sent invoices |
| `INVOICE_ALREADY_PAID` | 400 | Invoice already fully paid | No further action |
| `INVOICE_OVERDUE_AMOUNT` | 400 | Payment exceeds invoice amount | Check payment amount |

---

## Client Errors (4xx)

| Code | HTTP Status | Description | Resolution |
|------|--------------|-------------|------------|
| `CLIENT_NOT_FOUND` | 404 | Client not found | Verify client ID |
| `CLIENT_NAME_REQUIRED` | 400 | Client name is required | Provide a name |
| `CLIENT_HAS_INVOICES` | 400 | Cannot delete client with invoices | Delete invoices first |

---

## Payment Errors (4xx)

| Code | HTTP Status | Description | Resolution |
|------|--------------|-------------|------------|
| `PAYMENT_NOT_FOUND` | 404 | Payment not found | Verify payment ID |
| `PAYMENT_FAILED` | 400 | Payment processing failed | Try again or use different method |
| `PAYMENT_DUPLICATE` | 409 | Duplicate payment detected | Payment already processed |
| `PAYMENT_PENDING` | 202 | Payment is being processed | Wait for confirmation |

---

## M-Pesa / STK Push Errors (4xx)

| Code | HTTP Status | Description | Resolution |
|------|--------------|-------------|------------|
| `MPESA_INVALID_PHONE` | 400 | Invalid M-Pesa phone number | Use format: 07XX XXX XXX |
| `MPESA_NOT_CONFIGURED` | 503 | M-Pesa service not available | Contact support |
| `MPESA_TIMEOUT` | 504 | STK push timed out | Retry payment |
| `MPESA_CANCELLED` | 400 | Payment cancelled by user | Initiate new payment |

---

## KRA e-TIMS Errors (4xx/5xx)

| Code | HTTP Status | Description | Resolution |
|------|--------------|-------------|------------|
| `KRA_MOCK_MODE` | 200 | Running in sandbox mode | Not production - for testing |
| `KRA_INVALID_PIN` | 400 | Invalid KRA PIN format | Verify PIN format |
| `KRA_SUBMISSION_FAILED` | 500 | Failed to submit to KRA | Retry or contact support |
| `KRA_QUEUE_RETRY` | 202 | Queued for retry | Will be processed automatically |

---

## WhatsApp Errors (4xx)

| Code | HTTP Status | Description | Resolution |
|------|--------------|-------------|------------|
| `WHATSAPP_NOT_CONFIGURED` | 200 | WhatsApp not configured | Enable in settings |
| `WHATSAPP_INVALID_PHONE` | 400 | Invalid WhatsApp phone | Use international format |
| `WHATSAPP_SEND_FAILED` | 500 | Failed to send message | Retry or use email |

---

## SMS Errors (4xx)

| Code | HTTP Status | Description | Resolution |
|------|--------------|-------------|------------|
| `SMS_NOT_CONFIGURED` | 200 | SMS service not configured | Enable in settings |
| `SMS_INVALID_PHONE` | 400 | Invalid phone number | Use international format |
| `SMS_SEND_FAILED` | 500 | Failed to send SMS | Retry |

---

## Rate Limiting Headers

When rate limited, the following headers are included:
- `X-RateLimit-Limit`: Maximum requests allowed
- `X-RateLimit-Remaining`: Requests remaining
- `X-RateLimit-Reset`: Unix timestamp when limit resets

Response:
```json
{
  "error": "Too many requests",
  "code": "RATE_LIMITED"
}
```

---

## Webhook Errors

| Code | Description |
|------|-------------|
| `WEBHOOK_INVALID_SIGNATURE` | Intasend signature verification failed |
| `WEBHOOK_UNKNOWN_EVENT` | Unrecognized webhook event |
| `WEBHOOK_IDEMPOTENT` | Duplicate webhook - already processed |