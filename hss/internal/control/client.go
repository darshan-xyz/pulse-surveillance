package control

import (
	"log"
	"net/url"
	"time"

	"hss/internal/security"
	"hss/internal/webrtc"
	"github.com/gorilla/websocket"
)

type Client struct {
	Endpoint string
}

func NewClient(endpoint string) *Client {
	return &Client{
		Endpoint: endpoint,
	}
}

func (c *Client) Connect() {
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		log.Fatalf("Invalid control plane endpoint: %v", err)
	}

	log.Printf("Connecting to Central Control Plane: %s", u.String())

	for {
		c.dialAndListen(u.String())
		log.Println("Disconnected from Control Plane. Retrying in 5 seconds...")
		time.Sleep(5 * time.Second)
	}
}

func (c *Client) dialAndListen(endpoint string) {
	headers := map[string][]string{
		"X-Device-ID": {security.DeviceID},
	}

	conn, _, err := websocket.DefaultDialer.Dial(endpoint, headers)
	if err != nil {
		log.Printf("Control plane dial error: %v", err)
		return
	}
	defer conn.Close()

	log.Println("Successfully connected to Central Control Plane WSS.")

	// Ping loop
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	for {
		var msg webrtc.SignalingMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("Control WSS error: %v", err)
			return
		}

		// Handle remote signaling messages
		switch msg.Type {
		case "offer":
			log.Printf("Received Remote WebRTC Offer for session: %s", msg.SessionID)
			
			sdpAnswer, err := webrtc.DefaultManager.HandleOffer(msg.SessionID, msg.CameraID, msg.SDP)
			if err != nil {
				log.Printf("Failed to handle remote offer: %v", err)
				continue
			}

			// Send back answer
			resp := webrtc.SignalingMessage{
				Type:      "answer",
				SessionID: msg.SessionID,
				SDP:       sdpAnswer,
			}
			conn.WriteJSON(resp)

		case "candidate":
			if err := webrtc.DefaultManager.AddICECandidate(msg.SessionID, msg.Candidate); err != nil {
				log.Printf("Error adding remote ICE candidate: %v", err)
			}
		}
	}
}
