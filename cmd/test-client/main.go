// Test client is a minimal tunnel client for testing TunneLab server.
//
// This client simulates client behavior for testing purposes, similar to how
// hooklab (an open-source project) leverages TunneLab for tunneling services.
// It connects to the control server, authenticates, creates a tunnel,
// and forwards HTTP requests to a local server.
//
// Usage:
//
//   ./test-client -server ws://localhost:4443 -token TOKEN -subdomain test -port 8000
//
// Flags:
//   -server: Control server WebSocket URL (default: ws://localhost:4443)
//   -token: Authentication token (required)
//   -subdomain: Subdomain for the tunnel (default: test)
//   -port: Local port to forward traffic to (default: 8000)
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/essajiwa/tunnelab/pkg/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

// main is the entry point for the test client.
func main() {
	serverURL := flag.String("server", "ws://localhost:4443", "Control server URL")
	token := flag.String("token", "", "Authentication token")
	subdomain := flag.String("subdomain", "test", "Subdomain to use")
	localPort := flag.Int("port", 8000, "Local port to forward")
	flag.Parse()

	if *token == "" {
		log.Fatal("Token is required. Use -token flag")
	}

	log.Printf("Connecting to %s", *serverURL)
	conn, _, err := websocket.DefaultDialer.Dial(*serverURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	log.Println("Authenticating...")
	authMsg := protocol.NewControlMessage(
		protocol.MsgTypeAuth,
		uuid.New().String(),
		map[string]interface{}{
			"token": *token,
		},
	)

	if err := conn.WriteJSON(authMsg); err != nil {
		log.Fatalf("Failed to send auth: %v", err)
	}

	var authResp protocol.ControlMessage
	if err := conn.ReadJSON(&authResp); err != nil {
		log.Fatalf("Failed to read auth response: %v", err)
	}

	if authResp.Type != protocol.MsgTypeAuthResponse {
		log.Fatalf("Unexpected response: %s", authResp.Type)
	}

	success, _ := authResp.Payload["success"].(bool)
	if !success {
		msg, _ := authResp.Payload["message"].(string)
		log.Fatalf("Auth failed: %s", msg)
	}

	log.Println("✓ Authenticated successfully")

	log.Printf("Requesting tunnel for subdomain: %s", *subdomain)
	tunnelMsg := protocol.NewControlMessage(
		protocol.MsgTypeTunnelReq,
		uuid.New().String(),
		map[string]interface{}{
			"subdomain":  *subdomain,
			"protocol":   "http",
			"local_port": *localPort,
		},
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

	publicURL, _ := tunnelResp.Payload["public_url"].(string)
	tunnelID, _ := tunnelResp.Payload["tunnel_id"].(string)

	log.Printf("✓ Tunnel created!")
	log.Printf("  Tunnel ID: %s", tunnelID)
	log.Printf("  Public URL: %s", publicURL)
	log.Printf("  Forwarding to: localhost:%d", *localPort)

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
	defer muxConn.Close()

	session, err := yamux.Client(muxConn, nil)
	if err != nil {
		log.Fatalf("Failed to create yamux session: %v", err)
	}
	defer session.Close()

	log.Println("✓ Yamux session established")
	log.Printf("\n🎉 Tunnel is ready! Access your local server at: %s\n", publicURL)
	log.Printf("Press Ctrl+C to stop\n")

	go handleHeartbeat(conn)

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			log.Printf("Failed to accept stream: %v", err)
			continue
		}

		go handleStream(stream, *localPort)
	}
}

func handleStream(stream net.Conn, localPort int) {
	defer stream.Close()

	localConn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", localPort))
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
