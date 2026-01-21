# TunneLab Usage Guide

## Server Setup

### 1. Build the Server

```bash
# Using Makefile
make build

# Or manually
go build -o tunnelab-server ./cmd/server
```

### 2. Create Configuration

```bash
# Copy example config
cp configs/server.example.yaml configs/server.yaml

# Edit the configuration
nano configs/server.yaml
```

Update the domain and tunnel range:
```yaml
server:
  domain: "example.com"  # Change to your domain
  control_port: 4443
  http_port: 80

tunnels:
  tcp_port_range: "30000-31000"  # Forward this range through your firewall/router
```

### 3. Start the Server

```bash
# Using Makefile
make run

# Or manually
./tunnelab-server -config configs/server.yaml
```

Expected output:
```
Starting control server on :4443
Starting HTTP proxy on :80
TCP tunneling enabled on ports 30000-31000
TunneLab Server dev started
Domain: example.com
Control: :4443
HTTP: :80
TCP: 30000-31000
```

## Client Token Management

### Generate a New Token

```bash
# Using Makefile
make generate-token

# Or manually
./scripts/generate-token.sh ./tunnelab.db
```

Output:
```
Client ID: 550e8400-e29b-41d4-a716-446655440000
Client Name: default-client
Token: a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6

Use this token with any client that leverages TunneLab:
  token: a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6
```

### Generate Token with Custom Name

```bash
./scripts/generate-token.sh ./tunnelab.db "my-project"
```

### Basic Tunnel

With any client leveraging TunneLab:

```bash
./test-client -server ws://control.example.com:4443 \
  --token YOUR_TOKEN_HERE \
  --subdomain myapp \
  --port 3000 \
  --protocol http \
  --local-host localhost
```

Key flags:

- `--protocol` – choose `http` (default), `tcp`, or `grpc`
- `--local-host` – override the host the client connects to (useful for Docker or remote targets)

Example TCP tunnel (raw port-forward):

```bash
./test-client -server ws://localhost:4443 \
  --token YOUR_TOKEN \
  --subdomain tcp-echo \
  --port 9000 \
  --local-host 127.0.0.1 \
  --protocol tcp
```

The server responds with a `public_port` in the configured TCP port range. Point external clients to `yourdomain.com:PUBLIC_PORT`.

Example gRPC tunnel (raw TCP forwarding for gRPC services):

```bash
./test-client -server ws://localhost:4443 \
  --token YOUR_TOKEN \
  --subdomain grpc-demo \
  --port 50051 \
  --protocol grpc
```

Because gRPC rides over HTTP/2, your gRPC clients simply target `yourdomain.com:PUBLIC_PORT` using TLS/plaintext consistent with your backend.

#### TCP/gRPC smoke-test scripts

Use the helper scripts under `examples/` for quick end-to-end checks once you know the assigned public port:

- `examples/tcp-smoke-test.sh <host> <port> [message]` – pushes a payload over raw TCP using `nc` and prints the response
- `examples/grpc-smoke-test.sh <host:port> <service/method> [jsonData]` – wraps `grpcurl` invocations (set `USE_TLS=1` to enable TLS verification)

### Multiple Tunnels

```bash
# Terminal 1: Web app
./test-client -server ws://localhost:4443 -token TOKEN1 -subdomain webapp -port 3000

# Terminal 2: API server
./test-client -server ws://localhost:4443 -token TOKEN2 -subdomain api -port 8080

# Terminal 3: Database admin
./test-client -server ws://localhost:4443 -token TOKEN3 -subdomain admin -port 5432
```

## Testing the Server

### Health Check

```bash
curl http://localhost/health
```

Expected response:
```json
{"status":"healthy","tunnels":0}
```

### Test with Local Server

1. Start a simple HTTP server:
```bash
# Python
python3 -m http.server 3000

# Node.js
npx http-server -p 3000

# Go
cd /tmp && echo "Hello from TunneLab!" > index.html && python3 -m http.server 3000
```

2. Start test client tunnel (in another terminal):
```bash
./test-client -server ws://localhost:4443 -token YOUR_TOKEN -subdomain test -port 3000 -protocol http
```

3. Access from anywhere:
```bash
curl http://test.example.com
```

## Protocol Flow

### 1. Client Authentication

```
Client -> Server: WebSocket connection to ws://control.example.com:4443
Client -> Server: Auth message with token
Server -> Client: Auth response (success/failure)
```

### 2. Tunnel Creation

```
Client -> Server: Tunnel request (subdomain, protocol, local_host, local_port)
Server -> Client: Tunnel response (HTTP: public_url, TCP/gRPC: public_port)
Server -> Client: Mux establishment request
Client -> Server: TCP connection for yamux session
```

### 3. Request Proxying

```
Public User -> Server: HTTP request to myapp.example.com
Server: Lookup tunnel for "myapp" subdomain
Server -> Client: Open yamux stream
Server -> Client: Forward HTTP request through stream
Client -> Local Server: Proxy request to localhost:3000
Local Server -> Client: HTTP response
Client -> Server: Forward response through stream
Server -> Public User: Return HTTP response
```

