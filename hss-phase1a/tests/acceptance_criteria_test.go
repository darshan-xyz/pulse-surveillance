package tests

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/api"
	"hss/internal/cloud/cache"
	"hss/internal/cloud/db"
	"hss/internal/cloud/signaling"
	"hss/internal/security"
	"hss/internal/webrtc"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
	"golang.org/x/time/rate"
)

// setupAcceptanceTestServer creates a test server with all security and signaling routes
func setupAcceptanceTestServer(t *testing.T, wsLimiterRate float64, wsLimiterBurst int) (*httptest.Server, string, *security.IPRateLimiter, *security.IPRateLimiter) {
	mux := http.NewServeMux()

	loginLimiter := security.NewIPRateLimiter(rate.Limit(5.0), 5)
	wsLimiter := security.NewIPRateLimiter(rate.Limit(wsLimiterRate), wsLimiterBurst)

	loginHandler := security.RateLimitMiddleware(loginLimiter, api.HandleLogin)
	mux.HandleFunc("/api/login", loginHandler)
	mux.HandleFunc("/api/auth/login", loginHandler)
	mux.HandleFunc("/api/auth/refresh", api.HandleRefresh)
	mux.HandleFunc("/api/auth/revoke", api.HandleRevoke)
	mux.HandleFunc("/api/turn/credentials", api.HandleTurnCredentials)
	mux.HandleFunc("/api/devices", api.HandleDevices)
	mux.HandleFunc("/api/push/register", api.HandlePushRegister)

	mux.HandleFunc("/signaling", security.RateLimitMiddleware(wsLimiter, signaling.HandleDeviceConnection))
	mux.HandleFunc("/client", security.RateLimitMiddleware(wsLimiter, signaling.HandleViewerConnection))

	server := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	return server, wsURL, loginLimiter, wsLimiter
}

