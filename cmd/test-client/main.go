// Test client is a minimal tunnel client for testing TunneLab server.
//
// This client simulates client behavior for testing purposes, similar to how
// hooklab (an open-source project) leverages TunneLab for tunneling services.
// It connects to the control server, authenticates, creates a tunnel,
// and forwards HTTP requests to a local server.
//
// Usage:
//
//	./test-client -server ws://localhost:4443 -token TOKEN -subdomain test -port 8000
//
// Flags:
//
//	-server: Control server WebSocket URL (default: ws://localhost:4443)
//	-token: Authentication token (required)
//	-subdomain: Subdomain for the tunnel (default: test)
//	-port: Local port to forward traffic to (default: 8000)
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/essajiwa/tunnelab/pkg/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

// main is the entry point for the test client.
func main() {
	config := parseFlags()

	if err := validateConfig(config); err != nil {
		log.Fatal(err)
	}

	conn := connectToServer(config.ServerURL)
	defer conn.Close()

	if err := authenticate(conn, config.Token); err != nil {
		log.Fatal(err)
	}

	tunnelInfo := createTunnel(conn, config)

	muxSession := establishMuxSession(conn)
	defer muxSession.Close()

	if tunnelInfo.PublicURL != "" {
		log.Printf("\n🎉 Tunnel is ready! Access your local server at: %s\n", tunnelInfo.PublicURL)
	} else {
		log.Printf("\n🎉 Tunnel is ready! Public port: %d\n", tunnelInfo.PublicPort)
	}
	log.Printf("Press Ctrl+C to stop\n")

	go handleHeartbeat(conn)
	runTunnelLoop(muxSession, config.LocalHost, config.LocalPort)
}

type Config struct {
	ServerURL string
	Token     string
	Subdomain string
	LocalPort int
	LocalHost string
	Protocol  string
}

func parseFlags() *Config {
	serverURL := flag.String("server", "ws://localhost:4443", "Control server URL")
	token := flag.String("token", "", "Authentication token")
	subdomain := flag.String("subdomain", "test", "Subdomain to use")
	localPort := flag.Int("port", 8000, "Local port to forward")
	localHost := flag.String("local-host", "localhost", "Local host to forward (default: localhost)")
	protocol := flag.String("protocol", "http", "Protocol to tunnel (http|tcp|grpc)")
	flag.Parse()

	return &Config{
		ServerURL: *serverURL,
		Token:     *token,
		Subdomain: *subdomain,
		LocalPort: *localPort,
		LocalHost: *localHost,
		Protocol:  strings.ToLower(*protocol),
	}
}

func validateConfig(config *Config) error {
	if config.Token == "" {
		return fmt.Errorf("token is required. Use -token flag")
	}
	switch config.Protocol {
	case "http", "tcp", "grpc":
	default:
		return fmt.Errorf("unsupported protocol %q (use http, tcp, or grpc)", config.Protocol)
	}
	return nil
}

func connectToServer(serverURL string) *websocket.Conn {
	log.Printf("Connecting to %s", serverURL)
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	return conn
}

func authenticate(conn *websocket.Conn, token string) error {
	log.Println("Authenticating...")
	authMsg := protocol.NewControlMessage(
		protocol.MsgTypeAuth,
		uuid.New().String(),
		map[string]interface{}{
			"token": token,
		},
	)

	if err := conn.WriteJSON(authMsg); err != nil {
		return fmt.Errorf("failed to send auth: %v", err)
	}

	var authResp protocol.ControlMessage
	if err := conn.ReadJSON(&authResp); err != nil {
		return fmt.Errorf("failed to read auth response: %v", err)
	}

	if authResp.Type == protocol.MsgTypeError {
		msg, _ := authResp.Payload["message"].(string)
		if msg == "" {
			msg = "authentication rejected"
		}
		return fmt.Errorf("auth failed: %s", msg)
	}
	if authResp.Type != protocol.MsgTypeAuthResponse {
		return fmt.Errorf("unexpected response: %s", authResp.Type)
	}

	success, _ := authResp.Payload["success"].(bool)
	if !success {
		msg, _ := authResp.Payload["message"].(string)
		return fmt.Errorf("auth failed: %s", msg)
	}

	log.Println("✓ Authenticated successfully")
	return nil
}

type TunnelInfo struct {
	PublicURL  string
	PublicPort int
	TunnelID   string
	Protocol   string
}

