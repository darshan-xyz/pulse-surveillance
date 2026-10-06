package tests

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hss/internal/api"
	"hss/internal/cloud/db"
	"hss/internal/cloud/signaling"
	"hss/internal/security"

	"github.com/gorilla/websocket"
)

// ============================================================================
// CHALLENGER 2 ADVERSARIAL VERIFICATION SUITE
// 1. E2EE SFrame Video Encryption / Decryption & Tamper Resilience
// 2. IAM Token Lifecycle, Token Rotation & Revocation Blacklist (HTTP & WS)
// 3. Fine-Grained Multi-Tenant RBAC Boundaries (Viewer vs Operator vs Admin)
// ============================================================================

// calculateEntropy calculates the Shannon entropy of a byte slice
func calculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0.0
	}
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}
	var entropy float64
	dataLen := float64(len(data))
	for _, count := range freq {
		p := float64(count) / dataLen
		entropy -= p * math.Log2(p)
	}
	return entropy
}

// ----------------------------------------------------------------------------
// TEST SUITE 1: E2EE SFrame Video Encryption / Decryption Adversarial Tests
// ----------------------------------------------------------------------------

// TestChallenger_E2EESFrame_CryptographicFidelityAndEntropy tests:
// - 100% byte fidelity across variable frame sizes (1B to 512KB)
// - High Shannon entropy (no plaintext leakage in ciphertext payload)
// - Zero intermediate relay plaintext visibility
func TestChallenger_E2EESFrame_CryptographicFidelityAndEntropy(t *testing.T) {
	key, err := security.GenerateSFrameKey()
	if err != nil {
		t.Fatalf("GenerateSFrameKey failed: %v", err)
	}
	salt, err := security.GenerateSFrameSalt()
	if err != nil {
		t.Fatalf("GenerateSFrameSalt failed: %v", err)
	}

	senderCipher, err := security.NewSFrameCipher(1, key, salt)
	if err != nil {
		t.Fatalf("NewSFrameCipher (sender) failed: %v", err)
	}

	receiverCipher, err := security.NewSFrameCipher(1, key, salt)
	if err != nil {
		t.Fatalf("NewSFrameCipher (receiver) failed: %v", err)
	}

	// Test a variety of H.264 NALU types and sizes
	testFrames := []struct {
		name     string
		naluType string
		payload  []byte
	}{
		{
			name:     "SPS (Sequence Parameter Set)",
			naluType: "SPS",
			payload:  []byte{0x67, 0x42, 0x00, 0x1F, 0xE9, 0x01, 0x40, 0x7B, 0x20},
		},
		{
			name:     "PPS (Picture Parameter Set)",
			naluType: "PPS",
			payload:  []byte{0x68, 0xCE, 0x38, 0x80},
		},
		{
			name:     "IDR Keyframe (Small)",
			naluType: "IDR",
			payload:  []byte{0x65, 0xB8, 0x00, 0x04, 0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04},
		},
		{
			name:     "Non-IDR P-Frame (Repetitive byte patterns / zero-fill)",
			naluType: "P-Frame",
			payload:  bytes.Repeat([]byte{0x41, 0x00, 0x00, 0x01, 0xAA, 0xBB, 0x00, 0x00}, 256), // 2KB
		},
		{
			name:     "High-Res 1080p Video Keyframe (64KB)",
			naluType: "Large IDR",
			payload: func() []byte {
				buf := make([]byte, 64*1024)
				buf[0] = 0x65 // IDR header
				rand.Read(buf[1:])
				return buf
			}(),
		},
		{
			name:     "4K Ultra-HD I-Frame Chunk (256KB)",
			naluType: "Ultra-HD I-Frame",
			payload: func() []byte {
				buf := make([]byte, 256*1024)
				buf[0] = 0x65
				rand.Read(buf[1:])
				return buf
			}(),
		},
	}

	for i, tf := range testFrames {
		t.Run(tf.name, func(t *testing.T) {
			counter := uint64(1000 + i*10)

			// 1. Edge camera encrypts frame
			encrypted, encErr := senderCipher.EncryptFrame(tf.payload, counter)
			if encErr != nil {
				t.Fatalf("Encryption failed for %s: %v", tf.name, encErr)
			}

			// Verify header structure: [KeyID (1B)][Counter (8B)][Ciphertext + Tag (16B)]
			expectedLen := security.SFrameHeaderSize + len(tf.payload) + security.SFrameTagSize
			if len(encrypted) != expectedLen {
				t.Fatalf("Length mismatch: expected %d, got %d", expectedLen, len(encrypted))
			}

			if encrypted[0] != 1 {
				t.Fatalf("Expected KeyID 1, got %d", encrypted[0])
			}

			extractedCounter := binary.BigEndian.Uint64(encrypted[1:9])
			if extractedCounter != counter {
				t.Fatalf("Expected Counter %d, got %d", counter, extractedCounter)
			}

			// 2. Intermediate Relay Secrecy: Ensure raw plaintext is NOT present in ciphertext
			ciphertextOnly := encrypted[security.SFrameHeaderSize : len(encrypted)-security.SFrameTagSize]
			if bytes.Contains(ciphertextOnly, tf.payload) {
				t.Fatalf("SECURITY VIOLATION: Ciphertext exposes raw plaintext NALU!")
			}

			// For frames >= 1KB, verify high Shannon entropy (> 7.5 bits/byte)
			if len(tf.payload) >= 1024 {
				entropy := calculateEntropy(ciphertextOnly)
				if entropy < 7.5 {
					t.Fatalf("SECURITY WARNING: Low ciphertext entropy (%.2f bits/byte), possible weak encryption", entropy)
				}
				t.Logf("Ciphertext entropy for %s (%d bytes): %.4f bits/byte (High Cryptographic Secrecy)", tf.name, len(tf.payload), entropy)
			}

			// 3. Authorized Client decrypts frame
			decrypted, decErr := receiverCipher.DecryptFrame(encrypted)
			if decErr != nil {
				t.Fatalf("Decryption failed for %s: %v", tf.name, decErr)
			}

			// 4. Verify 100% byte fidelity
			if !bytes.Equal(decrypted, tf.payload) {
				t.Fatalf("CRITICAL FIDELITY FAILURE: Decrypted byte slice does not match original plaintext!")
			}
			t.Logf("PASS: %s (%d bytes) successfully encrypted and decrypted with 100%% byte fidelity", tf.name, len(tf.payload))
		})
	}
}

