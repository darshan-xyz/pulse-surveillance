package tests

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	pionwebrtc "github.com/pion/webrtc/v4"
	"hss/internal/cloud/cache"
	"hss/internal/cloud/signaling"
	"hss/internal/security"
	"hss/internal/webrtc"
)

// Helper: Generate a signed JWT token
func generateTestViewerJWT(username, tenantID, role string, expiry time.Duration, jti string) string {
	claims := jwt.MapClaims{
		"sub":       username,
		"username":  username,
		"tenant_id": tenantID,
		"role":      role,
		"exp":       time.Now().Add(expiry).Unix(),
		"iat":       time.Now().Unix(),
	}
	if jti != "" {
		claims["jti"] = jti
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString(security.GetJWTSecret())
	return tokenStr
}

// 1. Empirically verify WebRTC signaling over /client (Offer, Answer, Dynamic TURN, Trickle ICE)
func TestViewerDashboard_Signaling_OfferAnswerCandidateFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	targetCam := "test-camera-1"

	// 1. Connect Edge Camera Daemon to /signaling
	edgeWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/signaling?token=mock_device_jwt&camera_id="+targetCam, nil)
	if err != nil {
		t.Fatalf("Failed to connect edge camera daemon: %v", err)
	}
	defer edgeWS.Close()

	var edgeSendMu sync.Mutex
	var receivedOffer signaling.SignalingMessage
	offerReceivedChan := make(chan struct{}, 1)
	edgeCandidateReceivedChan := make(chan string, 5)

	edgeDone := make(chan struct{})
	defer close(edgeDone)

	go func() {
		for {
			select {
			case <-edgeDone:
				return
			default:
				var msg signaling.SignalingMessage
				if err := edgeWS.ReadJSON(&msg); err != nil {
					return
				}
				if msg.Type == "offer" {
					receivedOffer = msg
					offerReceivedChan <- struct{}{}

					// Answer back
					go func(sessID, sdp string) {
						time.Sleep(20 * time.Millisecond)
						answerSDP, handleErr := webrtc.DefaultManager.HandleOffer(
							sessID, targetCam, sdp, msg.TurnServer, msg.TurnUsername, msg.TurnPassword,
							func(c *pionwebrtc.ICECandidate) {
								if c != nil {
									edgeSendMu.Lock()
									_ = edgeWS.WriteJSON(signaling.SignalingMessage{
										Type:      "candidate",
										SessionID: sessID,
										DeviceID:  targetCam,
										Candidate: c.ToJSON().Candidate,
									})
									edgeSendMu.Unlock()
								}
							},
						)
						if handleErr == nil {
							edgeSendMu.Lock()
							_ = edgeWS.WriteJSON(signaling.SignalingMessage{
								Type:      "answer",
								SessionID: sessID,
								DeviceID:  targetCam,
								SDP:       answerSDP,
							})
							edgeSendMu.Unlock()
						}
					}(msg.SessionID, msg.SDP)
				} else if msg.Type == "candidate" {
					edgeCandidateReceivedChan <- msg.Candidate
				}
			}
		}
	}()

	// 2. Connect Viewer Client to /client
	viewerToken := generateTestViewerJWT("admin", "tenant-alpha", "admin", 1*time.Hour, "")
	viewerWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+viewerToken, nil)
	if err != nil {
		t.Fatalf("Failed to connect viewer to /client: %v", err)
	}
	defer viewerWS.Close()

	// 3. Create Pion WebRTC client PeerConnection in recvonly mode
	clientPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{
		ICEServers: []pionwebrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		t.Fatalf("Failed to create client peer connection: %v", err)
	}
	defer clientPC.Close()

	_, err = clientPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add recvonly transceiver: %v", err)
	}

	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create client offer: %v", err)
	}
	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("Failed to set client local description: %v", err)
	}

	sessionID := fmt.Sprintf("viewer-test-%d", time.Now().UnixNano())

	// Send initial offer over /client
	offerPayload := map[string]interface{}{
		"type":       "offer",
		"session_id": sessionID,
		"device_id":  targetCam,
		"sdp":        offer.SDP,
	}
	if err := viewerWS.WriteJSON(offerPayload); err != nil {
		t.Fatalf("Failed to send offer payload: %v", err)
	}

	// 4. Verify Edge Daemon received offer with Dynamic TURN injection
	select {
	case <-offerReceivedChan:
		if receivedOffer.SessionID != sessionID {
			t.Errorf("Expected session ID %s, got %s", sessionID, receivedOffer.SessionID)
		}
		if receivedOffer.DeviceID != targetCam {
			t.Errorf("Expected device ID %s, got %s", targetCam, receivedOffer.DeviceID)
		}
		if receivedOffer.TurnServer == "" || receivedOffer.TurnUsername == "" || receivedOffer.TurnPassword == "" {
			t.Errorf("Expected injected dynamic TURN credentials, got server=%s, user=%s", receivedOffer.TurnServer, receivedOffer.TurnUsername)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for edge daemon to receive offer")
	}

	// 5. Read Answer on Viewer WebSocket
	var answerMsg signaling.SignalingMessage
	if err := viewerWS.ReadJSON(&answerMsg); err != nil {
		t.Fatalf("Failed to read answer on viewer WS: %v", err)
	}

	if answerMsg.Type != "answer" {
		t.Fatalf("Expected message type 'answer', got '%s'", answerMsg.Type)
	}
	if answerMsg.SessionID != sessionID {
		t.Errorf("Expected answer session ID %s, got %s", sessionID, answerMsg.SessionID)
	}
	if !strings.HasPrefix(answerMsg.SDP, "v=0") {
		t.Errorf("Invalid SDP in answer: %s", answerMsg.SDP)
	}

	// Apply Answer to client PeerConnection
	if err := clientPC.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  answerMsg.SDP,
	}); err != nil {
		t.Fatalf("Failed to set remote description on client PC: %v", err)
	}

	// 6. Test Trickle ICE from Viewer to Edge
	testCandidate := "candidate:1 1 UDP 2130706431 127.0.0.1 50000 typ host"
	candPayload := map[string]interface{}{
		"type":       "candidate",
		"session_id": sessionID,
		"device_id":  targetCam,
		"candidate":  testCandidate,
	}
	if err := viewerWS.WriteJSON(candPayload); err != nil {
		t.Fatalf("Failed to send candidate from viewer: %v", err)
	}

	select {
	case cand := <-edgeCandidateReceivedChan:
		if !strings.Contains(cand, "candidate:1") {
			t.Errorf("Expected edge to receive candidate, got: %s", cand)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for edge daemon to receive trickled candidate")
	}

	webrtc.DefaultManager.RemoveSession(sessionID)
	t.Log("SUCCESS: WebRTC signaling over /client verified with Offer, Answer, TURN injection, and Trickle ICE!")
}

