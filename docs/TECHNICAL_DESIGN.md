# Tunneling Server Technical Design (TunneLab)

## Executive Summary

This document outlines the complete technical design for **TunneLab**, a tunneling **server** system similar to ngrok that exposes local servers to the public internet. TunneLab is the server-side component - clients can be implemented using the provided protocol documentation. The server will be implemented in Go and leverage existing open-source components where appropriate.

### Architecture Split
- **TunneLab** (this project): Server-side infrastructure (control server, proxy, tunnel registry)
- **Clients**: Any implementation using the documented protocol

---

## 1. System Overview

### 1.1 Purpose
Create a self-hosted tunneling solution that allows users to expose their local development servers to the internet through a public server with a domain.

### 1.2 Key Features
- HTTP/HTTPS tunneling
- TCP tunneling
- Custom subdomain support
- TLS termination
- WebSocket support
- Authentication and authorization
- Connection monitoring and logging
- Multi-client support
- Automatic reconnection

---

## 2. Architecture

### 2.1 High-Level Architecture

```
┌─────────────────┐         ┌──────────────────┐         ┌─────────────────┐
│  Public Client  │────────▶│  Public Server   │◀────────│  Local Client   │
│  (Browser/API)  │         │  (Your Domain)   │         │  (Tunnel Agent) │
└─────────────────┘         └──────────────────┘         └─────────────────┘
                                     │                             │
                                     │                             │
                                     ▼                             ▼
                            ┌─────────────────┐         ┌─────────────────┐
                            │  Reverse Proxy  │         │  Local Server   │
                            │  + TLS Handler  │         │  (localhost:*)  │
                            └─────────────────┘         └─────────────────┘
```

### 2.2 Component Architecture

```
TunneLab Server (Public - THIS PROJECT):
├── Control Server (manages tunnel connections)
│   ├── WebSocket handler for client connections
│   ├── Tunnel registry
│   └── Authentication service
├── Proxy Server (handles public traffic)
│   ├── HTTP/HTTPS reverse proxy
│   ├── TCP proxy
│   └── TLS termination
└── Admin API (optional)
    ├── Tunnel management
    └── Metrics/monitoring

Client (Local - Any implementation using TunneLab protocol):
├── Tunnel Client
│   ├── Control connection handler
│   ├── Tunnel multiplexer
│   └── Local proxy
└── CLI Interface
```

---

## 3. Open Source Solutions Analysis

### 3.1 frp (Fast Reverse Proxy)
- **License**: Apache 2.0 (Permissive)
- **Language**: Go
- **GitHub**: https://github.com/fatedier/frp
- **Pros**:
  - Production-ready and battle-tested
  - Supports HTTP, HTTPS, TCP, UDP
  - Built-in authentication
  - Plugin system
  - Actively maintained and documented
- **Cons**:
  - Requires customization to align with TunneLab branding and roadmap

### 3.2 Chisel
- **License**: MIT (Permissive)
- **Language**: Go
- **GitHub**: https://github.com/jpillora/chisel
- **Pros**:
  - Simple and lightweight
  - SSH-based tunneling
  - Easy to deploy
- **Cons**:
  - Limited feature set compared to frp
  - Primarily TCP-focused

### 3.3 Rathole
- **License**: Apache 2.0
- **Language**: Rust
- **Pros**:
  - High performance
  - Low resource usage
- **Cons**:
  - Not Go-based, so integration would add operational complexity

### 3.4 Build-from-Scratch Approach
TunneLab adopts a custom Go implementation while drawing architectural inspiration from projects such as frp. This approach preserves full control over features, roadmap, and branding while allowing incorporation of only the components that align with project goals.

---

## 4. Technical Stack

### 4.1 Core Technologies
- **Language**: Go 1.21+
- **Protocol**: WebSocket for control channel, HTTP/TCP for data channels
- **TLS**: Let's Encrypt (autocert) or custom certificates
- **Database**: SQLite/PostgreSQL for tunnel registry and auth
- **Configuration**: YAML/TOML

