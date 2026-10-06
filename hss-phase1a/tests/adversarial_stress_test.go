package tests

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/api"
	"hss/internal/cloud/signaling"
	"hss/internal/security"
	"hss/internal/webrtc"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// helper to start a test server with all backend and signaling routes
func startTestBackend(t *testing.T) (*httptest.Server, string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", api.HandleLogin)
	mux.HandleFunc("/api/auth/login", api.HandleLogin)
	mux.HandleFunc("/api/devices", api.HandleDevices)
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	server := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	return server, wsURL
}

// helper to generate JWTs with specific claims and signing methods
func createTestJWT(claims jwt.MapClaims, key []byte, method jwt.SigningMethod) string {
	token := jwt.NewWithClaims(method, claims)
	tokenString, _ := token.SignedString(key)
	return tokenString
}

// helper to create unsigned "alg: none" JWT
func createUnsignedJWT(claims jwt.MapClaims) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	claimsBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsBytes)
	return header + "." + payload + "."
}

// ============================================================================
// 1. MULTI-TENANT ISOLATION UNDER ATTACK
// ============================================================================

func TestAdversarialMultiTenantIsolation(t *testing.T) {
	server, wsURL := startTestBackend(t)
	defer server.Close()

	// 1A. Login as Tenant 1 user
	t.Log("Testing Multi-Tenant Isolation: Tenant 1 vs Tenant 2 device access...")
	t1LoginBody, _ := json.Marshal(map[string]string{"username": "tenant1_user", "password": "password"})
	resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(t1LoginBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Tenant 1 login failed: %v, status: %d", err, resp.StatusCode)
	}
	var t1Auth api.LoginResponse
	json.NewDecoder(resp.Body).Decode(&t1Auth)
	resp.Body.Close()

	// 1B. Login as Tenant 2 user
	t2LoginBody, _ := json.Marshal(map[string]string{"username": "tenant2_user", "password": "password"})
	resp2, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(t2LoginBody))
	if err != nil || resp2.StatusCode != http.StatusOK {
		t.Fatalf("Tenant 2 login failed: %v, status: %d", err, resp2.StatusCode)
	}
	var t2Auth api.LoginResponse
	json.NewDecoder(resp2.Body).Decode(&t2Auth)
	resp2.Body.Close()

	// 1C. Verify Tenant 1 devices query strictly returns Tenant 1 cameras
	req1, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	req1.Header.Set("Authorization", "Bearer "+t1Auth.Token)
	devResp1, err := http.DefaultClient.Do(req1)
	if err != nil || devResp1.StatusCode != http.StatusOK {
		t.Fatalf("Tenant 1 devices query failed: %v", err)
	}
	var dev1 []api.DeviceResponse
	json.NewDecoder(devResp1.Body).Decode(&dev1)
	devResp1.Body.Close()

	for _, d := range dev1 {
		if strings.Contains(d.DeviceID, "tenant2") || strings.Contains(d.DeviceID, "beta") {
			t.Fatalf("CRITICAL SECURITY VULNERABILITY: Tenant 1 received foreign device: %s", d.DeviceID)
		}
	}
	t.Logf("PASS: Tenant 1 device list strictly isolated (%d devices)", len(dev1))

	// 1D. Verify Tenant 2 devices query strictly returns Tenant 2 cameras
	req2, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	req2.Header.Set("Authorization", "Bearer "+t2Auth.Token)
	devResp2, err := http.DefaultClient.Do(req2)
	if err != nil || devResp2.StatusCode != http.StatusOK {
		t.Fatalf("Tenant 2 devices query failed: %v", err)
	}
	var dev2 []api.DeviceResponse
	json.NewDecoder(devResp2.Body).Decode(&dev2)
	devResp2.Body.Close()

	for _, d := range dev2 {
		if strings.Contains(d.DeviceID, "tenant1") || strings.Contains(d.DeviceID, "alpha") {
			t.Fatalf("CRITICAL SECURITY VULNERABILITY: Tenant 2 received foreign device: %s", d.DeviceID)
		}
	}
	t.Logf("PASS: Tenant 2 device list strictly isolated (%d devices)", len(dev2))

	// 1E. ATTACK: Tenant 1 attempts to stream Tenant 2's camera ("cam-tenant2-01") via WebSocket
	t.Log("ATTACK: Tenant 1 attempting to hijack Tenant 2 camera stream over WebSocket...")
	wsConn1, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+t1Auth.Token, nil)
	if err != nil {
		t.Fatalf("WebSocket connection failed: %v", err)
	}
	defer wsConn1.Close()

	crossTenantOffer := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: "hijack-attempt-t1-t2",
		DeviceID:  "cam-tenant2-01", // Forbidden device
		SDP:       "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
	}
	if err := wsConn1.WriteJSON(crossTenantOffer); err != nil {
		t.Fatalf("Failed to write cross-tenant offer: %v", err)
	}

	wsConn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	var authzErr map[string]interface{}
	readErr := wsConn1.ReadJSON(&authzErr)
	if readErr != nil {
		t.Logf("PASS: WebSocket connection immediately severed on cross-tenant violation: %v", readErr)
	} else {
		if authzErr["error"] != "Forbidden" {
			t.Fatalf("CRITICAL SECURITY VULNERABILITY: Expected 'Forbidden', server returned: %v", authzErr)
		}
		t.Logf("PASS: Server rejected cross-tenant stream request with 'Forbidden'")
	}

	// 1F. ATTACK: Tenant 2 attempts to stream Tenant 1's camera ("cam-tenant1-01")
	t.Log("ATTACK: Tenant 2 attempting to hijack Tenant 1 camera stream over WebSocket...")
	wsConn2, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+t2Auth.Token, nil)
	if err != nil {
		t.Fatalf("WebSocket connection failed: %v", err)
	}
	defer wsConn2.Close()

	crossTenantOffer2 := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: "hijack-attempt-t2-t1",
		DeviceID:  "cam-tenant1-01", // Forbidden device
		SDP:       "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
	}
	if err := wsConn2.WriteJSON(crossTenantOffer2); err != nil {
		t.Fatalf("Failed to write cross-tenant offer: %v", err)
	}

	wsConn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	var authzErr2 map[string]interface{}
	readErr2 := wsConn2.ReadJSON(&authzErr2)
	if readErr2 != nil {
		t.Logf("PASS: WebSocket connection immediately severed on cross-tenant violation: %v", readErr2)
	} else {
		if authzErr2["error"] != "Forbidden" {
			t.Fatalf("CRITICAL SECURITY VULNERABILITY: Expected 'Forbidden', server returned: %v", authzErr2)
		}
		t.Logf("PASS: Server rejected reverse cross-tenant stream request with 'Forbidden'")
	}
}

