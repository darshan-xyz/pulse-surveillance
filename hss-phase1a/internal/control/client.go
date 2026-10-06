package control

import (
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"hss/internal/security"
	"hss/internal/webrtc"

	"github.com/gorilla/websocket"
	pionwebrtc "github.com/pion/webrtc/v4"
)

type Client struct {
	Endpoint  string
	TLSConfig *tls.Config
}

var (
	activeConnMu sync.Mutex
	activeConn   *websocket.Conn
)

// SendSignalingMessage transmits an arbitrary signaling or detection message over the control plane WebSocket.
func SendSignalingMessage(v interface{}) error {
	activeConnMu.Lock()
	defer activeConnMu.Unlock()
	if activeConn == nil {
		return nil
	}
	return activeConn.WriteJSON(v)
}

func NewClient(endpoint string) *Client {
	return &Client{
		Endpoint: endpoint,
	}
}

func NewClientWithTLS(endpoint string, tlsConfig *tls.Config) *Client {
	return &Client{
		Endpoint:  endpoint,
		TLSConfig: tlsConfig,
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
	token, err := security.GenerateDeviceToken()
	if err != nil {
		log.Printf("Failed to generate device token: %v", err)
		return
	}

	headers := map[string][]string{
		"Authorization": {"Bearer " + token},
	}

	dialer := websocket.DefaultDialer
	if c.TLSConfig != nil {
		dialer = &websocket.Dialer{
			TLSClientConfig:  c.TLSConfig,
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 10 * time.Second,
		}
	} else if os.Getenv("TLS_CA_FILE") != "" {
		caFile := os.Getenv("TLS_CA_FILE")
		certFile := os.Getenv("TLS_CLIENT_CERT_FILE")
		keyFile := os.Getenv("TLS_CLIENT_KEY_FILE")
		if caFile != "" && certFile != "" && keyFile != "" {
			caPEM, certPEM, keyPEM, err := security.LoadMTLSCredentialsFromFiles(caFile, certFile, keyFile)
			if err == nil {
				tlsCfg, err := security.NewClientMTLSConfig(caPEM, certPEM, keyPEM)
				if err == nil {
					dialer = &websocket.Dialer{
						TLSClientConfig:  tlsCfg,
						Proxy:            http.ProxyFromEnvironment,
						HandshakeTimeout: 10 * time.Second,
					}
				}
			}
		}
	}

	conn, _, err := dialer.Dial(endpoint, headers)
	if err != nil {
		log.Printf("Control plane dial error: %v", err)
		return
	}
	defer conn.Close()

	activeConnMu.Lock()
	activeConn = conn
	activeConnMu.Unlock()
	defer func() {
		activeConnMu.Lock()
		if activeConn == conn {
			activeConn = nil
		}
		activeConnMu.Unlock()
	}()

	log.Println("Successfully connected to Central Control Plane WSS.")

	var writeMu sync.Mutex
	safeWriteMsg := func(messageType int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(messageType, data)
	}
	safeWriteJSON := func(v interface{}) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(v)
	}

	// Ping loop
	pingDone := make(chan struct{})
	defer close(pingDone)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-pingDone:
				return
			case <-ticker.C:
				if err := safeWriteMsg(websocket.PingMessage, nil); err != nil {
					return
				}
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
			camID := msg.CameraID
			if camID == "" {
				camID = msg.DeviceID
			}
			log.Printf("Received Remote WebRTC Offer for session: %s (camera: %s)", msg.SessionID, camID)
			
			// Callback to route local ICE candidates back to the Control Plane
			onCandidate := func(c *pionwebrtc.ICECandidate) {
				candJSON, _ := json.Marshal(c.ToJSON())
				resp := webrtc.SignalingMessage{
					Type:      "candidate",
					SessionID: msg.SessionID,
					CameraID:  camID,
					DeviceID:  msg.DeviceID,
					Candidate: string(candJSON),
				}
				_ = safeWriteJSON(resp)
			}

			sdpAnswer, err := webrtc.DefaultManager.HandleOffer(msg.SessionID, camID, msg.SDP, msg.TurnServer, msg.TurnUsername, msg.TurnPassword, onCandidate)
			if err != nil {
				log.Printf("Failed to handle remote offer: %v", err)
				continue
			}

			// Send back answer
			resp := webrtc.SignalingMessage{
				Type:      "answer",
				SessionID: msg.SessionID,
				CameraID:  camID,
				DeviceID:  msg.DeviceID,
				SDP:       sdpAnswer,
			}
			_ = safeWriteJSON(resp)

		case "candidate":
			if err := webrtc.DefaultManager.AddICECandidate(msg.SessionID, msg.Candidate); err != nil {
				log.Printf("Error adding remote ICE candidate for session %s: %v", msg.SessionID, err)
			}
		}
	}
}
