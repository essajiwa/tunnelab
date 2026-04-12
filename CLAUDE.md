# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
make build                    # Build tunnelab-server binary (requires CGo for sqlite3)
make run                      # Build + run with configs/server.yaml
make test                     # go test -v ./...
go test -v -run TestName ./internal/server/registry/  # Single test
make clean                    # Remove binary + .db files
make generate-token           # Create client auth token in SQLite
```

## Architecture

TunneLab is a self-hosted tunneling server (ngrok-like). Exposes local servers to public internet via WebSocket control channel + yamux-multiplexed data streams.

**Entry point:** `cmd/server/main.go` — loads config, opens SQLite, creates registry, starts 3 HTTP servers (control :4443, HTTP proxy :80, HTTPS proxy :443).

**Data flow:**
- Public request → HTTP/TCP proxy → registry lookup by subdomain/port → yamux stream to connected client → client forwards to local service
- Client connects via WebSocket to control server → authenticates → requests tunnel → establishes yamux session for data streams

**Key packages:**
- `internal/server/control` — WebSocket control handler, message dispatch loop, port allocator for TCP range
- `internal/server/proxy` — HTTP (subdomain routing via `ServeHTTP`), TCP (port-based), UDP (length-prefixed datagrams, per-tunnel on-demand listeners via `SetUDPProxy`)
- `internal/server/registry` — Thread-safe in-memory tunnel store (sync.RWMutex), indexed by subdomain, client ID, and port
- `internal/database` — SQLite repository with auto-migration (clients, tunnels, connection_logs tables)
- `pkg/protocol` — Public protocol messages package (importable by external clients). JSON `ControlMessage` with Type, RequestID, Payload map
- `internal/server/config` — YAML config loading with defaults and validation
- `internal/server/tls` — Let's Encrypt autocert + manual cert loading
- `internal/server/auth` — Token gen/hash (exists but not used by control handler yet)

**Config:** `configs/server.yaml` (copy from `server.example.yaml`). Sections: server, tls, database, auth, logging, tunnels.

**Test client:** `cmd/test-client/main.go` — connects WS, authenticates, requests tunnel, accepts yamux streams, forwards to local port.

## Key Patterns

- No Go interfaces defined — all concrete structs
- Protocol handler uses `map[string]interface{}` payloads (typed config structs in `pkg/protocol` exist but aren't used in handler dispatch)
- Port allocator: round-robin across configured TCP range, skips registered ports
- Registry entries hold yamux session for opening streams to client

## Dependencies

CGo required (`go-sqlite3`). Key deps: gorilla/websocket, hashicorp/yamux, mattn/go-sqlite3, golang.org/x/crypto (bcrypt), gopkg.in/yaml.v3.
