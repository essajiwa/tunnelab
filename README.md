# TunneLab

[![Go Report Card](https://goreportcard.com/badge/github.com/essajiwa/tunnelab)](https://goreportcard.com/report/github.com/essajiwa/tunnelab)
[![Go Reference](https://pkg.go.dev/badge/github.com/essajiwa/tunnelab.svg)](https://pkg.go.dev/github.com/essajiwa/tunnelab)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/dl/)

A self-hosted tunneling server that exposes local servers to the public internet, similar to ngrok.

## Overview

**TunneLab** is a self-hosted tunneling server that exposes local servers to the public internet, similar to ngrok. It provides the infrastructure for creating secure tunnels from your local development environment to the public internet.

### Architecture

- **TunneLab** (this repository): Server-side infrastructure
  - Control server for tunnel management
  - HTTP/HTTPS reverse proxy with automatic TLS
  - Authentication and authorization
  - Tunnel registry with SQLite persistence
  - WebSocket-based control connections

- **Client**: Any client implementing the TunneLab protocol
  - Test client included in `cmd/test-client/`
  - Protocol package available in `pkg/protocol/`
  - WebSocket control connection and yamux multiplexing

## Features

- ✅ **HTTP/HTTPS Tunneling** - Expose local web servers with custom subdomains
- ✅ **Automatic HTTPS** - Let's Encrypt certificate generation and renewal
- ✅ **TCP Tunneling** - Expose any TCP service via assigned public ports
- ⚙️ **gRPC over TCP** - Run gRPC services through raw TCP tunnels (client handles TLS)
- ✅ **WebSocket Support** - Full WebSocket upgrade support
- ✅ **TLS Termination** - Automatic HTTPS with Let's Encrypt or manual certificates
- ✅ **Multi-client Support** - Handle multiple clients and tunnels simultaneously
- ✅ **Authentication** - Token-based authentication
- ✅ **Monitoring** - Health checks and request logging
- ✅ **Auto-reconnection** - Clients automatically reconnect on connection loss

## Quick Start

### Prerequisites

**System Requirements:**
- Go 1.21 or higher
- Linux/macOS/Windows (tested on Linux)
- Public server with domain and static IP
- SQLite3 client tools (for database management)

**Install Dependencies:**

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install golang sqlite3

# CentOS/RHEL/Fedora
sudo yum install golang sqlite
# OR for newer versions
sudo dnf install golang sqlite

# macOS (using Homebrew)
brew install go sqlite

# Windows (using Chocolatey)
choco install golang sqlite
```

**Port Requirements:**
- Port 80 - HTTP traffic
- Port 443 - HTTPS traffic  
- Port 4443 - Control server (WebSocket)

**Network Requirements:**
- Public IP address
- Domain name with DNS control
- Firewall rules allowing the above ports

### Installation

```bash
# Clone the repository
git clone https://github.com/essajiwa/tunnelab.git
cd tunnelab

# Build the server
go build -o tunnelab-server ./cmd/server

# Create configuration
cp configs/server.example.yaml configs/server.yaml
# Edit configs/server.yaml with your settings

# Run the server (database is automatically created on first start)
./tunnelab-server -config configs/server.yaml
```

### Verify Installation

```bash
# Check Go version
go version

# Check SQLite3
sqlite3 --version

# Verify build
ls -la tunnelab-server

# Test database
sqlite3 tunnelab.db ".tables"
```

### DNS Configuration

Configure your DNS with the following records:

```
yourdomain.com          A    YOUR_PUBLIC_IP
control.yourdomain.com  A    YOUR_PUBLIC_IP
*.yourdomain.com        A    YOUR_PUBLIC_IP
```

### Generate Client Token

```bash
./scripts/generate-token.sh
```

Save this token for use with the test client or any client that leverages TunneLab.

## Configuration

Example server configuration:

```yaml
server:
  domain: "yourdomain.com"
  control_port: 4443
  http_port: 80
  https_port: 443

tls:
  mode: "auto"              # Let's Encrypt automatic HTTPS
  email: "admin@yourdomain.com"
  cache_dir: "./certs"
  staging: false            # Use production (true for testing)

database:
  type: "sqlite"
  path: "/var/lib/tunnelab/tunnelab.db"

auth:
  required: true

logging:
  level: "info"
  format: "json"
```

See [docs/USAGE.md](docs/USAGE.md) for full configuration options.

## Client Usage

Use the included test client to create tunnels:

```bash
# Build test client
go build -o test-client ./cmd/test-client

# Start a tunnel
./test-client -server ws://localhost:4443 -token YOUR_TOKEN -subdomain myapp -port 3000

# Output:
# Tunnel started: https://myapp.yourdomain.com
# Forwarding to: localhost:3000
```

## Architecture

```
┌─────────────┐    HTTPS    ┌─────────────────┐    WebSocket   ┌─────────────┐
│ Public      │ ──────────→ │ TunneLab        │ ─────────────→ │ Client      │
│ Client      │             │ Server          │                │             │
│ Browser     │             │                 │                │             │
└─────────────┘             └─────────────────┘                └─────────────┘
     │                           │                                  │
     │ myapp.yourdomain.com      │ Control: :4443                   │
     │                           │ Proxy: :80/:443                  │
     │                           │                                  │ Local HTTP
     │                           │ Yamux Stream (TCP) ─────────────→│ Server
     │                           │                                  │ :3000
     │                           │                                  │
     │←───────────────────────── Response ──────────────────────────│
```

## Protocol

TunneLab uses a WebSocket-based control channel and multiplexed data streams:

1. **Control Channel**: WebSocket connection for tunnel management
2. **Data Channel**: Multiplexed TCP streams (using yamux) for actual traffic
3. **Message Format**: JSON-encoded control messages

See [docs/API_DOCUMENTATION.md](docs/API_DOCUMENTATION.md#pkgprotocol-package) for the full protocol specification.

## Development

### Project Structure

```
tunnelab/
├── cmd/
│   ├── server/           # Server entry point
│   └── test-client/      # Test client implementation
├── internal/
│   ├── database/         # Database models and operations
│   └── server/           # Server implementation
│       ├── auth/         # Authentication service
│       ├── config/       # Configuration management
│       ├── control/      # WebSocket control handler
│       ├── proxy/        # HTTP reverse proxy
│       ├── registry/     # Tunnel registry
│       └── tls/          # TLS certificate management
├── pkg/
│   └── protocol/        # Public protocol package (for clients)
├── configs/              # Configuration files
│   └── server.example.yaml
├── scripts/              # Utility scripts
│   ├── setup.sh
│   ├── generate-token.sh
│   └── test-tunnel.sh
├── docs/                 # Documentation
│   ├── API_DOCUMENTATION.md
│   ├── LETSENCRYPT.md
│   ├── QUICKSTART.md
│   └── TESTING.md
├── Makefile              # Build automation
├── go.mod                # Go module definition
└── README.md             # This file
```

### Building

```bash
# Build using Makefile (recommended)
make build

# Build manually
go build -o tunnelab-server ./cmd/server

# Build for Linux
GOOS=linux GOARCH=amd64 go build -o tunnelab-server-linux ./cmd/server

# Build with version info
go build -ldflags "-X main.version=1.0.0" -o tunnelab-server ./cmd/server
```

## Configuration

The server is configured via YAML file. See `configs/server.example.yaml` for a complete example:

```bash
# Copy example configuration
cp configs/server.example.yaml configs/server.yaml

# Edit configuration
nano configs/server.yaml
```

## Monitoring

### Health Checks

```bash
# Basic health check
curl http://localhost/health
```

### Security

- **Authentication**: Token-based authentication required
- **TLS**: Automatic HTTPS with Let's Encrypt or manual certificates
- **Database**: SQLite for tunnel persistence and client management

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly
5. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Documentation

- [Technical Design](docs/TECHNICAL_DESIGN.md) - Complete system design
- [Let's Encrypt Setup](docs/LETSENCRYPT.md) - HTTPS with automatic certificates
- [Quick Start](docs/QUICKSTART.md) - Get started in 5 minutes
- [Usage Guide](docs/USAGE.md) - Detailed usage instructions
- [API Documentation](docs/API_DOCUMENTATION.md) - Protocol and API reference
- [Implementation Status](docs/IMPLEMENTATION_STATUS.md) - Current implementation status

## Contributing

Contributions are welcome! Please fork the repository and submit a pull request.

## Related Projects
- [frp](https://github.com/fatedier/frp) - Inspiration for architecture
- [ngrok](https://ngrok.com) - Commercial alternative

## Roadmap

- [x] Core HTTP tunneling
- [x] WebSocket support
- [x] Let's Encrypt integration (automatic HTTPS)
- [x] Manual certificate support
- [x] TCP tunneling
- [ ] Enhanced gRPC controls (service allowlists, TLS enforcement)
- [ ] UDP tunneling
- [ ] Web dashboard
- [ ] Custom domain support (BYOD)
- [ ] Load balancing
- [ ] Geographic routing
- [ ] Plugin system

## Support

- **Issues**: [GitHub Issues](https://github.com/essajiwa/tunnelab/issues)
- **Discussions**: [GitHub Discussions](https://github.com/essajiwa/tunnelab/discussions)
- **Email**: support@yourdomain.com

## Acknowledgments

- Inspired by [frp](https://github.com/fatedier/frp) and [ngrok](https://ngrok.com)
- Uses [yamux](https://github.com/hashicorp/yamux) for stream multiplexing
- Uses [gorilla/websocket](https://github.com/gorilla/websocket) for WebSocket support

---

**Made with ❤️ for developers who need to expose their local servers**
