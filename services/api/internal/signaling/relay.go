package signaling

import (
	"log"
	"net/http"
	"sync"
	"central-api/internal/database"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for MVP
	},
}

// SessionRelay tracks active WebRTC connections between clients and HSS.
type SessionRelay struct {
	mu          sync.RWMutex
	devices     map[string]*websocket.Conn // DeviceID -> WSS Connection
	clientConns map[string]*websocket.Conn // SessionID -> WSS Connection
}

func NewSessionRelay() *SessionRelay {
	return &SessionRelay{
		devices:     make(map[string]*websocket.Conn),
		clientConns: make(map[string]*websocket.Conn),
	}
}

// HandleDevice connects an HSS edge daemon to the control plane.
func (r *SessionRelay) HandleDevice(w http.ResponseWriter, req *http.Request) {
	deviceID := req.Header.Get("X-Device-ID")
	if deviceID == "" {
		http.Error(w, "Missing Device ID", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("Signaling Relay Upgrade Error (Device %s): %v", deviceID, err)
		return
	}
	defer conn.Close()

	r.mu.Lock()
	r.devices[deviceID] = conn
	r.mu.Unlock()
	
	log.Printf("HSS Device Connected to Control Plane: %s", deviceID)

	defer func() {
		r.mu.Lock()
		delete(r.devices, deviceID)
		r.mu.Unlock()
		log.Printf("HSS Device Disconnected: %s", deviceID)
	}()

	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		// Route message from Device -> Client
		if sessionID, ok := msg["session_id"].(string); ok {
			r.mu.RLock()
			clientConn, exists := r.clientConns[sessionID]
			r.mu.RUnlock()
			
			if exists {
				clientConn.WriteJSON(msg)
			}
		}
	}
}

// HandleClient connects an end-user's browser/mobile app to the control plane.
func (r *SessionRelay) HandleClient(w http.ResponseWriter, req *http.Request, userID string) {
	sessionID := req.URL.Query().Get("session_id")
	deviceID := req.URL.Query().Get("device_id")
	
	if sessionID == "" || deviceID == "" {
		http.Error(w, "Missing session parameters", http.StatusBadRequest)
		return
	}

	// Tenancy Check: verify the user is a member of the home that owns this device
	var role string
	query := `
		SELECT hm.role 
		FROM home_members hm
		JOIN devices d ON hm.home_id = d.home_id
		WHERE hm.user_id = $1 AND d.id = $2
	`
	err := database.DB.QueryRow(query, userID, deviceID).Scan(&role)
	if err != nil {
		http.Error(w, "Unauthorized or device not found", http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("Signaling Relay Upgrade Error (Client Session %s): %v", sessionID, err)
		return
	}
	defer conn.Close()

	r.mu.Lock()
	r.clientConns[sessionID] = conn
	r.mu.Unlock()

	log.Printf("Client Session Connected: %s (Targeting %s) by User %s", sessionID, deviceID, userID)

	defer func() {
		r.mu.Lock()
		delete(r.clientConns, sessionID)
		r.mu.Unlock()
		log.Printf("Client Session Disconnected: %s", sessionID)
	}()

	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		// Route message from Client -> Device
		r.mu.RLock()
		deviceConn, exists := r.devices[deviceID]
		r.mu.RUnlock()
		
		if exists {
			deviceConn.WriteJSON(msg)
		} else {
			log.Printf("Failed to route message: Target device %s offline", deviceID)
		}
	}
}