### 4.2 Key Go Libraries (TunneLab Server)

```go
// HTTP/Reverse Proxy
"net/http"
"net/http/httputil"
"github.com/gorilla/mux"           // HTTP routing
"github.com/gorilla/websocket"     // WebSocket support

// TLS/Certificates
"golang.org/x/crypto/acme/autocert" // Let's Encrypt
"crypto/tls"

// TCP Proxy
"net"
"io"

// Multiplexing (recommended)
"github.com/hashicorp/yamux"       // Stream multiplexing
// OR
"github.com/xtaci/smux"            // Alternative multiplexer

// Configuration
"github.com/spf13/viper"           // Config management
"gopkg.in/yaml.v3"

// Logging
"github.com/sirupsen/logrus"       // Structured logging
// OR
"go.uber.org/zap"                  // High-performance logging

// Database
"github.com/mattn/go-sqlite3"      // SQLite
// OR
"github.com/lib/pq"                // PostgreSQL

// Authentication
"github.com/golang-jwt/jwt/v5"     // JWT tokens
"golang.org/x/crypto/bcrypt"       // Password hashing

// Metrics (optional)
"github.com/prometheus/client_golang" // Prometheus metrics
```

### 4.3 Client Libraries (for clients leveraging TunneLab)

Clients leveraging TunneLab will need similar libraries:
```go
"github.com/gorilla/websocket"     // Control connection
"github.com/hashicorp/yamux"       // Stream multiplexing
"github.com/spf13/cobra"           // CLI framework
"github.com/spf13/viper"           // Config management
```

---

## 5. Detailed Component Design

### 5.1 Control Channel

**Purpose**: Maintain persistent connection between client and server for tunnel management.

**Protocol**: WebSocket over TLS

**Message Types**:
```go
type MessageType string

const (
    MsgTypeAuth         MessageType = "auth"
    MsgTypeAuthResponse MessageType = "auth_response"
    MsgTypeTunnelReq    MessageType = "tunnel_request"   // HTTP/HTTPS
    MsgTypeTunnelResp   MessageType = "tunnel_response"
    MsgTypeTCPReq       MessageType = "tcp_request"      // Raw TCP
    MsgTypeTCPResp      MessageType = "tcp_response"
    MsgTypeGRPCReq      MessageType = "grpc_request"     // gRPC over TCP
    MsgTypeGRPCResp     MessageType = "grpc_response"
    MsgTypeHeartbeat    MessageType = "heartbeat"
    MsgTypeNewConn      MessageType = "new_connection"
    MsgTypeCloseConn    MessageType = "close_connection"
    MsgTypeError        MessageType = "error"
)

type ControlMessage struct {
    Type      MessageType            `json:"type"`
    RequestID string                 `json:"request_id"`
    Payload   map[string]interface{} `json:"payload"`
    Timestamp int64                  `json:"timestamp"`
}
```

**Flow**:
1. Client connects to `wss://control.yourdomain.com`
2. Client sends authentication message
3. Server validates and responds
4. Client requests tunnel (subdomain, protocol, local host/port)
5. Server allocates tunnel and responds with public URL (HTTP) or public port (TCP/gRPC)
6. Heartbeat messages maintain connection
7. Server notifies client of new incoming connections

### 5.2 Data Channel

**Purpose**: Proxy actual traffic between public clients and local servers/services.

**Approach**: Multiplexed streams over a single TCP connection (yamux). The same stream transport is used for HTTP, raw TCP, and gRPC; HTTP tunnels carry HTTP requests, while TCP/gRPC tunnels copy raw bytes.

**Implementation**:
```go
// Server side: When public request arrives
1. Server receives HTTP request on subdomain.yourdomain.com
2. Lookup tunnel registration for subdomain
3. Notify client via control channel about new connection
4. Client opens new stream in multiplexed connection
5. Server proxies request/response through stream
6. Stream closed when request completes
```