// 1. TestABR_Congestion_ResponseTime
// Automated test verifying that the ABR engine successfully responds to bandwidth drops
// within 1 second and reduces packet rate >= 50% while preserving 100% keyframes.
func TestABR_Congestion_ResponseTime(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping ABR response time test in short mode")
	}

	abr := webrtc.NewABRController("acceptance-abr-sess-01", webrtc.DefaultBitrate)
	if abr.GetQuality() != webrtc.QualityHigh {
		t.Fatalf("Expected initial quality QualityHigh, got: %s", abr.GetQuality())
	}

	// 1A. Prepare H.264 Test RTP Packets
	// Keyframes: SPS (0x67), PPS (0x68), IDR (0x65)
	spsPayload := []byte{0x67, 0x42, 0x00, 0x1f, 0x96, 0x54, 0x05}
	ppsPayload := []byte{0x68, 0xce, 0x3c, 0x80}
	idrPayload := []byte{0x65, 0x88, 0x84, 0x00, 0x10, 0xff, 0xfe, 0x12, 0x34}
	// P-frame: Non-IDR (0x41)
	pFramePayload := []byte{0x41, 0x9a, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06}

	// 1B. Measure Baseline (High Quality / 30fps)
	const baselineFrames = 60
	var baselineForwarded int
	var baselineKeyframesIn, baselineKeyframesOut int

	for i := 0; i < baselineFrames; i++ {
		var payload []byte
		isKey := (i%15 == 0) // Keyframe every 15 frames
		if isKey {
			payload = idrPayload
			baselineKeyframesIn++
		} else {
			payload = pFramePayload
		}

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: uint16(i + 1),
				Timestamp:      uint32(i * 3000),
				SSRC:           0x12345678,
			},
			Payload: payload,
		}

		outPkt, forwarded := abr.ProcessPacket(pkt)
		if forwarded {
			baselineForwarded++
			if isKey {
				baselineKeyframesOut++
			}
			if outPkt == nil {
				t.Fatal("Forwarded packet was nil")
			}
		}
	}

	if baselineForwarded != baselineFrames {
		t.Fatalf("Expected 100%% baseline forwarding (%d), got: %d", baselineFrames, baselineForwarded)
	}
	if baselineKeyframesOut != baselineKeyframesIn {
		t.Fatalf("Expected all %d baseline keyframes forwarded, got: %d", baselineKeyframesIn, baselineKeyframesOut)
	}
	t.Logf("Baseline verified: %d/%d packets forwarded (100%% throughput at %s)",
		baselineForwarded, baselineFrames, abr.GetQuality())

	// 1C. Simulate 5G Bandwidth Drop with RTCP REMB (180,000 bps -> Emergency/Throttled)
	dropTriggerTime := time.Now()

	// Channel to capture async quality transition callback
	qualityChanged := make(chan webrtc.QualityMode, 10)
	abr.SetOnQualityChange(func(oldMode, newMode webrtc.QualityMode, bitrate float64) {
		select {
		case qualityChanged <- newMode:
		default:
		}
	})

	// Trigger bandwidth drop via REMB
	abr.HandleREMB(180_000.0) // 180 kbps triggers QualityEmergency / rate reduction
	adaptationLatency := time.Since(dropTriggerTime)

	if adaptationLatency > 1000*time.Millisecond {
		t.Fatalf("FAILURE: ABR engine took %v to respond to bandwidth drop (Limit: <= 1.0s)", adaptationLatency)
	}
	t.Logf("SUCCESS: ABR engine responded to bandwidth drop in %v (Threshold: <= 1.0s)", adaptationLatency)

	currentQuality := abr.GetQuality()
	if currentQuality != webrtc.QualityEmergency && currentQuality != webrtc.QualityThrottled {
		t.Fatalf("Expected quality transition to Throttled or Emergency, got: %s", currentQuality)
	}

	// 1D. Process video stream during congestion window
	const congestionFrames = 60
	var congestionForwarded int
	var congestionKeyframesIn, congestionKeyframesOut int
	var lastSeqNum uint16
	var firstSeq bool = true

	for i := 0; i < congestionFrames; i++ {
		frameIdx := baselineFrames + i
		var payload []byte
		isKey := (i%15 == 0) // Include keyframes regularly
		if isKey {
			if i == 0 {
				payload = spsPayload
			} else if i == 15 {
				payload = ppsPayload
			} else {
				payload = idrPayload
			}
			congestionKeyframesIn++
		} else {
			payload = pFramePayload
		}

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: uint16(frameIdx + 1),
				Timestamp:      uint32(frameIdx * 3000),
				SSRC:           0x12345678,
			},
			Payload: payload,
		}

		outPkt, forwarded := abr.ProcessPacket(pkt)
		if forwarded {
			congestionForwarded++
			if isKey {
				congestionKeyframesOut++
			}
			// Verify sequence number monotonicity
			if firstSeq {
				lastSeqNum = outPkt.SequenceNumber
				firstSeq = false
			} else {
				if outPkt.SequenceNumber != lastSeqNum+1 {
					t.Fatalf("Sequence number gap detected: expected %d, got %d", lastSeqNum+1, outPkt.SequenceNumber)
				}
				lastSeqNum = outPkt.SequenceNumber
			}
		}
	}

	// 1E. Verify Acceptance Criteria:
	// - Packet rate reduction >= 50%
	packetReduction := float64(congestionFrames-congestionForwarded) / float64(congestionFrames)
	t.Logf("Congestion Results: In=%d, Out=%d, KeyframesIn=%d, KeyframesOut=%d, RateReduction=%.1f%%",
		congestionFrames, congestionForwarded, congestionKeyframesIn, congestionKeyframesOut, packetReduction*100.0)

	if packetReduction < 0.50 {
		t.Fatalf("FAILURE: Rate reduction %.1f%% did not meet >= 50.0%% requirement (In: %d, Out: %d)",
			packetReduction*100.0, congestionFrames, congestionForwarded)
	}
	t.Logf("SUCCESS: ABR packet rate reduced by %.1f%% (>= 50%% requirement met)", packetReduction*100.0)

	// - 100% keyframes preserved
	if congestionKeyframesOut != congestionKeyframesIn {
		t.Fatalf("FAILURE: Keyframes were dropped during congestion! In: %d, Out: %d (Preservation: %.1f%%)",
			congestionKeyframesIn, congestionKeyframesOut, float64(congestionKeyframesOut)/float64(congestionKeyframesIn)*100.0)
	}
	t.Logf("SUCCESS: 100%% of keyframes preserved (%d/%d keyframes forwarded with zero loss)",
		congestionKeyframesOut, congestionKeyframesIn)

	// 1F. Verify Recovery upon REMB Bandwidth Restoration (3.0 Mbps)
	abr.HandleREMB(3_000_000.0)
	if abr.GetQuality() != webrtc.QualityHigh {
		t.Fatalf("Expected recovery to QualityHigh, got: %s", abr.GetQuality())
	}
	t.Log("SUCCESS: ABR engine cleanly recovered to QualityHigh upon bandwidth restoration")
}

