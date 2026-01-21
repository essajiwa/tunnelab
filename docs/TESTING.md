# Testing TunneLab

This guide shows how to test TunneLab server with the included test client.

## Quick Test

### 1. Start the Server

```bash
# Terminal 1: Start TunneLab server
make run
```

### 2. Generate a Test Token

```bash
# Terminal 2: Generate client token
make generate-token
# Save the token output
```

### 3. Run Test Client

```bash
# Build test client
go build -o test-client ./cmd/test-client

# Start a local HTTP server
python3 -m http.server 8000

# In another terminal, start test client
./test-client \
  -server ws://localhost:4443 \
  -token YOUR_TOKEN_HERE \
  -subdomain test \
  -port 8000 \
  -protocol http
```

Expected output:
```
Connecting to ws://localhost:4443
Authenticating...
✓ Authenticated successfully
Requesting tunnel for subdomain: test
✓ Tunnel created!
  Tunnel ID: 550e8400-e29b-41d4-a716-446655440000
  Public URL: http://test.localhost
  Forwarding to: localhost:8000
Establishing yamux connection to :12345
✓ Yamux session established

🎉 Tunnel is ready! Access your local server at: http://test.localhost
Press Ctrl+C to stop
```

### 4. Test the Tunnel

```bash
# In another terminal
curl http://test.localhost

# Or open in browser
# http://test.localhost
```

## Testing TCP & gRPC Tunnels

To test raw TCP forwarding (Redis, SSH, gRPC, etc.):

```bash
# Start a local TCP service, e.g., Redis on localhost:6379

# Run the test client with tcp protocol
./test-client \
  -server ws://localhost:4443 \
  -token YOUR_TOKEN_HERE \
  -subdomain tcp-demo \
  -port 6379 \
  -local-host 127.0.0.1 \
  -protocol tcp

# Note the public_port in the output, then connect with
redis-cli -h yourdomain.com -p PUBLIC_PORT
```

For gRPC (still raw TCP forwarding), use the same `-protocol grpc` flag and target the assigned public port from your gRPC client or the `examples/grpc-smoke-test.sh` helper.

## Automated Test Script

Use the provided test script:

```bash
chmod +x scripts/test-tunnel.sh
./scripts/test-tunnel.sh
```

This script will:
1. Check if server is running
2. Generate/use a test token
3. Build the test client
4. Start a local HTTP server
5. Create an HTTP tunnel
6. Test the connection
7. Report success/failure

For TCP/gRPC smoke tests, use:

```bash
./examples/tcp-smoke-test.sh yourdomain.com PUBLIC_PORT
./examples/grpc-smoke-test.sh yourdomain.com:PUBLIC_PORT helloworld.Greeter/SayHello '{"name":"Tunnel"}'
```

## Manual Testing with wscat

### 1. Install wscat

```bash
npm install -g wscat
```

### 2. Test WebSocket Connection

```bash
# Connect to control server
wscat -c ws://localhost:4443

# Send auth message
{"type":"auth","request_id":"test-1","payload":{"token":"YOUR_TOKEN"},"timestamp":1234567890}

# Expected response
{"type":"auth_response","request_id":"test-1","payload":{"success":true,"client_id":"..."},"timestamp":...}

# Send HTTP tunnel request
{"type":"tunnel_request","request_id":"test-2","payload":{"subdomain":"test","protocol":"http","local_port":8000},"timestamp":1234567890}

# Expected response
{"type":"tunnel_response","request_id":"test-2","payload":{"tunnel_id":"...","public_url":"http://test.localhost","status":"active"},"timestamp":...}

# Send TCP tunnel request
{"type":"tcp_request","request_id":"test-3","payload":{"subdomain":"tcp-demo","protocol":"tcp","local_port":6379},"timestamp":1234567890}

# Expected response
{"type":"tcp_response","request_id":"test-3","payload":{"tunnel_id":"...","public_port":30501,"status":"active"},"timestamp":...}
```

## Testing Different Scenarios

### Test 1: Basic HTTP Forwarding

```bash
# Start local server
python3 -m http.server 3000

# Start tunnel
./test-client -server ws://localhost:4443 -token TOKEN -subdomain myapp -port 3000

# Test
curl http://myapp.localhost
```

### Test 2: Multiple Tunnels

```bash
# Terminal 1: First tunnel
./test-client -server ws://localhost:4443 -token TOKEN -subdomain app1 -port 3000

# Terminal 2: Second tunnel
./test-client -server ws://localhost:4443 -token TOKEN -subdomain app2 -port 4000

# Test both
curl http://app1.localhost
curl http://app2.localhost
```

### Test 3: WebSocket Support