**Multiplexing Benefits**:
- Single TCP connection for all data
- Reduces connection overhead
- Better NAT traversal
- Automatic stream management

### 5.3 HTTP/HTTPS Proxy

**Server Implementation**:
```go
type HTTPProxy struct {
    tunnelRegistry *TunnelRegistry
    reverseProxy   *httputil.ReverseProxy
}

// Request flow:
// 1. Extract subdomain from Host header
// 2. Lookup tunnel in registry
// 3. Forward request through tunnel's multiplexed stream
// 4. Return response to public client
```

**Features**:
- Host-based routing (subdomain.yourdomain.com)
- WebSocket upgrade support
- Custom headers injection (X-Forwarded-For, etc.)
- Request/response logging
- Rate limiting per tunnel

### 5.4 TCP Proxy

**Implementation**:
```go
type TCPProxy struct {
    tunnelRegistry *TunnelRegistry
}

// Port allocation:
// - Dynamically assign ports from configured range (e.g., 30000-31000)
// - Map port to tunnel
// - Forward raw TCP traffic through multiplexed stream
```

**Notes**:
- The proxy listens on every port in the configured range.
- Incoming connections are matched to tunnels via `registry.GetByPort`.
- Raw TCP forwarding can be used by any protocol (Redis, SSH, gRPC, etc.).

### 5.5 Tunnel Registry

**Purpose**: Track active tunnels and their configurations.

**Data Model**:
```go
type Tunnel struct {
    ID         string
    ClientID   string
    Subdomain  string
    Protocol   string        // http, https, tcp, grpc
    LocalHost  string
    LocalPort  int
    PublicURL  string        // for HTTP/HTTPS
    PublicPort int           // for TCP/gRPC
    CreatedAt  time.Time
    LastActive time.Time
    Status     string

    ControlConn *websocket.Conn
    MuxSession  *yamux.Session
}

type TunnelRegistry struct {
    mu      sync.RWMutex
    tunnels map[string]*Tunnel   // key: subdomain
    ports   map[int]*Tunnel      // key: public port
    clients map[string][]*Tunnel // key: client_id
}
```

### 5.6 Authentication & Authorization

**Strategies**:

1. **Token-based** (Recommended for MVP):
```go
type Client struct {
    ID          string
    Name        string
    APIToken    string // bcrypt hashed
    MaxTunnels  int
    AllowedSubdomains []string // empty = any available
    CreatedAt   time.Time
}
```

2. **JWT-based** (For advanced scenarios):
```go
type Claims struct {
    ClientID   string   `json:"client_id"`
    Subdomains []string `json:"subdomains"`
    jwt.RegisteredClaims
}
```

### 5.7 TLS/SSL Management

**Options**:

1. **Let's Encrypt (Automatic)**:
```go
certManager := autocert.Manager{
    Prompt:     autocert.AcceptTOS,
    HostPolicy: autocert.HostWhitelist("*.yourdomain.com", "yourdomain.com"),
    Cache:      autocert.DirCache("/var/lib/tunnelab/certs"),
}

server := &http.Server{
    TLSConfig: &tls.Config{
        GetCertificate: certManager.GetCertificate,
    },
}
```

2. **Wildcard Certificate** (Manual):
- Obtain wildcard cert for `*.yourdomain.com`
- Load and use in TLS config

---

## 6. Protocol Specification

### 6.1 Connection Establishment

