# CLAUDE.md — TunneLab Codebase Guide

This document is the authoritative reference for AI assistants working in this repository. It covers architecture, conventions, development workflows, and critical rules to follow.

---

## Project Overview

**TunneLab** is a self-hosted tunneling server (similar to ngrok) written in Go. It exposes local HTTP/HTTPS, TCP, and gRPC services to the public internet via a WebSocket control channel and yamux-multiplexed data streams.

**Status**: MVP complete and functional. The server is ready for client development against the published protocol.

---

## Repository Layout

```
tunnelab/
├── cmd/
│   ├── server/main.go          # Server binary entry point
│   └── test-client/main.go     # Reference client implementation
├── configs/
│   └── server.example.yaml     # Config template (DO NOT modify; copy to server.yaml)
├── docs/                        # Detailed design and usage docs
│   ├── API_DOCUMENTATION.md
│   ├── IMPLEMENTATION_STATUS.md
│   ├── LETSENCRYPT.md
│   ├── QUICKSTART.md
│   ├── TECHNICAL_DESIGN.md
│   ├── TESTING.md
│   └── USAGE.md
├── internal/                    # Private implementation (not importable externally)
│   ├── database/
│   │   ├── models.go           # Client, Tunnel, ConnectionLog structs
│   │   └── repository.go       # Repository pattern for all DB ops
│   └── server/
│       ├── auth/auth.go        # Token generation (bcrypt + crypto/rand)
│       ├── config/config.go    # YAML config loading and validation
│       ├── control/
│       │   ├── handler.go      # WebSocket control message router
│       │   └── handler_test.go # Port allocator unit tests
│       ├── proxy/
│       │   ├── http.go         # HTTP/HTTPS reverse proxy
│       │   └── tcp.go          # Raw TCP proxy
│       ├── registry/
│       │   ├── registry.go     # In-memory tunnel registry (thread-safe)
│       │   └── registry_test.go
│       └── tls/autocert.go     # Let's Encrypt + manual cert support
├── pkg/
│   └── protocol/messages.go    # PUBLIC protocol types — importable by clients
├── scripts/
│   ├── generate-token.sh       # Add a client record to the DB
│   ├── setup.sh                # First-run setup
│   └── test-tunnel.sh          # End-to-end integration test
├── go.mod                       # Module: requires Go 1.21+
├── go.sum
├── Makefile                     # All build and dev commands
└── README.md
```

### Key Boundaries

- `pkg/protocol` — The only package external clients should import. All control message types live here.
- `internal/` — Server-side only. Never reference from client implementations.
- `configs/server.yaml` — Generated at runtime; gitignored. Use `configs/server.example.yaml` as the template.

---

## Build and Development Commands

All common tasks are wired through `make`:

| Command | What it does |
|---------|-------------|
| `make build` | Compiles `tunnelab-server` with version info from git |
| `make run` | Builds then runs with `configs/server.yaml` |
| `make test` | `go test -v ./...` — runs all tests |
| `make setup` | First-run: copies config template, builds binary, initializes DB |
| `make generate-token` | Creates a new client record in the SQLite DB |
| `make clean` | Removes binary and all `*.db` files |
| `make deps` | `go mod download && go mod tidy` |
| `make install` | Installs binary to `$GOPATH/bin` |

**Build flags used:**
```
-ldflags="-s -w -X main.version=$(VERSION)" -trimpath
```
Version is resolved from git tags → commit hash → `"dev"`.

---

## Dependencies

