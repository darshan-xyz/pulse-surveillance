package webrtc

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"hss/internal/media"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type WebRTCSession struct {
	PC        *webrtc.PeerConnection
	CamID     string
	Sender    *webrtc.RTPSender
	Track     *webrtc.TrackLocalStaticRTP
	ABR       *ABRController
	stopChan  chan struct{}
	closeOnce sync.Once
}

func (s *WebRTCSession) Close() {
	s.closeOnce.Do(func() {
		if s.stopChan != nil {
			close(s.stopChan)
		}
		if s.PC != nil {
			_ = s.PC.Close()
		}
	})
}

func (s *WebRTCSession) WriteRTP(pkt *rtp.Packet) error {
	if s.Track == nil {
		return fmt.Errorf("track not initialized")
	}
	if s.ABR != nil {
		processed, forward := s.ABR.ProcessPacket(pkt)
		if !forward || processed == nil {
			return nil // Packet dropped due to congestion adaptation
		}
		return s.Track.WriteRTP(processed)
	}
	return s.Track.WriteRTP(pkt)
}

type Manager struct {
	api          *webrtc.API
	sessions     map[string]*WebRTCSession
	viewerCounts map[string]int
	mu           sync.RWMutex
}

func NewManager() *Manager {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		log.Fatalf("Failed to register codecs: %v", err)
	}

	var se webrtc.SettingEngine
	// Restrict to IPv4 for fast candidate gathering and zero IPv6 DNS/binding timeouts
	se.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeTCP4,
	})
	// Aggressive surveillance ICE timeouts (2s disconnected, 4s failed, 1s keepalive)
	se.SetICETimeouts(2*time.Second, 4*time.Second, 1*time.Second)

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithSettingEngine(se))
	return &Manager{
		api:          api,
		sessions:     make(map[string]*WebRTCSession),
		viewerCounts: make(map[string]int),
	}
}

func (m *Manager) HandleOffer(sessionID, camID, sdpOffer string, turnURL, turnUser, turnPass string, onCandidate func(*webrtc.ICECandidate)) (string, error) {
	m.mu.Lock()
	if existing, ok := m.sessions[sessionID]; ok {
		existing.Close()
		delete(m.sessions, sessionID)
		if m.viewerCounts[existing.CamID] > 0 {
			m.viewerCounts[existing.CamID]--
		}
	}

	if count := m.viewerCounts[camID]; count >= 3 {
		m.mu.Unlock()
		return "", fmt.Errorf("camera %s has reached the maximum of 3 concurrent viewers", camID)
	}
	m.viewerCounts[camID]++
	m.mu.Unlock()

	rollbackViewerCount := func() {
		m.mu.Lock()
		if m.viewerCounts[camID] > 0 {
			m.viewerCounts[camID]--
		}
		m.mu.Unlock()
	}

	var iceServers []webrtc.ICEServer

	if turnURL != "" && turnUser != "" && turnPass != "" {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:           []string{turnURL},
			Username:       turnUser,
			Credential:     turnPass,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}

	iceServers = append(iceServers,
		webrtc.ICEServer{URLs: []string{"stun:stun.l.google.com:19302"}},
		webrtc.ICEServer{URLs: []string{"stun:openrelay.metered.ca:80"}},
		webrtc.ICEServer{
			URLs:           []string{"turn:openrelay.metered.ca:80", "turn:openrelay.metered.ca:443"},
			Username:       "openrelayproject",
			Credential:     "openrelayproject",
			CredentialType: webrtc.ICECredentialTypePassword,
		},
	)

	config := webrtc.Configuration{
		ICEServers: iceServers,
	}

	peerConnection, err := m.api.NewPeerConnection(config)
	if err != nil {
		rollbackViewerCount()
		return "", err
	}

	// Create a dedicated per-session video track to support independent ABR rate control
	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", fmt.Sprintf("pion-%s", sessionID))
	if err != nil {
		peerConnection.Close()
		rollbackViewerCount()
		return "", err
	}

	rtpSender, err := peerConnection.AddTrack(track)
	if err != nil {
		peerConnection.Close()
		rollbackViewerCount()
		return "", err
	}

	abr := NewABRController(sessionID, DefaultBitrate)
	stopChan := make(chan struct{})

	session := &WebRTCSession{
		PC:       peerConnection,
		CamID:    camID,
		Sender:   rtpSender,
		Track:    track,
		ABR:      abr,
		stopChan: stopChan,
	}

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("Session %s ICE Connection State has changed: %s\n", sessionID, connectionState.String())
	})

	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("Session %s Peer Connection State has changed: %s\n", sessionID, s.String())
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
			m.RemoveSession(sessionID)
		}
	})

	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil && onCandidate != nil {
			onCandidate(c)
		}
	})

	// Add RTCP reader for ABR congestion monitoring
	// We read RTCP packets from the peer to detect REMB (bandwidth estimation) and PLI (keyframe requests)
	go func() {
		for {
			packets, _, rtcpErr := rtpSender.ReadRTCP()
			if rtcpErr != nil {
				// PeerConnection closed or error
				return
			}
			for _, pkt := range packets {
				switch p := pkt.(type) {
				case *rtcp.ReceiverEstimatedMaximumBitrate:
					log.Printf("[ABR] Session %s received REMB bitrate: %.0f bps", sessionID, p.Bitrate)
					abr.UpdateBitrate(float64(p.Bitrate))
				case *rtcp.PictureLossIndication:
					log.Printf("[ABR] Session %s received PLI for SSRC: %d", sessionID, p.MediaSSRC)
					abr.HandlePLI()
					media.DefaultEngine.RequestKeyFrame(camID)
				}
			}
		}
	}()

	// Connect to media engine if camera is attached
	if subChan, err := media.DefaultEngine.Subscribe(camID); err == nil {
		go func() {
			for {
				select {
				case <-stopChan:
					media.DefaultEngine.Unsubscribe(camID, subChan)
					return
				case mp, ok := <-subChan:
					if !ok {
						return
					}
					if len(mp.RTPPackets) > 0 {
						for _, pkt := range mp.RTPPackets {
							_ = session.WriteRTP(pkt)
						}
					} else if mp.OriginalRTP != nil {
						_ = session.WriteRTP(mp.OriginalRTP)
					}
				}
			}
		}()
	}

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdpOffer,
	}

	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		session.Close()
		rollbackViewerCount()
		return "", err
	}

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		session.Close()
		rollbackViewerCount()
		return "", err
	}

	if err = peerConnection.SetLocalDescription(answer); err != nil {
		session.Close()
		rollbackViewerCount()
		return "", err
	}

	// Save session
	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	return answer.SDP, nil
}

