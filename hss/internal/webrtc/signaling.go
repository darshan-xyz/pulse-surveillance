package webrtc

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for MVP, secure in production
	},
}

type SignalingMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	CameraID  string `json:"camera_id"`
	SDP       string `json:"sdp,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

// HandleSignaling upgrades the HTTP connection and routes WebRTC SDPs.
func (m *Manager) HandleSignaling(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket: %v", err)
		return
	}
	activeSessions := make(map[string]bool)
	defer func() {
		for sessionID := range activeSessions {
			m.RemoveSession(sessionID)
		}
		conn.Close()
	}()

	for {
		var msg SignalingMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("WebSocket error or closed: %v", err)
			break
		}

		switch msg.Type {
		case "offer":
			log.Printf("Received WebRTC Offer for camera %s (session %s)", msg.CameraID, msg.SessionID)
			
			activeSessions[msg.SessionID] = true

			// Callback for local ICE candidates
			onCandidate := func(c *webrtc.ICECandidate) {
				candJSON, _ := json.Marshal(c.ToJSON())
				resp := SignalingMessage{
					Type:      "candidate",
					SessionID: msg.SessionID,
					Candidate: string(candJSON),
				}
				if err := conn.WriteJSON(resp); err != nil {
					log.Printf("Error sending ICE candidate: %v", err)
				}
			}

			// Process offer and get answer
			sdpAnswer, err := m.HandleOffer(msg.SessionID, msg.CameraID, msg.SDP, onCandidate)
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
