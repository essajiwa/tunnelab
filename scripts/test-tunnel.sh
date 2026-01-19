#!/bin/bash

set -e

echo "TunneLab Tunnel Test Script"
echo "============================"
echo ""

# Check if server is running
if ! curl -s http://localhost/health > /dev/null 2>&1; then
    echo "❌ TunneLab server is not running!"
    echo "Start it with: make run"
    exit 1
fi

echo "✓ TunneLab server is running"
echo ""

# Check if we have a token
if [ ! -f "tunnelab.db" ]; then
    echo "❌ Database not found. Run the server first to initialize it."
    exit 1
fi

# Get or create a test token
TOKEN=$(sqlite3 tunnelab.db "SELECT api_token FROM clients LIMIT 1" 2>/dev/null || echo "")

if [ -z "$TOKEN" ]; then
    echo "No client token found. Generating one..."
    ./scripts/generate-token.sh tunnelab.db "test-client"
    TOKEN=$(sqlite3 tunnelab.db "SELECT api_token FROM clients WHERE name='test-client' LIMIT 1")
fi

echo "Using token: ${TOKEN:0:20}..."
echo ""

# Build test client
echo "Building test client..."
go build -o test-client ./cmd/test-client
echo "✓ Test client built"
echo ""

# Start a simple HTTP server on port 8000
echo "Starting local HTTP server on port 8000..."
(cd /tmp && python3 -m http.server 8000 > /dev/null 2>&1) &
HTTP_SERVER_PID=$!

# Wait for HTTP server to start
sleep 2

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."
    kill $HTTP_SERVER_PID 2>/dev/null || true
    kill $CLIENT_PID 2>/dev/null || true
    exit
}

trap cleanup EXIT INT TERM

# Start test client
echo "Starting tunnel client..."
./test-client -server ws://localhost:4443 -token "$TOKEN" -subdomain test -port 8000 &
CLIENT_PID=$!

# Wait for tunnel to establish
sleep 3

echo ""
echo "Testing tunnel..."
echo ""

# Test the tunnel
RESPONSE=$(curl -s http://test.localhost 2>&1 || echo "FAILED")

if echo "$RESPONSE" | grep -q "Directory listing"; then
    echo "✅ SUCCESS! Tunnel is working!"
    echo ""
    echo "You can access your local server at:"
    echo "  http://test.localhost"
    echo ""
    echo "Press Ctrl+C to stop the test"
    
    # Keep running
    wait $CLIENT_PID
else
    echo "❌ FAILED! Could not access tunnel"
    echo "Response: $RESPONSE"
    exit 1
fi
