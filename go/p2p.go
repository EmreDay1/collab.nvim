package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

type PeerConnection struct {
	ID            string
	UserID        string
	Connection    *webrtc.PeerConnection
	DataChannel   *webrtc.DataChannel
	Connected     bool
	LastHeartbeat time.Time
}

type P2PManager struct {
	localUserID   string
	peers         map[string]*PeerConnection
	peersMutex    sync.RWMutex
	
	// WebRTC configuration
	config        webrtc.Configuration
	
	// Event handlers
	onPeerJoined  func(userID string)
	onPeerLeft    func(userID string)
	onMessage     func(userID string, data []byte)
	
	// Session signaling (placeholder for now)
	signalingURL  string
	
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewP2PManager() *P2PManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Configure WebRTC with STUN servers for NAT traversal
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
				},
			},
		},
	}
	
	return &P2PManager{
		peers:        make(map[string]*PeerConnection),
		config:       config,
		ctx:          ctx,
		cancel:       cancel,
		signalingURL: "ws://localhost:3000", // Placeholder signaling server
	}
}

// SetUserID sets the local user ID
func (p2p *P2PManager) SetUserID(userID string) {
	p2p.localUserID = userID
}

// SetEventHandlers sets callback functions for P2P events
func (p2p *P2PManager) SetEventHandlers(
	onPeerJoined func(string),
	onPeerLeft func(string), 
	onMessage func(string, []byte),
) {
	p2p.onPeerJoined = onPeerJoined
	p2p.onPeerLeft = onPeerLeft
	p2p.onMessage = onMessage
}

// CreateOffer creates a WebRTC offer for a new peer connection
func (p2p *P2PManager) CreateOffer(peerUserID string) (*webrtc.SessionDescription, error) {
	// Create new peer connection
	pc, err := webrtc.NewPeerConnection(p2p.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %v", err)
	}
	
	// Create data channel
	dc, err := pc.CreateDataChannel("collab", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create data channel: %v", err)
	}
	
	// Store peer connection
	peer := &PeerConnection{
		ID:            fmt.Sprintf("%s-%s", p2p.localUserID, peerUserID),
		UserID:        peerUserID,
		Connection:    pc,
		DataChannel:   dc,
		Connected:     false,
		LastHeartbeat: time.Now(),
	}
	
	p2p.peersMutex.Lock()
	p2p.peers[peerUserID] = peer
	p2p.peersMutex.Unlock()
	
	// Set up event handlers
	p2p.setupPeerHandlers(peer)
	
	// Create offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create offer: %v", err)
	}
	
	// Set local description
	err = pc.SetLocalDescription(offer)
	if err != nil {
		return nil, fmt.Errorf("failed to set local description: %v", err)
	}
	
	return &offer, nil
}

// HandleOffer handles an incoming WebRTC offer
func (p2p *P2PManager) HandleOffer(peerUserID string, offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	// Create new peer connection
	pc, err := webrtc.NewPeerConnection(p2p.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %v", err)
	}
	
	// Store peer connection (data channel will be created by remote peer)
	peer := &PeerConnection{
		ID:            fmt.Sprintf("%s-%s", peerUserID, p2p.localUserID),
		UserID:        peerUserID,
		Connection:    pc,
		DataChannel:   nil, // Will be set when data channel is received
		Connected:     false,
		LastHeartbeat: time.Now(),
	}
	
	p2p.peersMutex.Lock()
	p2p.peers[peerUserID] = peer
	p2p.peersMutex.Unlock()
	
	// Set up event handlers
	p2p.setupPeerHandlers(peer)
	
	// Set remote description
	err = pc.SetRemoteDescription(offer)
	if err != nil {
		return nil, fmt.Errorf("failed to set remote description: %v", err)
	}
	
	// Create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create answer: %v", err)
	}
	
	// Set local description
	err = pc.SetLocalDescription(answer)
	if err != nil {
		return nil, fmt.Errorf("failed to set local description: %v", err)
	}
	
	return &answer, nil
}

// HandleAnswer handles an incoming WebRTC answer
func (p2p *P2PManager) HandleAnswer(peerUserID string, answer webrtc.SessionDescription) error {
	p2p.peersMutex.RLock()
	peer, exists := p2p.peers[peerUserID]
	p2p.peersMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("no peer connection found for user %s", peerUserID)
	}
	
	// Set remote description
	err := peer.Connection.SetRemoteDescription(answer)
	if err != nil {
		return fmt.Errorf("failed to set remote description: %v", err)
	}
	
	return nil
}

// AddICECandidate adds an ICE candidate to a peer connection
func (p2p *P2PManager) AddICECandidate(peerUserID string, candidate webrtc.ICECandidateInit) error {
	p2p.peersMutex.RLock()
	peer, exists := p2p.peers[peerUserID]
	p2p.peersMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("no peer connection found for user %s", peerUserID)
	}
	
	err := peer.Connection.AddICECandidate(candidate)
	if err != nil {
		return fmt.Errorf("failed to add ICE candidate: %v", err)
	}
	
	return nil
}

// SendMessage sends a message to a specific peer
func (p2p *P2PManager) SendMessage(peerUserID string, data []byte) error {
	p2p.peersMutex.RLock()
	peer, exists := p2p.peers[peerUserID]
	p2p.peersMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("no peer connection found for user %s", peerUserID)
	}
	
	if !peer.Connected || peer.DataChannel == nil {
		return fmt.Errorf("peer %s is not connected", peerUserID)
	}
	
	err := peer.DataChannel.Send(data)
	if err != nil {
		return fmt.Errorf("failed to send message to peer %s: %v", peerUserID, err)
	}
	
	return nil
}

