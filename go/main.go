package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type CollabManager struct {
	sessionManager *SessionManager
	p2pManager     *P2PManager
	syncManager    *SyncManager
}

func NewCollabManager() *CollabManager {
	cm := &CollabManager{
		sessionManager: NewSessionManager(),
		p2pManager:     NewP2PManager(),
		syncManager:    NewSyncManager(),
	}
	
	// Set user ID for sync manager
	cm.syncManager.SetUserID(cm.sessionManager.GetUserID())
	
	// Set up event handlers for sync manager
	cm.syncManager.SetEventHandlers(
		func(content string) {
			// Document changed - could notify Neovim here
			log.Printf("Document changed: %d chars", len(content))
		},
		func(op Operation) {
			// Operation applied - could broadcast to peers here
			log.Printf("Operation applied: %s by %s", op.Type, op.UserID)
		},
		func(localOp, remoteOp, resolution Operation) {
			// Conflict resolved
			log.Printf("Conflict resolved between %s and %s", localOp.UserID, remoteOp.UserID)
		},
	)
	
	// Set up P2P event handlers
	cm.p2pManager.SetUserID(cm.sessionManager.GetUserID())
	cm.p2pManager.SetEventHandlers(
		func(userID string) {
			// Peer joined
			log.Printf("Peer joined: %s", userID)
		},
		func(userID string) {
			// Peer left
			log.Printf("Peer left: %s", userID)
		},
		func(userID string, data []byte) {
			// Message received from peer
			log.Printf("Message from %s: %d bytes", userID, len(data))
		},
	)
	
	return cm
}

// handleMessage processes incoming messages from Neovim
func (cm *CollabManager) handleMessage(msg *Message) *Message {
	switch msg.Type {
	// Session management
	case MsgCreateSession:
		var req CreateSessionRequest
		if err := msg.ParseData(&req); err != nil {
			return createErrorMessage("parse_error", err.Error())
		}
		return cm.handleCreateSession(&req)

	case MsgJoinSession:
		var req JoinSessionRequest
		if err := msg.ParseData(&req); err != nil {
			return createErrorMessage("parse_error", err.Error())
		}
		return cm.handleJoinSession(&req)

	case MsgLeaveSession:
		var req LeaveSessionRequest
		if err := msg.ParseData(&req); err != nil {
			return createErrorMessage("parse_error", err.Error())
		}
		return cm.handleLeaveSession(&req)

	// Document operations
	case MsgDocumentOperation:
		var op DocumentOperation
		if err := msg.ParseData(&op); err != nil {
			return createErrorMessage("parse_error", err.Error())
		}
		return cm.handleDocumentOperation(&op)

	case MsgCursorMove:
		var cursor CursorPosition
		if err := msg.ParseData(&cursor); err != nil {
			return createErrorMessage("parse_error", err.Error())
		}
		return cm.handleCursorMove(&cursor)

	// Control management
	case MsgRequestControl:
		var req ControlRequest
		if err := msg.ParseData(&req); err != nil {
			return createErrorMessage("parse_error", err.Error())
		}
		return cm.handleControlRequest(&req)

	case MsgReleaseControl:
		return cm.handleReleaseControl()

	// System messages
	case MsgHealthCheck:
		return createStatusMessage("healthy", "Go process running")

	default:
		return createErrorMessage("unknown_message_type", "Unknown message type: "+msg.Type)
	}
}

// Session handlers
func (cm *CollabManager) handleCreateSession(req *CreateSessionRequest) *Message {
	session, err := cm.sessionManager.CreateSession(req.FilePath, req.Content)
	if err != nil {
		return createErrorMessage("create_session_failed", err.Error())
	}
	
	// Initialize sync manager with document content
	cm.syncManager.InitializeDocument(req.Content)
	
	response := CreateSessionResponse{
		SessionID: session.ID,
		UserID:    cm.sessionManager.GetUserID(),
	}
	
	msg, _ := NewMessage(MsgSessionCreated, response)
	return msg
}

func (cm *CollabManager) handleJoinSession(req *JoinSessionRequest) *Message {
	session, err := cm.sessionManager.JoinSession(req.SessionID)
	if err != nil {
		return createErrorMessage("join_session_failed", err.Error())
	}
	
	// Initialize sync manager with session content
	cm.syncManager.InitializeDocument(session.Content)
	
	// Convert peers map to slice
	peers := make([]Peer, 0, len(session.Peers))
	for _, peer := range session.Peers {
		peers = append(peers, *peer)
	}
	
	response := JoinSessionResponse{
		UserID:  cm.sessionManager.GetUserID(),
		Content: session.Content,
		Peers:   peers,
	}
	
	msg, _ := NewMessage(MsgSessionJoined, response)
	return msg
}