// 2. TestWebSocket_RateLimiter_FloodRejection
// Automated test verifying that the WebSocket rate limiter rejects flood attacks
// with HTTP 429 Too Many Requests status.
func TestWebSocket_RateLimiter_FloodRejection(t *testing.T) {
	// Configure test server with strict limiter (rate 5.0, burst 5)
	server, wsURL, loginLimiter, wsLimiter := setupAcceptanceTestServer(t, 5.0, 5)
	defer server.Close()
	defer loginLimiter.Close()
	defer wsLimiter.Close()
	_ = wsURL

	// 2A. Test WebSocket Handshake Flood Attack against /client endpoint
	const floodAttempts = 20
	const clientIP = "203.0.113.42"
	var got429OnWSHandshake bool
	var first429Index int

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	for i := 1; i <= floodAttempts; i++ {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/client?token=admin", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("X-Forwarded-For", clientIP)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			got429OnWSHandshake = true
			first429Index = i

			retryAfter := resp.Header.Get("Retry-After")
			if retryAfter == "" {
				t.Fatal("Expected 'Retry-After' header on HTTP 429 response")
			}

			var errBody map[string]string
			json.NewDecoder(resp.Body).Decode(&errBody)
			resp.Body.Close()

			if errBody["error"] != "Too Many Requests" {
				t.Fatalf("Expected error 'Too Many Requests', got: %v", errBody)
			}
			t.Logf("PASS: WebSocket flood attempt #%d cleanly rejected with HTTP 429 (Retry-After: %s)", i, retryAfter)
			break
		}
		resp.Body.Close()
	}

	if !got429OnWSHandshake {
		t.Fatalf("SECURITY VULNERABILITY: Rate limiter failed to reject WebSocket flood attacks with HTTP 429!")
	}
	t.Logf("SUCCESS: WebSocket Rate Limiter successfully rejected flood attack at request #%d with HTTP 429", first429Index)

	// 2B. Test In-Connection WebSocket Message Flood Limiting (WSMessageLimiter)
	msgLimiter := security.NewWSMessageLimiter(10.0, 5)
	var allowedCount int
	var rejectedCount int

	for i := 0; i < 20; i++ {
		if msgLimiter.Allow() {
			allowedCount++
		} else {
			rejectedCount++
		}
	}

	if allowedCount != 5 || rejectedCount != 15 {
		t.Fatalf("Expected 5 allowed and 15 rejected for burst=5 limiter, got allowed=%d, rejected=%d",
			allowedCount, rejectedCount)
	}
	t.Logf("SUCCESS: WSMessageLimiter enforced burst bounds (Allowed: %d, Throttled: %d)", allowedCount, rejectedCount)
}

