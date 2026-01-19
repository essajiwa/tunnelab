# Let's Encrypt HTTPS Setup Guide

TunneLab now supports automatic HTTPS certificate generation using Let's Encrypt!

## Features

- ✅ Automatic certificate generation and renewal
- ✅ Wildcard certificate support for all subdomains
- ✅ Staging environment for testing
- ✅ Manual certificate support
- ✅ Secure TLS configuration (TLS 1.2+)

## Quick Setup

### 1. Update Configuration

Edit `configs/server.yaml`:

```yaml
server:
  domain: "tunnel.example.com"
  control_port: 4443
  http_port: 80
  https_port: 443

tls:
  mode: "auto"                          # Enable Let's Encrypt
  email: "admin@example.com"           # Your email for notifications
  cache_dir: "./certs"                  # Certificate cache directory
  staging: false                        # Use production (set true for testing)
```

### 2. DNS Requirements

Ensure your DNS is configured:

```
Type    Name                            Value
A       tunnel.example.com              YOUR_IP
A       control.tunnel.example.com      YOUR_IP
A       *.tunnel.example.com            YOUR_IP
```

### 3. Firewall Requirements

Open required ports:

```bash
# HTTP (required for ACME challenge)
sudo ufw allow 80/tcp

# HTTPS
sudo ufw allow 443/tcp

# Control server
sudo ufw allow 4443/tcp
```

### 4. Start Server

```bash
# Must run as root or with CAP_NET_BIND_SERVICE for port 80/443
sudo ./tunnelab-server -config configs/server.yaml
```

Expected output:
```
Let's Encrypt autocert enabled for domain: tunnel.example.com
Starting control server on :4443
Starting HTTP proxy on :80
Starting HTTPS proxy on :443 (Let's Encrypt)
TunneLab Server dev started
Domain: tunnel.example.com
Control: :4443
HTTP: :80
HTTPS: :443 (auto)
```

## How It Works

### Certificate Generation

1. **First Request**: When someone accesses `https://myapp.tunnel.example.com`
2. **ACME Challenge**: Let's Encrypt sends HTTP challenge to `http://tunnel.example.com/.well-known/acme-challenge/`
3. **Verification**: TunneLab responds with the challenge token
4. **Certificate Issued**: Let's Encrypt issues certificate
5. **Cached**: Certificate saved to `./certs/` directory
6. **Auto-Renewal**: Certificates automatically renewed before expiry

### Certificate Cache

Certificates are stored in the cache directory:

```
./certs/
├── acme_account+key
├── tunnel.example.com
├── myapp.tunnel.example.com
└── api.tunnel.example.com
```

## Testing with Staging

Before going to production, test with Let's Encrypt staging:

```yaml
tls:
  mode: "auto"
  email: "admin@example.com"
  staging: true  # Use staging environment
```

**Benefits of staging:**
- No rate limits
- Test your configuration
- Avoid hitting production rate limits (5 certs per week)

**Note**: Staging certificates will show as untrusted in browsers (this is expected).

## Production Deployment

### Option 1: Run as Root (Simple)

```bash
sudo ./tunnelab-server -config configs/server.yaml
```

### Option 2: Use Capabilities (Recommended)

```bash
# Give binary permission to bind to privileged ports
sudo setcap 'cap_net_bind_service=+ep' ./tunnelab-server

# Run as normal user
./tunnelab-server -config configs/server.yaml
```

### Option 3: Use Reverse Proxy

Run TunneLab on high ports, use nginx/Caddy for port 80/443:

```yaml
server:
  http_port: 8080
  https_port: 8443
```

Then configure nginx to proxy to TunneLab.

## Manual Certificates

If you have your own certificates:

```yaml
tls:
  mode: "manual"
  cert_path: "/etc/ssl/certs/tunnel.example.com.crt"
  key_path: "/etc/ssl/private/tunnel.example.com.key"
```

## Disable HTTPS

To run HTTP only:

```yaml
tls:
  mode: "disabled"
```

## Troubleshooting

### Certificate Generation Failed

**Error**: `acme/autocert: unable to satisfy http-01 challenge`

**Solutions**:
1. Ensure port 80 is accessible from internet
2. Check DNS is correctly configured
3. Verify no firewall blocking port 80
4. Check server logs for details

```bash
# Test ACME challenge endpoint
curl http://tunnel.example.com/.well-known/acme-challenge/test
```

### Rate Limit Exceeded

**Error**: `too many certificates already issued`

**Solution**: Use staging environment first:
```yaml
tls:
  staging: true
```

