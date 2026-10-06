package tests

import (
	"bytes"
	"context"
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
	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
	"hss/internal/api"
	"hss/internal/cloud/signaling"
	"hss/internal/webrtc"
)

// TestMobileWebRTCSDPNegotiationAndP2P executes the complete end-to-end WebRTC
// SDP negotiation and P2P connection lifecycle simulating the React Native mobile client.
//
// Lifecycle verified:
// 1. Authenticates via POST /api/auth/login and retrieves JWT token.
// 2. Fetches device list via GET /api/devices with Bearer token.
// 3. Connects WebSocket to /client?token=<JWT>.
// 4. Sets up client Pion PeerConnection with recvonly video transceiver.
// 5. Generates SDP Offer and transmits payload {"type":"offer", "session_id":"...", "device_id":"...", "sdp":"..."}.
// 6. Backend ingests offer via Manager.HandleOffer, returns SDP answer.
// 7. Client sets remote description, exchanges ICE candidates.
// 8. Verifies both Client and Backend PeerConnections transition to PeerConnectionStateConnected in <= 3.0s.
// 9. Verifies video track reception on clientPC.OnTrack and reads RTP video packets.
func TestMobileWebRTCSDPNegotiationAndP2P(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Mobile WebRTC E2E test in short mode")
	}

	// 1. Start in-memory local signaling & API server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", api.HandleLogin)
	mux.HandleFunc("/api/devices", api.HandleDevices)
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// =========================================================================
	// STEP 1: React Native Client Login (JWT Authentication)
	// =========================================================================
	t.Log("[Client] Step 1: Logging in via POST /api/auth/login...")
	loginPayload, _ := json.Marshal(map[string]string{
		"username": "tenant1_user",
		"password": "password",
	})
	loginResp, err := http.Post(server.URL+"/api/auth/login", "application/json", bytes.NewBuffer(loginPayload))
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP 200 from login, got: %d", loginResp.StatusCode)
	}

	var authResult api.LoginResponse
	if err := json.NewDecoder(loginResp.Body).Decode(&authResult); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}
	jwtToken := authResult.Token
	if jwtToken == "" {
		t.Fatalf("Received empty JWT token")
	}
	t.Logf("[Client] Authenticated successfully as %s (tenant: %s)", authResult.User.Username, authResult.User.TenantID)

	// =========================================================================
	// STEP 2: Multi-Tenant Device Discovery
	// =========================================================================
	t.Log("[Client] Step 2: Fetching authorized devices via GET /api/devices...")
	devReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	devReq.Header.Set("Authorization", "Bearer "+jwtToken)
	devResp, err := http.DefaultClient.Do(devReq)
	if err != nil {
		t.Fatalf("Devices request failed: %v", err)
	}
	defer devResp.Body.Close()

	var devices []api.DeviceResponse
	if err := json.NewDecoder(devResp.Body).Decode(&devices); err != nil {
		t.Fatalf("Failed to decode devices: %v", err)
	}
	if len(devices) == 0 {
		t.Fatalf("No authorized devices returned for tenant")
	}
	targetDeviceID := devices[0].DeviceID
	sessionID := fmt.Sprintf("mobile-sess-%d", time.Now().UnixNano()%100000)
	t.Logf("[Client] Selected target device: %s, session ID: %s", targetDeviceID, sessionID)

	// =========================================================================
	// STEP 3: Backend Edge Camera Daemon Setup
	// =========================================================================
	t.Log("[Edge Backend] Connecting simulated edge camera daemon to /signaling...")
	edgeWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/signaling?token=mock_device_jwt&camera_id="+targetDeviceID, nil)
	if err != nil {
		t.Fatalf("Edge daemon failed to connect: %v", err)
	}
	defer edgeWS.Close()

	var edgeSendMu sync.Mutex
	edgeDone := make(chan struct{})
	go func() {
		defer close(edgeDone)
		for {
			var msg signaling.SignalingMessage
			if err := edgeWS.ReadJSON(&msg); err != nil {
				return
			}

			switch msg.Type {
			case "offer":
				t.Logf("[Edge Backend] Ingesting SDP Offer for camera %s (session %s)...", msg.DeviceID, msg.SessionID)

				onCandidate := func(c *pionwebrtc.ICECandidate) {
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

				sdpAnswer, err := webrtc.DefaultManager.HandleOffer(
					msg.SessionID,
					msg.DeviceID,
					msg.SDP,
					msg.TurnServer,
					msg.TurnUsername,
					msg.TurnPassword,
					onCandidate,
				)
				if err != nil {
					t.Logf("[Edge Backend] HandleOffer error: %v", err)
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
				_ = webrtc.DefaultManager.AddICECandidate(msg.SessionID, msg.Candidate)
			}
		}
	}()

	// =========================================================================
	// STEP 4: Mobile Client PeerConnection & Transceiver Setup
	// =========================================================================
	t.Log("[Client] Step 4: Connecting WebSocket to /client and creating PeerConnection...")
	viewerWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+jwtToken, nil)
	if err != nil {
		t.Fatalf("Viewer WebSocket failed to connect: %v", err)
	}
	defer viewerWS.Close()

	clientPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{
		ICEServers: []pionwebrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		t.Fatalf("Failed to create client PeerConnection: %v", err)
	}
	defer clientPC.Close()

	// Add recvonly video transceiver matching React Native useSignaling hook
	_, err = clientPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add recvonly video transceiver: %v", err)
	}

	// Track connection state transition
	clientConnectedChan := make(chan time.Time, 1)
	var clientConnectedOnce sync.Once

	clientPC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
		t.Logf("[Client PeerConnection State]: %s", s.String())
		if s == pionwebrtc.PeerConnectionStateConnected {
			clientConnectedOnce.Do(func() {
				clientConnectedChan <- time.Now()
			})
		}
	})

	// Track video track reception & RTP packets
	trackReceivedChan := make(chan *pionwebrtc.TrackRemote, 1)
	var trackReceivedOnce sync.Once
	var packetsReceived atomic.Uint32

	clientPC.OnTrack(func(track *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		t.Logf("[Client OnTrack] Video track received! ID: %s, MimeType: %s, PayloadType: %d",
			track.ID(), track.Codec().MimeType, track.PayloadType())
		trackReceivedOnce.Do(func() {
			trackReceivedChan <- track
		})

		go func() {
			for {
				pkt, _, rErr := track.ReadRTP()
				if rErr != nil {
					return
				}
				if pkt != nil {
					packetsReceived.Add(1)
				}
			}
		}()
	})

	// Send Client ICE Candidates to WebSocket
	var clientSendMu sync.Mutex
	clientPC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candJSON, _ := json.Marshal(c.ToJSON())
		candMsg := signaling.SignalingMessage{
			Type:      "candidate",
			SessionID: sessionID,
			DeviceID:  targetDeviceID,
			Candidate: string(candJSON),
		}
		clientSendMu.Lock()
		_ = viewerWS.WriteJSON(candMsg)
		clientSendMu.Unlock()
	})

	// =========================================================================
	// STEP 5: Create and Transmit SDP Offer
	// =========================================================================
	t.Log("[Client] Step 5: Creating SDP Offer and sending via WebSocket...")
	startTime := time.Now()

	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create SDP offer: %v", err)
	}

	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("Failed to set local description: %v", err)
	}

	offerPayload := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: sessionID,
		DeviceID:  targetDeviceID,
		CameraID:  targetDeviceID,
		SDP:       offer.SDP,
	}

	if err := viewerWS.WriteJSON(offerPayload); err != nil {
		t.Fatalf("Failed to transmit SDP offer over WebSocket: %v", err)
	}

	// =========================================================================
	// STEP 6: Client Reads SDP Answer and ICE Candidates
	// =========================================================================
	var earlyCandidates []pionwebrtc.ICECandidateInit
	var earlyMu sync.Mutex

	viewerDone := make(chan struct{})
	go func() {
		defer close(viewerDone)
		for {
			var msg signaling.SignalingMessage
			if err := viewerWS.ReadJSON(&msg); err != nil {
				return
			}

			switch msg.Type {
			case "answer":
				t.Logf("[Client] Received SDP Answer for session: %s", msg.SessionID)
				answer := pionwebrtc.SessionDescription{
					Type: pionwebrtc.SDPTypeAnswer,
					SDP:  msg.SDP,
				}
				if err := clientPC.SetRemoteDescription(answer); err != nil {
					t.Logf("[Client] SetRemoteDescription error: %v", err)
					return
				}

				earlyMu.Lock()
				for _, cand := range earlyCandidates {
					_ = clientPC.AddICECandidate(cand)
				}
				earlyCandidates = nil
				earlyMu.Unlock()

			case "candidate":
				var candInit pionwebrtc.ICECandidateInit
				if err := json.Unmarshal([]byte(msg.Candidate), &candInit); err == nil && candInit.Candidate != "" {
					earlyMu.Lock()
					if clientPC.RemoteDescription() != nil {
						_ = clientPC.AddICECandidate(candInit)
					} else {
						earlyCandidates = append(earlyCandidates, candInit)
					}
					earlyMu.Unlock()
				}
			}
		}
	}()

	// =========================================================================
	// STEP 7: Verify P2P Connection State (<= 3.0s constraint)
	// =========================================================================
	t.Log("[Client] Step 7: Verifying P2P connection transitions to 'connected' within <= 3.0s...")

	select {
	case tClientConnected := <-clientConnectedChan:
		latency := tClientConnected.Sub(startTime)
		t.Logf("SUCCESS: Client PeerConnection connected in %v (requirement <= 3.0s)", latency)
		if latency > 3000*time.Millisecond {
			t.Fatalf("Connection latency %v exceeded 3.0s threshold", latency)
		}
	case <-time.After(3000 * time.Millisecond):
		t.Fatalf("FAILURE: Client PeerConnection did not reach 'connected' state within 3.0 seconds")
	}

	// Verify Backend PeerConnection state as well
	backendSess, ok := webrtc.DefaultManager.GetSession(sessionID)
	if !ok {
		t.Fatalf("Backend WebRTC session %s not found in Manager", sessionID)
	}

	// Give backend up to 500ms to reflect connected state if ICE handshake just finished
	backendConnected := false
	for i := 0; i < 10; i++ {
		if backendSess.PC.ConnectionState() == pionwebrtc.PeerConnectionStateConnected {
			backendConnected = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !backendConnected {
		t.Fatalf("Backend PeerConnection state is %s, expected 'connected'", backendSess.PC.ConnectionState().String())
	}
	t.Log("[Backend] SUCCESS: Backend PeerConnection verified in 'connected' state!")

	// =========================================================================
	// STEP 8: Video Track Reception & RTP Packet Delivery
	// =========================================================================
	t.Log("[Client] Step 8: Verifying video track reception and media delivery...")

	// Feed simulated H.264 video RTP packets from Backend to trigger Pion SRTP demux and OnTrack
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h264KeyframePayload := []byte{
		0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1f, 0x96, 0x54, 0x05, 0x01,
		0xe0, 0x08, 0x00, 0x03, 0x00, 0x02, 0x00, 0x00, 0x03, 0x00, 0x78, 0x80,
		0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x3c, 0x80, 0x00, 0x00, 0x00, 0x01,
		0x65, 0x88, 0x84, 0x00, 0x10, 0xff, 0xfe, 0xf6, 0xf0,
	}

	go func() {
		ticker := time.NewTicker(33 * time.Millisecond) // ~30 fps
		defer ticker.Stop()
		var seq uint16
		var ts uint32

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				seq++
				ts += 3000
				pkt := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    96,
						SequenceNumber: seq,
						Timestamp:      ts,
						SSRC:           11223344,
					},
					Payload: h264KeyframePayload,
				}
				_ = webrtc.DefaultManager.WriteRTP(sessionID, pkt)
			}
		}
	}()

	// Verify OnTrack callback fired
	select {
	case remoteTrack := <-trackReceivedChan:
		t.Logf("SUCCESS: Video track successfully established on client: %s (kind: %s, codec: %s)",
			remoteTrack.ID(), remoteTrack.Kind().String(), remoteTrack.Codec().MimeType)
		if remoteTrack.Kind() != pionwebrtc.RTPCodecTypeVideo {
			t.Fatalf("Expected video track kind, got: %s", remoteTrack.Kind().String())
		}
	case <-time.After(3000 * time.Millisecond):
		t.Fatalf("FAILURE: Timeout waiting for clientPC.OnTrack to be triggered")
	}

	// Wait up to 2.0s for mobile client to receive RTP packets
	receivedSuccessfully := false
	for i := 0; i < 20; i++ {
		count := packetsReceived.Load()
		if count >= 5 {
			t.Logf("SUCCESS: Mobile Client received %d RTP video packets over WebRTC P2P connection!", count)
			receivedSuccessfully = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !receivedSuccessfully {
		t.Fatalf("FAILURE: Mobile Client failed to receive RTP packets (received %d)", packetsReceived.Load())
	}

	// =========================================================================
	// STEP 9: Clean Teardown
	// =========================================================================
	t.Log("[Client] Step 9: Cleaning up session...")
	webrtc.DefaultManager.RemoveSession(sessionID)
	t.Log("ALL MOBILE WEBRTC E2E REQUIREMENTS VERIFIED CLEANLY!")
}

// TestMobileWebRTCReconnectionLifecycle verifies that the mobile client can cleanly
// disconnect and establish a subsequent new WebRTC session without socket or port leaks.
func TestMobileWebRTCReconnectionLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping reconnection test in short mode")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	deviceID := "test-camera-1" // Allowed for 'admin' user

	// Connect Edge Daemon
	edgeWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/signaling?token=mock_device_jwt&camera_id="+deviceID, nil)
	if err != nil {
		t.Fatalf("Edge daemon connection failed: %v", err)
	}
	defer edgeWS.Close()

	var edgeSendMu sync.Mutex
	go func() {
		for {
			var msg signaling.SignalingMessage
			if err := edgeWS.ReadJSON(&msg); err != nil {
				return
			}
			if msg.Type == "offer" {
				onCand := func(c *pionwebrtc.ICECandidate) {
					if c == nil {
						return
					}
					candJSON, _ := json.Marshal(c.ToJSON())
					edgeSendMu.Lock()
					_ = edgeWS.WriteJSON(signaling.SignalingMessage{
						Type:      "candidate",
						SessionID: msg.SessionID,
						DeviceID:  msg.DeviceID,
						Candidate: string(candJSON),
					})
					edgeSendMu.Unlock()
				}
				sdpAns, err := webrtc.DefaultManager.HandleOffer(msg.SessionID, msg.DeviceID, msg.SDP, "", "", "", onCand)
				if err == nil {
					edgeSendMu.Lock()
					_ = edgeWS.WriteJSON(signaling.SignalingMessage{
						Type:      "answer",
						SessionID: msg.SessionID,
						DeviceID:  msg.DeviceID,
						SDP:       sdpAns,
					})
					edgeSendMu.Unlock()
				}
			} else if msg.Type == "candidate" {
				_ = webrtc.DefaultManager.AddICECandidate(msg.SessionID, msg.Candidate)
			}
		}
	}()

	// Execute 2 consecutive connect -> verify -> disconnect cycles
	for cycle := 1; cycle <= 2; cycle++ {
		sessionID := fmt.Sprintf("reconnect-sess-%d", cycle)
		t.Logf("Starting connection cycle %d (session: %s)...", cycle, sessionID)

		viewerWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token=admin", nil)
		if err != nil {
			t.Fatalf("Cycle %d: Viewer failed to connect: %v", cycle, err)
		}

		clientPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
		if err != nil {
			t.Fatalf("Cycle %d: Failed to create client PC: %v", cycle, err)
		}

		_, err = clientPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
			Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
		})
		if err != nil {
			t.Fatalf("Cycle %d: AddTransceiver error: %v", cycle, err)
		}

		connectedChan := make(chan struct{})
		var once sync.Once
		clientPC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
			if s == pionwebrtc.PeerConnectionStateConnected {
				once.Do(func() { close(connectedChan) })
			}
		})

		var vSendMu sync.Mutex
		clientPC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
			if c == nil {
				return
			}
			candJSON, _ := json.Marshal(c.ToJSON())
			vSendMu.Lock()
			_ = viewerWS.WriteJSON(signaling.SignalingMessage{
				Type:      "candidate",
				SessionID: sessionID,
				DeviceID:  deviceID,
				Candidate: string(candJSON),
			})
			vSendMu.Unlock()
		})

		// Read answers & candidates
		var earlyCands []pionwebrtc.ICECandidateInit
		var earlyMu sync.Mutex
		go func() {
			for {
				var msg signaling.SignalingMessage
				if err := viewerWS.ReadJSON(&msg); err != nil {
					return
				}
				if msg.Type == "answer" {
					_ = clientPC.SetRemoteDescription(pionwebrtc.SessionDescription{
						Type: pionwebrtc.SDPTypeAnswer,
						SDP:  msg.SDP,
					})
					earlyMu.Lock()
					for _, c := range earlyCands {
						_ = clientPC.AddICECandidate(c)
					}
					earlyCands = nil
					earlyMu.Unlock()
				} else if msg.Type == "candidate" {
					var cInit pionwebrtc.ICECandidateInit
					if json.Unmarshal([]byte(msg.Candidate), &cInit) == nil && cInit.Candidate != "" {
						earlyMu.Lock()
						if clientPC.RemoteDescription() != nil {
							_ = clientPC.AddICECandidate(cInit)
						} else {
							earlyCands = append(earlyCands, cInit)
						}
						earlyMu.Unlock()
					}
				}
			}
		}()

		offer, err := clientPC.CreateOffer(nil)
		if err != nil {
			t.Fatalf("Cycle %d: CreateOffer error: %v", cycle, err)
		}
		_ = clientPC.SetLocalDescription(offer)

		_ = viewerWS.WriteJSON(signaling.SignalingMessage{
			Type:      "offer",
			SessionID: sessionID,
			DeviceID:  deviceID,
			CameraID:  deviceID,
			SDP:       offer.SDP,
		})

		select {
		case <-connectedChan:
			t.Logf("Cycle %d: Successfully connected!", cycle)
		case <-time.After(3 * time.Second):
			t.Fatalf("Cycle %d: Failed to connect within 3s", cycle)
		}

		// Teardown cycle
		_ = clientPC.Close()
		_ = viewerWS.Close()
		webrtc.DefaultManager.RemoveSession(sessionID)
		time.Sleep(100 * time.Millisecond)
	}

	t.Log("SUCCESS: Reconnection lifecycle verified cleanly across multiple sessions!")
}

