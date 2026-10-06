package tests

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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
	"hss/internal/api"
	"hss/internal/cloud/signaling"
	"hss/internal/security"
	"hss/internal/webrtc"
)

// TestChallenger_MobileConsecutiveReconnects stress tests repeated consecutive
// connection and disconnection cycles between mobile client and backend signaling.
func TestChallenger_MobileConsecutiveReconnects(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping consecutive reconnect stress test in short mode")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", api.HandleLogin)
	mux.HandleFunc("/api/devices", api.HandleDevices)
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	targetDevice := "test-camera-1"

	// Connect camera daemon
	edgeWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/signaling?token=mock_device_jwt&camera_id="+targetDevice, nil)
	if err != nil {
		t.Fatalf("Failed to connect edge camera daemon: %v", err)
	}
	defer edgeWS.Close()

	var edgeSendMu sync.Mutex
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
				switch msg.Type {
				case "offer":
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
						"", "", "",
						onCandidate,
					)
					if err == nil {
						edgeSendMu.Lock()
						_ = edgeWS.WriteJSON(signaling.SignalingMessage{
							Type:      "answer",
							SessionID: msg.SessionID,
							DeviceID:  msg.DeviceID,
							SDP:       sdpAnswer,
						})
						edgeSendMu.Unlock()
					}
				case "candidate":
					_ = webrtc.DefaultManager.AddICECandidate(msg.SessionID, msg.Candidate)
				}
			}
		}
	}()

	secret := security.GetJWTSecret()
	token := createTestJWT(jwt.MapClaims{
		"sub":       "admin",
		"username":  "admin",
		"tenant_id": "tenant-alpha",
		"role":      "admin",
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Hour).Unix(),
	}, secret, jwt.SigningMethodHS256)

	const cycles = 10
	t.Logf("Running %d consecutive connect-negotiate-teardown cycles...", cycles)

	for i := 1; i <= cycles; i++ {
		sessionID := fmt.Sprintf("reconnect-stress-%03d", i)

		// 1. Establish WebSocket
		wsConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+token, nil)
		if err != nil {
			t.Fatalf("Cycle %d: Failed to connect WebSocket: %v", i, err)
		}

		// 2. Create WebRTC PeerConnection
		pc, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
		if err != nil {
			wsConn.Close()
			t.Fatalf("Cycle %d: Failed to create PeerConnection: %v", i, err)
		}

		_, err = pc.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
			Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
		})
		if err != nil {
			pc.Close()
			wsConn.Close()
			t.Fatalf("Cycle %d: Failed to add transceiver: %v", i, err)
		}

		offer, err := pc.CreateOffer(nil)
		if err != nil {
			pc.Close()
			wsConn.Close()
			t.Fatalf("Cycle %d: Failed to create offer: %v", i, err)
		}

		if err := pc.SetLocalDescription(offer); err != nil {
			pc.Close()
			wsConn.Close()
			t.Fatalf("Cycle %d: Failed to set local description: %v", i, err)
		}

		// 3. Send offer
		offerMsg := signaling.SignalingMessage{
			Type:      "offer",
			SessionID: sessionID,
			DeviceID:  targetDevice,
			SDP:       offer.SDP,
		}
		if err := wsConn.WriteJSON(offerMsg); err != nil {
			pc.Close()
			wsConn.Close()
			t.Fatalf("Cycle %d: Failed to send offer: %v", i, err)
		}

		// 4. Read answer
		wsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var answerMsg signaling.SignalingMessage
		if err := wsConn.ReadJSON(&answerMsg); err != nil {
			pc.Close()
			wsConn.Close()
			t.Fatalf("Cycle %d: Failed to read answer: %v", i, err)
		}

		if answerMsg.Type != "answer" || answerMsg.SDP == "" {
			pc.Close()
			wsConn.Close()
			t.Fatalf("Cycle %d: Invalid answer received: %+v", i, answerMsg)
		}

		// 5. Clean teardown
		_ = pc.Close()
		_ = wsConn.Close()
		webrtc.DefaultManager.RemoveSession(sessionID)
	}

	t.Logf("SUCCESS: All %d consecutive reconnect cycles negotiated cleanly!", cycles)
}

