package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/essajiwa/tunnelab/internal/server/registry"
)

// TCPProxy forwards raw TCP connections to registered tunnels via yamux streams.
type TCPProxy struct {
	registry *registry.Registry
}

// NewTCPProxy creates a new TCP proxy.
func NewTCPProxy(reg *registry.Registry) *TCPProxy {
	return &TCPProxy{registry: reg}
}

// StartTCPServer starts listeners for the provided port range in the format "start-end".
func (p *TCPProxy) StartTCPServer(portRange string) error {
	start, end, err := parsePortRange(portRange)
	if err != nil {
		return err
	}

	for port := start; port <= end; port++ {
		go p.listenOnPort(port)
	}
	return nil
}

func (p *TCPProxy) listenOnPort(port int) {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("TCP proxy: failed to listen on %s: %v", addr, err)
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("TCP proxy: accept error on %s: %v", addr, err)
			continue
		}
		go p.handleConnection(conn, port)
	}
}

func (p *TCPProxy) handleConnection(conn net.Conn, port int) {
	defer conn.Close()

	tunnel, exists := p.registry.GetByPort(port)
	if !exists {
		log.Printf("TCP proxy: no tunnel registered on port %d", port)
		return
	}

	stream, err := p.registry.OpenStream(tunnel.Subdomain)
	if err != nil {
		log.Printf("TCP proxy: failed to open stream for %s: %v", tunnel.Subdomain, err)
		return
	}
	defer stream.Close()

	log.Printf("TCP proxy: forwarding connection on port %d to tunnel %s", port, tunnel.Subdomain)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(stream, conn)
		stream.Close()
	}()

	go func() {
		defer wg.Done()
		io.Copy(conn, stream)
		conn.Close()
	}()

	wg.Wait()
}

func parsePortRange(r string) (int, int, error) {
	parts := strings.Split(r, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid port range: %s", r)
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid port range start: %w", err)
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid port range end: %w", err)
	}
	if start <= 0 || end <= 0 || end < start {
		return 0, 0, fmt.Errorf("invalid port range values: %d-%d", start, end)
	}
	return start, end, nil
}
