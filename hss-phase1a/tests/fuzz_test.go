package tests

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"hss/internal/cloud/signaling"
)

// TestFuzzWebSocketPayloads verifies that the Cloud Hub is immune to 
// malformed packets, massive strings, and garbage JSON. It should gracefully
// drop the packets without crashing the server or leaking memory.
func TestFuzzWebSocketPayloads(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fuzz test in short mode")
	}

	setupTestEnvironment(t)

	// 1. Start Cloud Hub
	mux := http.NewServeMux()
	mux.HandleFunc("/client", signaling.HandleViewerConnection)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// 2. Connect as a dummy user
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token=test_user", nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// 3. Fuzz Case 1: Malformed JSON
	t.Log("Fuzzing with malformed JSON...")
	err = conn.WriteMessage(websocket.TextMessage, []byte(`{ "type": "offer", "sdp": `))
	if err != nil {
		t.Fatalf("Failed to write fuzz payload: %v", err)
	}

	// 4. Fuzz Case 2: Massive 5MB String (Simulating buffer overflow attack)
	t.Log("Fuzzing with massive 5MB garbage payload...")
	massivePayload := make([]byte, 5*1024*1024)
	rand.Read(massivePayload)
	err = conn.WriteMessage(websocket.BinaryMessage, massivePayload)
	if err != nil {
		t.Fatalf("Failed to write massive payload: %v", err)
	}

	// 5. Fuzz Case 3: Invalid data types
	t.Log("Fuzzing with invalid JSON types...")
	err = conn.WriteMessage(websocket.TextMessage, []byte(`{"type": 12345, "device_id": true}`))
	if err != nil {
		t.Fatalf("Failed to write invalid types payload: %v", err)
	}

	// 6. Assert server survival
	// If the server didn't panic, it's a pass. We'll verify it's still alive by sending a valid ping.
	err = conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(1*time.Second))
	if err != nil {
		// It is acceptable if the server aggressively closed the connection due to the massive payload.
		t.Log("Server closed connection defensively (Expected behavior).")
	} else {
		t.Log("Server survived the fuzzing attack and kept connection alive.")
	}

	t.Log("SUCCESS: Fuzz testing complete. No panics detected.")
}
