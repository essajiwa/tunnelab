# TunneLab Quick Start Guide

This guide will help you get TunneLab server up and running in minutes.

## Prerequisites

- Go 1.21 or higher
- A server with a public IP address
- A domain name pointing to your server
- Ports 80, 443, and 4443 accessible

## Step 1: DNS Configuration

Configure your DNS with these records:

```
Type    Name                    Value
A       yourdomain.com          YOUR_PUBLIC_IP
A       control.yourdomain.com  YOUR_PUBLIC_IP
A       *.yourdomain.com        YOUR_PUBLIC_IP (wildcard)
```

**Note**: Replace `yourdomain.com` with your actual domain and `YOUR_PUBLIC_IP` with your server's IP.

## Step 2: Clone and Setup

```bash
# Clone the repository
git clone https://github.com/essajiwa/tunnelab.git
cd tunnelab

# Run setup script
make setup
```

This will:
- Create a configuration file at `configs/server.yaml`
- Build the server binary
- Initialize the database

## Step 3: Configure

Edit `configs/server.yaml`:

```yaml
server:
  domain: "yourdomain.com"  # Change this to your domain
  control_port: 4443
  http_port: 80
  https_port: 443

database:
  type: "sqlite"
  path: "./tunnelab.db"

auth:
  required: true

logging:
  level: "info"
  format: "text"
```

## Step 4: Generate Client Token

```bash
make generate-token
```

This will output something like:

```
Client ID: 550e8400-e29b-41d4-a716-446655440000
Client Name: default-client
Token: a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6

Client created successfully!

Use this token with the test client or any client that leverages TunneLab:
  token: a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6
```

**Save this token** - you'll need it for clients that leverage TunneLab.

## Step 5: Start the Server

```bash
make run
```

You should see:

```
Starting TunneLab server...
Starting control server on :4443
Starting HTTP proxy on :80
TunneLab Server dev started
Domain: yourdomain.com
Control: :4443
HTTP: :80
```

## Step 6: Test the Server

In another terminal:

```bash
# Check health endpoint
curl http://localhost/health

# Expected output:
# {"status":"healthy","tunnels":0}
```

## Step 7: Use with Test Client or Other Clients

Now you can use the test client or any client that leverages TunneLab to create tunnels:

```bash
# Build the test client
go build -o test-client ./cmd/test-client

# Start a tunnel
./test-client -server ws://control.yourdomain.com:4443 \
  -token a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6 \
  -subdomain myapp \
  -port 3000
```

This will expose your local server at `http://localhost:3000` to `http://myapp.yourdomain.com`.

## Verification

1. **Check DNS Resolution**:
   ```bash
   nslookup myapp.yourdomain.com
   # Should return your server's IP
   ```

2. **Test the Tunnel**:
   ```bash
   # Start a simple local server
   python3 -m http.server 3000
   
   # In another terminal, start test client tunnel
   ./test-client -server ws://localhost:4443 -token YOUR_TOKEN -subdomain test -port 3000
   
   # Access from anywhere
   curl http://test.yourdomain.com
   ```

## Troubleshooting

### Port Already in Use

```bash
# Check what's using the port
sudo lsof -i :80
sudo lsof -i :4443

# Kill the process or change ports in config
```

### Connection Refused

```bash
# Check if server is running
ps aux | grep tunnelab-server

# Check firewall
sudo ufw status
sudo ufw allow 80/tcp
sudo ufw allow 4443/tcp
```

### Subdomain Not Resolving

```bash
# Verify DNS propagation
dig myapp.yourdomain.com

# Check server logs
tail -f tunnelab.log
```

## Production Deployment

For production, consider:

1. **Use systemd** for automatic restart:
   ```bash
   sudo cp deployments/systemd/tunnelab.service /etc/systemd/system/
   sudo systemctl enable tunnelab
   sudo systemctl start tunnelab
   ```

2. **Enable HTTPS** with Let's Encrypt (fully supported)

3. **Use PostgreSQL** instead of SQLite for better performance

4. **Set up monitoring** with Prometheus/Grafana

## Next Steps

- Read the [full documentation](../README.md)
- Check out [API documentation](API_DOCUMENTATION.md)
- Configure [advanced features](USAGE.md)

## Support

- GitHub Issues: https://github.com/essajiwa/tunnelab/issues
- Documentation: https://github.com/essajiwa/tunnelab/docs
