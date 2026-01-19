#!/bin/bash

set -e

echo "TunneLab Server Setup"
echo "===================="
echo ""

if [ ! -f "configs/server.yaml" ]; then
    echo "Creating server configuration..."
    cp configs/server.example.yaml configs/server.yaml
    echo "✓ Configuration file created at configs/server.yaml"
    echo ""
    echo "Please edit configs/server.yaml and set your domain before starting the server."
else
    echo "✓ Configuration file already exists"
fi

echo ""
echo "Building server..."
go build -o tunnelab-server ./cmd/server
echo "✓ Server built successfully"

echo ""
echo "Initializing database..."
./tunnelab-server -config configs/server.yaml &
SERVER_PID=$!
sleep 2
kill $SERVER_PID 2>/dev/null || true
echo "✓ Database initialized"

echo ""
echo "Setup complete!"
echo ""
echo "Next steps:"
echo "1. Edit configs/server.yaml and set your domain"
echo "2. Configure DNS records (A and wildcard)"
echo "3. Generate a client token: ./scripts/generate-token.sh"
echo "4. Start the server: ./tunnelab-server -config configs/server.yaml"
echo ""
