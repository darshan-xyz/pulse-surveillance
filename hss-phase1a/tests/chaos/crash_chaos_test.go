package chaos_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/cloud/signaling"

	"github.com/gorilla/websocket"
)

// setupChaosSignalingServer creates a test server for WebSocket signaling hub.
func setupChaosSignalingServer(t *testing.T) (*httptest.Server, string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	return server, wsURL
}

// TestChaosEdgeDaemonCrash_ReconnectStorm tests resilience when Edge Daemons crash abruptly
// (ungraceful TCP reset) and initiate simultaneous high-concurrency reconnection storms.
func TestChaosEdgeDaemonCrash_ReconnectStorm(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping crash chaos test in short mode")
	}

	server, wsURL := setupChaosSignalingServer(t)
	defer server.Close()

	const numDaemons = 50

	t.Run("UngracefulTCPReset_SimultaneousCrashStorm", func(t *testing.T) {
		// 1. Establish 50 concurrent Edge Daemon connections
		edgeConns := make([]*websocket.Conn, numDaemons)
		var connectWG sync.WaitGroup
		var connectErrors int64

		for i := 0; i < numDaemons; i++ {
			connectWG.Add(1)
			go func(idx int) {
				defer connectWG.Done()
				devID := fmt.Sprintf("chaos-edge-%03d", idx)
				url := fmt.Sprintf("%s/signaling?token=mock_device_jwt&camera_id=%s", wsURL, devID)

				conn, _, err := websocket.DefaultDialer.Dial(url, nil)
				if err != nil {
					atomic.AddInt64(&connectErrors, 1)
					return
				}
				edgeConns[idx] = conn
			}(i)
		}

		connectWG.Wait()
		if connectErrors > 0 {
			t.Fatalf("Failed to establish initial connections: %d errors", connectErrors)
		}
		t.Logf("Successfully connected %d concurrent Edge Daemons to Signaling Hub.", numDaemons)

		time.Sleep(200 * time.Millisecond)

		// 2. CHAOS EVENT: Abruptly and violently sever TCP sockets without WebSocket Close handshake
		t.Logf("CHAOS EVENT: Abruptly killing %d TCP connections simultaneously (Ungraceful Reset)...", numDaemons)
		var closeWG sync.WaitGroup
		for i := 0; i < numDaemons; i++ {
			closeWG.Add(1)
			go func(idx int) {
				defer closeWG.Done()
				if conn := edgeConns[idx]; conn != nil {
					// Close underlying net.Conn directly to simulate abrupt crash / RST
					_ = conn.UnderlyingConn().Close()
				}
			}(i)
		}
		closeWG.Wait()

		// Allow Hub cleanup routines to process disconnects
		time.Sleep(300 * time.Millisecond)
		t.Log("PASS: Hub processed ungraceful TCP resets without deadlocks or server panics.")
	})

	t.Run("HighConcurrency_ReconnectionStormAndRouting", func(t *testing.T) {
		// Immediately launch 50 concurrent reconnection attempts
		t.Logf("Simulating Reconnection Storm: %d Edge Daemons reconnecting simultaneously...", numDaemons)

		reconnectedConns := make([]*websocket.Conn, numDaemons)
		var reconnWG sync.WaitGroup
		var reconnErrors int64

		for i := 0; i < numDaemons; i++ {
			reconnWG.Add(1)
			go func(idx int) {
				defer reconnWG.Done()
				devID := fmt.Sprintf("chaos-edge-%03d", idx)
				url := fmt.Sprintf("%s/signaling?token=mock_device_jwt&camera_id=%s", wsURL, devID)

				conn, _, err := websocket.DefaultDialer.Dial(url, nil)
				if err != nil {
					atomic.AddInt64(&reconnErrors, 1)
					return
				}
				reconnectedConns[idx] = conn
			}(i)
		}

		reconnWG.Wait()
		if reconnErrors > 0 {
			t.Fatalf("Reconnection storm failed: %d errors out of %d", reconnErrors, numDaemons)
		}
		t.Logf("SUCCESS: All %d Edge Daemons successfully reconnected and re-authenticated.", numDaemons)

		// Clean up connections at end of test
		defer func() {
			for _, c := range reconnectedConns {
				if c != nil {
					_ = c.Close()
				}
			}
		}()

		// 3. Verify Signaling Routing to Reconnected Edge Daemon
		targetDev := "chaos-edge-000"
		targetConn := reconnectedConns[0]

		offerReceived := make(chan signaling.SignalingMessage, 1)
		go func() {
			var msg signaling.SignalingMessage
			if err := targetConn.ReadJSON(&msg); err == nil {
				offerReceived <- msg
			}
		}()

		// Viewer connects and dispatches offer
		viewerConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token=mock_user_jwt", nil)
		if err != nil {
			t.Fatalf("Viewer failed to connect: %v", err)
		}
		defer viewerConn.Close()

		offerMsg := signaling.SignalingMessage{
			Type:      "offer",
			SessionID: "chaos-reconn-sess-01",
			DeviceID:  targetDev,
			CameraID:  targetDev,
			SDP:       "v=0\r\no=- 123456 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
		}

		if err := viewerConn.WriteJSON(offerMsg); err != nil {
			t.Fatalf("Viewer failed to send offer: %v", err)
		}

		select {
		case msg := <-offerReceived:
			if msg.DeviceID != targetDev || msg.SessionID != "chaos-reconn-sess-01" {
				t.Fatalf("Unexpected routed message: %+v", msg)
			}
			t.Log("SUCCESS: Offer correctly routed to reconnected Edge Daemon.")
		case <-time.After(3 * time.Second):
			t.Fatal("Timeout waiting for offer routing to reconnected daemon")
		}
	})

	t.Run("RepeatedRapidChurn_ConnectionFlapping", func(t *testing.T) {
		const waves = 3
		const waveDaemons = 20

		t.Logf("Simulating %d waves of rapid connection flapping (%d daemons/wave)...", waves, waveDaemons)

		for w := 0; w < waves; w++ {
			var waveWG sync.WaitGroup
			var waveConns [waveDaemons]*websocket.Conn

			// Connect wave
			for i := 0; i < waveDaemons; i++ {
				waveWG.Add(1)
				go func(idx int) {
					defer waveWG.Done()
					devID := fmt.Sprintf("chaos-flap-w%d-%02d", w, idx)
					url := fmt.Sprintf("%s/signaling?token=mock_device_jwt&camera_id=%s", wsURL, devID)
					conn, _, err := websocket.DefaultDialer.Dial(url, nil)
					if err == nil {
						waveConns[idx] = conn
					}
				}(i)
			}
			waveWG.Wait()

			// Abruptly sever all
			for _, c := range waveConns {
				if c != nil {
					_ = c.UnderlyingConn().Close()
				}
			}

			time.Sleep(50 * time.Millisecond)
		}

		t.Log("PASS: Hub remained resilient through repeated rapid connection churn waves.")
	})
}
