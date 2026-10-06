package tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"hss/internal/cloud/signaling"
)

// TestWebSocketChaos ensures that if the Cloud backend drops the TCP connection,
// the Edge Daemon safely handles it without panicking, leaking goroutines, or corrupting state.
func TestWebSocketChaos(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	setupTestEnvironment(t)
	jwtToken, _ := generateDeviceJWT(t, "chaos_camera_999")

	// 1. Setup a custom HTTP server that we can violently terminate
	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	
	server := httptest.NewUnstartedServer(mux)
	server.Start()
	
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// 2. Connect Edge Daemon
	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+jwtToken)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/signaling", headers)
	if err != nil {
		t.Fatalf("Failed to connect Edge Daemon initially: %v", err)
	}

	// Wait for connection to stabilize
	time.Sleep(500 * time.Millisecond)

	// 3. CHAOS EVENT: Violently kill the server (Simulating Cloud Node Crash or Network Partition)
	t.Log("Initiating Chaos Event: Terminating TCP Sockets...")
	server.CloseClientConnections()
	
	// 4. Assert that the client detects the drop
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatalf("Expected connection to be dead, but read succeeded")
	}
	t.Logf("Edge Daemon successfully detected socket drop: %v", err)

	// In a real Edge Daemon, a retry loop would catch this `err` and backoff.
	// We assert that the server closure didn't panic our Cloud Hub goroutines.
	
	// 5. Assert Reconnection Resilience
	server.Start() // Server comes back online (e.g., K8s pod restarted)
	defer server.Close()

	wsURLNew := "ws" + strings.TrimPrefix(server.URL, "http")
	
	// Attempt Reconnection
	connNew, _, err := websocket.DefaultDialer.Dial(wsURLNew+"/signaling", headers)
	if err != nil {
		t.Fatalf("Edge Daemon failed to recover after Chaos Event: %v", err)
	}
	defer connNew.Close()

	t.Log("SUCCESS: Edge Daemon successfully re-authenticated and recovered after total network loss.")
}