**Direct (go.mod):**

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/google/uuid` | v1.6.0 | UUID generation |
| `github.com/gorilla/websocket` | v1.5.1 | WebSocket control channel |
| `github.com/hashicorp/yamux` | v0.1.1 | Stream multiplexing over TCP |
| `github.com/mattn/go-sqlite3` | v1.14.19 | SQLite driver (CGO required) |
| `golang.org/x/crypto` | v0.18.0 | bcrypt + Let's Encrypt ACME |
| `gopkg.in/yaml.v3` | v3.0.1 | Config file parsing |

**CGO note:** `go-sqlite3` requires CGO. Set `CGO_ENABLED=1` when building.

---

## Architecture

### Connection Lifecycle

```
Client ──WebSocket──► HandleWebSocket()
  1. Upgrade HTTP → WebSocket (port 4443)
  2. Client sends MsgTypeAuth {token}
  3. Server verifies token via DB → returns MsgTypeAuthResponse
  4. Client sends MsgTypeTunnelReq / MsgTypeTCPReq / MsgTypeGRPCReq
  5. Server registers tunnel in Registry, allocates port if TCP/gRPC
  6. Server sends MsgTypeTunnelResp / MsgTypeTCPResp with public URL/port
  7. Server listens on ephemeral port for yamux handshake
  8. Server sends MsgTypeNewConn {mux_addr}
  9. Client dials mux_addr → yamux.Server() session established
 10. HTTP requests arrive → HTTPProxy opens yamux stream, forwards
 11. TCP packets arrive → TCPProxy opens yamux stream, forwards
 12. Client sends MsgTypeHeartbeat every 30s
 13. On disconnect → cleanupClient() unregisters all tunnels
```

### Core Components

**`internal/server/control/handler.go`** — WebSocket message router
- `NewHandler(registry, repo, domain)` — Constructor
- `ConfigurePortAllocator(portRange string)` — Enables TCP port allocation
- `HandleWebSocket(w, r)` — HTTP handler; upgrades and authenticates
- `handleTunnelRequest(conn, clientID, msg)` — Creates tunnel + launches mux goroutine
- `waitForMuxConnection(tunnel)` — Goroutine: listens for yamux session
- `cleanupClient(clientID)` — Unregisters all tunnels for a client

**`internal/server/registry/registry.go`** — Thread-safe in-memory tunnel index
- Three maps: `subdomain → TunnelInfo`, `clientID → []TunnelInfo`, `port → TunnelInfo`
- All mutations under `sync.RWMutex`; reads use read lock
- `OpenStream(subdomain)` — Opens a new yamux stream to the client for a given tunnel

**`internal/server/proxy/http.go`** — HTTP reverse proxy
- Routes by `Host` header subdomain extraction
- Handles chunked/streaming responses (SSE-compatible)
- `HandleHealthCheck` — Returns JSON `{"status":"healthy","tunnels":N}`

**`internal/server/proxy/tcp.go`** — Raw TCP proxy
- Listens on configured port range
- `GetByPort()` → opens yamux stream → bidirectional `io.Copy` with WaitGroup

**`internal/server/auth/auth.go`** — Token auth service
- `GenerateToken()` — 32 random bytes → 64-char hex string
- `HashToken(token)` — bcrypt cost=10
- `VerifyToken(token, hash)` — bcrypt comparison

**`internal/database/repository.go`** — SQLite repository
- Auto-migrates schema on `NewRepository(path)`
- Key methods: `GetClientByToken`, `CreateClient`, `CreateTunnel`, `CloseTunnel`, `GetActiveTunnelsByClient`

---

## Protocol Reference

All message types are defined in `pkg/protocol/messages.go`.

```go
type ControlMessage struct {
    Type      MessageType            `json:"type"`
    RequestID string                 `json:"request_id"`
    Payload   map[string]interface{} `json:"payload"`
    Timestamp int64                  `json:"timestamp"`
}
```

**Message types:**

| Constant | Value | Direction |
|----------|-------|-----------|
| `MsgTypeAuth` | `"auth"` | Client → Server |
| `MsgTypeAuthResponse` | `"auth_response"` | Server → Client |
| `MsgTypeTunnelReq` | `"tunnel_request"` | Client → Server |
| `MsgTypeTunnelResp` | `"tunnel_response"` | Server → Client |
| `MsgTypeTCPReq` | `"tcp_request"` | Client → Server |
| `MsgTypeTCPResp` | `"tcp_response"` | Server → Client |
| `MsgTypeGRPCReq` | `"grpc_request"` | Client → Server |
| `MsgTypeGRPCResp` | `"grpc_response"` | Server → Client |
| `MsgTypeHeartbeat` | `"heartbeat"` | Client → Server |
| `MsgTypeNewConn` | `"new_connection"` | Server → Client |
| `MsgTypeCloseConn` | `"close_connection"` | Bidirectional |
| `MsgTypeError` | `"error"` | Server → Client |

See `cmd/test-client/main.go` for a complete reference client implementation.

---

## Configuration

Copy `configs/server.example.yaml` to `configs/server.yaml` before running. The file is gitignored.

**Key configuration sections:**

```yaml
server:
  domain: tunnel.example.com   # Required; used for subdomain routing
  control_port: 4443            # WebSocket control channel
  http_port: 80
  https_port: 443

