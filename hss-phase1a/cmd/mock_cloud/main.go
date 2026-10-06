package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Global state for our mock cloud
var (
	mu          sync.Mutex
	hssConn     *websocket.Conn // The edge daemon
	browserConn *websocket.Conn // The remote viewer
	
	// Access Control List (Authz)
	// Maps a mock user token to the allowed DeviceID
	acl = map[string]string{
		"mock_user_jwt": "mock_device", // Will be updated when HSS connects
	}
	activeDeviceID string
)

type SignalingMessage struct {
	Type         string `json:"type"`
	SessionID    string `json:"session_id"`
	CameraID     string `json:"camera_id"`
	SDP          string `json:"sdp,omitempty"`
	Candidate    string `json:"candidate,omitempty"`
	TurnServer   string `json:"turn_server,omitempty"`
	TurnUsername string `json:"turn_username,omitempty"`
	TurnPassword string `json:"turn_password,omitempty"`
}

func main() {
	log.Println("Starting Mock Central Control Plane on :9090...")

	// Serve the remote test client HTML
	http.Handle("/", http.FileServer(http.Dir("web_remote")))

	// Endpoint for HSS Edge Daemon to connect to
	http.HandleFunc("/signaling", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			log.Println("Missing or invalid Authorization header from HSS")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		
		// Parse JWT (without signature verification for mock purposes)
		token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
		if err != nil {
			log.Printf("Invalid HSS JWT: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			log.Println("Invalid JWT claims")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		deviceID := claims["sub"].(string)

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Failed to upgrade HSS connection: %v", err)
			return
		}
		
		mu.Lock()
		hssConn = conn
		activeDeviceID = deviceID
		acl["mock_user_jwt"] = deviceID // Dynamically allow the mock user to access this device
		mu.Unlock()
		
		log.Printf("HSS Edge Daemon [%s] authenticated and connected to Mock Cloud!", deviceID)

		defer func() {
			mu.Lock()
			if hssConn == conn {
				hssConn = nil
			}
			mu.Unlock()
			conn.Close()
			log.Println("HSS Edge Daemon disconnected.")
		}()

		// Read messages from HSS and forward to Browser
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			
			mu.Lock()
			bc := browserConn
			mu.Unlock()

			if bc != nil {
				bc.WriteMessage(websocket.TextMessage, msg)
			} else {
				log.Println("Received message from HSS but no browser connected. Dropping.")
			}
		}
	})

	// Endpoint for Remote Web Browser to connect to
	http.HandleFunc("/client", func(w http.ResponseWriter, r *http.Request) {
		userToken := r.URL.Query().Get("token")
		if userToken == "" {
			log.Println("Missing user token in request")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		mu.Lock()
		allowedDevice, ok := acl[userToken]
		currentDevice := activeDeviceID
		mu.Unlock()

		if !ok || allowedDevice != currentDevice {
			log.Printf("User %s is not authorized to access device %s", userToken, currentDevice)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Failed to upgrade Browser connection: %v", err)
			return
		}

		mu.Lock()
		browserConn = conn
		mu.Unlock()

		log.Println("Remote Browser connected to Mock Cloud!")

		defer func() {
			mu.Lock()
			if browserConn == conn {
				browserConn = nil
			}
			mu.Unlock()
			conn.Close()
			log.Println("Remote Browser disconnected.")
		}()

		// Read messages from Browser and forward to HSS
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}

			mu.Lock()
			hc := hssConn
			mu.Unlock()

			if hc != nil {
				// Intercept the message to inject TURN credentials if it's an offer
				var sigMsg SignalingMessage
				if err := json.Unmarshal(msg, &sigMsg); err == nil && sigMsg.Type == "offer" {
					sigMsg.TurnServer = "turn:coturn:3478"
					sigMsg.TurnUsername = "pulse"
					sigMsg.TurnPassword = "landing"
					if modifiedMsg, err := json.Marshal(sigMsg); err == nil {
						msg = modifiedMsg
					}
				}

				hc.WriteMessage(websocket.TextMessage, msg)
			} else {
				log.Println("Received message from Browser but no HSS connected. Dropping.")
			}
		}
	})

	log.Fatal(http.ListenAndServe(":9090", nil))
}
