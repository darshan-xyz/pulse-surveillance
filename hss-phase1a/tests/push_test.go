package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hss/internal/api"
	"hss/internal/cloud/db"
	"hss/internal/cloud/push"
	"hss/internal/events"
	"hss/internal/security"

	"github.com/golang-jwt/jwt/v5"
)

// Helper function to generate a test JWT for a given username and tenant
func generatePushTestJWT(username, tenantID, role string) (string, error) {
	claims := jwt.MapClaims{
		"sub":       username,
		"username":  username,
		"tenant_id": tenantID,
		"role":      role,
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(security.GetJWTSecret())
}

// TestPushTokenRegistrationAPI tests the POST /api/push/register HTTP endpoint
func TestPushTokenRegistrationAPI(t *testing.T) {
	db.ClearPushTokens()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/push/register", api.HandlePushRegister)
	server := httptest.NewServer(mux)
	defer server.Close()

	validToken, err := generatePushTestJWT("admin", "tenant-alpha", "admin")
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// 1. Unauthenticated request (Missing Authorization header)
	body := bytes.NewBufferString(`{"push_token": "ExponentPushToken[test1]", "platform": "ios"}`)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/push/register", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for missing auth, got: %d", resp.StatusCode)
	}

	// 2. Tampered / invalid JWT token
	body = bytes.NewBufferString(`{"push_token": "ExponentPushToken[test1]", "platform": "ios"}`)
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/api/push/register", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer invalid.fake.token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for tampered JWT, got: %d", resp.StatusCode)
	}

	// 3. Missing push_token in body
	body = bytes.NewBufferString(`{"platform": "ios"}`)
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/api/push/register", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+validToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request for missing push_token, got: %d", resp.StatusCode)
	}

	// 4. Invalid JSON payload
	body = bytes.NewBufferString(`invalid json data`)
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/api/push/register", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+validToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request for malformed JSON, got: %d", resp.StatusCode)
	}

	// 5. OPTIONS preflight CORS check
	req, _ = http.NewRequest(http.MethodOptions, server.URL+"/api/push/register", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for OPTIONS preflight, got: %d", resp.StatusCode)
	}
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Fatalf("Expected Access-Control-Allow-Origin: *, got: %s", origin)
	}

	// 6. Valid registration for admin
	adminPayload := `{"push_token": "ExponentPushToken[admin_iphone_abc123]", "platform": "ios"}`
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/api/push/register", bytes.NewBufferString(adminPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+validToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Valid register request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for valid push token registration, got: %d", resp.StatusCode)
	}

	var regResp api.PushRegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
		t.Fatalf("Failed to decode push register response: %v", err)
	}
	if regResp.Status != "registered" {
		t.Fatalf("Expected status 'registered', got: %s", regResp.Status)
	}

	// 7. Verify token was saved in database
	tokens, err := db.GetPushTokensForUser("admin")
	if err != nil {
		t.Fatalf("Failed to query push tokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0] != "ExponentPushToken[admin_iphone_abc123]" {
		t.Fatalf("Unexpected tokens stored for admin: %+v", tokens)
	}

	t.Log("SUCCESS: POST /api/push/register API verified cleanly with full auth and validation!")
}