// 3. TestE2EE_SFrame_VideoEncryption
// Automated test verifying that E2EE SFrame encrypts and decrypts video NALUs successfully
// without corrupting the video stream, and intermediate hub only receives ciphertext.
func TestE2EE_SFrame_VideoEncryption(t *testing.T) {
	// Establish shared session key between Edge Camera and Viewer Client
	sessionKey, err := security.GenerateSFrameKey()
	if err != nil {
		t.Fatalf("Failed to generate SFrame key: %v", err)
	}
	sessionSalt, err := security.GenerateSFrameSalt()
	if err != nil {
		t.Fatalf("Failed to generate SFrame salt: %v", err)
	}

	edgeCipher, err := security.NewSFrameCipher(1, sessionKey, sessionSalt)
	if err != nil {
		t.Fatalf("Failed to create Edge cipher: %v", err)
	}

	clientCipher, err := security.NewSFrameCipher(1, sessionKey, sessionSalt)
	if err != nil {
		t.Fatalf("Failed to create Client cipher: %v", err)
	}

	// Rogue intermediate hub or unauthorized third-party
	rogueKey, _ := security.GenerateSFrameKey()
	rogueCipher, _ := security.NewSFrameCipher(1, rogueKey, sessionSalt)

	// 3A. Prepare Real H.264 NALUs (SPS, PPS, IDR, P-frame)
	rawNALUs := [][]byte{
		{0x67, 0x42, 0x00, 0x1f, 0x96, 0x54, 0x05, 0x01, 0xe0},                         // SPS
		{0x68, 0xce, 0x3c, 0x80},                                                       // PPS
		{0x65, 0x88, 0x84, 0x00, 0x10, 0xff, 0xfe, 0xf6, 0xf0, 0xaa, 0xbb, 0xcc, 0xdd}, // IDR Keyframe
		{0x41, 0x9a, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},                   // P-frame
	}

	// 3B. Edge Encrypts NALUs before RTP packetization
	encryptedFrames, err := edgeCipher.EncryptNALUs(rawNALUs, 1000)
	if err != nil {
		t.Fatalf("Edge SFrame encryption failed: %v", err)
	}

	if len(encryptedFrames) != len(rawNALUs) {
		t.Fatalf("Expected %d encrypted frames, got: %d", len(rawNALUs), len(encryptedFrames))
	}

	// 3C. Intermediate Hub Inspection: Verify Hub Sees ONLY Ciphertext
	for i, encFrame := range encryptedFrames {
		// Verify ciphertext does not contain plaintext NALU bytes
		if bytes.Contains(encFrame, rawNALUs[i]) {
			t.Fatalf("SECURITY VULNERABILITY: Frame %d contains raw plaintext video NALU!", i)
		}

		// Verify header format (KeyID & Counter)
		if len(encFrame) <= 9+16 { // 9-byte header + 16-byte GCM tag
			t.Fatalf("Encrypted frame %d is too short: %d bytes", i, len(encFrame))
		}

		// Verify rogue intermediate hub cannot decrypt without key
		_, rogueErr := rogueCipher.DecryptFrame(encFrame)
		if rogueErr == nil {
			t.Fatalf("SECURITY VULNERABILITY: Rogue intermediate node decrypted video ciphertext without key!")
		}
	}
	t.Log("SUCCESS: Intermediate hub receives ONLY ciphertext and cannot inspect video content (Zero-Trust)")

	// 3D. Tamper Detection Test (Flipping bit in ciphertext)
	tamperedFrame := make([]byte, len(encryptedFrames[2]))
	copy(tamperedFrame, encryptedFrames[2])
	tamperedFrame[len(tamperedFrame)-1] ^= 0xFF // Corrupt authentication tag

	_, tamperErr := clientCipher.DecryptFrame(tamperedFrame)
	if tamperErr == nil {
		t.Fatal("SECURITY DEFECT: Client accepted tampered SFrame ciphertext without error!")
	}
	t.Logf("SUCCESS: SFrame AEAD correctly rejected tampered ciphertext: %v", tamperErr)

	// 3E. Authorized Client Decrypts SFrame Stream
	decryptedNALUs, err := clientCipher.DecryptNALUs(encryptedFrames)
	if err != nil {
		t.Fatalf("Client SFrame decryption failed: %v", err)
	}

	if len(decryptedNALUs) != len(rawNALUs) {
		t.Fatalf("Expected %d decrypted NALUs, got: %d", len(rawNALUs), len(decryptedNALUs))
	}

	for i := range rawNALUs {
		if !bytes.Equal(decryptedNALUs[i], rawNALUs[i]) {
			t.Fatalf("CORRUPTION: Decrypted NALU %d does not match original plaintext!", i)
		}
	}
	t.Log("SUCCESS: E2EE SFrame decrypted all H.264 video NALUs with 100% byte-exact fidelity (Zero Corruption)")
}