```bash
# Start a WebSocket echo server
npm install -g wscat
wscat -l 8080

# Create tunnel
./test-client -server ws://localhost:4443 -token TOKEN -subdomain ws -port 8080

# Test WebSocket through tunnel
wscat -c ws://ws.localhost
```

### Test 4: Large File Transfer

```bash
# Create a large file
dd if=/dev/zero of=/tmp/testfile bs=1M count=100

# Start HTTP server
cd /tmp && python3 -m http.server 8000

# Create tunnel
./test-client -server ws://localhost:4443 -token TOKEN -subdomain files -port 8000

# Download through tunnel
curl http://files.localhost/testfile -o downloaded.file

# Verify
ls -lh downloaded.file
```

### Test 5: Connection Persistence

### Test 6: TCP Service Forwarding (Redis example)

```bash
# Start Redis locally
redis-server --port 6379

# Start TCP tunnel
./test-client -server ws://localhost:4443 -token TOKEN -subdomain redis -port 6379 -protocol tcp

# Connect through public port
redis-cli -h yourdomain.com -p PUBLIC_PORT
```

### Test 7: gRPC via raw TCP

```bash
# Start your gRPC server locally (listening on :50051)

# Start tunnel
./test-client -server ws://localhost:4443 -token TOKEN -subdomain grpc -port 50051 -protocol grpc

# Invoke using grpcurl
grpcurl -plaintext yourdomain.com:PUBLIC_PORT your.service/Method -d '{"name":"Tunnel"}'
```

```bash
# Start tunnel
./test-client -server ws://localhost:4443 -token TOKEN -subdomain persist -port 8000

# Make multiple requests
for i in {1..10}; do
  curl http://persist.localhost
  sleep 1
done

# Check server logs for connection reuse
```

## Debugging

### Enable Verbose Logging

Modify test client to add debug output:

```go
// In cmd/test-client/main.go
log.SetFlags(log.LstdFlags | log.Lshortfile)
```

### Check Server Logs

```bash
# Watch server logs
tail -f tunnelab.log

# Or if running in terminal, check output
```

### Verify Tunnel Registration

```bash
# Check database
sqlite3 tunnelab.db "SELECT subdomain, public_url, status FROM tunnels;"

# Check health endpoint
curl http://localhost/health
```

### Test DNS Resolution

```bash
# If using real domain
nslookg test.tunnel.example.com

# Should return your server IP
```

## Common Issues

### "Connection refused"

**Problem**: Can't connect to control server

**Solution**:
```bash
# Check if server is running
curl http://localhost/health

# Check port
netstat -tlnp | grep 4443
```

### "Auth failed"

**Problem**: Invalid token

**Solution**:
```bash
# Verify token in database
sqlite3 tunnelab.db "SELECT * FROM clients;"

# Generate new token
./scripts/generate-token.sh
```

### "Subdomain taken"

**Problem**: Subdomain already in use

**Solution**:
```bash
# Check active tunnels
sqlite3 tunnelab.db "SELECT * FROM tunnels WHERE status='active';"

# Use different subdomain
./test-client -subdomain myapp2 ...
```

### "Tunnel not found"

**Problem**: Request to subdomain returns 404

**Solution**:
```bash
# Verify tunnel is registered
curl http://localhost/health

# Check server logs
# Ensure test client is still running
```

## Performance Testing

### Load Test with Apache Bench

```bash
# Start tunnel
./test-client -server ws://localhost:4443 -token TOKEN -subdomain load -port 8000

# Run load test
ab -n 1000 -c 10 http://load.localhost/

# Check results
```

### Concurrent Connections

```bash
# Start tunnel
./test-client -server ws://localhost:4443 -token TOKEN -subdomain concurrent -port 8000

# Test concurrent requests
seq 1 100 | xargs -P 10 -I {} curl -s http://concurrent.localhost > /dev/null

# Monitor server
watch -n 1 'curl -s http://localhost/health'
```

## Next Steps

Once basic testing works:

1. Test with real domain (not localhost)
2. Test HTTPS with Let's Encrypt
3. Test from external network
4. Implement additional clients that leverage TunneLab
5. Add more advanced features

## Test Checklist

- [ ] Server starts successfully (control, HTTP, TCP listeners up)
- [ ] Health check responds
- [ ] Client can authenticate
- [ ] HTTP tunnel can be created
- [ ] TCP/gRPC tunnel can be created (public port assigned)
- [ ] HTTP requests are forwarded
- [ ] TCP traffic (e.g., Redis) flows through assigned port
- [ ] Multiple tunnels work simultaneously
- [ ] WebSocket upgrade works
- [ ] Large files transfer correctly
- [ ] Connections are reused / client reconnects on drop
- [ ] HTTPS works (if enabled)
- [ ] External access works (if using real domain)
