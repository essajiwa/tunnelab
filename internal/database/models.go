// Package database provides data models and database operations for TunneLab.
//
// This package defines the database schema and models for clients, tunnels,
// and connection logs. It uses SQLite as the storage backend.
//
// Models:
//   - Client: Represents a client with authentication tokens
//   - Tunnel: Represents a tunnel configuration
//   - ConnectionLog: Logs tunnel connections and requests
//
// Usage:
//
//   repo, err := NewRepository("tunnelab.db")
//   if err != nil {
//       log.Fatal(err)
//   }
//
//   client, err := repo.GetClientByToken("token")
package database

import (
	"time"
)

// Client represents a client that can create tunnels.
type Client struct {
	ID                string    `db:"id"`                 // Unique client identifier
	Name              string    `db:"name"`               // Human-readable client name
	APIToken          string    `db:"api_token"`          // Authentication token
	MaxTunnels        int       `db:"max_tunnels"`        // Maximum tunnels allowed
	AllowedSubdomains string    `db:"allowed_subdomains"` // Comma-separated allowed subdomains
	CreatedAt         time.Time `db:"created_at"`         // Creation timestamp
	UpdatedAt         time.Time `db:"updated_at"`         // Last update timestamp
	Status            string    `db:"status"`             // Client status (active, inactive, etc.)
}

// Tunnel represents a tunnel configuration created by a client.
type Tunnel struct {
	ID         string    `db:"id"`         // Unique tunnel identifier
	ClientID   string    `db:"client_id"`  // ID of the owning client
	Subdomain  string    `db:"subdomain"`  // Subdomain for public access
	Protocol   string    `db:"protocol"`   // Protocol type (http, tcp, etc.)
	LocalPort  int       `db:"local_port"` // Local port to forward traffic to
	PublicPort int       `db:"public_port"`// Remote port for TCP tunnels
	PublicURL  string    `db:"public_url"` // Public URL for accessing the tunnel
	CreatedAt  time.Time `db:"created_at"` // Creation timestamp
	ClosedAt   *time.Time `db:"closed_at"` // Timestamp of tunnel closure
	Status     string    `db:"status"`     // Tunnel status (active, inactive, etc.)
}

// ConnectionLog represents a log entry for tunnel connections and requests.
type ConnectionLog struct {
	ID             int64     `db:"id"`          // Unique log entry identifier
	TunnelID       string    `db:"tunnel_id"`  // ID of the tunnel
	ClientIP       string    `db:"client_ip"`   // Client IP address
	RequestMethod  string    `db:"request_method"`      // HTTP method
	RequestPath    string    `db:"request_path"`        // Request path
	ResponseStatus int       `db:"response_status"` // HTTP response status code
	BytesSent      int64     `db:"bytes_sent"`    // Bytes sent
	BytesReceived  int64     `db:"bytes_received"`   // Bytes received
	DurationMs     int       `db:"duration_ms"`    // Request duration in milliseconds
	CreatedAt      time.Time `db:"created_at"`  // Timestamp of the request
}