// 4. TestMTLS_EdgeClientAuthentication
// Automated test verifying that the signaling hub rejects edge clients
// that lack a valid mTLS certificate.
func TestMTLS_EdgeClientAuthentication(t *testing.T) {
	// Generate genuine CA, Server, Client, and Rogue certificates
	creds, err := security.GenerateMTLSCredentials()
	if err != nil {
		t.Fatalf("Failed to generate mTLS credentials: %v", err)
	}

	serverTLS, err := security.NewServerMTLSConfig(creds.CACertPEM, creds.ServerCertPEM, creds.ServerKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create server mTLS config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)

	mtlsServer := httptest.NewUnstartedServer(mux)
	mtlsServer.TLS = serverTLS
	mtlsServer.StartTLS()
	defer mtlsServer.Close()

	wssURL := "wss" + strings.TrimPrefix(mtlsServer.URL, "https")

	// 4A. Valid Edge Client with trusted certificate connects successfully
	validClientTLS, err := security.NewClientMTLSConfig(creds.CACertPEM, creds.ClientCertPEM, creds.ClientKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create valid client TLS config: %v", err)
	}

	validDialer := &websocket.Dialer{
		TLSClientConfig: validClientTLS,
	}

	validConn, _, err := validDialer.Dial(wssURL+"/signaling?camera_id=cam-mtls-verified&token=mock_device_jwt", nil)
	if err != nil {
		t.Fatalf("Valid mTLS Edge connection unexpectedly failed: %v", err)
	}
	validConn.Close()
	t.Log("SUCCESS: Edge client with valid mTLS certificate authenticated and connected successfully")

	// 4B. Edge Client WITHOUT Certificate is rejected by TLS Handshake
	noCertClientTLS, err := security.NewClientMTLSConfig(creds.CACertPEM, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create client TLS config: %v", err)
	}

	noCertDialer := &websocket.Dialer{
		TLSClientConfig: noCertClientTLS,
	}

	_, _, err = noCertDialer.Dial(wssURL+"/signaling?camera_id=cam-mtls-unverified&token=mock_device_jwt", nil)
	if err == nil {
		t.Fatal("SECURITY VULNERABILITY: Edge client without client certificate was accepted by mTLS server!")
	}
	t.Logf("SUCCESS: Edge client without certificate was correctly rejected: %v", err)

	// 4C. Edge Client with UNTRUSTED ROGUE Certificate is rejected
	rogueClientTLS, err := security.NewClientMTLSConfig(creds.CACertPEM, creds.UntrustedClientPEM, creds.UntrustedKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create rogue client TLS config: %v", err)
	}

	rogueDialer := &websocket.Dialer{
		TLSClientConfig: rogueClientTLS,
	}

	_, _, err = rogueDialer.Dial(wssURL+"/signaling?camera_id=cam-mtls-rogue&token=mock_device_jwt", nil)
	if err == nil {
		t.Fatal("SECURITY VULNERABILITY: Edge client with rogue untrusted certificate was accepted by mTLS server!")
	}
	t.Logf("SUCCESS: Edge client with rogue certificate was correctly rejected: %v", err)
}

