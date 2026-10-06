package tests

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hss/internal/api"
	"hss/internal/cloud/cache"
	"hss/internal/cloud/signaling"
	"hss/internal/security"

	"github.com/gorilla/websocket"
)

// setupSecurityTestServer creates a test backend with full enterprise security routes
func setupSecurityTestServer(t *testing.T) (*httptest.Server, string, *security.IPRateLimiter) {
	mux := http.NewServeMux()

	// Rate limiter with burst 5 for testing
	loginLimiter := security.NewIPRateLimiter(5.0, 5)

	mux.HandleFunc("/api/login", security.RateLimitMiddleware(loginLimiter, api.HandleLogin))
	mux.HandleFunc("/api/auth/login", security.RateLimitMiddleware(loginLimiter, api.HandleLogin))
	mux.HandleFunc("/api/auth/refresh", api.HandleRefresh)
	mux.HandleFunc("/api/auth/revoke", api.HandleRevoke)
	mux.HandleFunc("/api/turn/credentials", api.HandleTurnCredentials)
	mux.HandleFunc("/api/devices", api.HandleDevices)
	mux.HandleFunc("/api/push/register", api.HandlePushRegister)

	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	return server, wsURL, loginLimiter
}

// 1. IAM, Short-Lived Tokens, Refresh Tokens, Revocation Blacklist & RBAC
func TestIAMTokenLifecycleAndRevocation(t *testing.T) {
	server, _, loginLimiter := setupSecurityTestServer(t)
	defer server.Close()
	defer loginLimiter.Close()

	// 1A. Login as Admin
	loginPayload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "password",
	})
	resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(loginPayload))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed: status %d, err %v", resp.StatusCode, err)
	}

	var authResp api.LoginResponse
	json.NewDecoder(resp.Body).Decode(&authResp)
	resp.Body.Close()

	if authResp.AccessToken == "" || authResp.RefreshToken == "" {
		t.Fatalf("Expected both AccessToken and RefreshToken, got: %+v", authResp)
	}
	if authResp.ExpiresIn != int64(api.AccessTokenDuration.Seconds()) {
		t.Fatalf("Expected short-lived AccessToken (900s), got: %d", authResp.ExpiresIn)
	}
	if authResp.User.Role != "admin" {
		t.Fatalf("Expected role 'admin', got: %s", authResp.User.Role)
	}
	if len(authResp.User.Permissions) == 0 {
		t.Fatal("Expected permissions list in UserInfo")
	}

	t.Logf("PASS: Short-lived Access Token (%ds) and Refresh Token issued", authResp.ExpiresIn)

	// 1B. Use Access Token to fetch devices
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	devResp, err := http.DefaultClient.Do(req)
	if err != nil || devResp.StatusCode != http.StatusOK {
		t.Fatalf("Fetch devices failed: %d, err: %v", devResp.StatusCode, err)
	}
	devResp.Body.Close()
	t.Log("PASS: Authenticated request with Access Token succeeded")

	// 1C. Refresh Access Token using Refresh Token
	refreshPayload, _ := json.Marshal(map[string]string{
		"refresh_token": authResp.RefreshToken,
	})
	refResp, err := http.Post(server.URL+"/api/auth/refresh", "application/json", bytes.NewBuffer(refreshPayload))
	if err != nil || refResp.StatusCode != http.StatusOK {
		t.Fatalf("Token refresh failed: %d, err: %v", refResp.StatusCode, err)
	}
	var newAuthResp api.LoginResponse
	json.NewDecoder(refResp.Body).Decode(&newAuthResp)
	refResp.Body.Close()

	if newAuthResp.AccessToken == "" {
		t.Fatal("Expected new Access Token on refresh")
	}
	t.Log("PASS: Access Token refreshed successfully using Refresh Token")

	// 1D. Revoke Tokens (Logout)
	revokePayload, _ := json.Marshal(map[string]string{
		"refresh_token": authResp.RefreshToken,
		"token":         authResp.AccessToken,
	})
	revResp, err := http.Post(server.URL+"/api/auth/revoke", "application/json", bytes.NewBuffer(revokePayload))
	if err != nil || revResp.StatusCode != http.StatusOK {
		t.Fatalf("Token revocation failed: %d, err: %v", revResp.StatusCode, err)
	}
	revResp.Body.Close()
	t.Log("PASS: Tokens revoked successfully")

	// 1E. Verify revoked Access Token is rejected with HTTP 401
	reqRevoked, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	reqRevoked.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	revDevResp, err := http.DefaultClient.Do(reqRevoked)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer revDevResp.Body.Close()

	if revDevResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("SECURITY VULNERABILITY: Revoked Access Token was accepted! Status: %d", revDevResp.StatusCode)
	}
	t.Log("PASS: Revoked Access Token correctly rejected by Blacklist with HTTP 401")

	// 1F. Verify revoked Refresh Token is rejected on /api/auth/refresh
	refResp2, err := http.Post(server.URL+"/api/auth/refresh", "application/json", bytes.NewBuffer(refreshPayload))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer refResp2.Body.Close()

	if refResp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("SECURITY VULNERABILITY: Revoked Refresh Token was accepted! Status: %d", refResp2.StatusCode)
	}
	t.Log("PASS: Revoked Refresh Token correctly rejected with HTTP 401")
}