Let's Encrypt production limits:
- 50 certificates per registered domain per week
- 5 duplicate certificates per week

### Permission Denied on Port 80/443

**Error**: `bind: permission denied`

**Solutions**:
```bash
# Option 1: Run as root
sudo ./tunnelab-server -config configs/server.yaml

# Option 2: Add capabilities
sudo setcap 'cap_net_bind_service=+ep' ./tunnelab-server

# Option 3: Use high ports with reverse proxy
# Change http_port to 8080, https_port to 8443
```

### Certificate Not Trusted

If using staging mode, certificates won't be trusted (expected).

For production:
1. Ensure `staging: false`
2. Delete old staging certs: `rm -rf ./certs/*`
3. Restart server
4. Wait for new certificate generation

### Wildcard Certificate Issues

Let's Encrypt HTTP-01 challenge doesn't support wildcard certificates directly, but TunneLab requests individual certificates per subdomain automatically.

Each subdomain gets its own certificate:
- `tunnel.example.com`
- `myapp.tunnel.example.com`
- `api.tunnel.example.com`

This is handled automatically - no action needed.

## Security Best Practices

### 1. Secure Certificate Cache

```bash
chmod 700 ./certs
```

### 2. Use Strong Email

Use a monitored email for expiry notifications:
```yaml
tls:
  email: "ssl-admin@example.com"
```

### 3. Monitor Certificate Expiry

Certificates auto-renew, but monitor logs:
```bash
tail -f tunnelab.log | grep -i cert
```

### 4. Backup Certificates

```bash
# Backup cert cache
tar -czf certs-backup.tar.gz ./certs/
```

### 5. Use Production Mode

Only use staging for testing:
```yaml
tls:
  staging: false  # Production
```

## Advanced Configuration

### Custom Cache Directory

```yaml
tls:
  cache_dir: "/var/lib/tunnelab/certs"
```

### Multiple Domains

TunneLab supports one primary domain. For multiple domains, run multiple instances.

### Certificate Renewal

Certificates auto-renew when they have 30 days or less remaining. No manual intervention needed.

## Monitoring

### Check Certificate Status

```bash
# View certificate details
openssl s_client -connect myapp.tunnel.example.com:443 -servername myapp.tunnel.example.com < /dev/null 2>/dev/null | openssl x509 -noout -dates

# Expected output:
# notBefore=Jan 19 00:00:00 2026 GMT
# notAfter=Apr 19 23:59:59 2026 GMT
```

### Check Certificate Issuer

```bash
openssl s_client -connect myapp.tunnel.example.com:443 -servername myapp.tunnel.example.com < /dev/null 2>/dev/null | openssl x509 -noout -issuer

# Production: issuer=C = US, O = Let's Encrypt, CN = R3
# Staging: issuer=C = US, O = (STAGING) Let's Encrypt, CN = (STAGING) Artificial Apricot R3
```

## Client Configuration

Clients leveraging TunneLab should use HTTPS:

```bash
# Use wss:// for secure WebSocket
./test-client -server wss://control.tunnel.example.com:4443 \
  -token YOUR_TOKEN \
  -subdomain myapp \
  --port 3000

# Access via HTTPS
curl https://myapp.tunnel.example.com
```

## Performance Impact

Let's Encrypt adds minimal overhead:
- First request: +100-500ms (certificate generation)
- Subsequent requests: +1-5ms (TLS handshake)
- Certificate cached in memory after first use

## Migration from HTTP to HTTPS

1. Update config to enable TLS
2. Restart server
3. Update client connections to use `wss://`
4. Both HTTP and HTTPS work simultaneously
5. Optionally redirect HTTP to HTTPS (future feature)

## FAQ

**Q: Do I need to renew certificates manually?**  
A: No, automatic renewal happens in the background.

**Q: Can I use my own domain registrar?**  
A: Yes, just point DNS to your server IP.

**Q: Does this work with subdomains like tunnel.example.com?**  
A: Yes, any domain depth works.

**Q: What happens if Let's Encrypt is down?**  
A: Cached certificates continue to work. New subdomains may fail temporarily.

**Q: Can I force certificate regeneration?**  
A: Delete the cert from cache directory and restart server.

**Q: Is there a certificate limit?**  
A: Let's Encrypt allows 50 certs per domain per week (production).

## Support

For issues:
1. Check server logs
2. Verify DNS configuration
3. Test with staging first
4. Check Let's Encrypt status: https://letsencrypt.status.io/

---

**Let's Encrypt Documentation**: https://letsencrypt.org/docs/  
**Rate Limits**: https://letsencrypt.org/docs/rate-limits/