func createTunnel(conn *websocket.Conn, cfg *Config) *TunnelInfo {
	log.Printf("Requesting %s tunnel for subdomain: %s", strings.ToUpper(cfg.Protocol), cfg.Subdomain)
	msgType := protocol.MsgTypeTunnelReq
	switch cfg.Protocol {
	case "tcp":
		msgType = protocol.MsgTypeTCPReq
	case "grpc":
		msgType = protocol.MsgTypeGRPCReq
	}
	payload := map[string]interface{}{
		"subdomain":  cfg.Subdomain,
		"protocol":   cfg.Protocol,
		"local_port": cfg.LocalPort,
		"local_host": cfg.LocalHost,
	}

	tunnelMsg := protocol.NewControlMessage(
		msgType,
		uuid.New().String(),
		payload,
	)

	if err := conn.WriteJSON(tunnelMsg); err != nil {
		log.Fatalf("Failed to send tunnel request: %v", err)
	}

	var tunnelResp protocol.ControlMessage
	if err := conn.ReadJSON(&tunnelResp); err != nil {
		log.Fatalf("Failed to read tunnel response: %v", err)
	}

	if tunnelResp.Type == protocol.MsgTypeError {
		msg, _ := tunnelResp.Payload["message"].(string)
		log.Fatalf("Tunnel creation failed: %s", msg)
	}

	expectedType := protocol.MsgTypeTunnelResp
	switch cfg.Protocol {
	case "tcp":
		expectedType = protocol.MsgTypeTCPResp
	case "grpc":
		expectedType = protocol.MsgTypeGRPCResp
	}
	if tunnelResp.Type != expectedType {
		log.Fatalf("Unexpected response type: %s", tunnelResp.Type)
	}

	publicURL, _ := tunnelResp.Payload["public_url"].(string)
	var publicPort int
	if v, ok := tunnelResp.Payload["public_port"].(float64); ok {
		publicPort = int(v)
	}
	tunnelID, _ := tunnelResp.Payload["tunnel_id"].(string)

	log.Printf("✓ Tunnel created!")
	log.Printf("  Tunnel ID: %s", tunnelID)
	if publicURL != "" {
		log.Printf("  Public URL: %s", publicURL)
	} else {
		log.Printf("  Public Port: %d", publicPort)
	}
	log.Printf("  Forwarding to: %s:%d", cfg.LocalHost, cfg.LocalPort)

	return &TunnelInfo{
		PublicURL:  publicURL,
		PublicPort: publicPort,
		TunnelID:   tunnelID,
		Protocol:   cfg.Protocol,
	}
}

func establishMuxSession(conn *websocket.Conn) *yamux.Session {
	var muxMsg protocol.ControlMessage
	if err := conn.ReadJSON(&muxMsg); err != nil {
		log.Fatalf("Failed to read mux message: %v", err)
	}

	if muxMsg.Type != protocol.MsgTypeNewConn {
		log.Fatalf("Expected mux establishment message, got: %s", muxMsg.Type)
	}

	muxAddr, _ := muxMsg.Payload["mux_addr"].(string)
	log.Printf("Establishing yamux connection to %s", muxAddr)

	muxConn, err := net.Dial("tcp", muxAddr)
	if err != nil {
		log.Fatalf("Failed to connect to mux: %v", err)
	}

	session, err := yamux.Client(muxConn, nil)
	if err != nil {
		log.Fatalf("Failed to create yamux session: %v", err)
	}

	log.Println("✓ Yamux session established")
	return session
}

func runTunnelLoop(session *yamux.Session, localHost string, localPort int) {
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			log.Printf("Failed to accept stream: %v", err)
			continue
		}

		go handleStream(stream, localHost, localPort)
	}
}

func handleStream(stream net.Conn, localHost string, localPort int) {
	defer stream.Close()

	localConn, err := net.Dial("tcp", net.JoinHostPort(localHost, fmt.Sprintf("%d", localPort)))
	if err != nil {
		log.Printf("Failed to connect to local server: %v", err)
		return
	}
	defer localConn.Close()

	done := make(chan struct{}, 2)

	go func() {
		io.Copy(stream, localConn)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(localConn, stream)
		done <- struct{}{}
	}()

	<-done
	log.Println("Request handled")
}

func handleHeartbeat(conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		msg := protocol.NewControlMessage(
			protocol.MsgTypeHeartbeat,
			uuid.New().String(),
			map[string]interface{}{},
		)
		if err := conn.WriteJSON(msg); err != nil {
			log.Printf("Heartbeat failed: %v", err)
			return
		}
	}
}
