#!/bin/bash
# TunneLab Server — GCP Compute Engine startup script
# Terraform variables substituted at plan time:
#   domain         = ${domain}
#   tls_email      = ${tls_email}
#   tls_staging    = ${tls_staging}
#   repo_url       = ${repo_url}
#   tcp_port_range = ${tcp_port_range}

set -euo pipefail

LOGFILE="/var/log/tunnelab-startup.log"
touch "$LOGFILE"

# Redirect all output to logfile. GCP serial console captures independently.
exec >> "$LOGFILE" 2>&1

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

log "=========================================="
log "TunneLab startup script starting"
log "=========================================="

# ── Idempotency guard ─────────────────────────────────────────────────────────
INSTALL_MARKER="/var/lib/tunnelab/.setup_complete"
if [ -f "$INSTALL_MARKER" ]; then
  log "Setup already complete. Starting service."
  systemctl start tunnelab 2>/dev/null || true
  exit 0
fi

# ── 1. System packages ────────────────────────────────────────────────────────
log "[1/9] Installing system packages..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq \
  gcc \
  libsqlite3-dev \
  sqlite3 \
  libcap2-bin \
  git \
  curl \
  wget \
  ca-certificates \
  uuid-runtime
log "System packages installed."

# ── 2. Kernel tuning ──────────────────────────────────────────────────────────
log "[2/9] Tuning kernel parameters..."
sysctl -w net.ipv4.ip_local_port_range="49152 65535"
echo "net.ipv4.ip_local_port_range = 49152 65535" > /etc/sysctl.d/99-tunnelab.conf
echo "tunnelab soft nofile 65536" >> /etc/security/limits.conf
echo "tunnelab hard nofile 65536" >> /etc/security/limits.conf
log "Kernel tuning done."

# ── 3. Install Go ─────────────────────────────────────────────────────────────
log "[3/9] Installing Go..."

# Try to get latest stable version; fall back to known-good
GO_VERSION=$(curl -fsSL --max-time 10 "https://go.dev/VERSION?m=text" 2>/dev/null | head -1 || echo "go1.24.2")
# Validate it looks like a Go version string; reset to fallback if not
case "$GO_VERSION" in
  go[0-9]*) ;;
  *) GO_VERSION="go1.24.2" ;;
esac

log "Installing Go: $GO_VERSION"
cd /tmp
wget -q "https://go.dev/dl/$GO_VERSION.linux-amd64.tar.gz" -O go.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go.tar.gz
rm -f go.tar.gz

export PATH="/usr/local/go/bin:$PATH"
export GOROOT="/usr/local/go"
export GOPATH="/root/go"
export GOCACHE="/root/.cache/go-build"

cat > /etc/profile.d/golang.sh << 'GOPROFILE'
export PATH="/usr/local/go/bin:$PATH"
export GOROOT="/usr/local/go"
GOPROFILE

go version
log "Go installed: $(go version)"

# ── 4. Clone and build ────────────────────────────────────────────────────────
log "[4/9] Cloning and building TunneLab..."
BUILD_DIR="/tmp/tunnelab-build"
rm -rf "$BUILD_DIR"
git clone "${repo_url}" "$BUILD_DIR"
cd "$BUILD_DIR"

CGO_ENABLED=1 CC=gcc go build \
  -ldflags="-s -w" \
  -trimpath \
  -o /usr/local/bin/tunnelab-server \
  ./cmd/server

chmod 755 /usr/local/bin/tunnelab-server
log "Build complete."

# ── 5. Grant cap_net_bind_service for port 443 ────────────────────────────────
log "[5/9] Setting capability for port 443..."
setcap 'cap_net_bind_service+ep' /usr/local/bin/tunnelab-server
log "Capability set: $(getcap /usr/local/bin/tunnelab-server)"

# ── 6. Create user and directories ───────────────────────────────────────────
log "[6/9] Creating system user and directories..."
if ! id -u tunnelab &>/dev/null; then
  useradd \
    --system \
    --no-create-home \
    --home-dir /var/lib/tunnelab \
    --shell /usr/sbin/nologin \
    --comment "TunneLab server daemon" \
    tunnelab
fi
install -d -m 755 -o tunnelab -g tunnelab /etc/tunnelab
install -d -m 750 -o tunnelab -g tunnelab /var/lib/tunnelab
install -d -m 750 -o tunnelab -g tunnelab /var/lib/tunnelab/certs
install -d -m 755                         /var/log/tunnelab
log "User and directories created."

