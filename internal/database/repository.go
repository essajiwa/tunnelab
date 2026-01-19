// Package database provides database operations for TunneLab.
//
// This package implements the Repository pattern for SQLite database operations
// including client management, tunnel configuration, and connection logging.
//
// Example:
//
//	repo, err := NewRepository("tunnelab.db")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer repo.Close()
//
//	client, err := repo.GetClientByToken("token")
//	if err != nil {
//	    log.Fatal(err)
//	}
package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Repository provides database operations for TunneLab data.
type Repository struct {
	db *sql.DB // SQLite database connection
}

// NewRepository creates a new Repository instance with the specified database path.
//
// It opens the database, verifies connectivity, and runs migrations if needed.
//
// Parameters:
//   - dbPath: Path to the SQLite database file
//
// Returns:
//   - *Repository: Repository instance
//   - error: Error if database cannot be opened or migrated
func NewRepository(dbPath string) (*Repository, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	repo := &Repository{db: db}
	if err := repo.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return repo, nil
}

func (r *Repository) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS clients (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		api_token TEXT NOT NULL UNIQUE,
		max_tunnels INTEGER DEFAULT 5,
		allowed_subdomains TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		status TEXT DEFAULT 'active'
	);

	CREATE TABLE IF NOT EXISTS tunnels (
		id TEXT PRIMARY KEY,
		client_id TEXT NOT NULL,
		subdomain TEXT,
		protocol TEXT NOT NULL,
		local_port INTEGER NOT NULL,
		public_port INTEGER,
		public_url TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		closed_at TIMESTAMP,
		status TEXT DEFAULT 'active',
		FOREIGN KEY (client_id) REFERENCES clients(id)
	);

	CREATE UNIQUE INDEX IF NOT EXISTS idx_tunnels_subdomain ON tunnels(subdomain) WHERE status = 'active';
	CREATE INDEX IF NOT EXISTS idx_tunnels_client_id ON tunnels(client_id);
	CREATE INDEX IF NOT EXISTS idx_tunnels_status ON tunnels(status);

	CREATE TABLE IF NOT EXISTS connection_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tunnel_id TEXT NOT NULL,
		client_ip TEXT,
		request_method TEXT,
		request_path TEXT,
		response_status INTEGER,
		bytes_sent INTEGER,
		bytes_received INTEGER,
		duration_ms INTEGER,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (tunnel_id) REFERENCES tunnels(id)
	);

	CREATE INDEX IF NOT EXISTS idx_connection_logs_tunnel_id ON connection_logs(tunnel_id);
	CREATE INDEX IF NOT EXISTS idx_connection_logs_created_at ON connection_logs(created_at);
	`

	_, err := r.db.Exec(schema)
	return err
}

// GetClientByToken retrieves a client by their API token.
//
// Parameters:
//   - token: The API token to look up
//
// Returns:
//   - *Client: The client if found and active
//   - error: Database error if any
//   - nil, nil: If token not found (not an error)
func (r *Repository) GetClientByToken(token string) (*Client, error) {
	var client Client
	var allowedSubdomains sql.NullString
	err := r.db.QueryRow(`
		SELECT id, name, api_token, max_tunnels, allowed_subdomains, created_at, updated_at, status
		FROM clients WHERE api_token = ? AND status = 'active'
	`, token).Scan(
		&client.ID, &client.Name, &client.APIToken, &client.MaxTunnels,
		&allowedSubdomains, &client.CreatedAt, &client.UpdatedAt, &client.Status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if allowedSubdomains.Valid {
		client.AllowedSubdomains = allowedSubdomains.String
	}
	return &client, nil
}

// CreateClient creates a new client in the database.
//
// Parameters:
//   - client: The client to create
//
// Returns:
//   - error: Database error if any
func (r *Repository) CreateClient(client *Client) error {
	_, err := r.db.Exec(`
		INSERT INTO clients (id, name, api_token, max_tunnels, allowed_subdomains, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, client.ID, client.Name, client.APIToken, client.MaxTunnels, client.AllowedSubdomains, client.Status)
	return err
}

func (r *Repository) CreateTunnel(tunnel *Tunnel) error {
	_, err := r.db.Exec(`
		INSERT INTO tunnels (id, client_id, subdomain, protocol, local_port, public_port, public_url, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, tunnel.ID, tunnel.ClientID, tunnel.Subdomain, tunnel.Protocol, tunnel.LocalPort, tunnel.PublicPort, tunnel.PublicURL, tunnel.Status)
	return err
}

func (r *Repository) GetTunnelBySubdomain(subdomain string) (*Tunnel, error) {
	var tunnel Tunnel
	var closedAt sql.NullTime
	err := r.db.QueryRow(`
		SELECT id, client_id, subdomain, protocol, local_port, public_port, public_url, created_at, closed_at, status
		FROM tunnels WHERE subdomain = ? AND status = 'active'
	`, subdomain).Scan(
		&tunnel.ID, &tunnel.ClientID, &tunnel.Subdomain, &tunnel.Protocol,
		&tunnel.LocalPort, &tunnel.PublicPort, &tunnel.PublicURL,
		&tunnel.CreatedAt, &closedAt, &tunnel.Status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if closedAt.Valid {
		tunnel.ClosedAt = &closedAt.Time
	}
	return &tunnel, nil
}

func (r *Repository) CloseTunnel(tunnelID string) error {
	now := time.Now()
	_, err := r.db.Exec(`
		UPDATE tunnels SET status = 'closed', closed_at = ? WHERE id = ?
	`, now, tunnelID)
	return err
}

func (r *Repository) GetActiveTunnelsByClient(clientID string) ([]*Tunnel, error) {
	rows, err := r.db.Query(`
		SELECT id, client_id, subdomain, protocol, local_port, public_port, public_url, created_at, closed_at, status
		FROM tunnels WHERE client_id = ? AND status = 'active'
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tunnels []*Tunnel
	for rows.Next() {
		var tunnel Tunnel
		var closedAt sql.NullTime
		if err := rows.Scan(
			&tunnel.ID, &tunnel.ClientID, &tunnel.Subdomain, &tunnel.Protocol,
			&tunnel.LocalPort, &tunnel.PublicPort, &tunnel.PublicURL,
			&tunnel.CreatedAt, &closedAt, &tunnel.Status,
		); err != nil {
			return nil, err
		}
		if closedAt.Valid {
			tunnel.ClosedAt = &closedAt.Time
		}
		tunnels = append(tunnels, &tunnel)
	}
	return tunnels, rows.Err()
}

func (r *Repository) Close() error {
	return r.db.Close()
}
