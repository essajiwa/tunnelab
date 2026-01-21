# TunneLab API Documentation

This document provides comprehensive documentation for all Go packages in the TunneLab project.

## Table of Contents

- [pkg/protocol](#pkgprotocol) - Protocol definitions
- [internal/database](#internaldatabase) - Database operations
- [internal/server/auth](#internalserverauth) - Authentication
- [internal/server/registry](#internalserverregistry) - Tunnel registry
- [internal/server/tls](#internalservertls) - TLS certificate management
- [cmd/server](#cmdserver) - Main server binary
- [cmd/test-client](#cmdtest-client) - Test client

---

## pkg/protocol

Package protocol defines the communication protocol between TunneLab server and clients.

The protocol uses JSON messages over WebSocket connections for control operations
and yamux multiplexed connections for data transfer. This protocol is used by
clients that leverage TunneLab for tunneling services.

### Message Types

- `auth`: Client authentication
- `auth_response`: Server authentication response
- `tunnel_request`: Request to create an HTTP(S) tunnel
- `tunnel_response`: Tunnel creation response for HTTP(S)
- `tcp_request`: Request to create a TCP tunnel (raw port forwarding)
- `tcp_response`: TCP tunnel creation response (returns public port)
- `grpc_request`: Request to create a tunnel intended for gRPC (raw TCP)
- `grpc_response`: gRPC tunnel creation response (returns public port/endpoint)
- `new_connection`: New multiplexed connection notification
- `heartbeat`: Keep-alive messages
- `error`: Error messages

### Types

```go
type ControlMessage struct {
    Type      MessageType            `json:"type"`       // Message type
    RequestID string                 `json:"request_id"` // Unique request identifier
    Payload   map[string]interface{} `json:"payload"`    // Message payload data
    Timestamp int64                  `json:"timestamp"`  // Unix timestamp
}

type TunnelConfig struct {
    Subdomain string `json:"subdomain"`   // Desired subdomain
    Protocol  string `json:"protocol"`    // Protocol type (http, tcp, grpc)
    LocalPort int    `json:"local_port"`  // Local port to forward
    LocalHost string `json:"local_host"` // Local host (defaults to localhost)
}

type GRPCTunnelConfig struct {
    Subdomain  string   `json:"subdomain"`
    LocalPort  int      `json:"local_port"`
    LocalHost  string   `json:"local_host,omitempty"`
    Services   []string `json:"services,omitempty"`
    RequireTLS bool     `json:"require_tls"`
    MaxStreams int      `json:"max_streams,omitempty"`
}

type TunnelResponse struct {
    TunnelID   string `json:"tunnel_id"`            // Unique tunnel identifier
    PublicURL  string `json:"public_url,omitempty"` // Public URL for HTTP(S)
    PublicPort int    `json:"public_port,omitempty"`// Assigned public port for TCP/gRPC
    Status     string `json:"status"`               // Tunnel status
}
```

### Functions

```go
func NewControlMessage(msgType MessageType, requestID string, payload map[string]interface{}) *ControlMessage
func NewErrorMessage(requestID, code, message string) *ControlMessage
```

### Usage Example

```go
// Create an authentication message
msg := NewControlMessage(MsgTypeAuth, uuid.New().String(), map[string]interface{}{
    "token": "your-token-here",
})

// Send over WebSocket
conn.WriteJSON(msg)
```

---

## internal/database

Package database provides data models and database operations for TunneLab.

This package implements the Repository pattern for SQLite database operations
including client management, tunnel configuration, and connection logging.

### Models

```go
type Client struct {
    ID                string    `json:"id"`                 // Unique client identifier
    Name              string    `json:"name"`               // Human-readable name
    APIToken          string    `json:"api_token"`          // Authentication token
    MaxTunnels        int       `json:"max_tunnels"`        // Maximum tunnels allowed
    AllowedSubdomains string    `json:"allowed_subdomains"` // Allowed subdomains
    CreatedAt         time.Time `json:"created_at"`         // Creation timestamp
    UpdatedAt         time.Time `json:"updated_at"`         // Last update timestamp
    Status            string    `json:"status"`             // Client status
}

type Tunnel struct {
    ID         string    `json:"id"`         // Unique tunnel identifier
    ClientID   string    `json:"client_id"`  // ID of the owning client
    Subdomain  string    `json:"subdomain"`  // Subdomain for public access
    Protocol   string    `json:"protocol"`   // Protocol type
    LocalPort  int       `json:"local_port"` // Local port to forward
    PublicURL  string    `json:"public_url"` // Public URL for access
    CreatedAt  time.Time `json:"created_at"` // Creation timestamp
    ClosedAt   *time.Time `json:"closed_at"` // Closure timestamp
    Status     string    `json:"status"`     // Tunnel status
}

type ConnectionLog struct {
    ID             int64     `json:"id"`             // Unique log entry identifier
    TunnelID       string    `json:"tunnel_id"`      // ID of the tunnel
    ClientIP       string    `json:"client_ip"`      // Client IP address
    RequestMethod  string    `json:"request_method"` // HTTP method
    RequestPath    string    `json:"request_path"`   // Request path
    ResponseStatus int       `json:"response_status"`// HTTP response status
    BytesSent      int64     `json:"bytes_sent"`     // Bytes sent
    BytesReceived  int64     `json:"bytes_received"` // Bytes received
    DurationMs     int       `json:"duration_ms"`    // Request duration in ms
    CreatedAt      time.Time `json:"created_at"`     // Timestamp of request
}
```

### Repository

```go
type Repository struct {
    db *sql.DB // SQLite database connection
}
```

### Functions

```go
func NewRepository(dbPath string) (*Repository, error)
func (r *Repository) GetClientByToken(token string) (*Client, error)
func (r *Repository) CreateClient(client *Client) error
func (r *Repository) CreateTunnel(tunnel *Tunnel) error
func (r *Repository) GetActiveTunnels() ([]*Tunnel, error)
func (r *Repository) Close() error
```

### Usage Example

```go
repo, err := NewRepository("tunnelab.db")
if err != nil {
    log.Fatal(err)
}
defer repo.Close()

client, err := repo.GetClientByToken("token")
if err != nil {
    log.Fatal(err)
}
```

---

## internal/server/auth

Package auth provides authentication services for TunneLab.

This package handles token generation, hashing, and verification for client
authentication using bcrypt for secure password hashing.

### Service

```go
type Service struct{}
```

### Functions

```go
func NewService() *Service
func (s *Service) GenerateToken() (string, error)
func (s *Service) HashToken(token string) (string, error)
func (s *Service) VerifyToken(token, hash string) bool
```

### Usage Example

```go
auth := NewService()

// Generate a new token
token, err := auth.GenerateToken()
if err != nil {
    log.Fatal(err)
}

// Hash the token for storage
hash, err := auth.HashToken(token)
if err != nil {
    log.Fatal(err)
}

// Verify a token
valid := auth.VerifyToken(token, hash)
```

---

## internal/server/registry

Package registry provides in-memory tunnel registry for TunneLab.

This package manages active tunnels, their connections, and multiplexed sessions.
It provides thread-safe operations for registering, unregistering, and accessing tunnels.

### Types

```go
type Registry struct {
    mu      sync.RWMutex              // Mutex for thread-safe operations
    tunnels map[string]*TunnelInfo   // Map of subdomain to tunnel info
    clients map[string][]*TunnelInfo // Map of client ID to tunnel info
    ports   map[int]*TunnelInfo      // Map of public port to tunnel info (for TCP/gRPC)
}

type TunnelInfo struct {
    ID         string          // Unique tunnel identifier
    ClientID   string          // ID of the owning client
    Subdomain  string          // Subdomain for public access
    Protocol   string          // Protocol type (http, tcp, grpc)
    LocalPort  int             // Local port to forward
    LocalHost  string          // Local host address
    PublicURL  string          // Public URL (HTTP/S tunnels)
    PublicPort int             // Public port (TCP/gRPC tunnels)
    ControlConn *websocket.Conn
    MuxSession  *yamux.Session
}
```

### Functions

```go
func NewRegistry() *Registry
func (r *Registry) Register(tunnel *TunnelInfo) error
func (r *Registry) Unregister(subdomain string)
func (r *Registry) GetBySubdomain(subdomain string) (*TunnelInfo, bool)
func (r *Registry) GetByPort(port int) (*TunnelInfo, bool)
func (r *Registry) GetByClient(clientID string) []*TunnelInfo
func (r *Registry) OpenStream(subdomain string) (net.Conn, error)
func (r *Registry) Count() int
```

### Usage Example

```go
reg := NewRegistry()

// Register a tunnel
tunnel := &TunnelInfo{
    ID:        uuid.New().String(),
    ClientID:  clientID,
    Subdomain: "myapp",
    Protocol:  "http",
    LocalPort: 3000,
}
err := reg.Register(tunnel)

// Get tunnel by subdomain
tunnel, exists := reg.GetBySubdomain("myapp")

// Open a stream to the tunnel
stream, err := reg.OpenStream("myapp")
```

---

## internal/server/tls

Package tls provides TLS certificate management for TunneLab.

This package supports automatic certificate generation using Let's Encrypt
as well as manual certificate loading. It handles certificate caching,
renewal, and provides secure TLS configurations.

### Types

```go
type CertManager struct {
    manager *autocert.Manager // Let's Encrypt manager
    config  *tls.Config       // TLS configuration
}

type Config struct {
    Domain   string // Domain for certificates
    Email    string // Email for Let's Encrypt notifications
    CacheDir string // Directory to cache certificates
    Staging  bool   // Use Let's Encrypt staging environment
}
```

### Functions

```go
func NewCertManager(cfg *Config) (*CertManager, error)
func (cm *CertManager) TLSConfig() *tls.Config
func (cm *CertManager) HTTPHandler() http.Handler
func LoadManualCerts(certPath, keyPath string) (*tls.Config, error)
func GetCertCachePath(domain string) string
```

### Usage Example

```go
// Automatic certificate management
certManager, err := NewCertManager(&Config{
    Domain:  "example.com",
    Email:   "admin@example.com",
    Staging: false,
})
if err != nil {
    log.Fatal(err)
}

// Use with HTTP server
server := &http.Server{
    Addr:      ":443",
    TLSConfig: certManager.TLSConfig(),
}
```

---

## cmd/server

TunneLab Server - A secure tunneling server that exposes local HTTP servers to the internet.

This server provides:
- WebSocket control connections for tunnel management
- HTTP/HTTPS proxy with subdomain routing
- Automatic Let's Encrypt certificate generation
- Token-based client authentication
- Yamux multiplexed connections for efficient data transfer

### Usage

```bash
./tunnelab-server -config configs/server.yaml
```

### Flags

- `-config`: Path to configuration file (default: configs/server.yaml)
- `-version`: Show version information

### Configuration

The server is configured via YAML file. See `configs/server.example.yaml` for a complete example.

---

## cmd/test-client

Test client is a minimal tunnel client for testing TunneLab server.

This client simulates client behavior for testing purposes. It connects to the 
control server, authenticates, creates a tunnel, and forwards HTTP requests 
to a local server.

### Usage

```bash
./test-client -server ws://localhost:4443 -token TOKEN -subdomain test -port 8000
```

### Flags

- `-server`: Control server WebSocket URL (default: ws://localhost:4443)
- `-token`: Authentication token (required)
- `-subdomain`: Subdomain for the tunnel (default: test)
- `-port`: Local port to forward traffic to (default: 8000)

---

## Generating Documentation

To generate documentation locally:

```bash
# Install godoc
go install golang.org/x/tools/cmd/godoc@latest

# Generate documentation
godoc -http=:6060

# View documentation at http://localhost:6060/pkg/github.com/essajiwa/tunnelab/
```

Or use the built-in go doc command:

```bash
# View package documentation
go doc github.com/essajiwa/tunnelab/pkg/protocol

# View all documentation
go doc -all github.com/essajiwa/tunnelab/...
```

## API Version

Current API version: v1.0.0

All APIs are versioned and backward compatible changes are made without changing the version number. Breaking changes will increment the major version.

## Contributing

When adding new functions or types, please include comprehensive godoc comments following the Go documentation conventions:

1. Start with a one-sentence summary
2. Provide detailed description if needed
3. Document parameters and return values
4. Include usage examples
5. Use proper formatting for code examples

For more information, see the [Go documentation guidelines](https://golang.org/doc/effective_go.html#documentation).