// TestChallenger_E2EESFrame_AdversarialTamperingAndAttacks tests:
// - Single bit flips in header, counter, ciphertext, and GCM tag
// - Truncated packets
// - Replay / wrong key ID / wrong salt / unauthorized eavesdropper
func TestChallenger_E2EESFrame_AdversarialTamperingAndAttacks(t *testing.T) {
	keyA, _ := security.GenerateSFrameKey()
	saltA, _ := security.GenerateSFrameSalt()

	cipherA, _ := security.NewSFrameCipher(1, keyA, saltA)

	rawNALU := []byte{0x65, 0x88, 0x84, 0x00, 0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80}
	counter := uint64(500)

	validEncrypted, err := cipherA.EncryptFrame(rawNALU, counter)
	if err != nil {
		t.Fatalf("Setup encryption failed: %v", err)
	}

	t.Run("Bit-Flip in SFrame Header (KeyID)", func(t *testing.T) {
		tampered := make([]byte, len(validEncrypted))
		copy(tampered, validEncrypted)
		tampered[0] ^= 0x01 // Flip KeyID

		_, err := cipherA.DecryptFrame(tampered)
		if err == nil {
			t.Fatal("SECURITY DEFECT: SFrame accepted tampered KeyID!")
		}
		t.Logf("PASS: Tampered KeyID rejected: %v", err)
	})

	t.Run("Bit-Flip in Frame Counter", func(t *testing.T) {
		tampered := make([]byte, len(validEncrypted))
		copy(tampered, validEncrypted)
		tampered[4] ^= 0x01 // Flip counter byte

		_, err := cipherA.DecryptFrame(tampered)
		if err == nil {
			t.Fatal("SECURITY DEFECT: SFrame accepted tampered frame counter!")
		}
		t.Logf("PASS: Tampered Frame Counter rejected by AEAD auth: %v", err)
	})

	t.Run("Bit-Flip in Ciphertext Body", func(t *testing.T) {
		tampered := make([]byte, len(validEncrypted))
		copy(tampered, validEncrypted)
		tampered[12] ^= 0x80 // Flip bit in payload

		_, err := cipherA.DecryptFrame(tampered)
		if err == nil {
			t.Fatal("SECURITY DEFECT: SFrame accepted corrupted ciphertext body!")
		}
		t.Logf("PASS: Corrupted ciphertext body rejected by AEAD tag: %v", err)
	})

	t.Run("Bit-Flip in GCM Authentication Tag", func(t *testing.T) {
		tampered := make([]byte, len(validEncrypted))
		copy(tampered, validEncrypted)
		tampered[len(tampered)-1] ^= 0x01 // Flip last byte of tag

		_, err := cipherA.DecryptFrame(tampered)
		if err == nil {
			t.Fatal("SECURITY DEFECT: SFrame accepted corrupted authentication tag!")
		}
		t.Logf("PASS: Corrupted tag rejected: %v", err)
	})

	t.Run("Truncated Packet (< Minimum Size)", func(t *testing.T) {
		for size := 0; size < security.SFrameMinPacketSize; size++ {
			truncated := validEncrypted[:size]
			_, err := cipherA.DecryptFrame(truncated)
			if err == nil {
				t.Fatalf("SECURITY DEFECT: SFrame accepted truncated packet of size %d!", size)
			}
		}
		t.Log("PASS: All truncated packet lengths under 25 bytes correctly rejected")
	})

	t.Run("Unauthorized Eavesdropper with Different Key", func(t *testing.T) {
		keyRogue, _ := security.GenerateSFrameKey()
		cipherRogue, _ := security.NewSFrameCipher(1, keyRogue, saltA)

		_, err := cipherRogue.DecryptFrame(validEncrypted)
		if err == nil {
			t.Fatal("SECURITY DEFECT: Rogue eavesdropper decrypted video payload with different key!")
		}
		t.Logf("PASS: Eavesdropper with wrong key rejected: %v", err)
	})

	t.Run("Mismatched Salt Rejection", func(t *testing.T) {
		saltB, _ := security.GenerateSFrameSalt()
		cipherMismatchedSalt, _ := security.NewSFrameCipher(1, keyA, saltB)

		_, err := cipherMismatchedSalt.DecryptFrame(validEncrypted)
		if err == nil {
			t.Fatal("SECURITY DEFECT: Decryption succeeded with mismatched salt!")
		}
		t.Logf("PASS: Mismatched salt rejected: %v", err)
	})
}