func (cm *CollabManager) handleLeaveSession(req *LeaveSessionRequest) *Message {
	err := cm.sessionManager.LeaveSession()
	if err != nil {
		return createErrorMessage("leave_session_failed", err.Error())
	}
	
	return createStatusMessage("left", "Left session successfully")
}

// Document operation handlers
func (cm *CollabManager) handleDocumentOperation(op *DocumentOperation) *Message {
	// Convert protocol operation to sync operation
	syncOp := Operation{
		Type:      OperationType(op.Type),
		Position:  op.Position,
		Content:   op.Content,
		Length:    op.Length,
		UserID:    op.UserID,
		Timestamp: time.Now().UnixNano(),
		ID:        generateOperationID(op.UserID),
	}
	
	// Apply as local or remote operation based on user ID
	var err error
	if op.UserID == cm.sessionManager.GetUserID() {
		err = cm.syncManager.ApplyLocalOperation(syncOp)
	} else {
		err = cm.syncManager.ApplyRemoteOperation(syncOp)
	}
	
	if err != nil {
		return createErrorMessage("operation_failed", err.Error())
	}
	
	return createStatusMessage("operation_applied", "Document operation processed successfully")
}

func (cm *CollabManager) handleCursorMove(cursor *CursorPosition) *Message {
	// TODO: Implement cursor handling
	return nil // No response needed for cursor moves
}

// Control handlers
func (cm *CollabManager) handleControlRequest(req *ControlRequest) *Message {
	// Only process if the request is from the current user
	if req.RequestedBy != cm.sessionManager.GetUserID() {
		return createErrorMessage("invalid_control_request", "Can only request control for yourself")
	}
	
	status, err := cm.sessionManager.RequestControl()
	if err != nil {
		return createErrorMessage("control_request_failed", err.Error())
	}
	
	msg, _ := NewMessage(MsgControlStatus, status)
	return msg
}

func (cm *CollabManager) handleReleaseControl() *Message {
	status, err := cm.sessionManager.ReleaseControl()
	if err != nil {
		return createErrorMessage("control_release_failed", err.Error())
	}
	
	msg, _ := NewMessage(MsgControlStatus, status)
	return msg
}

// Helper functions
func createErrorMessage(code, message string) *Message {
	errorMsg := ErrorMessage{
		Code:    code,
		Message: message,
	}
	
	msg, _ := NewMessage(MsgError, errorMsg)
	return msg
}

func createStatusMessage(status, info string) *Message {
	statusMsg := StatusMessage{
		Status: status,
		Info:   info,
	}
	
	msg, _ := NewMessage(MsgStatus, statusMsg)
	return msg
}

// sendMessage sends a message to Neovim via stdout
func sendMessage(msg *Message) error {
	if msg == nil {
		return nil
	}
	
	jsonData, err := msg.ToJSON()
	if err != nil {
		return err
	}
	
	fmt.Println(string(jsonData))
	return nil
}

// setupGracefulShutdown handles cleanup on process termination
func setupGracefulShutdown(cleanup func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		<-c
		log.Println("Shutting down gracefully...")
		cleanup()
		os.Exit(0)
	}()
}

func main() {
	// Setup logging to stderr (stdout is reserved for communication with Neovim)
	log.SetOutput(os.Stderr)
	log.SetPrefix("[collab.nvim] ")
	
	log.Println("Starting collab.nvim Go process")
	
	// Initialize collaboration manager
	collabManager := NewCollabManager()
	
	// Setup graceful shutdown
	setupGracefulShutdown(func() {
		// TODO: Cleanup connections, save state, etc.
		log.Println("Cleanup completed")
	})
	
	// Create scanner for reading from stdin
	scanner := bufio.NewScanner(os.Stdin)
	
	// Main message processing loop
	for scanner.Scan() {
		line := scanner.Text()
		
		// Parse incoming message
		msg, err := ParseMessage([]byte(line))
		if err != nil {
			log.Printf("Failed to parse message: %v", err)
			errorMsg := createErrorMessage("parse_error", err.Error())
			sendMessage(errorMsg)
			continue
		}
		
		log.Printf("Received message: %s", msg.Type)
		
		// Process message and get response
		response := collabManager.handleMessage(msg)
		
		// Send response back to Neovim
		if err := sendMessage(response); err != nil {
			log.Printf("Failed to send response: %v", err)
		}
	}
	
	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("Scanner error: %v", err)
	}
	
	log.Println("collab.nvim Go process terminated")
}
