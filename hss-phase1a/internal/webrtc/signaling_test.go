package webrtc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

func TestSafeWSConn_ConcurrentWritesAndClose(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		rawConn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := newSafeWS(rawConn)
		defer conn.Close()

		for {
			_, _, err := rawConn.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	safeClient := newSafeWS(clientConn)

	var wg sync.WaitGroup
	numWriters := 20
	numMessages := 50

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				msg := SignalingMessage{
					Type:      "candidate",
					SessionID: "sess-test",
					Candidate: "candidate:foo",
				}
				_ = safeClient.WriteJSON(msg)
				_ = safeClient.WriteMessage(websocket.TextMessage, []byte(`{"test":true}`))
				time.Sleep(100 * time.Microsecond)
			}
		}(i)
	}

	// Trigger close concurrently while writers are still active
	time.Sleep(5 * time.Millisecond)
	_ = safeClient.Close()
	wg.Wait()
}

func TestManager_TURNOptimizationPriority(t *testing.T) {
	mgr := NewManager()

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

	sessionID := "turn-priority-sess"
	camID := "turn-priority-cam"

	customTURN := "turn:custom-fast-turn.pulse.internal:3478"
	customUser := "pulseuser"
	customPass := "pulsepass"

	answerSDP, err := mgr.HandleOffer(sessionID, camID, offer.SDP, customTURN, customUser, customPass, nil)
	if err != nil {
		t.Fatalf("HandleOffer with custom TURN failed: %v", err)
	}
	defer mgr.RemoveSession(sessionID)

	if answerSDP == "" {
		t.Fatal("Expected valid answer SDP")
	}

	sess, ok := mgr.GetSession(sessionID)
	if !ok || sess == nil {
		t.Fatal("Session not found")
	}
}