```
Client                          Server
  |                               |
  |--- WebSocket Connect -------->|
  |    wss://control.domain.com   |
  |                               |
  |<-- Connection Accepted -------|
  |                               |
  |--- Auth Message ------------->|
  |    {type: "auth",             |
  |     payload: {token: "..."}}  |
  |                               |
  |<-- Auth Response -------------|
  |    {type: "auth_response",    |
  |     payload: {success: true}} |
  |                               |
  |--- Tunnel Request ----------->|
  |    {type: "tunnel_request",   |
  |     payload: {                |
  |       subdomain: "myapp",     |
  |       protocol: "http",       |
  |       local_port: 3000}}      |
  |                               |
  |<-- Tunnel Response -----------|
  |    {type: "tunnel_response",  |
  |     payload: {                |
  |       url: "myapp.domain.com",|
  |       tunnel_id: "..."}}      |
  |                               |
  |--- Mux Connection ----------->|
  |    TCP connection for yamux   |
  |                               |
```

### 6.2 Request Proxying

```
Public Client              Server                Client              Local Server
     |                       |                      |                      |
     |-- HTTP Request ------>|                      |                      |
     |   myapp.domain.com    |                      |                      |
     |                       |                      |                      |
     |                       |-- New Conn Msg ----->|                      |
     |                       |   (via WebSocket)    |                      |
     |                       |                      |                      |
     |                       |<-- Open Stream ------|                      |
     |                       |   (via yamux)        |                      |
     |                       |                      |                      |
     |                       |-- Forward Request -->|                      |
     |                       |   (via stream)       |                      |
     |                       |                      |-- HTTP Request ----->|
     |                       |                      |   localhost:3000     |
     |                       |                      |                      |
     |                       |                      |<-- HTTP Response ----|
     |                       |                      |                      |
     |                       |<-- Forward Response -|                      |
     |                       |   (via stream)       |                      |
     |                       |                      |                      |
     |<-- HTTP Response -----|                      |                      |
     |                       |                      |                      |
```

---

## 7. Configuration Design

### 7.1 Server Configuration

```yaml
# server.yaml
server:
  domain: "yourdomain.com"
  control_port: 4443
  http_port: 80
  https_port: 443
  
tls:
  mode: "auto" # auto (Let's Encrypt) or manual
  email: "admin@yourdomain.com"
  cert_path: "/etc/tunnelab/cert.pem" # if manual
  key_path: "/etc/tunnelab/key.pem"   # if manual
  
database:
  type: "sqlite" # sqlite or postgres
  path: "/var/lib/tunnelab/tunnelab.db"
  # For postgres:
  # host: "localhost"
  # port: 5432
  # user: "tunnelab"
  # password: "secret"
  # dbname: "tunnelab"

tunnels:
  subdomain_format: "{subdomain}.yourdomain.com"
  tcp_port_range: "10000-20000"
  max_tunnels_per_client: 5
  
auth:
  required: true
  token_length: 32
  
logging:
  level: "info" # debug, info, warn, error
  format: "json" # json or text
  output: "/var/log/tunnelab/server.log"
  
limits:
  max_connections_per_tunnel: 100
  request_timeout: "30s"
  idle_timeout: "5m"
```

### 7.2 Client Configuration

```yaml
# client.yaml
server:
  control_url: "wss://control.yourdomain.com:4443"
  
auth:
  token: "your-api-token-here"
  
tunnels:
  - name: "web-app"
    subdomain: "myapp"
    protocol: "http"
    local_port: 3000
    
  - name: "api-server"
    subdomain: "api"
    protocol: "https"
    local_port: 8080
    
  - name: "ssh-server"
    protocol: "tcp"
    local_port: 22
    # public_port will be assigned by server
    
logging:
  level: "info"
  output: "stdout"
  
reconnect:
  enabled: true
  max_retries: 10
  initial_delay: "1s"
  max_delay: "30s"
```

---

## 8. Database Schema

### 8.1 Tables

