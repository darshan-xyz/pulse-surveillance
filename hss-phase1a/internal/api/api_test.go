package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hss/internal/security"
	"github.com/golang-jwt/jwt/v5"
)

func TestAuthAndMultiTenantIsolation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", HandleLogin)
	mux.HandleFunc("/api/auth/login", HandleLogin)
	mux.HandleFunc("/api/devices", HandleDevices)

	server := httptest.NewServer(mux)
	defer server.Close()

	// 1. Test Login with Valid Credentials (admin)
	loginPayload := []byte(`{"username": "admin", "password": "password"}`)
	resp, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(loginPayload))
	if err != nil {
		t.Fatalf("Failed to execute login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP 200 on valid login, got: %d", resp.StatusCode)
	}

	var loginData LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginData); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}

	if loginData.Token == "" {
		t.Fatalf("Expected signed JWT token in login response, got empty string")
	}

	if loginData.User.Username != "admin" || loginData.User.TenantID != "tenant-alpha" {
		t.Fatalf("Unexpected user claims in response: %+v", loginData.User)
	}

	// Verify token signature and claims
	parsedToken, err := jwt.Parse(loginData.Token, func(t *jwt.Token) (interface{}, error) {
		return security.GetJWTSecret(), nil
	})
	if err != nil || !parsedToken.Valid {
		t.Fatalf("JWT signature verification failed: %v", err)
	}
	claims := parsedToken.Claims.(jwt.MapClaims)
	if claims["sub"] != "admin" || claims["tenant_id"] != "tenant-alpha" {
		t.Fatalf("Invalid JWT token claims: %+v", claims)
	}

	// 2. Test Login with /api/auth/login alias
	respAlias, err := http.Post(server.URL+"/api/auth/login", "application/json", bytes.NewBuffer(loginPayload))
	if err != nil || respAlias.StatusCode != http.StatusOK {
		t.Fatalf("Expected /api/auth/login to succeed with 200, got: %v", respAlias.StatusCode)
	}
	respAlias.Body.Close()

	// 3. Test Login with Invalid Password
	invalidPwPayload := []byte(`{"username": "admin", "password": "wrongpassword"}`)
	respBadPw, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(invalidPwPayload))
	if err != nil {
		t.Fatalf("Failed to send bad password login: %v", err)
	}
	respBadPw.Body.Close()
	if respBadPw.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected HTTP 401 on invalid password, got: %d", respBadPw.StatusCode)
	}

	// 4. Test Login with Unknown User
	unknownUserPayload := []byte(`{"username": "unknown_hacker", "password": "password"}`)
	respUnknown, err := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(unknownUserPayload))
	if err != nil {
		t.Fatalf("Failed to send unknown user login: %v", err)
	}
	respUnknown.Body.Close()
	if respUnknown.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected HTTP 401 on unknown user, got: %d", respUnknown.StatusCode)
	}

	// 5. Test Device Retrieval without Token
	reqNoAuth, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	respNoAuth, err := http.DefaultClient.Do(reqNoAuth)
	if err != nil {
		t.Fatalf("Failed to send unauthenticated device request: %v", err)
	}
	respNoAuth.Body.Close()
	if respNoAuth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected HTTP 401 for unauthenticated device retrieval, got: %d", respNoAuth.StatusCode)
	}

	// 6. Test Device Retrieval with Tampered Token
	reqTampered, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	reqTampered.Header.Set("Authorization", "Bearer invalid.fake.token")
	respTampered, err := http.DefaultClient.Do(reqTampered)
	if err != nil {
		t.Fatalf("Failed to send tampered token request: %v", err)
	}
	respTampered.Body.Close()
	if respTampered.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected HTTP 401 for tampered token, got: %d", respTampered.StatusCode)
	}

	// 7. Test Tenant Isolation: Admin (tenant-alpha)
	reqAdmin, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+loginData.Token)
	respAdmin, err := http.DefaultClient.Do(reqAdmin)
	if err != nil || respAdmin.StatusCode != http.StatusOK {
		t.Fatalf("Admin devices request failed: %v, status: %d", err, respAdmin.StatusCode)
	}
	var adminDevices []DeviceResponse
	json.NewDecoder(respAdmin.Body).Decode(&adminDevices)
	respAdmin.Body.Close()

	if len(adminDevices) != 2 || adminDevices[0].DeviceID != "test-camera-1" || adminDevices[1].DeviceID != "webcam-live-01" {
		t.Fatalf("Tenant isolation violation: admin received unexpected devices: %+v", adminDevices)
	}

	// 8. Test Tenant Isolation: Tenant 1 User (tenant-1)
	t1LoginPayload := []byte(`{"username": "tenant1_user", "password": "password"}`)
	respT1Login, _ := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(t1LoginPayload))
	var t1LoginData LoginResponse
	json.NewDecoder(respT1Login.Body).Decode(&t1LoginData)
	respT1Login.Body.Close()

	reqT1, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	reqT1.Header.Set("Authorization", "Bearer "+t1LoginData.Token)
	respT1, _ := http.DefaultClient.Do(reqT1)
	var t1Devices []DeviceResponse
	json.NewDecoder(respT1.Body).Decode(&t1Devices)
	respT1.Body.Close()

	if len(t1Devices) != 2 || t1Devices[0].DeviceID != "cam-tenant1-01" || t1Devices[1].DeviceID != "cam-tenant1-02" {
		t.Fatalf("Tenant 1 isolation violation: received unexpected devices: %+v", t1Devices)
	}

	// 9. Test Tenant Isolation: Tenant 2 User (tenant-2)
	t2LoginPayload := []byte(`{"username": "tenant2_user", "password": "password"}`)
	respT2Login, _ := http.Post(server.URL+"/api/login", "application/json", bytes.NewBuffer(t2LoginPayload))
	var t2LoginData LoginResponse
	json.NewDecoder(respT2Login.Body).Decode(&t2LoginData)
	respT2Login.Body.Close()

	reqT2, _ := http.NewRequest(http.MethodGet, server.URL+"/api/devices", nil)
	reqT2.Header.Set("Authorization", "Bearer "+t2LoginData.Token)
	respT2, _ := http.DefaultClient.Do(reqT2)
	var t2Devices []DeviceResponse
	json.NewDecoder(respT2.Body).Decode(&t2Devices)
	respT2.Body.Close()

	if len(t2Devices) != 1 || t2Devices[0].DeviceID != "cam-tenant2-01" {
		t.Fatalf("Tenant 2 isolation violation: received unexpected devices: %+v", t2Devices)
	}

	// Cross-tenant verification: verify Tenant 1 cannot see Tenant 2 devices and vice versa
	for _, d := range t1Devices {
		if d.DeviceID == "cam-tenant2-01" || d.DeviceID == "test-camera-1" {
			t.Fatalf("CRITICAL SECURITY LEAK: Tenant 1 accessed foreign device %s", d.DeviceID)
		}
	}
	for _, d := range t2Devices {
		if d.DeviceID == "cam-tenant1-01" || d.DeviceID == "cam-tenant1-02" || d.DeviceID == "test-camera-1" {
			t.Fatalf("CRITICAL SECURITY LEAK: Tenant 2 accessed foreign device %s", d.DeviceID)
		}
	}

	t.Log("SUCCESS: Multi-tenant JWT authentication and strict device isolation verified cleanly!")
}

func TestExpiredJWTRejection(t *testing.T) {
	// Create an expired JWT token
	expiredClaims := jwt.MapClaims{
		"sub":       "admin",
		"tenant_id": "tenant-alpha",
		"iat":       time.Now().Add(-2 * time.Hour).Unix(),
		"exp":       time.Now().Add(-1 * time.Hour).Unix(),
	}
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
	tokenStr, err := expiredToken.SignedString(security.GetJWTSecret())
	if err != nil {
		t.Fatalf("Failed to create expired token: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()

	HandleDevices(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("Expected expired JWT to be rejected with 401, got: %d", rr.Code)
	}
}
