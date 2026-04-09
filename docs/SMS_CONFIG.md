# SMS Configuration Guide

## Environment Variables

Add these to your `.env` file:

### Option 1: Africa's Talking (Recommended for Kenya)
```env
# SMS Configuration
SMS_ENABLED=true
SMS_PROVIDER=africastalking
SMS_API_KEY=your_api_key_here
SMS_SENDER_ID=INVOICEFAST
```

### Option 2: Twilio
```env
# SMS Configuration
SMS_ENABLED=true
SMS_PROVIDER=twilio
SMS_API_KEY=your_account_sid
SMS_API_SECRET=your_auth_token
SMS_SENDER_ID=+1234567890
```

### Option 3: Bulk SMS API (Generic)
```env
# SMS Configuration
SMS_ENABLED=true
SMS_PROVIDER=bulk
SMS_API_KEY=your_api_key
SMS_SENDER_ID=INVOICEFAST
SMS_ENDPOINT=https://api.bulksms.com/v1/send
```

## Configuration Fields

| Field | Description | Required |
|-------|-------------|----------|
| `SMS_ENABLED` | Enable/disable SMS service | Yes |
| `SMS_PROVIDER` | Provider: `africastalking`, `twilio`, or `bulk` | Yes |
| `SMS_API_KEY` | API key from provider | Yes |
| `SMS_API_SECRET` | API secret (Twilio only) | For Twilio |
| `SMS_SENDER_ID` | Sender ID (max 11 chars) | Yes |
| `SMS_ENDPOINT` | Custom API endpoint (bulk only) | For bulk provider |

## Supported Providers

### Africa's Talking
- Best for Kenya
- Competitive pricing
- Supports Kenyan networks
- Get API key: https://account.africastalking.com

### Twilio
- Global provider
- Higher cost
- More reliable
- Get credentials: https://console.twilio.com

### Bulk SMS (Generic)
- Use any bulk SMS provider
- Configure custom endpoint
- Requires API key authentication

## Testing

After configuring, test SMS with:
```bash
# The system will log to stdout when SMS is disabled
# Check logs for: [SMS MOCK] To: ..., Message: ...
```

## Cost Optimization

1. **Use Africa's Talking** for Kenya-focused deployment
2. **Set up alerts** only for critical events (payment received, overdue)
3. **Disable SMS** in development: `SMS_ENABLED=false`
4. **Monitor usage** via provider dashboard