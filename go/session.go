package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type Session struct {
	ID          string            `json:"id"`
	CreatedBy   string            `json:"created_by"`
	CreatedAt   time.Time         `json:"created_at"`
	FilePath    string            `json:"file_path"`
	Content     string            `json:"content"`
	Peers       map[string]*Peer  `json:"peers"`
	Controller  string            `json:"controller"`
	IsActive    bool              `json:"is_active"`
	mutex       sync.RWMutex
}

type SessionManager struct {
	currentSession *Session
	userID         string
	sessions       map[string]*Session
	mutex          sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		userID:   generateUserID(),
		sessions: make(map[string]*Session),
	}
}

func (sm *SessionManager) CreateSession(filePath, content string) (*Session, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	sessionID := generateSessionID(filePath, content, sm.userID)
	
	session := &Session{
		ID:         sessionID,
		CreatedBy:  sm.userID,
		CreatedAt:  time.Now(),
		FilePath:   filePath,
		Content:    content,
		Peers:      make(map[string]*Peer),
		Controller: sm.userID,
		IsActive:   true,
	}
	
	creatorPeer := &Peer{
		UserID: sm.userID,
		Name:   "Creator",
	}
	session.Peers[sm.userID] = creatorPeer
	
	sm.sessions[sessionID] = session
	sm.currentSession = session
	
	return session, nil
}

func (sm *SessionManager) JoinSession(sessionID string) (*Session, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	session := &Session{
		ID:         sessionID,
		CreatedBy:  "remote-user",
		CreatedAt:  time.Now().Add(-5 * time.Minute),
		FilePath:   "/path/to/shared/file.txt",
		Content:    "// This is shared content\n// from remote session",
		Peers:      make(map[string]*Peer),
		Controller: "remote-user",
		IsActive:   true,
	}
	
	remotePeer := &Peer{
		UserID: "remote-user",
		Name:   "Remote User",
	}
	session.Peers["remote-user"] = remotePeer
	
	currentPeer := &Peer{
		UserID: sm.userID,
		Name:   "Local User",
	}
	session.Peers[sm.userID] = currentPeer
	
	sm.sessions[sessionID] = session
	sm.currentSession = session
	
	return session, nil
}

func (sm *SessionManager) LeaveSession() error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	if sm.currentSession == nil {
		return fmt.Errorf("no active session to leave")
	}
	
	sm.currentSession.mutex.Lock()
	delete(sm.currentSession.Peers, sm.userID)
	
	if sm.currentSession.Controller == sm.userID {
		sm.currentSession.Controller = ""
		for peerID := range sm.currentSession.Peers {
			sm.currentSession.Controller = peerID
			break
		}
	}
	
	if len(sm.currentSession.Peers) == 0 {
		sm.currentSession.IsActive = false
	}
	sm.currentSession.mutex.Unlock()
	
	sm.currentSession = nil
	return nil
}

func (sm *SessionManager) RequestControl() (*ControlStatus, error) {
	sm.mutex.RLock()
	session := sm.currentSession
	sm.mutex.RUnlock()
	
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}
	
	session.mutex.Lock()
	defer session.mutex.Unlock()
	
	session.Controller = sm.userID
	
	status := &ControlStatus{
		CurrentController: session.Controller,
		HasControl:        true,
	}
	
	return status, nil
}

func (sm *SessionManager) ReleaseControl() (*ControlStatus, error) {
	sm.mutex.RLock()
	session := sm.currentSession
	sm.mutex.RUnlock()
	
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}
	
	session.mutex.Lock()
	defer session.mutex.Unlock()
	
	if session.Controller != sm.userID {
		return nil, fmt.Errorf("you don't have control")
	}
	
	session.Controller = ""
	
	status := &ControlStatus{
		CurrentController: "",
		HasControl:        false,
	}
	
	return status, nil
}

func (sm *SessionManager) GetUserID() string {
	return sm.userID
}

func generateUserID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func generateSessionID(filePath, content, userID string) string {
	data := fmt.Sprintf("%s:%s:%s:%d", filePath, content, userID, time.Now().Unix())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}
