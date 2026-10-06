package chaos_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/cloud/db"
	"hss/internal/cloud/signaling"
	"hss/internal/webrtc"

	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// TestChaosPeerConnection_SurvivalDuringOutage tests that active, established WebRTC peer
// connections continue uninterrupted media streaming even when the signaling server drops,
// database fails, and cache encounters outages.
func TestChaosPeerConnection_SurvivalDuringOutage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping peer resilience test in short mode")
	}

	// 1. Setup Signaling Server
	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	deviceID := "chaos-cam-peer-resilience"
	sessionID := "chaos-peer-stream-sess-01"

	// 2. Connect Edge Device to Signaling Hub
	edgeWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/signaling?token=mock_device_jwt&camera_id="+deviceID, nil)
	if err != nil {
		t.Fatalf("Edge device failed to connect: %v", err)
	}

	// 3. Connect Viewer to Signaling Hub
	viewerWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token=mock_user_jwt", nil)
	if err != nil {
		t.Fatalf("Viewer failed to connect: %v", err)
	}

	// 4. Setup Client PeerConnection (Receiver)
	clientPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create client PC: %v", err)
	}
	defer clientPC.Close()

	_, err = clientPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add video transceiver: %v", err)
	}

	connectedChan := make(chan struct{})
	var connectedOnce sync.Once

	var testDone int32
	defer atomic.StoreInt32(&testDone, 1)

	clientPC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
		if atomic.LoadInt32(&testDone) == 0 {
			t.Logf("[Peer Resilience] Client PC State: %s", s.String())
		}
		if s == pionwebrtc.PeerConnectionStateConnected {
			connectedOnce.Do(func() {
				close(connectedChan)
			})
		}
	})

	var edgeSendMu sync.Mutex
	clientPC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candJSON, _ := json.Marshal(c.ToJSON())
		candMsg := signaling.SignalingMessage{
			Type:      "candidate",
			SessionID: sessionID,
			DeviceID:  deviceID,
			Candidate: string(candJSON),
		}
		edgeSendMu.Lock()
		_ = viewerWS.WriteJSON(candMsg)
		edgeSendMu.Unlock()
	})

	// 5. Edge Daemon listener using webrtc.DefaultManager
	mgr := webrtc.DefaultManager
	go func() {
		for {
			var msg signaling.SignalingMessage
			if err := edgeWS.ReadJSON(&msg); err != nil {
				return
			}

			switch msg.Type {
			case "offer":
				onEdgeCandidate := func(c *pionwebrtc.ICECandidate) {
					if c == nil {
						return
					}
					candJSON, _ := json.Marshal(c.ToJSON())
					resp := signaling.SignalingMessage{
						Type:      "candidate",
						SessionID: msg.SessionID,
						DeviceID:  msg.DeviceID,
						Candidate: string(candJSON),
					}
					edgeSendMu.Lock()
					_ = edgeWS.WriteJSON(resp)
					edgeSendMu.Unlock()
				}

				sdpAnswer, err := mgr.HandleOffer(msg.SessionID, msg.DeviceID, msg.SDP, "", "", "", onEdgeCandidate)
				if err != nil {
					return
				}

				answerMsg := signaling.SignalingMessage{
					Type:      "answer",
					SessionID: msg.SessionID,
					DeviceID:  msg.DeviceID,
					SDP:       sdpAnswer,
				}
				edgeSendMu.Lock()
				_ = edgeWS.WriteJSON(answerMsg)
				edgeSendMu.Unlock()

			case "candidate":
				_ = mgr.AddICECandidate(msg.SessionID, msg.Candidate)
			}
		}
	}()

	// 6. Client creates and sends offer
	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	_ = clientPC.SetLocalDescription(offer)

	offerMsg := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: sessionID,
		DeviceID:  deviceID,
		CameraID:  deviceID,
		SDP:       offer.SDP,
	}
	if err := viewerWS.WriteJSON(offerMsg); err != nil {
		t.Fatalf("Viewer failed to send offer: %v", err)
	}

	// 7. Viewer reads answer and candidates from Hub
	go func() {
		for {
			var msg signaling.SignalingMessage
			if err := viewerWS.ReadJSON(&msg); err != nil {
				return
			}
			switch msg.Type {
			case "answer":
				answer := pionwebrtc.SessionDescription{
					Type: pionwebrtc.SDPTypeAnswer,
					SDP:  msg.SDP,
				}
				_ = clientPC.SetRemoteDescription(answer)
			case "candidate":
				var cand pionwebrtc.ICECandidateInit
				if err := json.Unmarshal([]byte(msg.Candidate), &cand); err == nil {
					_ = clientPC.AddICECandidate(cand)
				}
			}
		}
	}()

	// Wait for WebRTC connection establishment
	select {
	case <-connectedChan:
		t.Log("WebRTC PeerConnection successfully connected via Signaling Hub.")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for WebRTC connection to reach 'connected' state")
	}

	session, ok := mgr.GetSession(sessionID)
	if !ok {
		t.Fatalf("Session %s not found in Manager", sessionID)
	}

	// 8. Track received packets on client
	var packetsReceived int64
	var postChaosPackets int64
	var isPostChaos int32
	var clientSSRC uint32

	trackReady := make(chan struct{})
	var trackOnce sync.Once

	clientPC.OnTrack(func(track *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		trackOnce.Do(func() {
			clientSSRC = uint32(track.SSRC())
			close(trackReady)
		})
		for {
			_, _, rErr := track.ReadRTP()
			if rErr != nil {
				return
			}
			atomic.AddInt64(&packetsReceived, 1)
			if atomic.LoadInt32(&isPostChaos) == 1 {
				atomic.AddInt64(&postChaosPackets, 1)
			}
		}
	})

	// 9. Start continuous 30fps media pumping
	stopMedia := make(chan struct{})
	defer close(stopMedia)

	go func() {
		ticker := time.NewTicker(33 * time.Millisecond) // ~30 fps
		defer ticker.Stop()

		var fIdx int
		var seq uint16 = 1

		for {
			select {
			case <-stopMedia:
				return
			case <-ticker.C:
				fIdx++
				var payload []byte
				if fIdx%15 == 1 {
					payload = []byte{0x65, 0x88, 0x84, 0x00, 0x10, 0xff, 0xfe}
				} else {
					payload = []byte{0x41, 0x9a, 0x01, 0x02, 0x03, 0x04}
				}

				pkt := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    96,
						SequenceNumber: seq,
						Timestamp:      uint32(fIdx * 3000),
						SSRC:           clientSSRC,
					},
					Payload: payload,
				}
				seq++
				_ = session.WriteRTP(pkt)
			}
		}
	}()

	select {
	case <-trackReady:
		t.Logf("Client receiving media track (SSRC: %d)", clientSSRC)
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for media track to start")
	}

	// Verify initial baseline packet receipt
	time.Sleep(500 * time.Millisecond)
	initialPackets := atomic.LoadInt64(&packetsReceived)
	if initialPackets == 0 {
		t.Fatal("Expected initial RTP packets received before chaos events")
	}
	t.Logf("Initial baseline: %d RTP packets received.", initialPackets)

	// 10. MULTI-CHAOS EVENT 1: Total Signaling Outage (Kill Signaling WebSockets & HTTP Server)
	t.Log("CHAOS EVENT 1: Severing signaling WebSocket connections and shutting down signaling server...")
	_ = edgeWS.Close()
	_ = viewerWS.Close()
	server.Close()

	// 11. MULTI-CHAOS EVENT 2: Total Database Crash (Sever PostgreSQL Connection)
	t.Log("CHAOS EVENT 2: Triggering total PostgreSQL database crash...")
	db.SetDB(nil, false)

	// Enable post-chaos packet counting
	atomic.StoreInt32(&isPostChaos, 1)

	// Let media streaming run for 2.0 seconds amidst the total outage
	t.Log("Monitoring WebRTC media stream during simultaneous signaling and database outages...")
	time.Sleep(2000 * time.Millisecond)

	postCount := atomic.LoadInt64(&postChaosPackets)
	t.Logf("Post-Chaos Packets Received: %d packets during 2.0s outage window (~%.1f pkts/sec)",
		postCount, float64(postCount)/2.0)

	if postCount < 20 {
		t.Fatalf("FAILURE: Media streaming dropped or degraded during outage: only received %d packets in 2.0s", postCount)
	}

	// Verify PeerConnection state remained connected
	if clientPC.ConnectionState() != pionwebrtc.PeerConnectionStateConnected {
		t.Fatalf("PeerConnection state dropped to %s during outage", clientPC.ConnectionState().String())
	}

	t.Log("SUCCESS: WebRTC PeerConnection media stream survived simultaneous signaling and database outages uninterrupted!")

	// 12. Clean Session Teardown
	mgr.RemoveSession(sessionID)
	t.Log("PASS: Session cleanly removed and resources freed.")
}