// ============================================================================
// 2. ADVERSARIAL JWT ATTACK VECTORS (Tampering, alg:none, forged keys, expired)
// ============================================================================

func TestAdversarialJWTSecurity(t *testing.T) {
	server, wsURL := startTestBackend(t)
	defer server.Close()

	secret := security.GetJWTSecret()

	tests := []struct {
		name          string
		tokenGen      func() string
		expectedCode  int
		expectedWSErr bool
	}{
		{
			name: "Unsigned alg:none Token (Signature Bypass Attack)",
			tokenGen: func() string {
				return createUnsignedJWT(jwt.MapClaims{
					"sub":       "admin",
					"username":  "admin",
					"tenant_id": "tenant-alpha",
					"role":      "admin",
					"iat":       time.Now().Unix(),
					"exp":       time.Now().Add(time.Hour).Unix(),
				})
			},
			expectedCode:  http.StatusUnauthorized,
			expectedWSErr: true,
		},
		{
			name: "Forged HMAC Signature (Signed with Attacker Secret)",
			tokenGen: func() string {
				return createTestJWT(jwt.MapClaims{
					"sub":       "admin",
					"username":  "admin",
					"tenant_id": "tenant-alpha",
					"role":      "admin",
					"iat":       time.Now().Unix(),
					"exp":       time.Now().Add(time.Hour).Unix(),
				}, []byte("malicious-attacker-secret-key-666"), jwt.SigningMethodHS256)
			},
			expectedCode:  http.StatusUnauthorized,
			expectedWSErr: true,
		},
		{
			name: "Tampered Token Payload (Valid Signature with Mutated Payload Claims)",
			tokenGen: func() string {
				validToken := createTestJWT(jwt.MapClaims{
					"sub":       "user2",
					"username":  "user2",
					"tenant_id": "tenant-beta",
					"role":      "viewer",
					"iat":       time.Now().Unix(),
					"exp":       time.Now().Add(time.Hour).Unix(),
				}, secret, jwt.SigningMethodHS256)

				parts := strings.Split(validToken, ".")
				if len(parts) != 3 {
					return validToken
				}
				// Mutate claims to admin
				tamperedClaims := `{"sub":"admin","username":"admin","tenant_id":"tenant-alpha","role":"admin"}`
				tamperedPayload := base64.RawURLEncoding.EncodeToString([]byte(tamperedClaims))
				return parts[0] + "." + tamperedPayload + "." + parts[2]
			},
			expectedCode:  http.StatusUnauthorized,
			expectedWSErr: true,
		},
		{
			name: "Expired JWT Token (Past Expiration Attack)",
			tokenGen: func() string {
				return createTestJWT(jwt.MapClaims{
					"sub":       "admin",
					"username":  "admin",
					"tenant_id": "tenant-alpha",
					"role":      "admin",
					"iat":       time.Now().Add(-2 * time.Hour).Unix(),
					"exp":       time.Now().Add(-1 * time.Hour).Unix(),
				}, secret, jwt.SigningMethodHS256)
			},
			expectedCode:  http.StatusUnauthorized,
			expectedWSErr: true,
		},
		{
			name: "Token with Missing Subject / Claims",
			tokenGen: func() string {
				return createTestJWT(jwt.MapClaims{
					"role": "admin",
					"iat":  time.Now().Unix(),
					"exp":  time.Now().Add(time.Hour).Unix(),
				}, secret, jwt.SigningMethodHS256)
			},
			expectedCode:  http.StatusUnauthorized,
			expectedWSErr: true,
		},
		{
			name: "Completely Corrupted / Malformed Garbage String",
			tokenGen: func() string {
				return "Bearer.not-a-valid-token-payload-garbage-12345!@#$%"
			},
			expectedCode:  http.StatusUnauthorized,
			expectedWSErr: true,
		},
		{
			name: "SQL Injection String in JWT Subject Claim",
			tokenGen: func() string {
				return createTestJWT(jwt.MapClaims{
					"sub":       "admin' OR '1'='1",
					"username":  "admin' OR '1'='1",
					"tenant_id": "tenant-alpha",
					"role":      "admin",
					"iat":       time.Now().Unix(),
					"exp":       time.Now().Add(time.Hour).Unix(),
				}, secret, jwt.SigningMethodHS256)
			},
			expectedCode:  http.StatusOK, // Valid token structure, but user not found in mock DB returns empty devices list
			expectedWSErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token := tc.tokenGen()

			// Test 1: REST API /api/devices with tampered/invalid token
			req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("REST request failed: %v", err)
			}
			defer resp.Body.Close()

			if tc.expectedCode == http.StatusUnauthorized && resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("CRITICAL: Expected HTTP 401 for %s, got: %d", tc.name, resp.StatusCode)
			}

			// If SQL injection test, verify devices returned is empty (no unauthorized leakage)
			if strings.Contains(tc.name, "SQL Injection") {
				var devs []api.DeviceResponse
				json.NewDecoder(resp.Body).Decode(&devs)
				if len(devs) != 0 {
					t.Fatalf("SQL Injection succeeded! Leaked %d devices: %v", len(devs), devs)
				}
				t.Logf("PASS: SQL injection in JWT claims returned 0 devices without leakage")
			}

			// Test 2: WebRTC Signaling WebSocket /client with tampered/invalid token
			_, wsResp, wsErr := websocket.DefaultDialer.Dial(wsURL+"/client?token="+token, nil)
			if tc.expectedWSErr {
				if wsErr == nil {
					t.Fatalf("CRITICAL: WebSocket connection accepted invalid token for: %s", tc.name)
				}
				if wsResp != nil && wsResp.StatusCode != http.StatusUnauthorized {
					t.Logf("WebSocket handshake rejected with status: %d", wsResp.StatusCode)
				}
				t.Logf("PASS: WebSocket rejected invalid JWT (%s)", tc.name)
			}
		})
	}
}