// 2. Dynamic TURN Credentials RFC 5766
func TestDynamicTURNCredentialsAPI(t *testing.T) {
	server, _, loginLimiter := setupSecurityTestServer(t)
	defer server.Close()
	defer loginLimiter.Close()

	// Login
	loginPayload, _ := json.Marshal(map[string]string{"username": "admin", "password": "password"})
	loginResp, _ := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(loginPayload))
	var auth api.LoginResponse
	json.NewDecoder(loginResp.Body).Decode(&auth)
	loginResp.Body.Close()

	// Fetch dynamic TURN credentials
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/turn/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to fetch TURN credentials: %d, err: %v", resp.StatusCode, err)
	}

	var creds security.TURNCredentials
	json.NewDecoder(resp.Body).Decode(&creds)
	resp.Body.Close()

	if creds.Username == "" || creds.Password == "" {
		t.Fatalf("Empty TURN credentials returned: %+v", creds)
	}
	if !strings.Contains(creds.Username, ":admin") {
		t.Fatalf("Expected dynamic username timestamp:admin, got: %s", creds.Username)
	}

	// Verify HMAC signature
	valid, err := security.VerifyTURNCredentials(security.GetDefaultTURNSecret(), creds.Username, creds.Password)
	if err != nil || !valid {
		t.Fatalf("Dynamic TURN credentials failed verification: valid=%v, err=%v", valid, err)
	}

	t.Logf("PASS: Dynamic TURN credentials generated and HMAC verified: %s", creds.Username)
}

// 3. Token-Bucket Rate Limiting on Flood Attacks
func TestLoginRateLimitingFloodAttack(t *testing.T) {
	server, _, loginLimiter := setupSecurityTestServer(t)
	defer server.Close()
	defer loginLimiter.Close()

	loginPayload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "password",
	})

	var got429 bool
	const floodCount = 20

	for i := 0; i < floodCount; i++ {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/login", bytes.NewBuffer(loginPayload))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.25:44321"

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter == "" {
				t.Fatal("Expected Retry-After header on 429 response")
			}
			resp.Body.Close()
			t.Logf("PASS: Flood attack throttled with HTTP 429 on request #%d", i+1)
			break
		}
		resp.Body.Close()
	}

	if !got429 {
		t.Fatal("SECURITY DEFECT: Rate limiter failed to block rapid login flood attack!")
	}
}

// 4. Strict WebSocket 64KB Payload Limit Bound
func TestWebSocketStrictPayloadLimit(t *testing.T) {
	server, wsURL, loginLimiter := setupSecurityTestServer(t)
	defer server.Close()
	defer loginLimiter.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token=admin", nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer conn.Close()

	// Initial valid message to set up session
	_ = conn.WriteJSON(signaling.SignalingMessage{
		Type:      "offer",
		SessionID: "sess-limit-test",
		DeviceID:  "test-camera-1",
		SDP:       "v=0\r\n",
	})

	time.Sleep(50 * time.Millisecond)

	// Send an oversized payload > 64 KB (e.g. 70 KB)
	oversizedBytes := make([]byte, 70*1024)
	for i := range oversizedBytes {
		oversizedBytes[i] = 'A'
	}

	oversizedMsg := signaling.SignalingMessage{
		Type:      "candidate",
		SessionID: "sess-limit-test",
		DeviceID:  "test-camera-1",
		Candidate: string(oversizedBytes),
	}

	_ = conn.WriteJSON(oversizedMsg)

	// Read from connection: the server should close/terminate connection due to read limit
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, readErr := conn.ReadMessage()
	if readErr == nil {
		// Attempt one more write/read to confirm closure
		_ = conn.WriteJSON(oversizedMsg)
		_, _, readErr = conn.ReadMessage()
	}

	if readErr == nil {
		t.Fatal("SECURITY DEFECT: WebSocket accepted payload exceeding 64KB read limit!")
	}

	t.Logf("PASS: WebSocket connection cleanly severed on oversized payload: %v", readErr)
}