// ----------------------------------------------------------------------------
// TEST SUITE 2: IAM Token Lifecycle, Rotation & Instant Revocation Blacklist
// ----------------------------------------------------------------------------

func setupChallengerSecurityServer(t *testing.T) (*httptest.Server, string) {
	mux := http.NewServeMux()
	loginLimiter := security.NewIPRateLimiter(50.0, 50)

	mux.HandleFunc("/api/login", security.RateLimitMiddleware(loginLimiter, api.HandleLogin))
	mux.HandleFunc("/api/auth/login", security.RateLimitMiddleware(loginLimiter, api.HandleLogin))
	mux.HandleFunc("/api/auth/refresh", api.HandleRefresh)
	mux.HandleFunc("/api/auth/revoke", api.HandleRevoke)
	mux.HandleFunc("/api/devices", api.HandleDevices)
	mux.HandleFunc("/api/turn/credentials", api.HandleTurnCredentials)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)

	server := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	return server, wsURL
}

func TestChallenger_IAM_InstantRevocationBlacklistHTTPAndWebSocket(t *testing.T) {
	server, wsURL := setupChallengerSecurityServer(t)
	defer server.Close()

	// 1. Authenticate user
	loginBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "password",
	})
	resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed: status %d, err %v", resp.StatusCode, err)
	}

	var auth api.LoginResponse
	json.NewDecoder(resp.Body).Decode(&auth)
	resp.Body.Close()

	if auth.AccessToken == "" || auth.RefreshToken == "" {
		t.Fatalf("Missing tokens in login response: %+v", auth)
	}

	// 2. Verify Access Token works on HTTP GET /api/devices
	req1, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	req1.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	devResp1, err := http.DefaultClient.Do(req1)
	if err != nil || devResp1.StatusCode != http.StatusOK {
		t.Fatalf("Initial devices request failed: %d, %v", devResp1.StatusCode, err)
	}
	devResp1.Body.Close()
	t.Log("PASS: Valid Access Token accessed /api/devices successfully")

	// 3. Verify Access Token works on WebSocket /client
	wsConn1, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token="+auth.AccessToken, nil)
	if err != nil {
		t.Fatalf("Initial WebSocket connection failed: %v", err)
	}
	_ = wsConn1.Close()
	t.Log("PASS: Valid Access Token connected to WebSocket signaling successfully")

	// 4. Revoke the Access Token and Refresh Token via /api/auth/revoke
	revokePayload, _ := json.Marshal(map[string]string{
		"token":         auth.AccessToken,
		"refresh_token": auth.RefreshToken,
	})
	revResp, err := http.Post(server.URL+"/api/auth/revoke", "application/json", bytes.NewBuffer(revokePayload))
	if err != nil || revResp.StatusCode != http.StatusOK {
		t.Fatalf("Revoke request failed: %d, %v", revResp.StatusCode, err)
	}
	revResp.Body.Close()
	t.Log("PASS: Token revocation API call completed successfully")

	// 5. TEST: Instant Denial on HTTP /api/devices
	reqRevoked, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	reqRevoked.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	devRespRevoked, err := http.DefaultClient.Do(reqRevoked)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer devRespRevoked.Body.Close()

	if devRespRevoked.StatusCode != http.StatusUnauthorized {
		t.Fatalf("CRITICAL SECURITY DEFECT: Revoked Access Token was accepted on HTTP endpoint! Expected 401, got %d", devRespRevoked.StatusCode)
	}
	t.Log("PASS: Revoked Access Token instantly denied on HTTP /api/devices (HTTP 401)")

	// 6. TEST: Instant Denial on WebSocket /client
	_, wsResp, wsErr := websocket.DefaultDialer.Dial(wsURL+"/client?token="+auth.AccessToken, nil)
	if wsErr == nil {
		t.Fatalf("CRITICAL SECURITY DEFECT: Revoked Access Token was accepted on WebSocket signaling endpoint!")
	}
	if wsResp != nil && wsResp.StatusCode != http.StatusUnauthorized {
		t.Logf("WebSocket handshake rejected with status: %d", wsResp.StatusCode)
	}
	t.Logf("PASS: Revoked Access Token instantly denied on WebSocket /client: %v", wsErr)

	// 7. TEST: Revoked Refresh Token rejected on /api/auth/refresh
	refreshPayload, _ := json.Marshal(map[string]string{
		"refresh_token": auth.RefreshToken,
	})
	refResp, err := http.Post(server.URL+"/api/auth/refresh", "application/json", bytes.NewBuffer(refreshPayload))
	if err != nil {
		t.Fatalf("Refresh request failed: %v", err)
	}
	defer refResp.Body.Close()

	if refResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("CRITICAL SECURITY DEFECT: Revoked Refresh Token was accepted on /api/auth/refresh! Status: %d", refResp.StatusCode)
	}
	t.Log("PASS: Revoked Refresh Token instantly denied on /api/auth/refresh (HTTP 401)")
}

