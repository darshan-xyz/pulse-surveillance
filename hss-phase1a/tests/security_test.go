package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"hss/internal/cloud/db"
	"hss/internal/cloud/signaling"
)

// TestAuthorizationBypass verifies that a user cannot request a WebRTC session
// with a device they do not explicitly own in the PostgreSQL database.
func TestAuthorizationBypass(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping security test in short mode")
	}

	setupTestEnvironment(t) // Initializes test DB

	// 1. Seed two distinct users and one camera
	_, err := db.DB.Exec(`
		TRUNCATE users, devices, permissions CASCADE;
		INSERT INTO users (id, username, password_hash) VALUES (1, 'Alice', 'hash');
		INSERT INTO users (id, username, password_hash) VALUES (2, 'Eve', 'hash');
		
		INSERT INTO devices (device_id, public_key) VALUES ('camera_A', 'dummy_key');
		
		-- Alice owns Camera A. Eve owns nothing.
		INSERT INTO permissions (user_id, device_id, role) VALUES (1, 'camera_A', 'owner');
	`)
	if err != nil {
		t.Fatalf("Failed to seed security DB: %v", err)
	}

	// 2. Start Cloud Hub
	mux := http.NewServeMux()
	mux.HandleFunc("/client", signaling.HandleViewerConnection)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// 3. Eve (Malicious User) connects to the Cloud
	eveConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token=Eve", nil)
	if err != nil {
		t.Fatalf("Eve failed to connect to WebSocket: %v", err)
	}
	defer eveConn.Close()

	// 4. Eve attempts to request a WebRTC stream from Alice's Camera
	maliciousOffer := map[string]string{
		"type":       "offer",
		"device_id":  "camera_A",
		"session_id": "hacked_session",
		"sdp":        `{"type":"offer","sdp":"mock"}`,
	}
	if err := eveConn.WriteJSON(maliciousOffer); err != nil {
		t.Fatalf("Eve failed to send malicious offer: %v", err)
	}

	// 5. Assert the Cloud Hub detects the unauthorized access and terminates the socket
	eveConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := eveConn.ReadMessage()
	
	if err == nil {
		var resp map[string]interface{}
		json.Unmarshal(msg, &resp)
		
		// The server should explicitly return an error JSON before disconnecting
		if resp["error"] == nil {
			t.Fatalf("SECURITY VULNERABILITY: Server accepted unauthorized offer! Response: %v", resp)
		}
		t.Logf("Server rejected malicious offer with error: %v", resp["error"])
	}

	// Wait a moment for the server to forcibly close the connection
	time.Sleep(100 * time.Millisecond)
	
	// Any further writes should fail because the socket is closed
	err = eveConn.WriteJSON(maliciousOffer)
	if err == nil {
		t.Fatalf("SECURITY VULNERABILITY: WebSocket remained open after unauthorized attempt!")
	}

	t.Log("SUCCESS: Authorization Bypass thwarted! The system securely blocked Eve from accessing Alice's camera.")
}
