#!/bin/bash

set -e

DB_PATH="${1:-./tunnelab.db}"

if [ ! -f "$DB_PATH" ]; then
    echo "Error: Database file not found at $DB_PATH"
    echo "Usage: $0 [database_path]"
    exit 1
fi

TOKEN=$(openssl rand -hex 32)

CLIENT_ID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "client-$(date +%s)")

CLIENT_NAME="${2:-default-client}"

echo "Generating new client token..."
echo ""
echo "Client ID: $CLIENT_ID"
echo "Client Name: $CLIENT_NAME"
echo "Token: $TOKEN"
echo ""

sqlite3 "$DB_PATH" <<EOF
INSERT INTO clients (id, name, api_token, max_tunnels, status)
VALUES ('$CLIENT_ID', '$CLIENT_NAME', '$TOKEN', 5, 'active');
EOF

echo "Client created successfully!"
echo ""
echo "Use this token with hooklab or any client that leverages TunneLab:"
echo "  token: $TOKEN"
echo ""