// TestPushTokenDatabaseStorage tests token registration, multi-user isolation and device permission mapping
func TestPushTokenDatabaseStorage(t *testing.T) {
	db.ClearPushTokens()

	// Register token for user1
	err := db.RegisterPushToken("admin", "ExponentPushToken[admin_device_1]", "ios")
	if err != nil {
		t.Fatalf("RegisterPushToken failed: %v", err)
	}

	// Register token for tenant1_user
	err = db.RegisterPushToken("tenant1_user", "ExponentPushToken[tenant1_android]", "android")
	if err != nil {
		t.Fatalf("RegisterPushToken failed: %v", err)
	}

	// Register token for tenant2_user
	err = db.RegisterPushToken("tenant2_user", "ExponentPushToken[tenant2_phone]", "ios")
	if err != nil {
		t.Fatalf("RegisterPushToken failed: %v", err)
	}

	// Verify GetPushTokensForUser isolation
	adminTokens, err := db.GetPushTokensForUser("admin")
	if err != nil || len(adminTokens) != 1 || adminTokens[0] != "ExponentPushToken[admin_device_1]" {
		t.Fatalf("Unexpected tokens for admin: %+v", adminTokens)
	}

	t1Tokens, err := db.GetPushTokensForUser("tenant1_user")
	if err != nil || len(t1Tokens) != 1 || t1Tokens[0] != "ExponentPushToken[tenant1_android]" {
		t.Fatalf("Unexpected tokens for tenant1_user: %+v", t1Tokens)
	}

	// Verify Device-to-User mapping via GetPushTokensForDevice
	// In standalone mode:
	// admin -> test-camera-1
	// tenant1_user -> cam-tenant1-01, cam-tenant1-02
	// tenant2_user -> cam-tenant2-01

	dev1Tokens, err := db.GetPushTokensForDevice("test-camera-1")
	if err != nil || len(dev1Tokens) != 1 || dev1Tokens[0] != "ExponentPushToken[admin_device_1]" {
		t.Fatalf("Device test-camera-1 returned unexpected tokens: %+v", dev1Tokens)
	}

	t1CamTokens, err := db.GetPushTokensForDevice("cam-tenant1-01")
	if err != nil || len(t1CamTokens) != 1 || t1CamTokens[0] != "ExponentPushToken[tenant1_android]" {
		t.Fatalf("Device cam-tenant1-01 returned unexpected tokens: %+v", t1CamTokens)
	}

	// Ensure tenant 2 token is NOT returned for tenant 1 camera
	for _, tok := range t1CamTokens {
		if tok == "ExponentPushToken[tenant2_phone]" {
			t.Fatalf("SECURITY VIOLATION: Tenant 2 token leaked to Tenant 1 device!")
		}
	}

	// Test Token Upsert / Update
	err = db.RegisterPushToken("admin", "ExponentPushToken[admin_device_1]", "web")
	if err != nil {
		t.Fatalf("Upsert token failed: %v", err)
	}
	adminTokensUpdated, _ := db.GetPushTokensForUser("admin")
	if len(adminTokensUpdated) != 1 {
		t.Fatalf("Expected exactly 1 token after upsert, got %d", len(adminTokensUpdated))
	}

	t.Log("SUCCESS: Push token DB storage and device authorization mapping verified cleanly!")
}