// BroadcastMessage sends a message to all connected peers
func (p2p *P2PManager) BroadcastMessage(data []byte) error {
	p2p.peersMutex.RLock()
	defer p2p.peersMutex.RUnlock()
	
	var lastErr error
	sentCount := 0
	
	for userID, peer := range p2p.peers {
		if peer.Connected && peer.DataChannel != nil {
			err := peer.DataChannel.Send(data)
			if err != nil {
				log.Printf("Failed to send message to peer %s: %v", userID, err)
				lastErr = err
			} else {
				sentCount++
			}
		}
	}
	
	if sentCount == 0 && lastErr != nil {
		return fmt.Errorf("failed to send message to any peer: %v", lastErr)
	}
	
	return nil
}

// DisconnectPeer closes connection to a specific peer
func (p2p *P2PManager) DisconnectPeer(peerUserID string) error {
	p2p.peersMutex.Lock()
	defer p2p.peersMutex.Unlock()
	
	peer, exists := p2p.peers[peerUserID]
	if !exists {
		return nil // Already disconnected
	}
	
	// Close data channel
	if peer.DataChannel != nil {
		peer.DataChannel.Close()
	}
	
	// Close peer connection
	peer.Connection.Close()
	
	// Remove from peers map
	delete(p2p.peers, peerUserID)
	
	// Notify about peer leaving
	if p2p.onPeerLeft != nil {
		p2p.onPeerLeft(peerUserID)
	}
	
	return nil
}

// GetConnectedPeers returns list of connected peer user IDs
func (p2p *P2PManager) GetConnectedPeers() []string {
	p2p.peersMutex.RLock()
	defer p2p.peersMutex.RUnlock()
	
	var connectedPeers []string
	for userID, peer := range p2p.peers {
		if peer.Connected {
			connectedPeers = append(connectedPeers, userID)
		}
	}
	
	return connectedPeers
}

// Shutdown closes all peer connections and cleans up
func (p2p *P2PManager) Shutdown() {
	p2p.cancel() // Cancel context
	
	p2p.peersMutex.Lock()
	defer p2p.peersMutex.Unlock()
	
	// Close all peer connections
	for userID := range p2p.peers {
		peer := p2p.peers[userID]
		if peer.DataChannel != nil {
			peer.DataChannel.Close()
		}
		peer.Connection.Close()
	}
	
	// Clear peers map
	p2p.peers = make(map[string]*PeerConnection)
}

// setupPeerHandlers sets up event handlers for a peer connection
func (p2p *P2PManager) setupPeerHandlers(peer *PeerConnection) {
	// Connection state handler
	peer.Connection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("Peer %s connection state: %s", peer.UserID, state.String())
		
		switch state {
		case webrtc.PeerConnectionStateConnected:
			peer.Connected = true
			if p2p.onPeerJoined != nil {
				p2p.onPeerJoined(peer.UserID)
			}
		case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			peer.Connected = false
			p2p.DisconnectPeer(peer.UserID)
		}
	})
	
	// ICE candidate handler
	peer.Connection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		
		// TODO: Send ICE candidate to peer via signaling
		log.Printf("Generated ICE candidate for peer %s: %s", peer.UserID, candidate.String())
	})
	
	// Data channel handler (for incoming data channels)
	peer.Connection.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("Received data channel from peer %s", peer.UserID)
		peer.DataChannel = dc
		p2p.setupDataChannelHandlers(peer, dc)
	})
	
	// If we have a data channel (outgoing connection), set up handlers
	if peer.DataChannel != nil {
		p2p.setupDataChannelHandlers(peer, peer.DataChannel)
	}
}

// setupDataChannelHandlers sets up handlers for a data channel
func (p2p *P2PManager) setupDataChannelHandlers(peer *PeerConnection, dc *webrtc.DataChannel) {
	dc.OnOpen(func() {
		log.Printf("Data channel opened with peer %s", peer.UserID)
		peer.Connected = true
	})
	
	dc.OnClose(func() {
		log.Printf("Data channel closed with peer %s", peer.UserID)
		peer.Connected = false
	})
	
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		peer.LastHeartbeat = time.Now()
		
		// Handle incoming message
		if p2p.onMessage != nil {
			p2p.onMessage(peer.UserID, msg.Data)
		}
	})
	
	dc.OnError(func(err error) {
		log.Printf("Data channel error with peer %s: %v", peer.UserID, err)
	})
}

// StartHeartbeat starts a heartbeat routine to monitor peer connections
func (p2p *P2PManager) StartHeartbeat() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-p2p.ctx.Done():
				return
			case <-ticker.C:
				p2p.sendHeartbeats()
				p2p.checkPeerTimeouts()
			}
		}
	}()
}

// sendHeartbeats sends heartbeat messages to all connected peers
func (p2p *P2PManager) sendHeartbeats() {
	heartbeat := map[string]interface{}{
		"type": "heartbeat",
		"from": p2p.localUserID,
		"time": time.Now().Unix(),
	}
	
	data, _ := json.Marshal(heartbeat)
	p2p.BroadcastMessage(data)
}

// checkPeerTimeouts checks for and removes timed-out peers
func (p2p *P2PManager) checkPeerTimeouts() {
	timeout := 60 * time.Second
	now := time.Now()
	
	p2p.peersMutex.RLock()
	var timedOutPeers []string
	for userID, peer := range p2p.peers {
		if now.Sub(peer.LastHeartbeat) > timeout {
			timedOutPeers = append(timedOutPeers, userID)
		}
	}
	p2p.peersMutex.RUnlock()
	
	// Disconnect timed-out peers
	for _, userID := range timedOutPeers {
		log.Printf("Peer %s timed out, disconnecting", userID)
		p2p.DisconnectPeer(userID)
	}
}