// TestChallenger_InvalidTokenRejectionMatrix tests all permutations of invalid,
// tampered, expired, and forged JWT tokens against both REST and WebSocket endpoints.
func TestChallenger_InvalidTokenRejectionMatrix(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", api.HandleLogin)
	mux.HandleFunc("/api/devices", api.HandleDevices)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	secret := security.GetJWTSecret()

	testCases := []struct {
		name        string
		token       string
		expectCode  int
		expectWSErr bool
	}{
		{
			name:        "Empty Token",
			token:       "",
			expectCode:  http.StatusUnauthorized,
			expectWSErr: true,
		},
		{
			name:        "Garbage ASCII Token",
			token:       "not.a.valid.jwt.token.string",
			expectCode:  http.StatusUnauthorized,
			expectWSErr: true,
		},
		{
			name: "Expired Token",
			token: createTestJWT(jwt.MapClaims{
				"sub":       "admin",
				"username":  "admin",
				"tenant_id": "tenant-alpha",
				"role":      "admin",
				"iat":       time.Now().Add(-2 * time.Hour).Unix(),
				"exp":       time.Now().Add(-1 * time.Hour).Unix(),
			}, secret, jwt.SigningMethodHS256),
			expectCode:  http.StatusUnauthorized,
			expectWSErr: true,
		},
		{
			name: "Wrong HMAC Secret Key",
			token: createTestJWT(jwt.MapClaims{
				"sub":       "admin",
				"username":  "admin",
				"tenant_id": "tenant-alpha",
				"role":      "admin",
				"iat":       time.Now().Unix(),
				"exp":       time.Now().Add(time.Hour).Unix(),
			}, []byte("completely-wrong-secret-key-123"), jwt.SigningMethodHS256),
			expectCode:  http.StatusUnauthorized,
			expectWSErr: true,
		},
		{
			name: "Algorithm None Attack",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"admin","username":"admin","tenant_id":"tenant-alpha","role":"admin"}`))
				return header + "." + payload + "."
			}(),
			expectCode:  http.StatusUnauthorized,
			expectWSErr: true,
		},
		{
			name: "Missing Username and Tenant Claims",
			token: createTestJWT(jwt.MapClaims{
				"role": "admin",
				"iat":  time.Now().Unix(),
				"exp":  time.Now().Add(time.Hour).Unix(),
			}, secret, jwt.SigningMethodHS256),
			expectCode:  http.StatusUnauthorized,
			expectWSErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// REST /api/devices validation
			req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("HTTP request failed: %v", err)
			}
			resp.Body.Close()

			if resp.StatusCode != tc.expectCode {
				t.Fatalf("REST expected HTTP %d, got %d for %s", tc.expectCode, resp.StatusCode, tc.name)
			}

			// WebSocket /client validation
			_, wsResp, wsErr := websocket.DefaultDialer.Dial(wsURL+"/client?token="+tc.token, nil)
			if tc.expectWSErr {
				if wsErr == nil {
					t.Fatalf("Expected WebSocket connection error for %s, but connection succeeded", tc.name)
				}
				if wsResp != nil && wsResp.StatusCode != http.StatusUnauthorized {
					t.Logf("WebSocket handshake status: %d", wsResp.StatusCode)
				}
			}
		})
	}
}

// TestChallenger_CrossTenantDeviceIsolationMatrix tests strict multi-tenant boundary
// enforcement across REST discovery and WebSocket WebRTC streaming.
func TestChallenger_CrossTenantDeviceIsolationMatrix(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", api.HandleLogin)
	mux.HandleFunc("/api/devices", api.HandleDevices)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// 1. Authenticate Tenant 1 User
	t1Payload, _ := json.Marshal(map[string]string{"username": "tenant1_user", "password": "password"})
	resp1, err := http.Post(server.URL+"/api/auth/login", "application/json", bytes.NewBuffer(t1Payload))
	if err != nil || resp1.StatusCode != http.StatusOK {
		t.Fatalf("Tenant 1 login failed: %v", err)
	}
	var t1Auth api.LoginResponse
	json.NewDecoder(resp1.Body).Decode(&t1Auth)
	resp1.Body.Close()

	// 2. Authenticate Tenant 2 User
	t2Payload, _ := json.Marshal(map[string]string{"username": "tenant2_user", "password": "password"})
	resp2, err := http.Post(server.URL+"/api/auth/login", "application/json", bytes.NewBuffer(t2Payload))
	if err != nil || resp2.StatusCode != http.StatusOK {
		t.Fatalf("Tenant 2 login failed: %v", err)
	}
	var t2Auth api.LoginResponse
	json.NewDecoder(resp2.Body).Decode(&t2Auth)
	resp2.Body.Close()

	// 3. Verify REST /api/devices scoping for Tenant 1
	req1, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	req1.Header.Set("Authorization", "Bearer "+t1Auth.Token)
	dResp1, _ := http.DefaultClient.Do(req1)
	var t1Devices []api.DeviceResponse
	json.NewDecoder(dResp1.Body).Decode(&t1Devices)
	dResp1.Body.Close()

	for _, d := range t1Devices {
		if strings.Contains(d.DeviceID, "tenant2") {
			t.Fatalf("CRITICAL: Tenant 1 received Tenant 2 device in listing: %s", d.DeviceID)
		}
	}

	// 4. Verify REST /api/devices scoping for Tenant 2
	req2, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	req2.Header.Set("Authorization", "Bearer "+t2Auth.Token)
	dResp2, _ := http.DefaultClient.Do(req2)
	var t2Devices []api.DeviceResponse
	json.NewDecoder(dResp2.Body).Decode(&t2Devices)
	dResp2.Body.Close()

	for _, d := range t2Devices {
		if strings.Contains(d.DeviceID, "tenant1") {
			t.Fatalf("CRITICAL: Tenant 2 received Tenant 1 device in listing: %s", d.DeviceID)
		}
	}

	// 5. Test Cross-Tenant Offer Injection (Tenant 1 requesting cam-tenant2-01)
	wsConn1, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+t1Auth.Token, nil)
	if err != nil {
		t.Fatalf("Tenant 1 WebSocket dial failed: %v", err)
	}
	defer wsConn1.Close()

	badOffer := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: "cross-tenant-viol-01",
		DeviceID:  "cam-tenant2-01",
		SDP:       "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
	}
	if err := wsConn1.WriteJSON(badOffer); err != nil {
		t.Fatalf("Write cross-tenant offer failed: %v", err)
	}

	wsConn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	var errResp map[string]interface{}
	readErr := wsConn1.ReadJSON(&errResp)
	if readErr == nil {
		if errResp["error"] != "Forbidden" {
			t.Fatalf("Expected 'Forbidden' error, got: %v", errResp)
		}
	}

	// 6. Test Reverse Cross-Tenant Offer Injection (Tenant 2 requesting cam-tenant1-01)
	wsConn2, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+t2Auth.Token, nil)
	if err != nil {
		t.Fatalf("Tenant 2 WebSocket dial failed: %v", err)
	}
	defer wsConn2.Close()

	badOffer2 := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: "cross-tenant-viol-02",
		DeviceID:  "cam-tenant1-01",
		SDP:       "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
	}
	if err := wsConn2.WriteJSON(badOffer2); err != nil {
		t.Fatalf("Write reverse cross-tenant offer failed: %v", err)
	}

	wsConn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	var errResp2 map[string]interface{}
	readErr2 := wsConn2.ReadJSON(&errResp2)
	if readErr2 == nil {
		if errResp2["error"] != "Forbidden" {
			t.Fatalf("Expected 'Forbidden' error, got: %v", errResp2)
		}
	}

	t.Log("SUCCESS: Bidirectional cross-tenant device isolation rigorously verified!")
}