```sql
-- Clients table
CREATE TABLE clients (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    api_token VARCHAR(255) NOT NULL UNIQUE,
    max_tunnels INTEGER DEFAULT 5,
    allowed_subdomains TEXT, -- JSON array
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(20) DEFAULT 'active' -- active, suspended
);

-- Tunnels table (for persistence and history)
CREATE TABLE tunnels (
    id VARCHAR(36) PRIMARY KEY,
    client_id VARCHAR(36) NOT NULL,
    subdomain VARCHAR(255),
    protocol VARCHAR(10) NOT NULL,
    local_port INTEGER NOT NULL,
    public_port INTEGER,
    public_url VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    closed_at TIMESTAMP,
    status VARCHAR(20) DEFAULT 'active',
    FOREIGN KEY (client_id) REFERENCES clients(id),
    UNIQUE(subdomain) -- ensure subdomain uniqueness
);

-- Connection logs (optional, for analytics)
CREATE TABLE connection_logs (
    id BIGSERIAL PRIMARY KEY,
    tunnel_id VARCHAR(36) NOT NULL,
    client_ip VARCHAR(45),
    request_method VARCHAR(10),
    request_path TEXT,
    response_status INTEGER,
    bytes_sent BIGINT,
    bytes_received BIGINT,
    duration_ms INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (tunnel_id) REFERENCES tunnels(id)
);

-- Indexes
CREATE INDEX idx_tunnels_client_id ON tunnels(client_id);
CREATE INDEX idx_tunnels_subdomain ON tunnels(subdomain);
CREATE INDEX idx_tunnels_status ON tunnels(status);
CREATE INDEX idx_connection_logs_tunnel_id ON connection_logs(tunnel_id);
CREATE INDEX idx_connection_logs_created_at ON connection_logs(created_at);
```

---

## 9. Security Considerations

### 9.1 Authentication
- API tokens with bcrypt hashing
- Token rotation capability
- Rate limiting on authentication attempts
- Optional IP whitelisting per client

### 9.2 Authorization
- Subdomain restrictions per client
- Maximum tunnel limits
- Protocol restrictions (e.g., some clients only HTTP)

### 9.3 Network Security
- TLS for all control connections
- Optional TLS for data connections
- DDoS protection (rate limiting, connection limits)
- Request size limits
- Timeout configurations

### 9.4 Data Privacy
- No request/response logging by default (opt-in)
- Secure token storage
- Encrypted database connections

### 9.5 Abuse Prevention
- Connection rate limiting
- Bandwidth throttling (optional)
- Subdomain blacklist
- Automatic tunnel cleanup for inactive clients

---

## 10. Monitoring & Observability

### 10.1 Metrics (Prometheus)
```go
// Key metrics to track
- tunnel_active_count
- tunnel_total_requests
- tunnel_request_duration_seconds
- tunnel_bytes_transferred
- client_connections_active
- control_connection_errors
- proxy_errors_total
```

### 10.2 Logging
- Structured JSON logging
- Log levels: DEBUG, INFO, WARN, ERROR
- Key events:
  - Client connections/disconnections
  - Tunnel creation/deletion
  - Authentication failures
  - Proxy errors
  - Performance issues

### 10.3 Health Checks
```go
// Endpoints
GET /health        // Basic health check
GET /health/ready  // Readiness check (DB, etc.)
GET /metrics       // Prometheus metrics
```

---

## 11. Deployment Architecture

### 11.1 Server Deployment

```
┌─────────────────────────────────────────┐
│         Load Balancer (Optional)        │
│              (nginx/HAProxy)            │
└─────────────────────────────────────────┘
                    │
        ┌───────────┴───────────┐
        │                       │
┌───────▼────────┐    ┌────────▼────────┐
│  TunneLab      │    │  TunneLab       │
│  Server 1      │    │  Server 2       │
│  (Primary)     │    │  (Replica)      │
└────────────────┘    └─────────────────┘
        │                       │
        └───────────┬───────────┘
                    │
        ┌───────────▼───────────┐
        │   PostgreSQL/SQLite   │
        │   (Shared State)      │
        └───────────────────────┘
```

### 11.2 DNS Configuration

