// Package protocol defines the communication protocol between TunneLab server and clients.
//
// The protocol uses JSON messages over WebSocket connections for control operations
// and yamux multiplexed connections for data transfer. This protocol is used by
// hooklab and other clients that leverage TunneLab for tunneling services.
//
// Message Types:
//   - auth: Client authentication
//   - auth_response: Server authentication response
//   - tunnel_request: Request to create a tunnel
//   - tunnel_response: Tunnel creation response
//   - new_conn: New multiplexed connection notification
//   - heartbeat: Keep-alive messages
//   - error: Error messages
//
// Usage:
//
//	// Create an authentication message
//	msg := NewControlMessage(MsgTypeAuth, uuid.New().String(), map[string]interface{}{
//	    "token": "your-token-here",
//	})
//
//	// Send over WebSocket
//	conn.WriteJSON(msg)
package protocol

import (
	"time"
)

// MessageType represents the type of a protocol message.
type MessageType string

const (
	// MsgTypeAuth is the message type for client authentication.
	MsgTypeAuth MessageType = "auth"
	// MsgTypeAuthResponse is the message type for server authentication response.
	MsgTypeAuthResponse MessageType = "auth_response"
	// MsgTypeTunnelReq is the message type for tunnel creation request.
	MsgTypeTunnelReq MessageType = "tunnel_request"
	// MsgTypeTunnelResp is the message type for tunnel creation response.
	MsgTypeTunnelResp MessageType = "tunnel_response"
	// MsgTypeTCPReq is the message type for TCP tunnel creation request.
	MsgTypeTCPReq MessageType = "tcp_request"
	// MsgTypeTCPResp is the message type for TCP tunnel creation response.
	MsgTypeTCPResp MessageType = "tcp_response"
	// MsgTypeHeartbeat is the message type for keep-alive messages.
	MsgTypeHeartbeat MessageType = "heartbeat"
	MsgTypeNewConn   MessageType = "new_connection"
	MsgTypeCloseConn MessageType = "close_connection"
	MsgTypeError     MessageType = "error"
	// MsgTypeGRPCReq is the message type for gRPC tunnel creation request.
	MsgTypeGRPCReq MessageType = "grpc_request"
	// MsgTypeGRPCResp is the message type for gRPC tunnel creation response.
	MsgTypeGRPCResp MessageType = "grpc_response"
)

// ControlMessage represents a protocol message sent between server and client.
type ControlMessage struct {
	Type      MessageType            `json:"type"`       // Message type (auth, tunnel_request, etc.)
	RequestID string                 `json:"request_id"` // Unique request identifier
	Payload   map[string]interface{} `json:"payload"`    // Message payload data
	Timestamp int64                  `json:"timestamp"`  // Unix timestamp
}

// TunnelConfig contains tunnel configuration parameters.
type TunnelConfig struct {
	Subdomain string `json:"subdomain"`  // Desired subdomain for the tunnel
	Protocol  string `json:"protocol"`   // Protocol type (http, tcp, etc.)
	LocalPort int    `json:"local_port"` // Local port to forward traffic to
	LocalHost string `json:"local_host,omitempty"`
}

// GRPCTunnelConfig contains gRPC tunnel parameters.
type GRPCTunnelConfig struct {
	Subdomain   string   `json:"subdomain"`
	LocalPort   int      `json:"local_port"`
	LocalHost   string   `json:"local_host,omitempty"`
	Services    []string `json:"services,omitempty"`
	RequireTLS  bool     `json:"require_tls"`
	MaxStreams  int      `json:"max_streams,omitempty"`
	Compression string   `json:"compression,omitempty"`
}

type TunnelResponse struct {
	TunnelID   string `json:"tunnel_id"`  // Unique tunnel identifier
	PublicURL  string `json:"public_url"` // Public URL for accessing the tunnel
	PublicPort int    `json:"public_port,omitempty"`
	Status     string `json:"status"` // Tunnel status (active, error, etc.)
	Message    string `json:"message,omitempty"`
}

// GRPCTunnelResponse extends TunnelResponse with gRPC metadata.
type GRPCTunnelResponse struct {
	TunnelID string   `json:"tunnel_id"`
	Endpoint string   `json:"endpoint"`
	Status   string   `json:"status"`
	Message  string   `json:"message,omitempty"`
	Services []string `json:"services,omitempty"`
}

type AuthRequest struct {
	Token string `json:"token"` // Authentication token
}

type AuthResponse struct {
	Success   bool   `json:"success"`             // Whether authentication succeeded
	ClientID  string `json:"client_id,omitempty"` // Client identifier
	Message   string `json:"message,omitempty"`   // Response message
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type ErrorPayload struct {
	Code    string                 `json:"code"`    // Error code
	Message string                 `json:"message"` // Error message
	Details map[string]interface{} `json:"details,omitempty"`
}

// NewControlMessage creates a new ControlMessage with the specified parameters.
//
// Parameters:
//   - msgType: The type of message (e.g., MsgTypeAuth, MsgTypeTunnelReq)
//   - requestID: Unique identifier for this request
//   - payload: Message payload data
//
// Returns:
//   - *ControlMessage: A new protocol message with current timestamp
func NewControlMessage(msgType MessageType, requestID string, payload map[string]interface{}) *ControlMessage {
	return &ControlMessage{
		Type:      msgType,
		RequestID: requestID,
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}
}

// NewErrorMessage creates an error message with the specified error code and message.
//
// Parameters:
//   - requestID: Unique identifier for the request that caused the error
//   - code: Error code string
//   - message: Human-readable error message
//
// Returns:
//   - *ControlMessage: An error message ready to be sent
func NewErrorMessage(requestID, code, message string) *ControlMessage {
	return &ControlMessage{
		Type:      MsgTypeError,
		RequestID: requestID,
		Payload: map[string]interface{}{
			"code":    code,
			"message": message,
		},
		Timestamp: time.Now().Unix(),
	}
}