func (m *Manager) AddICECandidate(sessionID, candidate string) error {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	candStr := strings.TrimSpace(candidate)
	if candStr == "" || candStr == "null" || candStr == "{}" {
		return nil
	}

	// Try unmarshaling as full ICECandidateInit JSON object
	var candInit webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(candStr), &candInit); err == nil {
		if candInit.Candidate == "" {
			// End-of-candidates indication from browser
			return nil
		}
		return session.PC.AddICECandidate(candInit)
	}

	// Try unmarshaling as JSON string (in case it was double-encoded)
	var rawStr string
	if err := json.Unmarshal([]byte(candStr), &rawStr); err == nil && rawStr != "" {
		var innerInit webrtc.ICECandidateInit
		if err := json.Unmarshal([]byte(rawStr), &innerInit); err == nil {
			if innerInit.Candidate == "" {
				return nil
			}
			return session.PC.AddICECandidate(innerInit)
		}
		if !strings.HasPrefix(rawStr, "{") {
			return session.PC.AddICECandidate(webrtc.ICECandidateInit{Candidate: rawStr})
		}
		return nil
	}

	// If it starts with '{', it's JSON that couldn't be parsed properly; ignore
	if strings.HasPrefix(candStr, "{") {
		return nil
	}

	// Fallback to raw candidate string
	return session.PC.AddICECandidate(webrtc.ICECandidateInit{Candidate: candStr})
}

func (m *Manager) RemoveSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, ok := m.sessions[sessionID]; ok {
		session.Close()
		delete(m.sessions, sessionID)

		if m.viewerCounts[session.CamID] > 0 {
			m.viewerCounts[session.CamID]--
		}

		log.Printf("Closed and removed WebRTC session: %s", sessionID)
	}
}

func (m *Manager) GetSession(sessionID string) (*WebRTCSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[sessionID]
	return s, ok
}

func (m *Manager) GetABRController(sessionID string) (*ABRController, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[sessionID]
	if !ok || s.ABR == nil {
		return nil, false
	}
	return s.ABR, true
}

func (m *Manager) WriteRTP(sessionID string, pkt *rtp.Packet) error {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	return session.WriteRTP(pkt)
}

func (m *Manager) BroadcastRTP(camID string, pkt *rtp.Packet) {
	m.mu.RLock()
	var targets []*WebRTCSession
	for _, sess := range m.sessions {
		if sess.CamID == camID {
			targets = append(targets, sess)
		}
	}
	m.mu.RUnlock()

	for _, sess := range targets {
		_ = sess.WriteRTP(pkt)
	}
}

var DefaultManager = NewManager()