```
# A Records
yourdomain.com          -> YOUR_PUBLIC_IP
control.yourdomain.com  -> YOUR_PUBLIC_IP

# Wildcard for subdomains
*.yourdomain.com        -> YOUR_PUBLIC_IP
```

### 11.3 Firewall Rules

```bash
# Allow HTTP
80/tcp    -> TunneLab HTTP proxy

# Allow HTTPS
443/tcp   -> TunneLab HTTPS proxy

# Allow Control Connection
4443/tcp  -> TunneLab control server

# Allow TCP tunnels (if enabled)
10000-20000/tcp -> TunneLab TCP proxy
```

### 11.4 Systemd Service

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

---

## 12. Client Usage Examples

### 12.1 CLI Usage

```bash
# Start tunnel with config file
tunnelab-client -config client.yaml

# Start tunnel with flags
tunnelab-client \
  -server wss://control.yourdomain.com:4443 \
  -token YOUR_API_TOKEN \
  -subdomain myapp \
  -local-port 3000

# Start TCP tunnel
tunnelab-client \
  -server wss://control.yourdomain.com:4443 \
  -token YOUR_API_TOKEN \
  -protocol tcp \
  -local-port 22

# List active tunnels
tunnelab-client list

# Stop tunnel
tunnelab-client stop myapp
```

### 12.2 Programmatic Usage (Go SDK)

```go
package main

import (
    "github.com/essajiwa/tunnelab/client"
)

func main() {
    cfg := &client.Config{
        ServerURL: "wss://control.yourdomain.com:4443",
        Token:     "YOUR_API_TOKEN",
    }
    
    c, err := client.New(cfg)
    if err != nil {
        panic(err)
    }
    
    tunnel, err := c.StartTunnel(&client.TunnelConfig{
        Subdomain: "myapp",
        Protocol:  "http",
        LocalPort: 3000,
    })
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Tunnel started: %s\n", tunnel.PublicURL)
    
    // Keep running
    select {}
}
```

---

## 13. Performance Considerations

### 13.1 Optimization Strategies

1. **Connection Pooling**
   - Reuse multiplexed streams
   - Connection keep-alive
   - Efficient buffer management

2. **Caching**
   - Cache tunnel lookups
   - DNS caching
   - TLS session resumption

3. **Concurrency**
   - Goroutine per connection
   - Worker pools for heavy operations
   - Non-blocking I/O

4. **Resource Limits**
   ```go
   // Example limits
   MaxConcurrentStreams: 1000
   MaxStreamBufferSize:  32KB
   ReadTimeout:          30s
   WriteTimeout:         30s
   IdleTimeout:          5m
   ```

### 13.2 Expected Performance

- **Latency**: +5-15ms overhead vs direct connection
- **Throughput**: 80-95% of direct connection (depends on network)
- **Concurrent Tunnels**: 1000+ per server (with 4GB RAM)
- **Requests/sec**: 10,000+ (for HTTP tunnels)

---

## 14. Testing Strategy

### 14.1 Unit Tests
- Protocol message encoding/decoding
- Tunnel registry operations
- Authentication logic
- Configuration parsing

### 14.2 Integration Tests
- Client-server connection flow
- HTTP proxy functionality
- TCP proxy functionality
- Reconnection logic
- Multi-tunnel scenarios

### 14.3 Load Tests
- Concurrent connections
- High request rate
- Large payload handling
- Memory leak detection

### 14.4 Security Tests
- Authentication bypass attempts
- Rate limiting effectiveness
- DDoS simulation
- Malformed request handling

---

## 15. Implementation Roadmap (TunneLab Server Only)

### Phase 1: Core Infrastructure (Week 1-2)
- [ ] Project structure setup
- [ ] Configuration management
- [ ] Database schema and models
- [ ] Authentication system
- [ ] Logging framework