// TestPushExpoClient tests the HTTP client interactions against a mock Expo Push API server
func TestPushExpoClient(t *testing.T) {
	var receivedPayload []push.ExpoPushMessage
	var requestCount int

	mockExpoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		var msgs []push.ExpoPushMessage
		if err := json.Unmarshal(bodyBytes, &msgs); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receivedPayload = msgs

		// Simulate response
		w.Header().Set("Content-Type", "application/json")
		if len(msgs) > 0 && msgs[0].To == "ExponentPushToken[invalid_device]" {
			resp := push.ExpoPushResponse{
				Data: []push.ExpoPushTicket{
					{
						Status:  "error",
						Message: "\"ExponentPushToken[invalid_device]\" is not a registered push notification recipient",
						Details: map[string]interface{}{"error": "DeviceNotRegistered"},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		resp := push.ExpoPushResponse{
			Data: []push.ExpoPushTicket{
				{
					Status: "ok",
					ID:     "ticket-uuid-123456",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockExpoServer.Close()

	client := push.NewExpoPushService(mockExpoServer.URL)

	// 1. Test SendNotification with valid parameters
	msg := push.ExpoPushMessage{
		To:        "ExponentPushToken[valid_token_xyz]",
		Title:     "Person Alert: cam-tenant1-01",
		Body:      "Person detected on camera cam-tenant1-01 (Confidence: 92%)",
		Sound:     "default",
		Priority:  "high",
		ChannelID: "alerts",
		Data: map[string]interface{}{
			"camera_id":  "cam-tenant1-01",
			"event_type": "PERSON_DETECTED",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := client.SendNotification(ctx, msg)
	if err != nil {
		t.Fatalf("SendNotification failed: %v", err)
	}

	if len(receivedPayload) != 1 {
		t.Fatalf("Expected 1 payload sent to Expo API, got %d", len(receivedPayload))
	}
	sent := receivedPayload[0]
	if sent.To != msg.To || sent.Title != msg.Title || sent.Priority != "high" || sent.ChannelID != "alerts" {
		t.Fatalf("Payload sent to Expo mismatch: %+v", sent)
	}

	// 2. Test empty recipient validation
	err = client.SendNotification(ctx, push.ExpoPushMessage{To: ""})
	if err == nil {
		t.Fatalf("Expected error for empty recipient, got nil")
	}

	// 3. Test Expo Ticket Error handling (DeviceNotRegistered)
	errMsg := push.ExpoPushMessage{
		To:    "ExponentPushToken[invalid_device]",
		Title: "Test",
		Body:  "Test",
	}
	err = client.SendNotification(ctx, errMsg)
	if err == nil || !strings.Contains(err.Error(), "DeviceNotRegistered") && !strings.Contains(err.Error(), "not a registered") {
		t.Fatalf("Expected DeviceNotRegistered error, got: %v", err)
	}

	t.Log("SUCCESS: Expo Push Service client tested with full HTTP payload and ticket error verification!")
}

// TestPushEventToAlertPipeline verifies that emitting an event in the Event Engine
// triggers push notifications asynchronously to authorized users.
func TestPushEventToAlertPipeline(t *testing.T) {
	db.ClearPushTokens()

	// Register push token for admin (authorized for test-camera-1)
	adminToken := "ExponentPushToken[admin_motion_alert_device]"
	if err := db.RegisterPushToken("admin", adminToken, "ios"); err != nil {
		t.Fatalf("Failed to register admin token: %v", err)
	}

	// Mock Expo HTTP Server
	notificationChan := make(chan push.ExpoPushMessage, 10)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var msgs []push.ExpoPushMessage
		if err := json.Unmarshal(bodyBytes, &msgs); err == nil {
			for _, m := range msgs {
				notificationChan <- m
			}
		}
		w.Header().Set("Content-Type", "application/json")
		resp := push.ExpoPushResponse{
			Data: []push.ExpoPushTicket{{Status: "ok", ID: "ticket-123"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	// Inject mock push service into engine
	mockPushService := push.NewExpoPushService(mockServer.URL)
	push.SetDefaultService(mockPushService)

	// Initialize Event Engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events.InitEngine(ctx)

	// Emit Motion Detected Event on test-camera-1
	testEventID := "evt-test-motion-999"
	events.DefaultEngine.Emit(events.Event{
		ID:         testEventID,
		CameraID:   "test-camera-1",
		Type:       events.EventMotionDetected,
		StartedAt:  time.Now(),
		Confidence: 0.96,
	})

	// Wait for asynchronous push notification dispatch
	select {
	case received := <-notificationChan:
		t.Logf("Received push notification: %+v", received)
		if received.To != adminToken {
			t.Fatalf("Expected notification to %s, got %s", adminToken, received.To)
		}
		if !strings.Contains(received.Title, "Motion Alert") && !strings.Contains(received.Title, "test-camera-1") {
			t.Fatalf("Unexpected title: %s", received.Title)
		}
		if !strings.Contains(received.Body, "96%") || !strings.Contains(received.Body, "test-camera-1") {
			t.Fatalf("Unexpected body: %s", received.Body)
		}
		if received.Priority != "high" || received.ChannelID != "alerts" {
			t.Fatalf("Unexpected priority/channel: priority=%s, channel=%s", received.Priority, received.ChannelID)
		}
		if received.Data["camera_id"] != "test-camera-1" || received.Data["event_type"] != string(events.EventMotionDetected) {
			t.Fatalf("Unexpected metadata in push payload: %+v", received.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Timed out waiting for push notification to be dispatched by Event Engine")
	}

	// Test Person Detected Event
	events.DefaultEngine.Emit(events.Event{
		ID:         "evt-test-person-888",
		CameraID:   "test-camera-1",
		Type:       events.EventPersonDetected,
		StartedAt:  time.Now(),
		Confidence: 0.88,
	})

	select {
	case received := <-notificationChan:
		t.Logf("Received person alert: %+v", received)
		if !strings.Contains(received.Title, "Person Alert") {
			t.Fatalf("Expected Person Alert title, got: %s", received.Title)
		}
		if !strings.Contains(received.Body, "88%") {
			t.Fatalf("Expected 88%% in body, got: %s", received.Body)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Timed out waiting for person detected push notification")
	}

	t.Log("SUCCESS: Full Event Engine -> SQLite/DB Permissions -> Expo Push Alert pipeline verified cleanly!")
}