// 2. Empirically verify Token Auth Validation on /client (valid, invalid, expired, revoked, header vs query)
func TestViewerDashboard_TokenAuthValidationMatrix(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	testCases := []struct {
		name          string
		tokenQuery    string
		authHeader    string
		expectSuccess bool
		expectedCode  int
	}{
		{
			name:          "Missing Token (no query, no header)",
			tokenQuery:    "",
			authHeader:    "",
			expectSuccess: false,
			expectedCode:  http.StatusUnauthorized,
		},
		{
			name:          "Malformed / Garbage Token",
			tokenQuery:    "not.a.valid.jwt.string",
			authHeader:    "",
			expectSuccess: false,
			expectedCode:  http.StatusUnauthorized,
		},
		{
			name:          "Expired Token",
			tokenQuery:    generateTestViewerJWT("admin", "tenant-alpha", "admin", -10*time.Minute, ""),
			authHeader:    "",
			expectSuccess: false,
			expectedCode:  http.StatusUnauthorized,
		},
		{
			name:          "Forged Token (Invalid Signature)",
			tokenQuery:    generateForgedJWT("admin", "tenant-alpha", "admin"),
			authHeader:    "",
			expectSuccess: false,
			expectedCode:  http.StatusUnauthorized,
		},
		{
			name:          "Revoked Token (Blacklisted JTI)",
			tokenQuery:    generateBlacklistedJWT(t, "admin", "tenant-alpha", "admin"),
			authHeader:    "",
			expectSuccess: false,
			expectedCode:  http.StatusUnauthorized,
		},
		{
			name:          "Valid Token in Query Param",
			tokenQuery:    generateTestViewerJWT("admin", "tenant-alpha", "admin", 1*time.Hour, ""),
			authHeader:    "",
			expectSuccess: true,
			expectedCode:  http.StatusSwitchingProtocols,
		},
		{
			name:          "Valid Token in Authorization Bearer Header",
			tokenQuery:    "",
			authHeader:    "Bearer " + generateTestViewerJWT("admin", "tenant-alpha", "admin", 1*time.Hour, ""),
			expectSuccess: true,
			expectedCode:  http.StatusSwitchingProtocols,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := wsURL + "/client"
			if tc.tokenQuery != "" {
				url += "?token=" + tc.tokenQuery
			}

			header := http.Header{}
			if tc.authHeader != "" {
				header.Set("Authorization", tc.authHeader)
			}

			conn, resp, err := websocket.DefaultDialer.Dial(url, header)
			if tc.expectSuccess {
				if err != nil {
					t.Fatalf("Expected connection success, got error: %v", err)
				}
				if conn == nil {
					t.Fatal("Expected non-nil websocket connection")
				}
				conn.Close()
			} else {
				if err == nil {
					if conn != nil {
						conn.Close()
					}
					t.Fatalf("Expected connection rejection, but connection succeeded")
				}
				if resp != nil && resp.StatusCode != tc.expectedCode {
					t.Errorf("Expected status %d, got %d", tc.expectedCode, resp.StatusCode)
				}
			}
		})
	}
}