### Phase 2: Control Channel Server (Week 2-3)
- [ ] WebSocket server
- [ ] Control message protocol definition
- [ ] Client authentication handler
- [ ] Tunnel registration logic
- [ ] Heartbeat mechanism

### Phase 3: HTTP Proxy (Week 3-4)
- [ ] HTTP reverse proxy
- [ ] Host-based routing (subdomain mapping)
- [ ] Multiplexed stream handling (server-side)
- [ ] WebSocket upgrade support
- [ ] TLS termination with Let's Encrypt

### Phase 4: TCP Proxy (Week 4-5)
- [ ] TCP listener
- [ ] Dynamic port allocation
- [ ] TCP stream proxying
- [ ] Port pool management

### Phase 5: Advanced Features (Week 5-6)
- [ ] Admin API endpoints
- [ ] Prometheus metrics
- [ ] Rate limiting middleware
- [ ] Connection logging
- [ ] Tunnel analytics

### Phase 6: Testing & Documentation (Week 6-7)
- [ ] Unit tests for all components
- [ ] Integration tests (with mock client)
- [ ] Load tests
- [ ] API documentation
- [ ] Protocol specification for client integration

### Phase 7: Deployment (Week 7)
- [ ] Docker image
- [ ] Systemd service files
- [ ] Deployment scripts
- [ ] Monitoring setup
- [ ] Production testing

### Phase 8: Client Integration Support (Week 8)
- [ ] Client protocol documentation
- [ ] Example client code/SDK
- [ ] Integration testing with test client
- [ ] Performance tuning

**Note**: Client implementation will be done by developers using the protocol documentation.

---

## 16. File Structure (TunneLab Server)

```
tunnelab/
├── cmd/
│   └── server/
│       └── main.go           # Server entry point
├── internal/
│   ├── server/
│   │   ├── control/          # Control channel WebSocket handler
│   │   ├── proxy/            # HTTP/TCP proxy implementation
│   │   │   ├── http.go
│   │   └── tcp.go
│   │   ├── registry/         # Tunnel registry and management
│   │   ├── auth/             # Authentication & authorization
│   │   └── config/           # Server configuration
│   ├── protocol/             # Protocol definitions (shared with clients)
│   │   ├── messages.go       # Control message types
│   │   └── constants.go      # Protocol constants
│   ├── database/             # Database models and migrations
│   │   ├── models.go
│   │   ├── migrations/
│   │   └── repository.go
│   └── utils/                # Shared utilities
│       ├── logger/
│       └── crypto/
├── pkg/
│   └── protocol/             # Public protocol package (for clients to import)
│       ├── control.go        # Control message definitions
│       └── tunnel.go         # Tunnel configuration types
├── api/
│   └── admin/                # Admin API (optional)
│       └── handlers.go
├── configs/
│   └── server.example.yaml   # Example server configuration
├── deployments/
│   ├── docker/
│   │   └── Dockerfile
│   ├── kubernetes/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   └── systemd/
│       └── tunnelab.service
├── scripts/
│   ├── setup.sh
│   ├── generate-token.sh
│   └── migrate.sh
├── docs/
│   ├── API.md                # Admin API documentation
│   ├── DEPLOYMENT.md         # Deployment guide
│   ├── PROTOCOL.md           # Protocol spec for client integration
│   └── CONFIGURATION.md      # Configuration reference
├── tests/
│   ├── integration/
│   │   └── server_test.go
│   └── load/
│       └── benchmark_test.go
├── go.mod
├── go.sum
├── README.md
├── LICENSE
└── TECHNICAL_DESIGN.md       # This file
```

### Client Integration

The `pkg/protocol/` package will be importable by clients:

```go
// In client project
import "github.com/essajiwa/tunnelab/pkg/protocol"

// Use shared protocol definitions
msg := protocol.ControlMessage{
    Type: protocol.MsgTypeAuth,
    // ...
}
```

---

## 17. Alternative Approaches

