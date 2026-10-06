package webrtc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

// TestSafeWSConn_AdversarialCandidateFlood tests safeWSConn under a high-intensity flood
// of concurrent candidate messages across 50 simultaneous goroutines.
func TestSafeWSConn_AdversarialCandidateFlood(t *testing.T) {
	var serverReceivedCount int64
	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawConn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer rawConn.Close()

		for {
			var msg SignalingMessage
			if err := rawConn.ReadJSON(&msg); err != nil {
				break
			}
			atomic.AddInt64(&serverReceivedCount, 1)
		}
	}))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to signaling test server: %v", err)
	}
	safeClient := newSafeWS(clientConn)
	defer safeClient.Close()

	const numWorkers = 50
	const messagesPerWorker = 100
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	startGate := make(chan struct{})

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			<-startGate

			for m := 0; m < messagesPerWorker; m++ {
				candMsg := SignalingMessage{
					Type:      "candidate",
					SessionID: fmt.Sprintf("sess-flood-%d", workerID),
					CameraID:  "cam-stress-01",
					Candidate: fmt.Sprintf(`{"candidate":"candidate:%d 1 UDP 2130706431 127.0.0.1 %d typ host","sdpMid":"1","sdpMLineIndex":1}`, m, 50000+m),
				}
				if err := safeClient.WriteJSON(candMsg); err != nil {
					t.Errorf("Worker %d failed to write message %d: %v", workerID, m, err)
					return
				}
			}
		}(w)
	}

	// Unleash all workers simultaneously
	close(startGate)
	wg.Wait()

	// Wait for server to process incoming messages
	time.Sleep(100 * time.Millisecond)

	expectedTotal := int64(numWorkers * messagesPerWorker)
	actualTotal := atomic.LoadInt64(&serverReceivedCount)
	if actualTotal != expectedTotal {
		t.Fatalf("Expected %d messages delivered through safeWSConn, got %d", expectedTotal, actualTotal)
	}
}

// TestSafeWSConn_SimultaneousCloseAndWriteRace tests race conditions between concurrent writes
// and simultaneous connection termination.
func TestSafeWSConn_SimultaneousCloseAndWriteRace(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawConn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer rawConn.Close()

		for {
			if _, _, err := rawConn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer s.Close()

	for trial := 0; trial < 10; trial++ {
		wsURL := "ws" + strings.TrimPrefix(s.URL, "http")
		clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Trial %d: Failed to connect: %v", trial, err)
		}
		safeClient := newSafeWS(clientConn)

		const numWriters = 30
		var wg sync.WaitGroup
		wg.Add(numWriters + 5) // 30 writers + 5 concurrent closers

		startBarrier := make(chan struct{})

		for w := 0; w < numWriters; w++ {
			go func(id int) {
				defer wg.Done()
				<-startBarrier

				for i := 0; i < 50; i++ {
					msg := SignalingMessage{
						Type:      "candidate",
						SessionID: fmt.Sprintf("sess-close-race-%d", id),
						Candidate: "candidate:1 1 UDP 2130706431 127.0.0.1 50000 typ host",
					}
					_ = safeClient.WriteJSON(msg)
					_ = safeClient.WriteMessage(websocket.TextMessage, []byte(`{"ping":true}`))
				}
			}(w)
		}

		for c := 0; c < 5; c++ {
			go func(closerID int) {
				defer wg.Done()
				<-startBarrier
				time.Sleep(time.Duration(closerID) * 50 * time.Microsecond)
				_ = safeClient.Close()
			}(c)
		}

		close(startBarrier)
		wg.Wait()

		// Subsequent Close after all routines finish must be safe and idempotent
		if err := safeClient.Close(); err != nil {
			t.Errorf("Trial %d: Post-race Close() failed: %v", trial, err)
		}
	}
}

