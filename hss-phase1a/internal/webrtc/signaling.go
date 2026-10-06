package webrtc

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for MVP, secure in production
	},
}

// safeWSConn provides thread-safe writing and closing for gorilla/websocket connections
type safeWSConn struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	closed bool
}

func newSafeWS(c *websocket.Conn) *safeWSConn {
	return &safeWSConn{conn: c}
}

func (s *safeWSConn) WriteMessage(messageType int, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return websocket.ErrCloseSent
	}
	return s.conn.WriteMessage(messageType, data)
}

func (s *safeWSConn) WriteJSON(v interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return websocket.ErrCloseSent
	}
	return s.conn.WriteJSON(v)
}

func (s *safeWSConn) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.conn.Close()
}

type SignalingMessage struct {
	Type         string `json:"type"`
	SessionID    string `json:"session_id"`
	CameraID     string `json:"camera_id,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`
	SDP          string `json:"sdp,omitempty"`
	Candidate    string `json:"candidate,omitempty"`
	TurnServer   string `json:"turn_server,omitempty"`
	TurnUsername string `json:"turn_username,omitempty"`
	TurnPassword string `json:"turn_password,omitempty"`
}

// HandleSignaling upgrades the HTTP connection and routes WebRTC SDPs.
func (m *Manager) HandleSignaling(w http.ResponseWriter, r *http.Request) {
	rawConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket: %v", err)
		return
	}
	rawConn.SetReadLimit(64 * 1024)
	conn := newSafeWS(rawConn)
	activeSessions := make(map[string]bool)
	defer func() {
		for sessionID := range activeSessions {
			m.RemoveSession(sessionID)
		}
		conn.Close()
	}()

	for {
		var msg SignalingMessage
		if err := rawConn.ReadJSON(&msg); err != nil {
			log.Printf("WebSocket error or closed: %v", err)
			break
		}

		switch msg.Type {
		case "offer":
			camID := msg.CameraID
			if camID == "" {
				camID = msg.DeviceID
			}
			log.Printf("Received WebRTC Offer for camera %s (session %s)", camID, msg.SessionID)
			
			activeSessions[msg.SessionID] = true

			// Callback for local ICE candidates
			onCandidate := func(c *webrtc.ICECandidate) {
				candJSON, _ := json.Marshal(c.ToJSON())
				resp := SignalingMessage{
					Type:      "candidate",
					SessionID: msg.SessionID,
					CameraID:  camID,
					DeviceID:  msg.DeviceID,
					Candidate: string(candJSON),
				}
				if err := conn.WriteJSON(resp); err != nil {
					log.Printf("Error sending ICE candidate: %v", err)
				}
			}

			// Process offer and get answer
			sdpAnswer, err := m.HandleOffer(msg.SessionID, camID, msg.SDP, msg.TurnServer, msg.TurnUsername, msg.TurnPassword, onCandidate)
			if err != nil {
				log.Printf("Error handling offer: %v", err)
				continue
			}

			// Send answer back
			resp := SignalingMessage{
				Type:      "answer",
				SessionID: msg.SessionID,
				SDP:       sdpAnswer,
			}
			if err := conn.WriteJSON(resp); err != nil {
				log.Printf("Error sending answer: %v", err)
				break
			}

		case "candidate":
			log.Printf("Received ICE Candidate for session %s", msg.SessionID)
			if err := m.AddICECandidate(msg.SessionID, msg.Candidate); err != nil {
				log.Printf("Error adding ICE candidate: %v", err)
			}

		default:
			log.Printf("Unknown signaling message type: %s", msg.Type)
		}
	}
}
