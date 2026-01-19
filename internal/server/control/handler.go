package control

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
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

type Handler struct {
	registry *registry.Registry
	repo     *database.Repository
	domain   string
}

func NewHandler(registry *registry.Registry, repo *database.Repository, domain string) *Handler {
	return &Handler{
		registry: registry,
		repo:     repo,
		domain:   domain,
	}
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
	localPort, _ := msg.Payload["local_port"].(float64)

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
	publicURL := fmt.Sprintf("https://%s.%s", subdomain, h.domain)

	tunnel := &database.Tunnel{
		ID:        tunnelID,
		ClientID:  clientID,
		Subdomain: subdomain,
		Protocol:  protocolType,
		LocalPort: int(localPort),
		PublicURL: publicURL,
		Status:    "active",
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
		PublicURL:   publicURL,
		ControlConn: conn,
	}

	if err := h.registry.Register(tunnelInfo); err != nil {
		h.repo.CloseTunnel(tunnelID)
		h.sendError(conn, msg.RequestID, "REGISTRATION_FAILED", err.Error())
		return
	}

	go h.waitForMuxConnection(tunnelInfo)

	response := protocol.NewControlMessage(
		protocol.MsgTypeTunnelResp,
		msg.RequestID,
		map[string]interface{}{
			"tunnel_id":  tunnelID,
			"public_url": publicURL,
			"status":     "active",
		},
	)

	if err := conn.WriteJSON(response); err != nil {
		log.Printf("Failed to send tunnel response: %v", err)
		h.registry.Unregister(subdomain)
		h.repo.CloseTunnel(tunnelID)
	}

	log.Printf("Tunnel created: %s -> %s (client: %s)", publicURL, subdomain, clientID)
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
