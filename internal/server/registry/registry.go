// Package registry provides in-memory tunnel registry for TunneLab.
//
// This package manages active tunnels, their connections, and multiplexed sessions.
// It provides thread-safe operations for registering, unregistering, and accessing tunnels.
//
// Usage:
//
//	reg := NewRegistry()
//
//	// Register a tunnel
//	reg.Register(tunnelInfo)
//
//	// Get tunnel by subdomain
//	tunnel, exists := reg.GetBySubdomain("myapp")
//
//	// Open a stream to the tunnel
//	stream, err := reg.OpenStream("myapp")
package registry

import (
	"fmt"
	"net"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

// Registry manages active tunnels and their connections.
type Registry struct {
	mu      sync.RWMutex             // Mutex for thread-safe operations
	tunnels map[string]*TunnelInfo   // Map of subdomain to tunnel info
	clients map[string][]*TunnelInfo // Map of client ID to tunnel info
	ports   map[int]*TunnelInfo      // Map of public port to tunnel info
}

// TunnelInfo contains information about an active tunnel.
type TunnelInfo struct {
	ID           string          // Unique tunnel identifier
	ClientID     string          // ID of the owning client
	Subdomain    string          // Subdomain for public access
	Protocol     string          // Protocol type (http, tcp, etc.)
	LocalPort    int             // Local port to forward traffic to
	LocalHost    string          // Local host for tunneling
	PublicURL    string          // Public URL for the tunnel
	PublicPort   int             // Public port for the tunnel
	GRPCServices []string        // Allowed gRPC services
	MaxStreams   int             // Max concurrent gRPC streams
	ControlConn  *websocket.Conn // WebSocket connection
	MuxSession   *yamux.Session  // Yamux multiplexed session
}

// NewRegistry creates a new Registry instance.
//
// Returns:
//   - *Registry: A new registry ready to manage tunnels
func NewRegistry() *Registry {
	return &Registry{
		tunnels: make(map[string]*TunnelInfo),
		clients: make(map[string][]*TunnelInfo),
		ports:   make(map[int]*TunnelInfo),
	}
}

// Register registers a new tunnel in the registry.
//
// Parameters:
//   - tunnel: The tunnel information to register
//
// Returns:
//   - error: Error if the subdomain is already in use
func (r *Registry) Register(tunnel *TunnelInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tunnels[tunnel.Subdomain]; exists {
		return fmt.Errorf("subdomain %s is already in use", tunnel.Subdomain)
	}
	if tunnel.PublicPort > 0 {
		if _, exists := r.ports[tunnel.PublicPort]; exists {
			return fmt.Errorf("port %d is already in use", tunnel.PublicPort)
		}
		r.ports[tunnel.PublicPort] = tunnel
	}

	r.tunnels[tunnel.Subdomain] = tunnel
	r.clients[tunnel.ClientID] = append(r.clients[tunnel.ClientID], tunnel)

	return nil
}

// Unregister removes a tunnel from the registry by subdomain.
//
// Parameters:
//   - subdomain: The subdomain of the tunnel to remove
func (r *Registry) Unregister(subdomain string) {
	r.mu.Lock()
	tunnel, exists := r.tunnels[subdomain]
	if exists {
		delete(r.tunnels, subdomain)
		if tunnel.PublicPort > 0 {
			delete(r.ports, tunnel.PublicPort)
		}
		clientTunnels := r.clients[tunnel.ClientID]
		for i, t := range clientTunnels {
			if t.Subdomain == subdomain {
				r.clients[tunnel.ClientID] = append(clientTunnels[:i], clientTunnels[i+1:]...)
				break
			}
		}
	}
	r.mu.Unlock()

	if exists && tunnel.MuxSession != nil {
		tunnel.MuxSession.Close()
	}
}

// GetBySubdomain retrieves a tunnel by its subdomain.
//
// Parameters:
//   - subdomain: The subdomain of the tunnel to retrieve
//
// Returns:
//   - *TunnelInfo: The tunnel information, or nil if not found
//   - bool: Whether the tunnel was found
func (r *Registry) GetBySubdomain(subdomain string) (*TunnelInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tunnel, exists := r.tunnels[subdomain]
	return tunnel, exists
}

func (r *Registry) GetByClient(clientID string) []*TunnelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.clients[clientID]
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tunnels)
}

func (r *Registry) SetMuxSession(subdomain string, session *yamux.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tunnel, exists := r.tunnels[subdomain]
	if !exists {
		return fmt.Errorf("tunnel not found: %s", subdomain)
	}

	tunnel.MuxSession = session
	return nil
}

func (r *Registry) OpenStream(subdomain string) (net.Conn, error) {
	r.mu.RLock()
	tunnel, exists := r.tunnels[subdomain]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tunnel not found: %s", subdomain)
	}

	if tunnel.MuxSession == nil {
		return nil, fmt.Errorf("mux session not established for tunnel: %s", subdomain)
	}

	stream, err := tunnel.MuxSession.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	return stream, nil
}

// GetByPort retrieves tunnel info by public port.
func (r *Registry) GetByPort(port int) (*TunnelInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tunnel, exists := r.ports[port]
	return tunnel, exists
}