# ── 7. Write config ───────────────────────────────────────────────────────────
log "[7/9] Writing server configuration..."

cat > /etc/tunnelab/server.yaml << YAMLEOF
server:
  domain: "${domain}"
  control_port: 4443
  http_port: 80
  https_port: 443

tls:
  mode: "auto"
  email: "${tls_email}"
  cache_dir: "/var/lib/tunnelab/certs"
  staging: ${tls_staging}
  cert_path: ""
  key_path: ""

database:
  type: "sqlite"
  path: "/var/lib/tunnelab/tunnelab.db"

auth:
  required: true
  token_length: 32

logging:
  level: "info"
  format: "text"
  output: "stdout"

tunnels:
  subdomain_format: "{subdomain}.${domain}"
  tcp_port_range: "${tcp_port_range}"
  max_tunnels_per_client: 5
  max_connections_per_tunnel: 100
YAMLEOF

chown tunnelab:tunnelab /etc/tunnelab/server.yaml
chmod 640 /etc/tunnelab/server.yaml
log "Config written."

# ── 8. Initialize DB and generate token ───────────────────────────────────────
log "[8/9] Initializing database..."
DB_PATH="/var/lib/tunnelab/tunnelab.db"

# Run server briefly to trigger SQLite auto-migration (creates schema)
sudo -u tunnelab /usr/local/bin/tunnelab-server -config /etc/tunnelab/server.yaml &
INIT_PID=$!
log "Server init PID $INIT_PID — waiting 6s for schema creation..."
sleep 6
kill "$INIT_PID" 2>/dev/null || true
wait "$INIT_PID" 2>/dev/null || true

if [ ! -f "$DB_PATH" ]; then
  log "ERROR: Database not found at $DB_PATH"
  exit 1
fi
log "Database initialized."

TOKEN=$(openssl rand -hex 32)
CLIENT_ID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid)
CLIENT_NAME="default-client"

sqlite3 "$DB_PATH" \
  "INSERT INTO clients (id, name, api_token, max_tunnels, status) VALUES ('$CLIENT_ID', '$CLIENT_NAME', '$TOKEN', 5, 'active');"

chown tunnelab:tunnelab "$DB_PATH"
chmod 640 "$DB_PATH"

cat > /var/lib/tunnelab/initial_token.txt << TOKENEOF
TunneLab Initial Client Token
==============================
Client ID   : $CLIENT_ID
Client Name : $CLIENT_NAME
Token       : $TOKEN

Domain      : ${domain}
Control WS  : wss://${domain}:4443

Connect with the test client:
  go run ./cmd/test-client \\
    -server wss://${domain}:4443 \\
    -token $TOKEN \\
    -subdomain myapp \\
    -port 3000 \\
    -protocol http
TOKENEOF

chmod 640 /var/lib/tunnelab/initial_token.txt
chown tunnelab:tunnelab /var/lib/tunnelab/initial_token.txt
log "Token saved to /var/lib/tunnelab/initial_token.txt"

# ── 9. Systemd service ────────────────────────────────────────────────────────
log "[9/9] Creating and starting systemd service..."

cat > /etc/systemd/system/tunnelab.service << 'SERVICEEOF'
[Unit]
Description=TunneLab Tunneling Server
Documentation=https://github.com/essajiwa/tunnelab
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=300
StartLimitBurst=5

[Service]
Type=simple
User=tunnelab
Group=tunnelab
WorkingDirectory=/var/lib/tunnelab
ExecStart=/usr/local/bin/tunnelab-server -config /etc/tunnelab/server.yaml
Restart=on-failure
RestartSec=10s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tunnelab
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ReadWritePaths=/var/lib/tunnelab /var/log/tunnelab
ProtectHome=yes
LockPersonality=yes
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
LimitNOFILE=65536
Environment="HOME=/var/lib/tunnelab"

[Install]
WantedBy=multi-user.target
SERVICEEOF

systemctl daemon-reload
systemctl enable tunnelab
systemctl start tunnelab
sleep 3

systemctl status tunnelab --no-pager || true

touch "$INSTALL_MARKER"
chown tunnelab:tunnelab "$INSTALL_MARKER"

log "=========================================="
log "TunneLab setup complete!"
log "  Service : $(systemctl is-active tunnelab)"
log "  Token   : /var/lib/tunnelab/initial_token.txt"
log "  Config  : /etc/tunnelab/server.yaml"
log "  Logs    : journalctl -u tunnelab -f"
log "=========================================="