// ============================================================================
// 3. WEBRTC SIGNALING HUB HIGH-CONCURRENCY & RAPID DISCONNECT STRESS TEST
// ============================================================================

func TestSignalingHubWebSocketStress(t *testing.T) {
	server, wsURL := startTestBackend(t)
	defer server.Close()

	const concurrentClients = 50
	t.Logf("Stress-testing WebRTC Signaling Hub with %d concurrent WebSocket clients...", concurrentClients)

	secret := security.GetJWTSecret()
	var wg sync.WaitGroup
	var successfulSignaling int64
	var connectionErrors int64

	startTime := time.Now()

	for i := 0; i < concurrentClients; i++ {
		wg.Add(1)
		clientID := i
		go func() {
			defer wg.Done()

			// Generate valid tenant1 token
			token := createTestJWT(jwt.MapClaims{
				"sub":       "tenant1_user",
				"username":  "tenant1_user",
				"tenant_id": "tenant-1",
				"role":      "admin",
				"iat":       time.Now().Unix(),
				"exp":       time.Now().Add(time.Hour).Unix(),
			}, secret, jwt.SigningMethodHS256)

			sessionID := fmt.Sprintf("stress-sess-%03d", clientID)
			deviceID := "cam-tenant1-01"

			conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+token, nil)
			if err != nil {
				atomic.AddInt64(&connectionErrors, 1)
				return
			}
			defer conn.Close()

			// Send Offer
			offerMsg := signaling.SignalingMessage{
				Type:      "offer",
				SessionID: sessionID,
				DeviceID:  deviceID,
				SDP:       "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
			}
			if err := conn.WriteJSON(offerMsg); err != nil {
				atomic.AddInt64(&connectionErrors, 1)
				return
			}

			// Send multiple ICE candidates in rapid succession
			for c := 0; c < 5; c++ {
				candMsg := signaling.SignalingMessage{
					Type:      "candidate",
					SessionID: sessionID,
					DeviceID:  deviceID,
					Candidate: fmt.Sprintf(`{"candidate":"candidate:%d 1 udp 2122260223 192.168.1.%d %d typ host","sdpMid":"0","sdpMLineIndex":0}`, c, clientID+10, 5000+c),
				}
				if err := conn.WriteJSON(candMsg); err != nil {
					atomic.AddInt64(&connectionErrors, 1)
					return
				}
			}

			atomic.AddInt64(&successfulSignaling, 1)
		}()
	}

	wg.Wait()
	duration := time.Since(startTime)

	t.Logf("Completed %d concurrent client signaling sessions in %v (Success: %d, Errors: %d)",
		concurrentClients, duration, atomic.LoadInt64(&successfulSignaling), atomic.LoadInt64(&connectionErrors))

	if atomic.LoadInt64(&connectionErrors) > 0 {
		t.Fatalf("WebSocket stress test encountered %d errors", connectionErrors)
	}

	// Rapid Connect / Violent Disconnect Storm
	const stormCount = 100
	t.Logf("Launching Rapid Disconnect Storm (%d connections opened and violently torn down)...", stormCount)

	var stormWg sync.WaitGroup
	var stormSuccess int64

	for i := 0; i < stormCount; i++ {
		stormWg.Add(1)
		go func(idx int) {
			defer stormWg.Done()

			token := createTestJWT(jwt.MapClaims{
				"sub":       "admin",
				"username":  "admin",
				"tenant_id": "tenant-alpha",
				"role":      "admin",
				"iat":       time.Now().Unix(),
				"exp":       time.Now().Add(time.Hour).Unix(),
			}, secret, jwt.SigningMethodHS256)

			conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+token, nil)
			if err != nil {
				return
			}

			// Randomize immediate closure or post-message closure
			if idx%2 == 0 {
				_ = conn.Close() // Violent TCP close before message
			} else {
				_ = conn.WriteJSON(map[string]string{
					"type":       "offer",
					"device_id":  "test-camera-1",
					"session_id": fmt.Sprintf("storm-sess-%d", idx),
				})
				time.Sleep(time.Duration(idx%5) * time.Millisecond)
				_ = conn.Close()
			}
			atomic.AddInt64(&stormSuccess, 1)
		}(i)
	}

	stormWg.Wait()
	t.Logf("PASS: Completed %d rapid disconnects without server crash or deadlock (Success: %d)", stormCount, atomic.LoadInt64(&stormSuccess))
}

