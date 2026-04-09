# InvoiceFast Monitoring & Alerting Guide

## Overview

InvoiceFast supports external monitoring through health check endpoints and structured logging.

---

## Health Check Endpoints

| Endpoint | Description | Auth |
|----------|-------------|------|
| `GET /health` | Basic health check (no auth) | Public |
| `GET /health/ready` | Readiness probe (includes DB check) | Public |
| `GET /health/live` | Liveness probe | Public |
| `GET /metrics` | Prometheus metrics | Public |

### Response Examples

```bash
# Basic health
curl https://api.invoicefast.co/health
# {"status":"ok","timestamp":"2026-04-09T12:00:00Z"}

# Readiness (includes dependencies)
curl https://api.invoicefast.co/health/ready
# {"status":"ready","database":"connected","redis":"connected"}
```

---

## Prometheus Metrics

Enable metrics collection:
```env
ENABLE_METRICS=true
METRICS_PORT=9090
```

### Available Metrics

```prometheus
# HTTP Server
http_requests_total{method="POST",status="200",path="/api/v1/tenant/invoices"}
http_request_duration_seconds{method="GET",path="/api/v1/tenant/clients"}

# Database
db_query_duration_seconds{operation="select",table="invoices"}
db_connections_active{state="active"}

# Business Metrics
invoice_created_total{tenant_id="xxx",currency="KES"}
payment_processed_total{status="completed",method="mpesa"}

# Auth
auth_login_total{status="success"}
auth_token_refresh_total{status="success"}
auth_failed_login_total{reason="invalid_credentials"}
```

---

## Recommended External Monitoring Stack

### 1. Uptime Monitoring (Uptrends/Downtime.robot)

```yaml
# Check every 5 minutes
GET https://api.invoicefast.co/health
Expected: 200 OK with {"status":"ok"}
```

### 2. Application Performance (Grafana Cloud / Self-hosted)

#### Prometheus Configuration
```yaml
scrape_configs:
  - job_name: 'invoicefast'
    static_configs:
      - targets: ['invoicefast:9090']
    metrics_path: '/metrics'
```

#### Grafana Dashboard
Import `dashboards/invoicefast-dashboard.json` for:
- Request rate (RPM)
- Response time (p50, p95, p99)
- Error rate by endpoint
- Database connection pool
- Payment success rate

### 3. Log Aggregation (Loki / ELK)

#### Structured Log Format
```
level=info ts=2026-04-09T12:00:00Z component=payment_service 
msg="Payment processed" invoice_id=xxx amount=5000 
payment_id=yyy mpesa_receipt=ABC123
```

#### Recommended Log Fields
- `timestamp` - ISO8601
- `level` - debug, info, warn, error
- `component` - service name
- `msg` - human readable message
- `tenant_id` - for multi-tenant filtering
- `request_id` - correlation ID

### 4. Alerting (Alertmanager / PagerDuty)

#### Critical Alerts

| Alert | Condition | Severity | Action |
|-------|-----------|----------|--------|
| High Error Rate | error_rate > 5% for 5min | Critical | Page on-call |
| Payment Failure | payment_failed > 10 for 5min | Critical | Page payments team |
| DB Connection Pool | active_connections > 20 for 5min | Warning | Create ticket |
| High Latency | p99_latency > 2s for 10min | Warning | Create ticket |
| KRA Submission | kra_failed > 5 for 10min | Warning | Create ticket |

#### Alertmanager Config
```yaml
route:
  group_by: ['alertname']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  receiver: 'pagerduty'

receivers:
  - name: 'pagerduty'
    pagerduty_configs:
      - service_key: 'YOUR_SERVICE_KEY'
        severity: 'critical'
```

---

## Environment-Specific Configuration

### Development
```env
# Minimal monitoring
ENABLE_METRICS=false
LOG_LEVEL=debug
```

### Staging
```env
# Basic monitoring
ENABLE_METRICS=true
LOG_LEVEL=info
ALERT_WEBHOOK_URL=https://hooks.slack.com/services/xxx
```

### Production
```env
# Full monitoring
ENABLE_METRICS=true
METRICS_PORT=9090
LOG_LEVEL=info
ALERT_WEBHOOK_URL=https://pagerduty.com/xxx
SLO_TARGET=99.9
```

---

## SLO (Service Level Objectives)

| Metric | Target | Critical Threshold |
|--------|--------|-------------------|
| Availability | 99.9% | 99% |
| Latency p99 | < 500ms | > 1s |
| Error Rate | < 0.1% | > 1% |
| Payment Success | > 99.5% | < 98% |

---

## Runbook: Payment Failures

1. **Check Intasend Dashboard**
   - Verify API keys not expired
   - Check for service outages

2. **Check Database**
   - Verify pending payments
   - Check for deadlocks

3. **Check M-Pesa Status**
   - Visit Safaricom status page

4. **Common Issues**
   - `TIMEOUT`: STK push timed out - retry
   - `INVALID_PHONE`: Client phone format wrong
   - `INSUFFICIENT_FUNDS`: Client M-Pesa balance low

---

## Log Aggregation Query Examples

### Find all payment errors last 24h
```logql
{job="invoicefast"} | json | component="payment_service" | level="error"
```

### Payment success rate by hour
```logql
sum(rate(payment_processed_total{status="completed"}[1h])) by (hour) 
/ 
sum(rate(payment_processed_total[1h])) by (hour)
```

### Tenant-specific errors
```logql
{job="invoicefast"} | json | tenant_id="tenant-123" | level="error"
```