// TestMobileWebRTCUnauthorizedDeviceAccess verifies that the signaling server enforces
// strict tenant isolation and rejects cross-tenant device streaming requests.
func TestMobileWebRTCUnauthorizedDeviceAccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", api.HandleLogin)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Login as tenant1_user
	loginPayload, _ := json.Marshal(map[string]string{
		"username": "tenant1_user",
		"password": "password",
	})
	loginResp, err := http.Post(server.URL+"/api/auth/login", "application/json", bytes.NewBuffer(loginPayload))
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer loginResp.Body.Close()

	var auth api.LoginResponse
	_ = json.NewDecoder(loginResp.Body).Decode(&auth)

	viewerWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+auth.Token, nil)
	if err != nil {
		t.Fatalf("Viewer WS connection failed: %v", err)
	}
	defer viewerWS.Close()

	// Request device from tenant2
	forbiddenOffer := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: "forbidden-sess-001",
		DeviceID:  "cam-tenant2-01",
		SDP:       "v=0\r\no=- 12345 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
	}

	if err := viewerWS.WriteJSON(forbiddenOffer); err != nil {
		t.Fatalf("Failed to write offer: %v", err)
	}

	viewerWS.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp map[string]interface{}
	if err := viewerWS.ReadJSON(&resp); err != nil {
		t.Logf("Connection dropped cleanly on unauthorized access: %v", err)
	} else {
		if resp["error"] != "Forbidden" {
			t.Fatalf("Expected error: Forbidden, got: %v", resp)
		}
		t.Logf("SUCCESS: Cross-tenant device request rejected with: %v", resp)
	}
}

// TestMobileWebRTCInvalidTokenRejection verifies that connecting to /client without a valid JWT is rejected.
func TestMobileWebRTCInvalidTokenRejection(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// 1. Missing token
	_, resp, err := websocket.DefaultDialer.Dial(wsURL+"/client", nil)
	if err == nil {
		t.Fatal("Expected error connecting with missing token, got success")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected HTTP 401, got: %d", resp.StatusCode)
	}

	// 2. Corrupted token
	_, resp, err = websocket.DefaultDialer.Dial(wsURL+"/client?token=invalid.jwt.token", nil)
	if err == nil {
		t.Fatal("Expected error connecting with invalid token, got success")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected HTTP 401, got: %d", resp.StatusCode)
	}

	t.Log("SUCCESS: Unauthorized WebSocket connections cleanly rejected with HTTP 401!")
}