// 5. TestDynamicTURN_Credentials
// Automated test verifying RFC 5766 HMAC-SHA1 dynamic TURN credential generation and validation.
func TestDynamicTURN_Credentials(t *testing.T) {
	secret := security.GetDefaultTURNSecret()
	username := "test_operator_01"
	ttl := 1 * time.Hour
	turnServer := "turn:coturn.pulse.internal:3478"

	// 5A. Generate Dynamic TURN Credentials
	creds, err := security.GenerateTURNCredentials(secret, username, ttl, turnServer)
	if err != nil {
		t.Fatalf("Failed to generate TURN credentials: %v", err)
	}

	if creds.Username == "" || creds.Password == "" {
		t.Fatalf("Generated empty TURN credentials: %+v", creds)
	}
	if creds.TurnServer != turnServer {
		t.Fatalf("Expected server %s, got: %s", turnServer, creds.TurnServer)
	}
	if !strings.HasSuffix(creds.Username, ":"+username) {
		t.Fatalf("Expected dynamic username format '<timestamp>:%s', got: %s", username, creds.Username)
	}

	// 5B. Verify Valid Credentials
	valid, err := security.VerifyTURNCredentials(secret, creds.Username, creds.Password)
	if err != nil || !valid {
		t.Fatalf("Valid TURN credentials failed verification: valid=%v, err=%v", valid, err)
	}
	t.Logf("SUCCESS: Dynamic TURN credentials verified successfully for user %s (%s)", username, creds.Username)

	// 5C. Verify Expired Credentials Rejection
	expiredCreds, err := security.GenerateTURNCredentials(secret, username, -10*time.Minute, turnServer)
	if err != nil {
		t.Fatalf("Generate expired credentials failed: %v", err)
	}

	expiredValid, expiredErr := security.VerifyTURNCredentials(secret, expiredCreds.Username, expiredCreds.Password)
	if expiredValid || expiredErr == nil {
		t.Fatal("SECURITY DEFECT: Expired TURN credentials were treated as valid!")
	}
	t.Logf("SUCCESS: Expired TURN credentials correctly rejected: %v", expiredErr)

	// 5D. Verify Tampered Signature Rejection
	tamperedValid, tamperedErr := security.VerifyTURNCredentials(secret, creds.Username, "TAMPERED_BASE64_PASSWORD==")
	if tamperedValid || tamperedErr == nil {
		t.Fatal("SECURITY DEFECT: Tampered TURN password signature was accepted!")
	}
	t.Logf("SUCCESS: Tampered TURN password signature correctly rejected: %v", tamperedErr)

	// 5E. Verify REST API Endpoint GET /api/turn/credentials
	server, _, loginLimiter, wsLimiter := setupAcceptanceTestServer(t, 60.0, 100)
	defer server.Close()
	defer loginLimiter.Close()
	defer wsLimiter.Close()

	// Login to get access token
	loginPayload, _ := json.Marshal(map[string]string{"username": "admin", "password": "password"})
	loginResp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(loginPayload))
	if err != nil || loginResp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed: %v", err)
	}
	var auth api.LoginResponse
	json.NewDecoder(loginResp.Body).Decode(&auth)
	loginResp.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/turn/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	turnResp, err := http.DefaultClient.Do(req)
	if err != nil || turnResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/turn/credentials failed: status %d, err: %v", turnResp.StatusCode, err)
	}

	var apiCreds security.TURNCredentials
	json.NewDecoder(turnResp.Body).Decode(&apiCreds)
	turnResp.Body.Close()

	apiValid, apiErr := security.VerifyTURNCredentials(secret, apiCreds.Username, apiCreds.Password)
	if !apiValid || apiErr != nil {
		t.Fatalf("TURN credentials from REST API failed verification: valid=%v, err=%v", apiValid, apiErr)
	}
	t.Logf("SUCCESS: REST API /api/turn/credentials returned valid dynamic credentials: %s", apiCreds.Username)
}