// TestManager_AddICECandidate_AdversarialInputs tests Manager.AddICECandidate with various
// corrupt, malformed, empty, and edge case candidate formats.
func TestManager_AddICECandidate_AdversarialInputs(t *testing.T) {
	mgr := NewManager()

	// 1. Candidate sent to non-existent session
	err := mgr.AddICECandidate("non-existent-session-id", `{"candidate":"candidate:1 1 UDP 2130706431 127.0.0.1 50000 typ host"}`)
	if err == nil {
		t.Fatal("Expected error when adding candidate to non-existent session, got nil")
	}

	// 2. Create valid session
	clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create client PC: %v", err)
	}
	defer clientPC.Close()

	_, _ = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	_ = clientPC.SetLocalDescription(offer)

	sessionID := "adv-cand-session"
	camID := "adv-cand-cam"

	_, err = mgr.HandleOffer(sessionID, camID, offer.SDP, "", "", "", nil)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}
	defer mgr.RemoveSession(sessionID)

	// Adversarial candidate strings: empty, whitespace, null, json null, broken json, nested string
	adversarialCandidates := []string{
		"",
		"   ",
		"null",
		"{}",
		`{"candidate":""}`,
		`{"candidate":null}`,
		`{"invalid_field": 123}`,
		`"null"`,
		`""`,
		`"{\"candidate\":\"\"}"`,
		`"{\"candidate\":\"candidate:1 1 UDP 2130706431 127.0.0.1 50000 typ host\"}"`,
		`candidate:2 1 UDP 2130706431 127.0.0.1 50001 typ host`,
		`NOT_A_VALID_CANDIDATE_FORMAT`,
	}

	for idx, cand := range adversarialCandidates {
		err := mgr.AddICECandidate(sessionID, cand)
		// Should not panic or corrupt state
		t.Logf("Candidate test #%d (%q) result: %v", idx, cand, err)
	}
}

// TestManager_ConcurrentSessionsAndChurn tests creating, streaming, flooding candidates,
// and closing sessions concurrently under high load.
func TestManager_ConcurrentSessionsAndChurn(t *testing.T) {
	mgr := NewManager()
	const numConcurrent = 20
	var wg sync.WaitGroup
	wg.Add(numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		go func(id int) {
			defer wg.Done()
			sessID := fmt.Sprintf("churn-sess-%d", id)
			camID := fmt.Sprintf("churn-cam-%d", id%3)

			clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
			if err != nil {
				return
			}
			defer clientPC.Close()

			_, _ = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
				Direction: webrtc.RTPTransceiverDirectionRecvonly,
			})
			offer, err := clientPC.CreateOffer(nil)
			if err != nil {
				return
			}
			_ = clientPC.SetLocalDescription(offer)

			var candMutex sync.Mutex
			var receivedCandidates []*webrtc.ICECandidate
			onCandidate := func(c *webrtc.ICECandidate) {
				candMutex.Lock()
				defer candMutex.Unlock()
				receivedCandidates = append(receivedCandidates, c)
			}

			ansSDP, err := mgr.HandleOffer(sessID, camID, offer.SDP, "", "", "", onCandidate)
			if err != nil {
				// Camera viewer limit (3) might reject beyond limit, which is expected
				return
			}

			if ansSDP == "" {
				t.Errorf("Session %s returned empty answer SDP", sessID)
			}

			// Flood incoming candidates
			for c := 0; c < 10; c++ {
				candPayload, _ := json.Marshal(webrtc.ICECandidateInit{
					Candidate: fmt.Sprintf("candidate:%d 1 UDP 2130706431 127.0.0.1 %d typ host", c, 50000+c),
				})
				_ = mgr.AddICECandidate(sessID, string(candPayload))
			}

			// End-of-candidates
			_ = mgr.AddICECandidate(sessID, "null")

			time.Sleep(10 * time.Millisecond)
			mgr.RemoveSession(sessID)
		}(i)
	}

	wg.Wait()
}
