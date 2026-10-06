package chaos_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"hss/internal/api"
	"hss/internal/cloud/db"
	"hss/internal/security"
)

// setupChaosHTTPServer creates a test HTTP server hosting auth, device, and push APIs.
func setupChaosHTTPServer(t *testing.T) (*httptest.Server, string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", api.HandleLogin)
	mux.HandleFunc("/api/auth/login", api.HandleLogin)
	mux.HandleFunc("/api/auth/refresh", api.HandleRefresh)
	mux.HandleFunc("/api/auth/revoke", api.HandleRevoke)
	mux.HandleFunc("/api/devices", api.HandleDevices)
	mux.HandleFunc("/api/push/register", api.HandlePushRegister)

	server := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	return server, wsURL
}

// TestChaosDatabaseOutage_DegradedFallbackAndRecovery simulates total database outage
// during startup and runtime, verifies seamless fallback to degraded mode, and tests recovery.
func TestChaosDatabaseOutage_DegradedFallbackAndRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database chaos test in short mode")
	}

	t.Run("TotalStartupOutage_DegradedMode", func(t *testing.T) {
		// 1. Force complete database disconnection and nil handle
		db.SetDB(nil, false)
		db.ClearAllMemoryStores()

		server, _ := setupChaosHTTPServer(t)
		defer server.Close()

		// 2. Authenticate users in degraded in-memory mode
		testUsers := []struct {
			username string
			password string
			role     string
			tenant   string
		}{
			{"admin", "password", "admin", "tenant-alpha"},
			{"operator", "password", "operator", "tenant-alpha"},
			{"user2", "password", "viewer", "tenant-beta"},
			{"tenant1_user", "password", "admin", "tenant-1"},
			{"tenant2_user", "password", "admin", "tenant-2"},
		}

		userTokens := make(map[string]api.LoginResponse)

		for _, u := range testUsers {
			loginBody, _ := json.Marshal(map[string]string{
				"username": u.username,
				"password": u.password,
			})

			resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(loginBody))
			if err != nil {
				t.Fatalf("[%s] Failed to send login request: %v", u.username, err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("[%s] Expected 200 OK in degraded mode, got: %d", u.username, resp.StatusCode)
			}

			var loginResp api.LoginResponse
			if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
				resp.Body.Close()
				t.Fatalf("[%s] Failed to decode login response: %v", u.username, err)
			}
			resp.Body.Close()

			if loginResp.AccessToken == "" || loginResp.RefreshToken == "" {
				t.Fatalf("[%s] Missing tokens in response: %+v", u.username, loginResp)
			}
			if loginResp.User.Role != u.role {
				t.Errorf("[%s] Expected role %s, got: %s", u.username, u.role, loginResp.User.Role)
			}
			userTokens[u.username] = loginResp
		}

		// 3. Verify Invalid Credentials are still rejected in degraded mode
		invalidBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrongpassword"})
		resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(invalidBody))
		if err != nil {
			t.Fatalf("Failed to send invalid login: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("Expected 401 Unauthorized for invalid password, got: %d", resp.StatusCode)
		}
		resp.Body.Close()

		// 4. Verify /api/devices multi-tenant isolation under degraded mode
		for _, u := range testUsers {
			req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
			req.Header.Set("Authorization", "Bearer "+userTokens[u.username].AccessToken)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("[%s] Devices request failed: %v", u.username, err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("[%s] Expected 200 OK for devices, got: %d", u.username, resp.StatusCode)
			}

			var devices []api.DeviceResponse
			if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
				resp.Body.Close()
				t.Fatalf("[%s] Failed to decode devices: %v", u.username, err)
			}
			resp.Body.Close()

			switch u.username {
			case "admin":
				if len(devices) != 2 || devices[0].DeviceID != "test-camera-1" || devices[1].DeviceID != "webcam-live-01" {
					t.Fatalf("admin expected [test-camera-1, webcam-live-01], got: %+v", devices)
				}
			case "user2":
				if len(devices) != 1 || devices[0].DeviceID != "test-camera-2" {
					t.Fatalf("user2 expected [test-camera-2], got: %+v", devices)
				}
			case "tenant1_user":
				if len(devices) != 2 {
					t.Fatalf("tenant1_user expected 2 devices, got: %d", len(devices))
				}
				for _, d := range devices {
					if !strings.HasPrefix(d.DeviceID, "cam-tenant1-") {
						t.Errorf("tenant1_user leaked other tenant device: %s", d.DeviceID)
					}
				}
			case "tenant2_user":
				if len(devices) != 1 || devices[0].DeviceID != "cam-tenant2-01" {
					t.Fatalf("tenant2_user expected [cam-tenant2-01], got: %+v", devices)
				}
			}
		}

		// 5. Test Token Refresh and Revocation in In-Memory Degraded Mode
		adminTokens := userTokens["admin"]
		refreshBody, _ := json.Marshal(map[string]string{"refresh_token": adminTokens.RefreshToken})
		resp, err = http.Post(server.URL+"/api/auth/refresh", "application/json", bytes.NewBuffer(refreshBody))
		if err != nil {
			t.Fatalf("Refresh request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK for refresh, got: %d", resp.StatusCode)
		}
		var refreshed api.LoginResponse
		_ = json.NewDecoder(resp.Body).Decode(&refreshed)
		resp.Body.Close()

		if refreshed.AccessToken == "" {
			t.Fatal("Expected non-empty refreshed access token")
		}

		// Revoke the refresh token
		revokeBody, _ := json.Marshal(map[string]string{"refresh_token": adminTokens.RefreshToken})
		resp, err = http.Post(server.URL+"/api/auth/revoke", "application/json", bytes.NewBuffer(revokeBody))
		if err != nil {
			t.Fatalf("Revoke request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK for revoke, got: %d", resp.StatusCode)
		}
		resp.Body.Close()

		// Subsequent refresh attempt with revoked token must fail with 401
		resp, err = http.Post(server.URL+"/api/auth/refresh", "application/json", bytes.NewBuffer(refreshBody))
		if err != nil {
			t.Fatalf("Refresh after revoke failed: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("Expected 401 Unauthorized for revoked refresh token, got: %d", resp.StatusCode)
		}
		resp.Body.Close()

		// 6. Test Push Token Registration and Query in Degraded Mode
		pushReqBody, _ := json.Marshal(map[string]string{
			"push_token": "ExponentPushToken[chaos_admin_01]",
			"platform":   "android",
		})
		pushReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/push/register", bytes.NewBuffer(pushReqBody))
		pushReq.Header.Set("Authorization", "Bearer "+refreshed.AccessToken)
		pushReq.Header.Set("Content-Type", "application/json")

		resp, err = http.DefaultClient.Do(pushReq)
		if err != nil {
			t.Fatalf("Push registration failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK for push registration in degraded mode, got: %d", resp.StatusCode)
		}
		resp.Body.Close()

		tokens, err := db.GetPushTokensForUser("admin")
		if err != nil || len(tokens) == 0 {
			t.Fatalf("Failed to retrieve push tokens in degraded mode: tokens=%v, err=%v", tokens, err)
		}

		deviceTokens, err := db.GetPushTokensForDevice("test-camera-1")
		if err != nil || len(deviceTokens) == 0 {
			t.Fatalf("Failed to retrieve device push tokens in degraded mode: tokens=%v, err=%v", deviceTokens, err)
		}

		t.Log("PASS: Total startup outage handled seamlessly in in-memory degraded mode.")
	})

	t.Run("RuntimeDatabaseOutage_MidFlightSeveranceAndRecovery", func(t *testing.T) {
		server, _ := setupChaosHTTPServer(t)
		defer server.Close()

		// Start in connected state
		db.SetDBConnection(true)
		db.ClearAllMemoryStores()

		// Register an initial token
		jti := "chaos-jti-midflight-01"
		exp := time.Now().Add(1 * time.Hour)
		if err := db.StoreRefreshToken(jti, "admin", exp); err != nil {
			t.Fatalf("Failed to store initial refresh token: %v", err)
		}

		// CHAOS EVENT: Sever database connection mid-flight
		t.Log("Simulating Chaos Event: Dropping PostgreSQL connection mid-flight...")
		db.SetDBConnection(false)

		if db.IsDBHealthy() {
			t.Fatal("Expected db.IsDBHealthy to report false after connection drop")
		}

		// Execute concurrent requests during database outage
		var wg sync.WaitGroup
		errChan := make(chan error, 50)

		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				// 1. Verify refresh token validity in degraded mode
				if !db.IsRefreshTokenValid(jti) {
					errChan <- fmt.Errorf("worker %d: refresh token unexpectedly invalid during outage", idx)
					return
				}

				// 2. Perform login request
				loginBody, _ := json.Marshal(map[string]string{
					"username": "admin",
					"password": "password",
				})
				resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(loginBody))
				if err != nil {
					errChan <- fmt.Errorf("worker %d: login failed during outage: %w", idx, err)
					return
				}
				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					errChan <- fmt.Errorf("worker %d: login status %d during outage", idx, resp.StatusCode)
					return
				}
				var lResp api.LoginResponse
				_ = json.NewDecoder(resp.Body).Decode(&lResp)
				resp.Body.Close()

				// 3. Check device permission
				if !db.VerifyDevicePermission("admin", "test-camera-1", security.PermStreamAccess) {
					errChan <- fmt.Errorf("worker %d: permission check failed during outage", idx)
					return
				}

				// 4. Register push token
				tokenStr := fmt.Sprintf("ExponentPushToken[chaos_pacing_%d]", idx)
				if err := db.RegisterPushToken("admin", tokenStr, "ios"); err != nil {
					errChan <- fmt.Errorf("worker %d: register push token failed: %w", idx, err)
					return
				}
			}(i)
		}

		wg.Wait()
		close(errChan)

		for err := range errChan {
			t.Fatalf("Chaos error during DB outage: %v", err)
		}

		t.Log("PASS: High-concurrency operations during DB outage succeeded via degraded mode.")

		// CHAOS RESTORATION: Restore database connectivity
		t.Log("Simulating Chaos Recovery: Restoring database connectivity...")
		db.SetDBConnection(true)

		if !db.IsConnected {
			t.Fatal("Expected db.IsConnected to be true after restoration")
		}

		// Verify operations continue seamlessly post-recovery
		if !db.VerifyDevicePermission("admin", "test-camera-1", security.PermStreamAccess) {
			t.Fatal("Permission check failed after database restoration")
		}

		t.Log("PASS: System restored seamlessly from database outage without restart.")
	})

	t.Run("NilSafety_UninitializedDatabaseAndKeyLookup", func(t *testing.T) {
		// Set DB to nil and disconnected
		db.SetDB(nil, false)

		// 1. GetDevicePublicKey on empty device ID
		_, err := db.GetDevicePublicKey("")
		if err == nil {
			t.Fatal("Expected error for empty device ID")
		}

		// 2. GetDevicePublicKey on unknown device ID
		_, err = db.GetDevicePublicKey("unknown_device_999")
		if err == nil {
			t.Fatal("Expected error for unknown device ID in degraded mode")
		}

		// 3. Register in-memory device key and lookup
		testDevID := "chaos-ed25519-dev-01"
		mockKey := "-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEAMockPublicKeyForChaosTesting1234567890\n-----END PUBLIC KEY-----"
		db.RegisterDevicePublicKey(testDevID, mockKey)

		foundKey, err := db.GetDevicePublicKey(testDevID)
		if err != nil {
			t.Fatalf("Failed to retrieve in-memory registered device key: %v", err)
		}
		if foundKey != mockKey {
			t.Fatalf("Device key mismatch: got %s, want %s", foundKey, mockKey)
		}

		// 4. Safe operations across all DB functions with nil DB
		if db.VerifyDeviceAccess("admin", "invalid-cam") {
			t.Fatal("Expected false for unauthorized cam")
		}
		if !db.VerifyDeviceAccess("admin", "test-camera-1") {
			t.Fatal("Expected true for admin accessing test-camera-1")
		}

		role := db.GetUserRole("operator")
		if role != "operator" {
			t.Fatalf("Expected operator role, got: %s", role)
		}

		t.Log("PASS: Nil safety and in-memory key registry verified with 0 panics.")
	})
}
