package control

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/essajiwa/tunnelab/internal/database"
	"github.com/essajiwa/tunnelab/internal/server/registry"
	"github.com/essajiwa/tunnelab/pkg/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func ensureProtocolType(msg *protocol.ControlMessage, proto string) {
	if msg.Payload == nil {
		msg.Payload = make(map[string]interface{})
	}
	msg.Payload["protocol"] = proto
}

func (h *Handler) assignPublicPort(payload map[string]interface{}) (int, error) {
	if value, ok := payload["public_port"].(float64); ok && value > 0 {
		port := int(value)
		if _, exists := h.registry.GetByPort(port); exists {
			return 0, fmt.Errorf("port %d already in use", port)
		}
		return port, nil
	}
	if h.portAllocator == nil {
		return 0, fmt.Errorf("tcp tunneling not enabled")
	}
	return h.portAllocator.allocate(h.registry)
}

type portAllocator struct {
	start int
	end   int
	next  int
	mu    sync.Mutex
}

func (a *portAllocator) allocate(reg *registry.Registry) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	rangeSize := a.end - a.start + 1
	if rangeSize <= 0 {
		return 0, fmt.Errorf("invalid port range")
	}

	for i := 0; i < rangeSize; i++ {
		candidate := a.start + ((a.next - a.start + i + rangeSize) % rangeSize)
		if _, exists := reg.GetByPort(candidate); !exists {
			a.next = candidate + 1
			if a.next > a.end {
				a.next = a.start
			}
			return candidate, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", a.start, a.end)
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

type Handler struct {
	registry      *registry.Registry
	repo          *database.Repository
	domain        string
	portAllocator *portAllocator
}

func NewHandler(registry *registry.Registry, repo *database.Repository, domain string) *Handler {
	return &Handler{
		registry: registry,
		repo:     repo,
		domain:   domain,
	}
}

// ConfigurePortAllocator enables automatic public-port assignment for TCP/gRPC tunnels.
func (h *Handler) ConfigurePortAllocator(portRange string) error {
	if portRange == "" {
		h.portAllocator = nil
		return nil
	}
	start, end, err := parsePortRange(portRange)
	if err != nil {
		return err
	}
	h.portAllocator = &portAllocator{start: start, end: end, next: start}
	return nil
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	clientID, authenticated := h.authenticate(conn)
	if !authenticated {
		return
	}

	log.Printf("Client %s authenticated successfully", clientID)

	h.handleClient(conn, clientID)
}

func (h *Handler) authenticate(conn *websocket.Conn) (string, bool) {
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	var msg protocol.ControlMessage
	if err := conn.ReadJSON(&msg); err != nil {
		log.Printf("Failed to read auth message: %v", err)
		return "", false
	}

	if msg.Type != protocol.MsgTypeAuth {
		h.sendError(conn, msg.RequestID, "INVALID_MESSAGE", "Expected auth message")
		return "", false
	}

	token, ok := msg.Payload["token"].(string)
	if !ok || token == "" {
		h.sendError(conn, msg.RequestID, "INVALID_TOKEN", "Token is required")
		return "", false
	}

	client, err := h.repo.GetClientByToken(token)
	if err != nil {
		log.Printf("Database error: %v", err)
		h.sendError(conn, msg.RequestID, "AUTH_FAILED", "Authentication failed")
		return "", false
	}

	if client == nil {
		h.sendError(conn, msg.RequestID, "AUTH_FAILED", "Invalid token")
		return "", false
	}

	response := protocol.NewControlMessage(
		protocol.MsgTypeAuthResponse,
		msg.RequestID,
		map[string]interface{}{
			"success":   true,
			"client_id": client.ID,
		},
	)

	if err := conn.WriteJSON(response); err != nil {
		log.Printf("Failed to send auth response: %v", err)
		return "", false
	}

	conn.SetReadDeadline(time.Time{})
	return client.ID, true
}

func (h *Handler) handleClient(conn *websocket.Conn, clientID string) {
	for {
		var msg protocol.ControlMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("Client %s disconnected: %v", clientID, err)
			h.cleanupClient(clientID)
			return
		}

		switch msg.Type {
		case protocol.MsgTypeTunnelReq:
			h.handleTunnelRequest(conn, clientID, &msg)
		case protocol.MsgTypeTCPReq:
			ensureProtocolType(&msg, "tcp")
			h.handleTunnelRequest(conn, clientID, &msg)
		case protocol.MsgTypeGRPCReq:
			ensureProtocolType(&msg, "grpc")
			h.handleTunnelRequest(conn, clientID, &msg)
		case protocol.MsgTypeHeartbeat:
			h.handleHeartbeat(conn, &msg)
		default:
			log.Printf("Unknown message type: %s", msg.Type)
		}
	}
}

func (h *Handler) handleTunnelRequest(conn *websocket.Conn, clientID string, msg *protocol.ControlMessage) {
	subdomain, _ := msg.Payload["subdomain"].(string)
	protocolType, _ := msg.Payload["protocol"].(string)
	protocolType = strings.ToLower(protocolType)
	localPort, _ := msg.Payload["local_port"].(float64)
	localHost, _ := msg.Payload["local_host"].(string)
	if localHost == "" {
		localHost = "localhost"
	}

	if subdomain == "" || protocolType == "" || localPort == 0 {
		h.sendError(conn, msg.RequestID, "INVALID_REQUEST", "Missing required fields")
		return
	}

	existing, _ := h.repo.GetTunnelBySubdomain(subdomain)
	if existing != nil {
		h.sendError(conn, msg.RequestID, "SUBDOMAIN_TAKEN", fmt.Sprintf("Subdomain %s is already in use", subdomain))
		return
	}

	tunnelID := uuid.New().String()
	var publicURL string
	var publicPort int
	switch protocolType {
	case "http", "https":
		publicURL = fmt.Sprintf("https://%s.%s", subdomain, h.domain)
	default:
		var err error
		publicPort, err = h.assignPublicPort(msg.Payload)
		if err != nil {
			h.sendError(conn, msg.RequestID, "PORT_ALLOCATION_FAILED", err.Error())
			return
		}
	}

	tunnel := &database.Tunnel{
		ID:         tunnelID,
		ClientID:   clientID,
		Subdomain:  subdomain,
		Protocol:   protocolType,
		LocalPort:  int(localPort),
		PublicURL:  publicURL,
		PublicPort: publicPort,
		Status:     "active",
	}

	if err := h.repo.CreateTunnel(tunnel); err != nil {
		log.Printf("Failed to create tunnel in database: %v", err)
		h.sendError(conn, msg.RequestID, "INTERNAL_ERROR", "Failed to create tunnel")
		return
	}

	tunnelInfo := &registry.TunnelInfo{
		ID:          tunnelID,
		ClientID:    clientID,
		Subdomain:   subdomain,
		Protocol:    protocolType,
		LocalPort:   int(localPort),
		LocalHost:   localHost,
		PublicURL:   publicURL,
		PublicPort:  publicPort,
		ControlConn: conn,
	}

	if err := h.registry.Register(tunnelInfo); err != nil {
		h.repo.CloseTunnel(tunnelID)
		h.sendError(conn, msg.RequestID, "REGISTRATION_FAILED", err.Error())
		return
	}

	go h.waitForMuxConnection(tunnelInfo)

	respPayload := map[string]interface{}{
		"tunnel_id": tunnelID,
		"status":    "active",
	}
	if publicURL != "" {
		respPayload["public_url"] = publicURL
	}
	if publicPort > 0 {
		respPayload["public_port"] = publicPort
	}

	responseType := protocol.MsgTypeTunnelResp
	switch protocolType {
	case "tcp":
		responseType = protocol.MsgTypeTCPResp
	case "grpc":
		responseType = protocol.MsgTypeGRPCResp
	}

	response := protocol.NewControlMessage(
		responseType,
		msg.RequestID,
		respPayload,
	)

	if err := conn.WriteJSON(response); err != nil {
		log.Printf("Failed to send tunnel response: %v", err)
		h.registry.Unregister(subdomain)
		h.repo.CloseTunnel(tunnelID)
	}

	if publicPort > 0 {
		log.Printf("Tunnel created: port %d -> %s (client: %s)", publicPort, subdomain, clientID)
	} else {
		log.Printf("Tunnel created: %s -> %s (client: %s)", publicURL, subdomain, clientID)
	}
}

func (h *Handler) waitForMuxConnection(tunnel *registry.TunnelInfo) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Printf("Failed to create listener for mux: %v", err)
		return
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	msg := protocol.NewControlMessage(
		protocol.MsgTypeNewConn,
		uuid.New().String(),
		map[string]interface{}{
			"action":    "establish_mux",
			"tunnel_id": tunnel.ID,
			"mux_port":  port,
			"mux_addr":  fmt.Sprintf(":%d", port),
		},
	)

	if err := tunnel.ControlConn.WriteJSON(msg); err != nil {
		log.Printf("Failed to send mux establishment message: %v", err)
		return
	}

	listener.(*net.TCPListener).SetDeadline(time.Now().Add(30 * time.Second))

	conn, err := listener.Accept()
	if err != nil {
		log.Printf("Failed to accept mux connection: %v", err)
		return
	}

	session, err := yamux.Server(conn, nil)
	if err != nil {
		log.Printf("Failed to create yamux session: %v", err)
		conn.Close()
		return
	}

	if err := h.registry.SetMuxSession(tunnel.Subdomain, session); err != nil {
		log.Printf("Failed to set mux session: %v", err)
		session.Close()
		return
	}

	log.Printf("Mux session established for tunnel: %s", tunnel.Subdomain)
}

func (h *Handler) handleHeartbeat(conn *websocket.Conn, msg *protocol.ControlMessage) {
	response := protocol.NewControlMessage(
		protocol.MsgTypeHeartbeat,
		msg.RequestID,
		map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	)
	conn.WriteJSON(response)
}

func (h *Handler) sendError(conn *websocket.Conn, requestID, code, message string) {
	errMsg := protocol.NewErrorMessage(requestID, code, message)
	if err := conn.WriteJSON(errMsg); err != nil {
		log.Printf("Failed to send error message: %v", err)
	}
}

func (h *Handler) cleanupClient(clientID string) {
	tunnels := h.registry.GetByClient(clientID)
	for _, tunnel := range tunnels {
		h.registry.Unregister(tunnel.Subdomain)
		h.repo.CloseTunnel(tunnel.ID)
		log.Printf("Cleaned up tunnel: %s", tunnel.Subdomain)
	}
}

func (h *Handler) MarshalPayload(payload interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}