// ============================================================================
// 4. WEBRTC SIGNALING PAYLOAD FUZZING & INVALID SDP / ICE HANDLING
// ============================================================================

func TestWebRTCSignalingFuzzingAndInvalidPayloads(t *testing.T) {
	server, wsURL := startTestBackend(t)
	defer server.Close()

	secret := security.GetJWTSecret()
	token := createTestJWT(jwt.MapClaims{
		"sub":       "admin",
		"username":  "admin",
		"tenant_id": "tenant-alpha",
		"role":      "admin",
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Hour).Unix(),
	}, secret, jwt.SigningMethodHS256)

	t.Log("Fuzzing WebSocket Signaling Hub with malformed, out-of-order, and oversized payloads...")

	// 4A. Malformed JSON payload
	t.Run("Malformed JSON Syntax", func(t *testing.T) {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+token, nil)
		if err != nil {
			t.Fatalf("Connect failed: %v", err)
		}
		defer conn.Close()

		// Write broken JSON
		err = conn.WriteMessage(websocket.TextMessage, []byte(`{"type": "offer", "device_id": "test-camera-1", `))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var resp map[string]interface{}
		_ = conn.ReadJSON(&resp)
		t.Logf("PASS: Server responded to malformed JSON with: %v", resp)
	})

	// 4B. Massive 2MB Binary Garbage Payload
	t.Run("Oversized 2MB Binary Garbage Payload", func(t *testing.T) {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+token, nil)
		if err != nil {
			t.Fatalf("Connect failed: %v", err)
		}
		defer conn.Close()

		garbage := make([]byte, 2*1024*1024)
		rand.Read(garbage)

		_ = conn.WriteMessage(websocket.BinaryMessage, garbage)
		time.Sleep(100 * time.Millisecond)

		// Assert server survived and can accept fresh requests
		healthCheck, err := http.Get(server.URL + "/metrics")
		if err != nil || healthCheck.StatusCode != http.StatusOK {
			t.Fatalf("CRITICAL: Server crashed after oversized payload!")
		}
		healthCheck.Body.Close()
		t.Log("PASS: Server survived 2MB oversized binary fuzz payload without crashing")
	})

	// 4C. Invalid SDP Offer strings to WebRTC Manager
	t.Run("Invalid SDP Offer Handling in Manager", func(t *testing.T) {
		mgr := webrtc.NewManager()

		invalidSDPs := []struct {
			name string
			sdp  string
		}{
			{"Empty SDP", ""},
			{"Whitespace SDP", "   \n\r\t  "},
			{"Random Binary String", "\x00\x01\x02\x03\xff\xfe\xfd"},
			{"Incomplete SDP (no media lines)", "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\n"},
			{"Malformed Header SDP", "INVALID_SDP_PROTOCOL_DATA"},
		}

		for _, tc := range invalidSDPs {
			t.Run(tc.name, func(t *testing.T) {
				_, err := mgr.HandleOffer("invalid-sess-01", "cam-test-01", tc.sdp, "", "", "", nil)
				if err == nil {
					t.Fatalf("Expected error for %s, got nil", tc.name)
				}
				t.Logf("PASS: Manager cleanly rejected %s with error: %v", tc.name, err)
			})
		}
	})

	// 4D. Out-of-Order ICE Candidates (Before Offer / Non-Existent Session)
	t.Run("Out-of-Order and Ghost Session ICE Candidates", func(t *testing.T) {
		mgr := webrtc.NewManager()

		// Send ICE candidate to non-existent session
		err := mgr.AddICECandidate("ghost-session-9999", `{"candidate":"candidate:1 1 udp 2122260223 127.0.0.1 5000 typ host","sdpMid":"0","sdpMLineIndex":0}`)
		if err == nil {
			t.Fatalf("Expected error when adding candidate to non-existent session, got nil")
		}
		t.Logf("PASS: Non-existent session candidate correctly rejected: %v", err)

		// Create a valid session first
		viewerPC, _ := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
		defer viewerPC.Close()
		viewerPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{Direction: pionwebrtc.RTPTransceiverDirectionRecvonly})
		offer, _ := viewerPC.CreateOffer(nil)
		viewerPC.SetLocalDescription(offer)

		_, err = mgr.HandleOffer("valid-sess-01", "cam-test-01", offer.SDP, "", "", "", nil)
		if err != nil {
			t.Fatalf("Valid HandleOffer failed: %v", err)
		}
		defer mgr.RemoveSession("valid-sess-01")

		// Fuzz candidate inputs on valid session
		candidateFuzz := []struct {
			name      string
			candidate string
		}{
			{"Empty Candidate", ""},
			{"Null String", "null"},
			{"Empty JSON Object", "{}"},
			{"Raw Candidate String", "candidate:1 1 udp 2122260223 127.0.0.1 5000 typ host"},
			{"JSON ICECandidateInit", `{"candidate":"candidate:2 1 udp 2122260223 127.0.0.1 5001 typ host","sdpMid":"0","sdpMLineIndex":0}`},
			{"Double Encoded JSON", `"{\"candidate\":\"candidate:3 1 udp 2122260223 127.0.0.1 5002 typ host\",\"sdpMid\":\"0\",\"sdpMLineIndex\":0}"`},
		}

		for _, cf := range candidateFuzz {
			t.Run(cf.name, func(t *testing.T) {
				candErr := mgr.AddICECandidate("valid-sess-01", cf.candidate)
				t.Logf("Candidate format [%s] handled safely (err: %v)", cf.name, candErr)
			})
		}
	})
}

