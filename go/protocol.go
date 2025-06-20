package main

import "encoding/json"

// Message represents the base message structure between Lua and Go
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Session Management Messages
type CreateSessionRequest struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

type CreateSessionResponse struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
}

type JoinSessionRequest struct {
	SessionID string `json:"session_id"`
}

type JoinSessionResponse struct {
	UserID    string `json:"user_id"`
	Content   string `json:"content"`
	Peers     []Peer `json:"peers"`
}

type LeaveSessionRequest struct {
	SessionID string `json:"session_id"`
}

// Peer Management
type Peer struct {
	UserID string `json:"user_id"`
	Name   string `json:"name,omitempty"`
}

type PeerJoinedEvent struct {
	Peer Peer `json:"peer"`
}

type PeerLeftEvent struct {
	UserID string `json:"user_id"`
}

// Document Operations
type DocumentOperation struct {
	Type     string `json:"type"`     // "insert", "delete", "retain"
	Position int    `json:"position"`
	Content  string `json:"content,omitempty"`
	Length   int    `json:"length,omitempty"`
	UserID   string `json:"user_id"`
}

type CursorPosition struct {
	UserID string `json:"user_id"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Control Management
type ControlRequest struct {
	RequestedBy string `json:"requested_by"`
}

type ControlTransfer struct {
	FromUser string `json:"from_user"`
	ToUser   string `json:"to_user"`
}

type ControlStatus struct {
	CurrentController string `json:"current_controller"`
	HasControl        bool   `json:"has_control"`
}

// System Messages
type ErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type StatusMessage struct {
	Status string `json:"status"`
	Info   string `json:"info,omitempty"`
}

// Message type constants
const (
	// Session messages
	MsgCreateSession     = "create_session"
	MsgJoinSession       = "join_session"
	MsgLeaveSession      = "leave_session"
	MsgSessionCreated    = "session_created"
	MsgSessionJoined     = "session_joined"
	MsgSessionLeft       = "session_left"
	
	// Peer messages
	MsgPeerJoined        = "peer_joined"
	MsgPeerLeft          = "peer_left"
	
	// Document messages
	MsgDocumentOperation = "document_operation"
	MsgCursorMove        = "cursor_move"
	
	// Control messages
	MsgRequestControl    = "request_control"
	MsgGrantControl      = "grant_control"
	MsgReleaseControl    = "release_control"
	MsgControlStatus     = "control_status"
	
	// System messages
	MsgError             = "error"
	MsgStatus            = "status"
	MsgHealthCheck       = "health_check"
)

// Helper functions for message creation and parsing
func NewMessage(msgType string, data interface{}) (*Message, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	
	return &Message{
		Type: msgType,
		Data: dataBytes,
	}, nil
}

func (m *Message) ParseData(target interface{}) error {
	return json.Unmarshal(m.Data, target)
}

func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

func ParseMessage(data []byte) (*Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	return &msg, err
}