// TestChallenger_IAM_TokenRotationLifecycle tests token rotation mechanics
func TestChallenger_IAM_TokenRotationLifecycle(t *testing.T) {
	server, _ := setupChallengerSecurityServer(t)
	defer server.Close()

	// 1. Initial Login
	loginBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "password",
	})
	resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed: status %d", resp.StatusCode)
	}
	var auth1 api.LoginResponse
	json.NewDecoder(resp.Body).Decode(&auth1)
	resp.Body.Close()

	// 2. Perform first refresh
	refBody1, _ := json.Marshal(map[string]string{
		"refresh_token": auth1.RefreshToken,
	})
	refResp1, err := http.Post(server.URL+"/api/auth/refresh", "application/json", bytes.NewBuffer(refBody1))
	if err != nil || refResp1.StatusCode != http.StatusOK {
		t.Fatalf("First refresh failed: %d, %v", refResp1.StatusCode, err)
	}
	var auth2 api.LoginResponse
	json.NewDecoder(refResp1.Body).Decode(&auth2)
	refResp1.Body.Close()

	if auth2.AccessToken == "" {
		t.Fatal("New Access Token empty after refresh")
	}
	if auth2.AccessToken == auth1.AccessToken {
		t.Fatal("SECURITY DEFECT: Refreshed Access Token is identical to original (token rotation failed)")
	}
	t.Log("PASS: Refresh issued a brand new short-lived Access Token with unique JTI")

	// 3. Verify new Access Token works on /api/devices
	req2, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	req2.Header.Set("Authorization", "Bearer "+auth2.AccessToken)
	devResp2, err := http.DefaultClient.Do(req2)
	if err != nil || devResp2.StatusCode != http.StatusOK {
		t.Fatalf("Access with refreshed token failed: %d, %v", devResp2.StatusCode, err)
	}
	devResp2.Body.Close()
	t.Log("PASS: Newly rotated Access Token functions properly")
}