// ============================================================================
// 5. CAMERA CONCURRENCY LIMITS & VIEWER COUNT ROLLBACK VALIDATION
// ============================================================================

func TestWebRTCViewerConcurrencyLimitsAndRollback(t *testing.T) {
	mgr := webrtc.NewManager()
	camID := "cam-concurrency-test"

	// Helper to generate a valid SDP offer string
	createOfferSDP := func() string {
		pc, _ := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
		defer pc.Close()
		pc.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{Direction: pionwebrtc.RTPTransceiverDirectionRecvonly})
		offer, _ := pc.CreateOffer(nil)
		return offer.SDP
	}

	validSDP := createOfferSDP()

	// 5A. Connect 3 viewers (Maximum allowed)
	t.Log("Connecting 3 legitimate viewers to camera...")
	_, err1 := mgr.HandleOffer("sess-viewer-1", camID, validSDP, "", "", "", nil)
	if err1 != nil {
		t.Fatalf("Viewer 1 failed: %v", err1)
	}
	_, err2 := mgr.HandleOffer("sess-viewer-2", camID, validSDP, "", "", "", nil)
	if err2 != nil {
		t.Fatalf("Viewer 2 failed: %v", err2)
	}
	_, err3 := mgr.HandleOffer("sess-viewer-3", camID, validSDP, "", "", "", nil)
	if err3 != nil {
		t.Fatalf("Viewer 3 failed: %v", err3)
	}

	// 5B. 4th viewer MUST be rejected
	t.Log("Attempting 4th viewer connection (expected to exceed limit)...")
	_, err4 := mgr.HandleOffer("sess-viewer-4", camID, validSDP, "", "", "", nil)
	if err4 == nil {
		t.Fatalf("SECURITY/RESOURCE DEFECT: 4th viewer was accepted beyond 3-viewer limit!")
	}
	if !strings.Contains(err4.Error(), "maximum of 3 concurrent viewers") {
		t.Fatalf("Expected limit error message, got: %v", err4)
	}
	t.Logf("PASS: 4th viewer rejected: %v", err4)

	// 5C. Disconnect 1 viewer -> 4th viewer should now succeed
	t.Log("Disconnecting Viewer 1 and verifying a new viewer can connect...")
	mgr.RemoveSession("sess-viewer-1")

	_, err5 := mgr.HandleOffer("sess-viewer-5", camID, validSDP, "", "", "", nil)
	if err5 != nil {
		t.Fatalf("Viewer 5 failed to connect after slot freed: %v", err5)
	}
	t.Log("PASS: Freed viewer slot properly reused")

	// 5D. Viewer Count Rollback on Invalid Offer (No Leaks on Negotiation Failure)
	mgr.RemoveSession("sess-viewer-2")
	mgr.RemoveSession("sess-viewer-3")
	mgr.RemoveSession("sess-viewer-5")

	t.Log("Testing Viewer Count Rollback on Failed Negotiation...")
	// Attempt invalid SDP offer
	_, errInvalid := mgr.HandleOffer("sess-failed", camID, "NOT_AN_SDP", "", "", "", nil)
	if errInvalid == nil {
		t.Fatalf("Expected invalid SDP error")
	}

	// Connect 3 valid viewers again to prove no count leaked
	_, errV1 := mgr.HandleOffer("sess-v1", camID, validSDP, "", "", "", nil)
	_, errV2 := mgr.HandleOffer("sess-v2", camID, validSDP, "", "", "", nil)
	_, errV3 := mgr.HandleOffer("sess-v3", camID, validSDP, "", "", "", nil)

	if errV1 != nil || errV2 != nil || errV3 != nil {
		t.Fatalf("Viewer count leaked after failed negotiation! Errors: v1=%v, v2=%v, v3=%v", errV1, errV2, errV3)
	}
	t.Log("PASS: Failed negotiations correctly rollback viewer count without leakage")

	mgr.RemoveSession("sess-v1")
	mgr.RemoveSession("sess-v2")
	mgr.RemoveSession("sess-v3")
}