// 3. Empirically verify Concurrency Limits (Cap of 3 viewers per camera)
func TestViewerDashboard_ViewerConcurrencyLimit_3ViewersPerCamera(t *testing.T) {
	mgr := webrtc.NewManager()
	cam1 := "cam-tenant1-01"
	cam2 := "cam-tenant1-02"

	createOfferSDP := func() string {
		pc, _ := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
		defer pc.Close()
		_, _ = pc.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
			Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
		})
		offer, _ := pc.CreateOffer(nil)
		return offer.SDP
	}

	// 1. Connect 3 viewers to cam1 -> All 3 must succeed
	var cam1Sessions []string
	for i := 1; i <= 3; i++ {
		sessID := fmt.Sprintf("cam1-viewer-%d", i)
		cam1Sessions = append(cam1Sessions, sessID)
		answer, err := mgr.HandleOffer(sessID, cam1, createOfferSDP(), "", "", "", nil)
		if err != nil {
			t.Fatalf("Viewer %d on cam1 failed: %v", i, err)
		}
		if answer == "" {
			t.Fatalf("Viewer %d on cam1 received empty answer", i)
		}
	}

	// 2. 4th viewer on cam1 must be REJECTED with concurrency limit error
	_, err4 := mgr.HandleOffer("cam1-viewer-4", cam1, createOfferSDP(), "", "", "", nil)
	if err4 == nil {
		t.Fatal("Expected 4th viewer on cam1 to be rejected, but succeeded")
	}
	if !strings.Contains(err4.Error(), "maximum of 3 concurrent viewers") {
		t.Errorf("Unexpected error message for 4th viewer: %v", err4)
	}

	// 3. Connect 3 viewers to cam2 at the same time -> All 3 must succeed (independent camera tracking)
	var cam2Sessions []string
	for i := 1; i <= 3; i++ {
		sessID := fmt.Sprintf("cam2-viewer-%d", i)
		cam2Sessions = append(cam2Sessions, sessID)
		answer, err := mgr.HandleOffer(sessID, cam2, createOfferSDP(), "", "", "", nil)
		if err != nil {
			t.Fatalf("Viewer %d on cam2 failed: %v", i, err)
		}
		if answer == "" {
			t.Fatalf("Viewer %d on cam2 received empty answer", i)
		}
	}

	// 4. 4th viewer on cam2 must also be REJECTED
	_, errCam2_4 := mgr.HandleOffer("cam2-viewer-4", cam2, createOfferSDP(), "", "", "", nil)
	if errCam2_4 == nil {
		t.Fatal("Expected 4th viewer on cam2 to be rejected, but succeeded")
	}

	// 5. Disconnect Viewer 2 on cam1
	mgr.RemoveSession("cam1-viewer-2")

	// 6. 4th viewer on cam1 retries -> Now MUST succeed
	answer4Retry, err4Retry := mgr.HandleOffer("cam1-viewer-4-retry", cam1, createOfferSDP(), "", "", "", nil)
	if err4Retry != nil {
		t.Fatalf("Expected 4th viewer to succeed after disconnect, got error: %v", err4Retry)
	}
	if answer4Retry == "" {
		t.Fatal("Expected non-empty answer for reconnected viewer")
	}

	// Cleanup
	mgr.RemoveSession("cam1-viewer-1")
	mgr.RemoveSession("cam1-viewer-3")
	mgr.RemoveSession("cam1-viewer-4-retry")
	for _, s := range cam2Sessions {
		mgr.RemoveSession(s)
	}

	t.Log("SUCCESS: Concurrency cap of 3 viewers per camera verified with per-camera isolation and slot reclamation!")
}