// 5. Zero-Trust Video Pipeline (E2EE SFrame Video Encryption)
func TestZeroTrustE2EESFramePipeline(t *testing.T) {
	// Session key established between Edge camera and authorized Client
	sessionKey, _ := security.GenerateSFrameKey()
	sessionSalt, _ := security.GenerateSFrameSalt()

	edgeCipher, err := security.NewSFrameCipher(1, sessionKey, sessionSalt)
	if err != nil {
		t.Fatalf("Edge cipher init failed: %v", err)
	}

	clientCipher, err := security.NewSFrameCipher(1, sessionKey, sessionSalt)
	if err != nil {
		t.Fatalf("Client cipher init failed: %v", err)
	}

	// Rogue/Unauthorized Intermediate Hub cipher (has wrong key)
	rogueKey, _ := security.GenerateSFrameKey()
	rogueCipher, _ := security.NewSFrameCipher(1, rogueKey, sessionSalt)

	// 5A. Raw H.264 NALUs from RTSP camera stream
	rawSPS := []byte{0x67, 0x42, 0x00, 0x1F, 0xE9, 0x01, 0x40, 0x7B, 0x20}
	rawPPS := []byte{0x68, 0xCE, 0x38, 0x80}
	rawIDR := []byte{0x65, 0xB8, 0x00, 0x04, 0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x12, 0x34}

	rawFrames := [][]byte{rawSPS, rawPPS, rawIDR}

	// 5B. Edge Daemon encrypts raw NALUs before RTP transmission
	encryptedFrames, err := edgeCipher.EncryptNALUs(rawFrames, 1000)
	if err != nil {
		t.Fatalf("Edge SFrame encryption failed: %v", err)
	}

	// 5C. Intermediate Hub Inspection: Hub sees ONLY ciphertext
	for i, encFrame := range encryptedFrames {
		if bytes.Contains(encFrame, rawFrames[i]) {
			t.Fatalf("SECURITY DEFECT: Frame %d contains unencrypted plaintext video NALU!", i)
		}
		// Rogue intermediate node tries to decrypt without key
		_, rogueErr := rogueCipher.DecryptFrame(encFrame)
		if rogueErr == nil {
			t.Fatalf("SECURITY DEFECT: Rogue intermediate node decrypted video without key!")
		}
	}
	t.Log("PASS: Intermediate Hub sees only authenticated ciphertext and cannot decrypt")

	// 5D. Authorized Client decrypts the SFrame video stream
	decryptedFrames, err := clientCipher.DecryptNALUs(encryptedFrames)
	if err != nil {
		t.Fatalf("Client decryption failed: %v", err)
	}

	for i := range rawFrames {
		if !bytes.Equal(decryptedFrames[i], rawFrames[i]) {
			t.Fatalf("Frame %d mismatch after E2EE decryption!", i)
		}
	}
	t.Log("PASS: Authorized Client cleanly decrypted all H.264 video NALUs (100% fidelity)")
}

// 6. Mutual TLS (mTLS) Edge Daemon Client Verification
func TestMutualTLSEdgeAuthentication(t *testing.T) {
	creds, err := security.GenerateMTLSCredentials()
	if err != nil {
		t.Fatalf("Failed to generate mTLS credentials: %v", err)
	}

	serverTLS, err := security.NewServerMTLSConfig(creds.CACertPEM, creds.ServerCertPEM, creds.ServerKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create server TLS config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)

	mtlsServer := httptest.NewUnstartedServer(mux)
	mtlsServer.TLS = serverTLS
	mtlsServer.StartTLS()
	defer mtlsServer.Close()

	wssURL := "wss" + strings.TrimPrefix(mtlsServer.URL, "https")

	// 6A. Edge client with valid certificate connects
	validClientTLS, err := security.NewClientMTLSConfig(creds.CACertPEM, creds.ClientCertPEM, creds.ClientKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create valid client TLS: %v", err)
	}

	validDialer := &websocket.Dialer{
		TLSClientConfig: validClientTLS,
	}

	validConn, _, err := validDialer.Dial(wssURL+"/signaling?camera_id=cam-mtls-01&token=mock_device_jwt", nil)
	if err != nil {
		t.Fatalf("Valid mTLS Edge connection failed: %v", err)
	}
	defer validConn.Close()
	t.Log("PASS: Edge Daemon with valid mTLS certificate connected successfully")

	// 6B. Edge client without certificate is rejected by TLS handshake
	noCertClientTLS, err := security.NewClientMTLSConfig(creds.CACertPEM, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create client TLS: %v", err)
	}

	noCertDialer := &websocket.Dialer{
		TLSClientConfig: noCertClientTLS,
	}

	_, _, err = noCertDialer.Dial(wssURL+"/signaling?camera_id=cam-mtls-01&token=mock_device_jwt", nil)
	if err == nil {
		t.Fatal("SECURITY DEFECT: Edge connection without client certificate was accepted by mTLS server!")
	}
	t.Logf("PASS: Edge connection without client certificate rejected by mTLS: %v", err)

	// 6C. Edge client with untrusted rogue certificate is rejected
	rogueClientTLS, err := security.NewClientMTLSConfig(creds.CACertPEM, creds.UntrustedClientPEM, creds.UntrustedKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create rogue client TLS: %v", err)
	}

	rogueDialer := &websocket.Dialer{
		TLSClientConfig: rogueClientTLS,
	}

	_, _, err = rogueDialer.Dial(wssURL+"/signaling?camera_id=cam-mtls-01&token=mock_device_jwt", nil)
	if err == nil {
		t.Fatal("SECURITY DEFECT: Edge connection with rogue client certificate was accepted by mTLS server!")
	}
	t.Logf("PASS: Edge connection with untrusted rogue certificate rejected by mTLS: %v", err)
}

// Ensure unused imports don't trigger compile errors
var _ = tls.VersionTLS13
var _ = cache.IsConnected
var _ = fmt.Sprintf