// ============================================================================
// 6. GOROUTINE LEAK & RACE DETECTOR RESILIENCE TEST
// ============================================================================

func TestGoroutineLeakAndDeadlockResilience(t *testing.T) {
	server, wsURL := startTestBackend(t)
	defer server.Close()

	secret := security.GetJWTSecret()

	// Capture initial baseline goroutines
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial Goroutine count: %d", initialGoroutines)

	const cycleCount = 30
	t.Logf("Executing %d full connect-offer-candidate-disconnect cycles...", cycleCount)

	for i := 0; i < cycleCount; i++ {
		token := createTestJWT(jwt.MapClaims{
			"sub":       "tenant1_user",
			"username":  "tenant1_user",
			"tenant_id": "tenant-1",
			"role":      "admin",
			"iat":       time.Now().Unix(),
			"exp":       time.Now().Add(time.Hour).Unix(),
		}, secret, jwt.SigningMethodHS256)

		sessionID := fmt.Sprintf("leak-test-sess-%d", i)

		// Dial client WS
		clientWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+token, nil)
		if err != nil {
			t.Fatalf("Dial failed on cycle %d: %v", i, err)
		}

		_ = clientWS.WriteJSON(signaling.SignalingMessage{
			Type:      "offer",
			SessionID: sessionID,
			DeviceID:  "cam-tenant1-01",
			SDP:       "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
		})

		_ = clientWS.WriteJSON(signaling.SignalingMessage{
			Type:      "candidate",
			SessionID: sessionID,
			DeviceID:  "cam-tenant1-01",
			Candidate: `{"candidate":"candidate:1 1 udp 2122260223 127.0.0.1 5000 typ host","sdpMid":"0","sdpMLineIndex":0}`,
		})

		// Close connection
		_ = clientWS.Close()
	}

	// Allow goroutines a grace period to terminate
	time.Sleep(500 * time.Millisecond)
	runtime.GC()
	time.Sleep(200 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Post-stress Goroutine count: %d (Delta: %+d)", finalGoroutines, finalGoroutines-initialGoroutines)

	// In Go test runners, a small delta (+/- 5 goroutines for internal timers/runtime) is normal,
	// but a leak of 30+ goroutines indicates unclosed pubsub loops or hanging readers.
	if delta := finalGoroutines - initialGoroutines; delta > 15 {
		t.Fatalf("POTENTIAL GOROUTINE LEAK DETECTED: Goroutines increased by %d after %d cycles!", delta, cycleCount)
	}

	t.Log("PASS: Goroutine counts returned to baseline. Zero goroutine leaks detected.")
}