// 4. Empirically verify Rapid Camera Switching and PeerConnection Teardown
func TestViewerDashboard_RapidCameraSwitching_Stress(t *testing.T) {
	mgr := webrtc.NewManager()
	cameras := []string{"cam-front", "cam-back", "cam-garage", "cam-hallway", "cam-yard"}

	createOfferSDP := func() string {
		pc, _ := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
		defer pc.Close()
		_, _ = pc.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
			Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
		})
		offer, _ := pc.CreateOffer(nil)
		return offer.SDP
	}

	// Simulate a user rapidly clicking between cameras 20 times
	currentSessionID := ""
	for i := 0; i < 20; i++ {
		cam := cameras[i%len(cameras)]

		// Teardown previous session
		if currentSessionID != "" {
			mgr.RemoveSession(currentSessionID)
		}

		currentSessionID = fmt.Sprintf("rapid-switch-sess-%03d", i)
		answer, err := mgr.HandleOffer(currentSessionID, cam, createOfferSDP(), "", "", "", nil)
		if err != nil {
			t.Fatalf("Rapid switch step %d (cam %s) failed: %v", i, cam, err)
		}
		if answer == "" {
			t.Fatalf("Rapid switch step %d (cam %s) returned empty answer", i, cam)
		}
	}

	// Final cleanup
	if currentSessionID != "" {
		mgr.RemoveSession(currentSessionID)
	}

	// Verify all cameras have 0 active viewers and can accept 3 new viewers
	for _, cam := range cameras {
		for v := 1; v <= 3; v++ {
			sID := fmt.Sprintf("post-stress-%s-v%d", cam, v)
			_, err := mgr.HandleOffer(sID, cam, createOfferSDP(), "", "", "", nil)
			if err != nil {
				t.Fatalf("Post-stress viewer %d on cam %s failed: %v", v, cam, err)
			}
			mgr.RemoveSession(sID)
		}
	}

	t.Log("SUCCESS: Rapid camera switching stress completed 20 cycles with zero leaked sessions or viewer count drift!")
}

// 5. Empirically verify Cross-Tenant Device Access Rejection over /client
func TestViewerDashboard_CrossTenantIsolation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// User from Tenant 1
	tokenTenant1 := generateTestViewerJWT("tenant1_user", "tenant-1", "viewer", 1*time.Hour, "")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+tokenTenant1, nil)
	if err != nil {
		t.Fatalf("Failed to connect tenant1 user: %v", err)
	}
	defer conn.Close()

	// Tenant 1 user attempts to view Tenant 2's camera
	rogueOffer := map[string]interface{}{
		"type":       "offer",
		"session_id": "rogue-cross-tenant-session",
		"device_id":  "cam-tenant2-01", // belonging to tenant-2
		"sdp":        "v=0\r\no=- 123 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
	}

	if err := conn.WriteJSON(rogueOffer); err != nil {
		t.Fatalf("Failed to send rogue offer: %v", err)
	}

	var resp map[string]string
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("Failed to read server response: %v", err)
	}

	if resp["error"] != "Forbidden" {
		t.Errorf("Expected error 'Forbidden', got: %v", resp)
	}

	t.Log("SUCCESS: Cross-tenant device access strictly forbidden over /client!")
}

// Helper: Generate forged JWT with wrong secret
func generateForgedJWT(username, tenantID, role string) string {
	claims := jwt.MapClaims{
		"sub":       username,
		"username":  username,
		"tenant_id": tenantID,
		"role":      role,
		"exp":       time.Now().Add(1 * time.Hour).Unix(),
		"iat":       time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte("wrong-secret-key-1234567890123456"))
	return tokenStr
}

// Helper: Generate blacklisted JWT
func generateBlacklistedJWT(t *testing.T, username, tenantID, role string) string {
	jti := fmt.Sprintf("blacklisted-jti-%d", time.Now().UnixNano())
	tokenStr := generateTestViewerJWT(username, tenantID, role, 1*time.Hour, jti)
	cache.BlacklistToken(context.Background(), jti, 1*time.Hour)
	return tokenStr
}

// Suppress unused imports
var _ = bytes.Buffer{}