### 17.1 Use frp Directly
**Pros**: Battle-tested, feature-rich, ready to deploy
**Cons**: Less control, external dependency

**Recommendation**: Good for quick deployment, but custom build better for learning and full control

### 17.2 Use Chisel
**Pros**: Simple, SSH-based, easy to understand
**Cons**: Limited features, less suitable for HTTP-specific needs

### 17.3 Hybrid Approach
- Use frp as reference implementation
- Build custom solution with similar architecture
- Reuse some libraries (yamux, websocket, etc.)
- Add custom features as needed

---

## 18. Risk Mitigation

### 18.1 Technical Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Connection drops | High | Automatic reconnection, heartbeat |
| Memory leaks | High | Proper resource cleanup, testing |
| DDoS attacks | High | Rate limiting, connection limits |
| Certificate expiry | Medium | Auto-renewal with Let's Encrypt |
| Database corruption | Medium | Regular backups, WAL mode |
| Port exhaustion | Medium | Port pool management |

### 18.2 Operational Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Server downtime | High | Load balancing, health checks |
| DNS issues | High | Multiple DNS providers |
| Disk space | Medium | Log rotation, monitoring |
| High traffic costs | Medium | Bandwidth monitoring, limits |

---

## 19. Future Enhancements

### 19.1 Short-term (3-6 months)
- Web dashboard for tunnel management
- Multiple authentication methods (OAuth, SAML)
- Custom domain support (bring your own domain)
- Traffic analytics and visualization
- Mobile client apps

### 19.2 Long-term (6-12 months)
- UDP tunneling support
- Load balancing across multiple tunnels
- Geographic routing
- API gateway features
- Webhook support
- Plugin system for custom protocols

---

## 20. Cost Estimation

### 20.1 Infrastructure Costs (Monthly)

**Small Scale** (< 100 users):
- VPS: $10-20/month (2GB RAM, 2 vCPU)
- Domain: $10-15/year
- Total: ~$15-25/month

**Medium Scale** (100-1000 users):
- VPS: $40-80/month (8GB RAM, 4 vCPU)
- Load balancer: $10-20/month
- Database: $20-40/month (managed)
- Total: ~$70-140/month

**Large Scale** (1000+ users):
- Multiple servers: $200-500/month
- Load balancer: $50-100/month
- Database: $100-200/month
- CDN/DDoS protection: $50-100/month
- Total: ~$400-900/month

### 20.2 Development Costs

- Initial development: 6-8 weeks (1 developer)
- Maintenance: 10-20 hours/month
- Infrastructure management: 5-10 hours/month

---

## 21. Success Metrics

### 21.1 Technical Metrics
- Uptime: > 99.9%
- Latency overhead: < 15ms p95
- Connection success rate: > 99%
- Reconnection time: < 5 seconds

### 21.2 User Metrics
- Active tunnels
- Data transferred
- Average session duration
- User retention rate

---

## 22. Conclusion

This design provides a comprehensive blueprint for building a production-ready tunneling server similar to ngrok. The architecture is:

- **Scalable**: Can handle thousands of concurrent tunnels
- **Secure**: Multiple layers of authentication and encryption
- **Performant**: Optimized for low latency and high throughput
- **Maintainable**: Clean architecture with clear separation of concerns
- **Extensible**: Easy to add new features and protocols

The recommended approach is to build a custom solution using Go and proven libraries, taking architectural inspiration from frp while maintaining full control over the codebase.

---

## 23. References

- frp: https://github.com/fatedier/frp
- Chisel: https://github.com/jpillora/chisel
- yamux: https://github.com/hashicorp/yamux
- WebSocket RFC: https://tools.ietf.org/html/rfc6455
- HTTP/2 RFC: https://tools.ietf.org/html/rfc7540
- Let's Encrypt: https://letsencrypt.org/docs/

---

**Document Version**: 1.0  
**Last Updated**: January 2026  
**Author**: TunneLab Team
