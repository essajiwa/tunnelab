package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/essajiwa/tunnelab/internal/server/registry"
	"github.com/essajiwa/tunnelab/pkg/protocol"
)

// UDPProxy forwards UDP packets from a public port to the client's local UDP
// port via the tunnel's yamux stream. Each UDP datagram is length-prefixed
// (4-byte big-endian uint32) so the stream receiver can reconstruct individual
// packets.
type UDPProxy struct {
	registry  *registry.Registry
	mu        sync.Mutex
	listeners map[string]net.PacketConn
}

// NewUDPProxy creates a new UDPProxy.
func NewUDPProxy(reg *registry.Registry) *UDPProxy {
	return &UDPProxy{
		registry:  reg,
		listeners: make(map[string]net.PacketConn),
	}
}

// ListenAndForward binds a UDP socket on the given port and forwards all
// incoming datagrams to the tunnel identified by subdomain.
func (p *UDPProxy) ListenAndForward(port int, subdomain string) error {
	addr := fmt.Sprintf(":%d", port)
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("udp proxy: failed to listen on %s: %w", addr, err)
	}

	p.mu.Lock()
	p.listeners[subdomain] = conn
	p.mu.Unlock()

	go p.serve(conn, port, subdomain)
	return nil
}

// StopForwarding closes the UDP listener for the given subdomain.
func (p *UDPProxy) StopForwarding(subdomain string) {
	p.mu.Lock()
	conn, ok := p.listeners[subdomain]
	if ok {
		delete(p.listeners, subdomain)
	}
	p.mu.Unlock()

	if ok {
		conn.Close()
	}
}

// Close stops all active UDP listeners.
func (p *UDPProxy) Close() {
	p.mu.Lock()
	conns := make([]net.PacketConn, 0, len(p.listeners))
	for _, conn := range p.listeners {
		conns = append(conns, conn)
	}
	p.listeners = make(map[string]net.PacketConn)
	p.mu.Unlock()

	for _, conn := range conns {
		conn.Close()
	}
}

func (p *UDPProxy) serve(conn net.PacketConn, port int, subdomain string) {
	defer conn.Close()
	buf := make([]byte, 65535)

	for {
		n, clientAddr, err := conn.ReadFrom(buf)
		if err != nil {
			log.Printf("UDP proxy: read error on port %d: %v", port, err)
			return
		}
		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		go p.forwardPacket(conn, clientAddr, pkt, subdomain)
	}
}

func (p *UDPProxy) forwardPacket(conn net.PacketConn, clientAddr net.Addr, pkt []byte, subdomain string) {
	stream, err := p.registry.OpenStream(subdomain)
	if err != nil {
		log.Printf("UDP proxy: failed to open stream for %s: %v", subdomain, err)
		return
	}
	defer stream.Close()

	if err := protocol.WriteLengthPrefixed(stream, pkt); err != nil {
		log.Printf("UDP proxy: failed to write to stream for %s: %v", subdomain, err)
		return
	}

	resp, err := protocol.ReadLengthPrefixed(stream)
	if err != nil {
		if err != io.EOF {
			log.Printf("UDP proxy: failed to read response for %s: %v", subdomain, err)
		}
		return
	}

	if _, err := conn.WriteTo(resp, clientAddr); err != nil {
		log.Printf("UDP proxy: failed to write response to client %s: %v", clientAddr, err)
	}
}
