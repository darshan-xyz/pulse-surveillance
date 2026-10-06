package webrtc

import (
	"log"

	"sync"
	"hss/internal/media"
	"github.com/pion/webrtc/v4"
)

type Manager struct {
	api      *webrtc.API
	sessions map[string]*webrtc.PeerConnection
	mu       sync.RWMutex
}

func NewManager() *Manager {
	// Simple config for MVP
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		log.Fatalf("Failed to register codecs: %v", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))
	return &Manager{
		api:      api,
		sessions: make(map[string]*webrtc.PeerConnection),
	}
}

func (m *Manager) HandleOffer(sessionID, camID, sdpOffer string, onCandidate func(*webrtc.ICECandidate)) (string, error) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
			{
				URLs:           []string{"turn:coturn:3478", "turn:127.0.0.1:3478"},
				Username:       "pulse",
				Credential:     "landing",
				CredentialType: webrtc.ICECredentialTypePassword,
			},
		},
	}

	peerConnection, err := m.api.NewPeerConnection(config)
	if err != nil {
		return "", err
	}

	track, err := media.DefaultEngine.GetTrack(camID, "high")
	if err != nil {
		return "", err
	}
	if _, err = peerConnection.AddTrack(track); err != nil {
		return "", err
	}

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("Session %s ICE Connection State has changed: %s\n", sessionID, connectionState.String())
	})

	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil && onCandidate != nil {
			onCandidate(c)
		}
	})

	// Save session
	m.mu.Lock()
	m.sessions[sessionID] = peerConnection
	m.mu.Unlock()

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdpOffer,
	}

	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		return "", err
	}

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		return "", err
	}

	if err = peerConnection.SetLocalDescription(answer); err != nil {
		return "", err
	}

	return answer.SDP, nil
}

func (m *Manager) AddICECandidate(sessionID, candidate string) error {
	m.mu.RLock()
	pc, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	return pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate})
}

func (m *Manager) RemoveSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pc, ok := m.sessions[sessionID]; ok {
		pc.Close()
		delete(m.sessions, sessionID)
		log.Printf("Closed and removed WebRTC session: %s", sessionID)
	}
}

var DefaultManager = NewManager()
