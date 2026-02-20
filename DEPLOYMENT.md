# Deployment Guide

## Quick Start with Docker

### Prerequisites
- Docker
- Docker Compose

### Steps

1. **Clone and configure**
   ```bash
   cd InvoiceFast
   cp .env.example .env
   ```

2. **Edit .env** with your settings:
   - `JWT_SECRET` - Generate a secure random string
   - `INTASEND_*` - Your Intasend API keys
   - `CALLBACK_URL` - Your domain

3. **Start services**
   ```bash
   # Start with SQLite (development)
   docker-compose up -d
   
   # Or with PostgreSQL (production)
   docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
   ```

4. **Check status**
   ```bash
   docker-compose ps
   docker-compose logs -f invoicefast
   ```

5. **Access the app**
   - Open http://localhost:8082
   - Health check: http://localhost:8082/health

## Production Deployment

### 1. Domain & SSL

```bash
# Get SSL certificate (using Certbot)
certbot certonly --nginx -d yourdomain.com
```

### 2. Update nginx.conf

Uncomment the HTTPS server section and update SSL paths.

### 3. Use PostgreSQL

```bash
# Edit .env
DB_DRIVER=postgres
DB_DSN=host=postgres user=invoicefast password=YOUR_PASSWORD dbname=invoicefast port=5432 sslmode=disable
```

### 4. Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `JWT_SECRET` | Secret for JWT signing | Yes |
| `INTASEND_PUBLISHABLE_KEY` | Intasend publishable key | Yes (for payments) |
| `INTASEND_SECRET_KEY` | Intasend secret key | Yes (for payments) |
| `SMTP_HOST` | Email server | For emails |
| `CALLBACK_URL` | Public URL for webhooks | Yes (production) |

## Scaling

### Horizontal Scaling (Multiple Instances)

```yaml
# docker-compose.scale.yml
services:
  invoicefast:
    deploy:
      replicas: 3
    # ... rest of config
```

### Load Balancer

Use nginx or a cloud load balancer (AWS ALB, Cloudflare) in front.

## Monitoring

### Health Check
```
GET http://localhost:8082/health
```

Response:
```json
{"status": "ok", "time": "2024-01-01T00:00:00Z"}
```

### Logs
```bash
# View logs
docker-compose logs -f

# View specific container
docker-compose logs -f invoicefast
```

## Troubleshooting

### Common Issues

1. **Database locked**
   - Ensure only one instance writes to SQLite
   - Use PostgreSQL for multi-instance

2. **M-Pesa not working**
   - Check Intasend keys are correct
   - Verify callback URL is accessible

3. **Email not sending**
   - Check SMTP credentials
   - Use App Password for Gmail

4. **Slow performance**
   - Enable Redis caching
   - Use PostgreSQL
   - Check resource limits

## Backup & Restore

### Backup
```bash
# Backup SQLite
docker-compose exec invoicefast cp /app/data/invoicefast.db /app/data/backup.db

# Backup PostgreSQL
docker-compose exec postgres pg_dump -U invoicefast invoicefast > backup.sql
```

### Restore
```bash
# Restore SQLite
docker-compose exec -T invoicefast cp backup.db /app/data/invoicefast.db

# Restore PostgreSQL
cat backup.sql | docker-compose exec -T postgres psql -U invoicefast invoicefast
```

## Security Checklist

- [ ] Change JWT_SECRET to random string
- [ ] Enable HTTPS in production
- [ ] Configure CORS allowed origins
- [ ] Use PostgreSQL in production
- [ ] Set up regular backups
- [ ] Enable rate limiting
- [ ] Monitor logs regularly
- [ ] Keep Docker images updated