// 6. TestIAM_TokenRotationAndRBAC
// Automated test verifying Access/Refresh token rotation, revocation blacklist,
// and RBAC role enforcement.
func TestIAM_TokenRotationAndRBAC(t *testing.T) {
	server, _, loginLimiter, wsLimiter := setupAcceptanceTestServer(t, 60.0, 100)
	defer server.Close()
	defer loginLimiter.Close()
	defer wsLimiter.Close()

	// 6A. Authenticate and verify Short-Lived Access Token + Refresh Token
	loginPayload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "password",
	})
	resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(loginPayload))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed: status %d, err: %v", resp.StatusCode, err)
	}

	var authResp api.LoginResponse
	json.NewDecoder(resp.Body).Decode(&authResp)
	resp.Body.Close()

	if authResp.AccessToken == "" || authResp.RefreshToken == "" {
		t.Fatalf("Expected AccessToken and RefreshToken, got: %+v", authResp)
	}
	if authResp.ExpiresIn != 900 { // 15 minutes = 900s
		t.Fatalf("Expected 900s AccessToken duration, got: %d", authResp.ExpiresIn)
	}

	// 6B. Access protected resource with Access Token
	devReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	devReq.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	devResp, err := http.DefaultClient.Do(devReq)
	if err != nil || devResp.StatusCode != http.StatusOK {
		t.Fatalf("Authorized GET /api/devices failed: status %d, err: %v", devResp.StatusCode, err)
	}
	devResp.Body.Close()
	t.Log("PASS: Protected resource accessed successfully using valid Access Token")

	// 6C. Execute Token Rotation via /api/auth/refresh
	refreshPayload, _ := json.Marshal(map[string]string{
		"refresh_token": authResp.RefreshToken,
	})
	refResp, err := http.Post(server.URL+"/api/auth/refresh", "application/json", bytes.NewBuffer(refreshPayload))
	if err != nil || refResp.StatusCode != http.StatusOK {
		t.Fatalf("Token refresh failed: status %d, err: %v", refResp.StatusCode, err)
	}
	var newAuth api.LoginResponse
	json.NewDecoder(refResp.Body).Decode(&newAuth)
	refResp.Body.Close()

	if newAuth.AccessToken == "" {
		t.Fatal("Expected new Access Token upon rotation")
	}
	t.Log("PASS: Access Token successfully rotated using Refresh Token")

	// 6D. Revoke Tokens via /api/auth/revoke (Blacklist JTI)
	revokePayload, _ := json.Marshal(map[string]string{
		"token":         authResp.AccessToken,
		"refresh_token": authResp.RefreshToken,
	})
	revResp, err := http.Post(server.URL+"/api/auth/revoke", "application/json", bytes.NewBuffer(revokePayload))
	if err != nil || revResp.StatusCode != http.StatusOK {
		t.Fatalf("Token revocation failed: status %d, err: %v", revResp.StatusCode, err)
	}
	revResp.Body.Close()
	t.Log("PASS: Tokens revoked and blacklisted successfully")

	// 6E. Assert Revoked Access Token is Rejected with HTTP 401
	revReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	revReq.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	revDevResp, err := http.DefaultClient.Do(revReq)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer revDevResp.Body.Close()

	if revDevResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("SECURITY VULNERABILITY: Revoked Access Token was accepted! Status: %d", revDevResp.StatusCode)
	}
	t.Log("PASS: Revoked Access Token correctly rejected with HTTP 401 by Blacklist")

	// 6F. Assert Revoked Refresh Token is Rejected on /api/auth/refresh
	refResp2, err := http.Post(server.URL+"/api/auth/refresh", "application/json", bytes.NewBuffer(refreshPayload))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer refResp2.Body.Close()

	if refResp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("SECURITY VULNERABILITY: Revoked Refresh Token was accepted! Status: %d", refResp2.StatusCode)
	}
	t.Log("PASS: Revoked Refresh Token correctly rejected with HTTP 401")

	// 6G. Verify Role-Based Access Control (RBAC) Matrix
	// 1. Admin has all permissions
	if !security.RoleHasPermission(security.RoleAdmin, security.PermDeviceControl) ||
		!security.RoleHasPermission(security.RoleAdmin, security.PermDeviceManageUsers) ||
		!security.RoleHasPermission(security.RoleAdmin, security.PermStreamAccess) {
		t.Fatalf("Admin role missing expected permissions")
	}

	// 2. Operator has Device:Control and Stream:Access, but NOT Device:ManageUsers
	if !security.RoleHasPermission(security.RoleOperator, security.PermDeviceControl) ||
		!security.RoleHasPermission(security.RoleOperator, security.PermStreamAccess) {
		t.Fatalf("Operator role missing expected permissions")
	}
	if security.RoleHasPermission(security.RoleOperator, security.PermDeviceManageUsers) {
		t.Fatalf("SECURITY DEFECT: Operator has unauthorized PermDeviceManageUsers!")
	}

	// 3. Viewer has Device:View and Stream:Access, but NOT Device:Control or Device:ManageUsers
	if !security.RoleHasPermission(security.RoleViewer, security.PermDeviceView) ||
		!security.RoleHasPermission(security.RoleViewer, security.PermStreamAccess) {
		t.Fatalf("Viewer role missing expected permissions")
	}
	if security.RoleHasPermission(security.RoleViewer, security.PermDeviceControl) ||
		security.RoleHasPermission(security.RoleViewer, security.PermDeviceManageUsers) {
		t.Fatalf("SECURITY DEFECT: Viewer role has unauthorized Control/ManageUsers permissions!")
	}

	// 4. Verify DB/Fallback permission evaluation
	if !db.VerifyDevicePermission("admin", "test-camera-1", security.PermStreamAccess) {
		t.Fatal("Expected admin to have stream access")
	}

	t.Log("SUCCESS: IAM Token Rotation, Revocation Blacklist, and RBAC Role Enforcement 100% verified")
}

// Suppress unused imports
var _ = fmt.Sprintf
var _ = tls.VersionTLS13
var _ = atomic.AddInt64
var _ = pionwebrtc.Configuration{}
var _ = jwt.ErrSignatureInvalid
var _ = cache.IsConnected