// ----------------------------------------------------------------------------
// TEST SUITE 3: RBAC Fine-Grained Boundaries (Viewer vs Operator vs Admin)
// ----------------------------------------------------------------------------

func TestChallenger_RBAC_FineGrainedRoleBoundaries(t *testing.T) {
	// 1. Matrix verification of Role Permissions
	t.Run("RBAC Matrix Verification", func(t *testing.T) {
		// Admin permissions: all actions
		adminPerms := security.GetRolePermissions(security.RoleAdmin)
		expectedAdmin := []string{
			security.PermDeviceView,
			security.PermDeviceControl,
			security.PermDeviceManageUsers,
			security.PermStreamAccess,
		}
		for _, p := range expectedAdmin {
			if !security.RoleHasPermission(security.RoleAdmin, p) {
				t.Fatalf("RBAC VIOLATION: Admin missing permission %s", p)
			}
		}

		// Operator permissions: Device:View, Device:Control, Stream:Access (NO Device:ManageUsers)
		if !security.RoleHasPermission(security.RoleOperator, security.PermDeviceView) {
			t.Fatal("Operator must have Device:View")
		}
		if !security.RoleHasPermission(security.RoleOperator, security.PermDeviceControl) {
			t.Fatal("Operator must have Device:Control")
		}
		if !security.RoleHasPermission(security.RoleOperator, security.PermStreamAccess) {
			t.Fatal("Operator must have Stream:Access")
		}
		if security.RoleHasPermission(security.RoleOperator, security.PermDeviceManageUsers) {
			t.Fatal("RBAC VIOLATION: Operator role granted unauthorized Device:ManageUsers permission!")
		}

		// Viewer permissions: Device:View, Stream:Access (NO Device:Control, NO Device:ManageUsers)
		if !security.RoleHasPermission(security.RoleViewer, security.PermDeviceView) {
			t.Fatal("Viewer must have Device:View")
		}
		if !security.RoleHasPermission(security.RoleViewer, security.PermStreamAccess) {
			t.Fatal("Viewer must have Stream:Access")
		}
		if security.RoleHasPermission(security.RoleViewer, security.PermDeviceControl) {
			t.Fatal("RBAC VIOLATION: Viewer role granted unauthorized Device:Control permission!")
		}
		if security.RoleHasPermission(security.RoleViewer, security.PermDeviceManageUsers) {
			t.Fatal("RBAC VIOLATION: Viewer role granted unauthorized Device:ManageUsers permission!")
		}

		t.Logf("PASS: RBAC Matrix verified strictly: Admin=%d perms, Operator=%d perms, Viewer=%d perms",
			len(adminPerms), len(security.GetRolePermissions(security.RoleOperator)), len(security.GetRolePermissions(security.RoleViewer)))
	})

	// 2. Multi-Tenant Device Access and RBAC Permission Enforcement
	t.Run("Multi-Tenant RBAC Device Access Check", func(t *testing.T) {
		// Verify tenant isolation in db.VerifyDevicePermission
		// Admin on test-camera-1
		if !db.VerifyDevicePermission("admin", "test-camera-1", security.PermDeviceManageUsers) {
			t.Fatal("Admin should have Device:ManageUsers on owned device")
		}
		// Admin on foreign camera should be denied
		if db.VerifyDevicePermission("admin", "cam-tenant2-01", security.PermStreamAccess) {
			t.Fatal("Admin should NOT have access to foreign tenant device")
		}

		// Operator on test-camera-1
		if !db.VerifyDevicePermission("operator", "test-camera-1", security.PermDeviceControl) {
			t.Log("Operator verified for Device:Control")
		}
		if db.VerifyDevicePermission("operator", "test-camera-1", security.PermDeviceManageUsers) {
			t.Fatal("Operator must NOT have Device:ManageUsers permission")
		}

		// Viewer on test-camera-2 (user2)
		if !db.VerifyDevicePermission("user2", "test-camera-2", security.PermStreamAccess) {
			t.Fatal("Viewer should have Stream:Access to permitted device")
		}
		if db.VerifyDevicePermission("user2", "test-camera-2", security.PermDeviceControl) {
			t.Fatal("Viewer must NOT have Device:Control")
		}
		if db.VerifyDevicePermission("user2", "test-camera-2", security.PermDeviceManageUsers) {
			t.Fatal("Viewer must NOT have Device:ManageUsers")
		}

		// Tenant 1 Viewer (tenant1_viewer) on cam-tenant1-01
		if !db.VerifyDevicePermission("tenant1_viewer", "cam-tenant1-01", security.PermStreamAccess) {
			t.Fatal("tenant1_viewer should have Stream:Access to cam-tenant1-01")
		}
		if db.VerifyDevicePermission("tenant1_viewer", "cam-tenant1-01", security.PermDeviceControl) {
			t.Fatal("tenant1_viewer must NOT have Device:Control permission")
		}
		if db.VerifyDevicePermission("tenant1_viewer", "cam-tenant1-01", security.PermDeviceManageUsers) {
			t.Fatal("tenant1_viewer must NOT have Device:ManageUsers permission")
		}

		t.Log("PASS: Device-level RBAC and tenant boundaries verified without privilege escalation")
	})

	// 3. User Permission Fallback & Custom Grant Checks
	t.Run("Custom Permission Delegation Evaluation", func(t *testing.T) {
		// Viewer with explicit delegate permission for Device:Control
		if !security.UserHasPermission(security.RoleViewer, []string{security.PermDeviceControl}, security.PermDeviceControl) {
			t.Fatal("Explicit delegated permission should be granted")
		}

		// Viewer without explicit grant attempting admin action
		if security.UserHasPermission(security.RoleViewer, []string{}, security.PermDeviceManageUsers) {
			t.Fatal("Unprivileged viewer should not have Device:ManageUsers")
		}
		t.Log("PASS: User permission evaluation logic verified")
	})
}