## Configuration Options

### Server Configuration

```yaml
server:
  domain: "example.com"       # Your domain
  control_port: 4443          # WebSocket control port
  http_port: 80               # HTTP proxy port
  https_port: 443             # HTTPS proxy port (future)

database:
  type: "sqlite"              # Database type
  path: "./tunnelab.db"       # Database file path

auth:
  required: true              # Require authentication
  token_length: 32            # Token length in bytes

logging:
  level: "info"               # debug, info, warn, error
  format: "text"              # text or json
  output: "stdout"            # stdout or file path

tunnels:
  max_tunnels_per_client: 5   # Max tunnels per client
  max_connections_per_tunnel: 100  # Max concurrent connections
```

## Monitoring

### View Active Tunnels

Query the database:
```bash
sqlite3 tunnelab.db "SELECT subdomain, public_url, status FROM tunnels WHERE status='active';"
```

### View Logs

```bash
# If running in foreground
# Logs appear in stdout

# If running as systemd service
sudo journalctl -u tunnelab -f
```

### Metrics Endpoint (Future)

```bash
curl http://localhost/metrics
```

## Troubleshooting

### Port Already in Use

```bash
# Find process using port 80
sudo lsof -i :80

# Kill the process
sudo kill -9 PID

# Or change port in config
```

### Database Locked

```bash
# Check for other processes
lsof tunnelab.db

# Remove lock files
rm -f tunnelab.db-shm tunnelab.db-wal
```

### Tunnel Not Found

```bash
# Check if tunnel exists in database
sqlite3 tunnelab.db "SELECT * FROM tunnels WHERE subdomain='myapp';"

# Check if tunnel is registered in memory
# (restart server to reload from database)
```

### Client Can't Connect

```bash
# Test WebSocket endpoint
wscat -c ws://control.example.com:4443

# Check firewall
sudo ufw status
sudo ufw allow 4443/tcp

# Check if server is listening
netstat -tlnp | grep 4443
```

## Production Deployment

### Using systemd

1. Create service file:
```bash
sudo nano /etc/systemd/system/tunnelab.service
```

```ini
[Unit]
Description=TunneLab Server
After=network.target

[Service]
Type=simple
User=tunnelab
Group=tunnelab
WorkingDirectory=/opt/tunnelab
ExecStart=/opt/tunnelab/tunnelab-server -config /etc/tunnelab/server.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

2. Enable and start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable tunnelab
sudo systemctl start tunnelab
sudo systemctl status tunnelab
```

### Using Docker

```bash
# Build image
docker build -t tunnelab-server .

# Run container
docker run -d \
  --name tunnelab \
  -p 80:80 \
  -p 4443:4443 \
  -v $(pwd)/configs:/etc/tunnelab \
  -v $(pwd)/data:/var/lib/tunnelab \
  tunnelab-server
```

## Security Best Practices

1. **Use HTTPS**: Enable TLS for production (fully supported)
2. **Rotate Tokens**: Regularly generate new client tokens
3. **Limit Tunnels**: Set appropriate `max_tunnels_per_client`
4. **Monitor Usage**: Track connection logs for abuse
5. **Firewall**: Only expose necessary ports
6. **Database Backups**: Regularly backup `tunnelab.db`

## Advanced Usage

### Custom Port for Control Server

```yaml
server:
  control_port: 8443  # Use custom port
```

Then connect with:
```bash
./test-client -server ws://control.example.com:8443 -token YOUR_TOKEN
```

### Multiple Clients

Generate separate tokens for different users/projects:

```bash
./scripts/generate-token.sh ./tunnelab.db "project-a"
./scripts/generate-token.sh ./tunnelab.db "project-b"
./scripts/generate-token.sh ./tunnelab.db "team-frontend"
```

### Subdomain Restrictions

Edit database to restrict subdomains:

```bash
sqlite3 tunnelab.db
```

```sql
UPDATE clients 
SET allowed_subdomains = '["app1", "app2", "api"]' 
WHERE name = 'project-a';
```

## API Reference

### Health Check

```
GET /health
```

Response:
```json
{
  "status": "healthy",
  "tunnels": 3
}
```

### WebSocket Control Protocol

See [API_DOCUMENTATION.md](docs/API_DOCUMENTATION.md) for complete protocol specification.

## Performance Tips

1. **Use SQLite WAL mode** for better concurrency:
   ```bash
   sqlite3 tunnelab.db "PRAGMA journal_mode=WAL;"
   ```

2. **Increase file descriptors**:
   ```bash
   ulimit -n 65536
   ```

3. **Use connection pooling** in clients leveraging TunneLab

4. **Monitor memory usage**:
   ```bash
   ps aux | grep tunnelab-server
   ```

## Next Steps

- Read [API_DOCUMENTATION.md](API_DOCUMENTATION.md) for client implementation
- Check [TECHNICAL_DESIGN.md](TECHNICAL_DESIGN.md) for architecture details
- See [QUICKSTART.md](QUICKSTART.md) for quick setup guide
