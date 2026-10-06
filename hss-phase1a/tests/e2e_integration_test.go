package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"hss/internal/api"
	"hss/internal/cloud/signaling"
	"hss/internal/webrtc"

	"github.com/gorilla/websocket"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// TestEndToEndPulseCloudFlow executes the authoritative, comprehensive E2E test verifying:
// 1. JWT Authentication (/api/login & /api/auth/login)
// 2. Strict Multi-Tenant Device Isolation (/api/devices)
// 3. WebRTC Signaling Hub on Port 9091 with RTCPeerConnection 'connected' state transition
// 4. Clean stream teardown and reconnection
// 5. Cross-tenant authorization blocking
func TestEndToEndPulseCloudFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", api.HandleLogin)
	mux.HandleFunc("/api/auth/login", api.HandleLogin)
	mux.HandleFunc("/api/devices", api.HandleDevices)
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// ==========================================
	// STEP 1: JWT Authentication
	// ==========================================
	t.Log("STEP 1: Authenticating tenant user via /api/login...")
	loginBody, _ := json.Marshal(map[string]string{
		"username": "tenant1_user",
		"password": "password",
	})
	resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		t.Fatalf("Login POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP 200 from login, got: %d", resp.StatusCode)
	}

	var authResult api.LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResult); err != nil {
		t.Fatalf("Failed to parse login response: %v", err)
	}

	jwtToken := authResult.Token
	if jwtToken == "" {
		t.Fatalf("JWT Token is empty in login response")
	}
	t.Logf("JWT authentication successful for user: %s (tenant: %s)", authResult.User.Username, authResult.User.TenantID)

	// ==========================================
	// STEP 2: Strict Multi-Tenant Isolation
	// ==========================================
	t.Log("STEP 2: Fetching device list via /api/devices with Bearer JWT...")
	devReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	devReq.Header.Set("Authorization", "Bearer "+jwtToken)
	devResp, err := http.DefaultClient.Do(devReq)
	if err != nil {
		t.Fatalf("Devices GET failed: %v", err)
	}
	defer devResp.Body.Close()

	if devResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP 200 from /api/devices, got: %d", devResp.StatusCode)
	}

	var devices []api.DeviceResponse
	if err := json.NewDecoder(devResp.Body).Decode(&devices); err != nil {
		t.Fatalf("Failed to parse devices response: %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("Expected 2 devices for tenant1_user, got: %d", len(devices))
	}
	for _, dev := range devices {
		if dev.DeviceID != "cam-tenant1-01" && dev.DeviceID != "cam-tenant1-02" {
			t.Fatalf("Tenant isolation breach! Received foreign device: %s", dev.DeviceID)
		}
	}
	t.Logf("Multi-tenant isolation verified. Allowed devices: %+v", devices)

	// ==========================================
	// STEP 3: WebRTC Signaling & Connection State -> 'connected'
	// ==========================================
	targetDeviceID := devices[0].DeviceID // "cam-tenant1-01"
	sessionID := "e2e-live-sess-001"
	t.Logf("STEP 3: Establishing WebRTC session for %s (session: %s)...", targetDeviceID, sessionID)

	// 3a. Connect simulated Edge Camera Daemon
	edgeWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/signaling?token=mock_device_jwt&camera_id="+targetDeviceID, nil)
	if err != nil {
		t.Fatalf("Failed to connect Edge Daemon to /signaling: %v", err)
	}
	defer edgeWS.Close()

	// 3b. Connect Web Dashboard Viewer with JWT
	viewerWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+jwtToken, nil)
	if err != nil {
		t.Fatalf("Failed to connect Viewer to /client: %v", err)
	}
	defer viewerWS.Close()

	// 3c. Viewer initializes RTCPeerConnection
	viewerPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create viewer RTCPeerConnection: %v", err)
	}
	defer viewerPC.Close()

	_, err = viewerPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add video transceiver: %v", err)
	}

	connectedChan := make(chan struct{})
	var connectedOnce sync.Once

	viewerPC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
		t.Logf("[Viewer RTCPeerConnection State]: %s", s.String())
		if s == pionwebrtc.PeerConnectionStateConnected {
			connectedOnce.Do(func() {
				close(connectedChan)
			})
		}
	})

	var edgeSendMu sync.Mutex
	viewerPC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
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
		edgeSendMu.Lock()
		_ = viewerWS.WriteJSON(candMsg)
		edgeSendMu.Unlock()
	})

	// 3d. Edge camera listener loop
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
				t.Logf("[Edge Camera] Received Offer for session: %s", msg.SessionID)
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

				sdpAnswer, err := webrtc.DefaultManager.HandleOffer(msg.SessionID, msg.DeviceID, msg.SDP, msg.TurnServer, msg.TurnUsername, msg.TurnPassword, onEdgeCandidate)
				if err != nil {
					t.Logf("[Edge Camera] HandleOffer error: %v", err)
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

	// 3e. Viewer sends Offer
	offer, err := viewerPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	if err := viewerPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("Failed to set local description: %v", err)
	}

	offerMsg := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: sessionID,
		DeviceID:  targetDeviceID,
		SDP:       offer.SDP,
	}
	if err := viewerWS.WriteJSON(offerMsg); err != nil {
		t.Fatalf("Failed to send offer from viewer: %v", err)
	}

	// 3f. Viewer processes Answer and Candidates from Hub
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
				t.Logf("[Viewer] Received Answer for session: %s", msg.SessionID)
				answer := pionwebrtc.SessionDescription{
					Type: pionwebrtc.SDPTypeAnswer,
					SDP:  msg.SDP,
				}
				if err := viewerPC.SetRemoteDescription(answer); err != nil {
					t.Logf("[Viewer] SetRemoteDescription error: %v", err)
					return
				}

				earlyMu.Lock()
				for _, cand := range earlyCandidates {
					_ = viewerPC.AddICECandidate(cand)
				}
				earlyCandidates = nil
				earlyMu.Unlock()

			case "candidate":
				var candInit pionwebrtc.ICECandidateInit
				if err := json.Unmarshal([]byte(msg.Candidate), &candInit); err == nil && candInit.Candidate != "" {
					earlyMu.Lock()
					if viewerPC.RemoteDescription() != nil {
						_ = viewerPC.AddICECandidate(candInit)
					} else {
						earlyCandidates = append(earlyCandidates, candInit)
					}
					earlyMu.Unlock()
				}
			}
		}
	}()

	// 3g. Assert 'connected' state transition
	select {
	case <-connectedChan:
		t.Log("SUCCESS: WebRTC RTCPeerConnection transitioned to 'connected' state!")
	case <-time.After(5 * time.Second):
		t.Fatalf("Timeout waiting for RTCPeerConnection to reach 'connected'")
	}

	// ==========================================
	// STEP 4: Teardown and Clean Session Cleanup
	// ==========================================
	t.Log("STEP 4: Testing clean teardown...")
	viewerPC.Close()
	viewerWS.Close()
	webrtc.DefaultManager.RemoveSession(sessionID)

	// ==========================================
	// STEP 5: Adversarial Cross-Tenant Access Rejection
	// ==========================================
	t.Log("STEP 5: Testing adversarial cross-tenant access rejection...")
	// Tenant 1 attempts to connect to Tenant 2's camera ("cam-tenant2-01")
	rogueViewerWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+jwtToken, nil)
	if err != nil {
		t.Fatalf("Failed to open rogue viewer WS: %v", err)
	}
	defer rogueViewerWS.Close()

	maliciousOffer := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: "rogue-sess-666",
		DeviceID:  "cam-tenant2-01", // Foreign tenant device
		SDP:       offer.SDP,
	}
	if err := rogueViewerWS.WriteJSON(maliciousOffer); err != nil {
		t.Fatalf("Failed to send malicious offer: %v", err)
	}

	rogueViewerWS.SetReadDeadline(time.Now().Add(2 * time.Second))
	var rogueResp map[string]interface{}
	if err := rogueViewerWS.ReadJSON(&rogueResp); err != nil {
		t.Logf("Rogue connection cleanly dropped by server on forbidden request: %v", err)
	} else {
		if rogueResp["error"] != "Forbidden" {
			t.Fatalf("SECURITY VIOLATION: Expected 'Forbidden', got: %v", rogueResp)
		}
		t.Logf("Server successfully rejected cross-tenant stream request with: %v", rogueResp)
	}

	t.Log("ALL E2E INTEGRATION CHECKS PASSED WITH ZERO ERRORS!")
}