tls:
  mode: auto                    # "auto" (Let's Encrypt), "manual", or "disabled"
  email: admin@example.com      # Required for auto mode
  cache_dir: ./certs
  staging: false                # Set true to test against LE staging

database:
  type: sqlite
  path: ./tunnelab.db

auth:
  required: true
  token_length: 32

tunnels:
  tcp_port_range: "10000-20000" # Range for TCP tunnel ports
  max_tunnels_per_client: 5
  max_connections_per_tunnel: 100
```

**Defaults applied when fields are absent:**
- `control_port`: 4443
- `http_port`: 80 / `https_port`: 443
- `database.path`: `./tunnelab.db`
- `tls.mode`: `disabled`
- `tls.cache_dir`: `./certs`
- `tunnels.tcp_port_range`: `30000-31000`
- `tunnels.max_tunnels_per_client`: 5
- `logging.level`: `info`, `logging.format`: `text`

---

## Database Schema

SQLite; auto-migrated by `NewRepository()`.

### `clients`
| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | UUID |
| `name` | TEXT | Human label |
| `api_token` | TEXT UNIQUE | Bcrypt-hashed token |
| `max_tunnels` | INTEGER | Default 5 |
| `allowed_subdomains` | TEXT | Comma-separated; nullable |
| `status` | TEXT | `active` / `inactive` |
| `created_at`, `updated_at` | TIMESTAMP | |

### `tunnels`
| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | UUID |
| `client_id` | TEXT FK | → clients(id) |
| `subdomain` | TEXT | Unique among active tunnels |
| `protocol` | TEXT | `http` / `tcp` |
| `local_port` | INTEGER | |
| `public_port` | INTEGER | TCP tunnels only |
| `public_url` | TEXT | Full public URL |
| `status` | TEXT | `active` / `closed` |
| `created_at`, `closed_at` | TIMESTAMP | |

### `connection_logs`
| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK AUTOINCREMENT | |
| `tunnel_id` | TEXT FK | → tunnels(id) |
| `client_ip`, `request_method`, `request_path` | TEXT | HTTP fields |
| `response_status`, `bytes_sent`, `bytes_received`, `duration_ms` | INTEGER | |
| `created_at` | TIMESTAMP | |

---

## Code Conventions

### Naming
- Package names: short, lowercase (`auth`, `proxy`, `registry`, `tls`)
- Exported symbols: `PascalCase` — e.g., `NewRegistry()`, `HandleWebSocket()`
- Unexported symbols: `camelCase` — e.g., `handleClient()`, `cleanupClient()`
- Variables: `camelCase` — e.g., `clientID`, `localPort`, `tunnelID`
- Protocol constants: SCREAMING_SNAKE or typed string constants — e.g., `MsgTypeAuth`

### Error Handling
- Wrap errors with context: `fmt.Errorf("description: %w", err)`
- Log errors immediately with `log.Printf()` before returning
- For WebSocket protocol errors, send a `MsgTypeError` message to the client before closing
- Hard-fail on startup errors (config missing, DB init failure, TLS setup failure)

### Concurrency
- All registry mutations require a write lock (`sync.RWMutex`)
- Reads use read locks
- Port allocator has its own `sync.Mutex`
- Bidirectional copies use `sync.WaitGroup` + 2 goroutines
- Each client WebSocket connection is handled in its own goroutine

### Logging
- Use standard `log` package (`log.Printf`)
- Format: `log.Printf("[component] action: %v", detail)`
- HTTP proxy logs: `log.Printf("[%s] %s %s -> %d (%d bytes, %v)", subdomain, method, path, status, bytes, duration)`
- Do not add structured fields — keep existing style consistent

### Testing
- Use standard `testing` package; no external test frameworks
- Use `t.Fatalf()` / `t.Fatal()` for assertion failures
- Test files live alongside source: `foo_test.go` next to `foo.go`
- Run: `make test` or `go test -v ./...`

**Current test coverage:**
- `internal/server/control/handler_test.go` — Port allocator (exhaustion + skip-used)
- `internal/server/registry/registry_test.go` — Registry lifecycle + duplicate rejection

---

## Adding New Features

### Adding a new protocol type
1. Add message type constants to `pkg/protocol/messages.go`
2. Add a handler case in `internal/server/control/handler.go:handleClient()`
3. Add registry support in `internal/server/registry/registry.go` if routing differs
4. Add proxy logic in `internal/server/proxy/` if needed
5. Update `cmd/test-client/main.go` with a reference implementation
6. Document in `docs/API_DOCUMENTATION.md`

### Adding a new config option
1. Add field to the appropriate struct in `internal/server/config/config.go`
2. Set default value in `config.go` before validation
3. Add the field to `configs/server.example.yaml` with a comment
4. Wire the value through `cmd/server/main.go` to the consuming component

### Adding a new DB operation
1. Add method to `internal/database/repository.go`
2. Add the model field/table to `internal/database/models.go` if needed
3. Update the `initSchema()` SQL in `repository.go` if adding a table/index

---

## Gitignored Files (Do Not Commit)

```
tunnelab-server          # compiled binary
*.db, *.db-shm, *.db-wal # SQLite files
configs/server.yaml      # runtime config (use server.example.yaml)
*.pem, *.key, *.crt      # TLS certificates
certs/                   # Let's Encrypt cache
*.log, logs/
.env, .env.*
dist/, build/
```

---

## Common Pitfalls

1. **CGO required** — `go-sqlite3` needs `CGO_ENABLED=1`. Cross-compilation requires a C toolchain for the target.
2. **Port range overlap** — TCP tunnel ports (`tcp_port_range`) must not overlap with `control_port`, `http_port`, or `https_port`.
3. **Subdomain uniqueness** — The DB enforces a partial unique index on `subdomain` for active tunnels. `CloseTunnel()` must be called before the same subdomain can be reused.
4. **Payload type casting** — JSON numbers deserialize as `float64` in `map[string]interface{}`. Always cast via `float64` first: `int(payload["port"].(float64))`.
5. **Mux session timing** — `waitForMuxConnection()` runs in a goroutine after `handleTunnelRequest()` returns. There is a brief window where the tunnel exists in the registry without a mux session; `OpenStream()` will return an error during this window.
6. **Token storage** — The database stores the bcrypt **hash**, not the plaintext token. The plaintext token is only available at generation time (from `generate-token.sh` output or `auth.GenerateToken()`).
7. **server.yaml not in repo** — `configs/server.yaml` is gitignored. Always document config changes in `configs/server.example.yaml`.

---

## Quick Reference: Key File Locations

| What | Where |
|------|-------|
| Protocol message types | `pkg/protocol/messages.go` |
| WebSocket message routing | `internal/server/control/handler.go` |
| In-memory tunnel registry | `internal/server/registry/registry.go` |
| HTTP reverse proxy | `internal/server/proxy/http.go` |
| TCP proxy | `internal/server/proxy/tcp.go` |
| Auth (token gen/verify) | `internal/server/auth/auth.go` |
| Config structs + loading | `internal/server/config/config.go` |
| DB models + repository | `internal/database/` |
| TLS / Let's Encrypt | `internal/server/tls/autocert.go` |
| Server startup + wiring | `cmd/server/main.go` |
| Reference client | `cmd/test-client/main.go` |
| Config template | `configs/server.example.yaml` |
| Technical design doc | `docs/TECHNICAL_DESIGN.md` |
| API / protocol reference | `docs/API_DOCUMENTATION.md` |
