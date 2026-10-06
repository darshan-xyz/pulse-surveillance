package tests

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"hss/internal/cloud/cache"
	"hss/internal/cloud/db"
	"hss/internal/cloud/signaling"
)

// Mock configurations matching docker-compose.test.yml
const (
	TestDBURL    = "postgres://pulse_test:test_password@localhost:5433/pulse_test_db?sslmode=disable"
	TestRedisURL = "localhost:6380"
)

func setupTestEnvironment(t *testing.T) {
	// Initialize DB
	if err := db.InitDB(TestDBURL); err != nil {
		t.Skipf("Skipping integration test: test PostgreSQL DB not available: %v", err)
	}

	// Initialize Valkey
	if err := cache.InitValkey(TestRedisURL); err != nil {
		t.Logf("Test Valkey not available (%v), using in-memory fallback", err)
	}

	// Insert mock test data
	if db.DB != nil {
		_, err := db.DB.Exec(`
			TRUNCATE users, devices, permissions CASCADE;
			INSERT INTO users (id, username, password_hash) VALUES (1, 'test_user', 'hash');
		`)
		if err != nil {
			t.Fatalf("Failed to seed users: %v", err)
		}
	}
}

func generateDeviceJWT(t *testing.T, deviceID string) (string, string) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate ed25519 key: %v", err)
	}

	// Format pubKey to DER PKIX PEM format
	derBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		t.Fatalf("Failed to marshal public key: %v", err)
	}
	pubKeyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derBytes,
	}))

	// Seed DB with device public key
	if db.DB != nil {
		_, err = db.DB.Exec(`
			INSERT INTO devices (device_id, public_key) VALUES ($1, $2);
			INSERT INTO permissions (user_id, device_id, role) VALUES (1, $1, 'owner');
		`, deviceID, pubKeyPEM)
		if err != nil {
			t.Fatalf("Failed to seed device: %v", err)
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{
		"sub": deviceID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	
	tokenString, err := token.SignedString(privKey)
	if err != nil {
		t.Fatalf("Failed to sign JWT: %v", err)
	}
	
	return tokenString, pubKeyPEM
}

func TestEndToEndSignaling(t *testing.T) {
	// Skip if not explicitly running integration tests to save time during standard builds
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	setupTestEnvironment(t)
	
	deviceID := "test_camera_123"
	jwtToken, _ := generateDeviceJWT(t, deviceID)

	// Spin up the Hub using httptest
	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// 1. Simulate Edge Daemon Connection
	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+jwtToken)
	edgeConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/signaling", headers)
	if err != nil {
		t.Fatalf("Edge Daemon failed to connect: %v", err)
	}
	defer edgeConn.Close()

	// 2. Simulate Mobile App Connection
	mobileConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token=test_user", nil)
	if err != nil {
		t.Fatalf("Mobile App failed to connect: %v", err)
	}
	defer mobileConn.Close()

	// 3. Mobile App sends an Offer
	testSessionID := "sess_999"
	offerMsg := map[string]string{
		"type":       "offer",
		"device_id":  deviceID,
		"session_id": testSessionID,
		"sdp":        `{"type":"offer","sdp":"mock_sdp_data"}`,
	}
	if err := mobileConn.WriteJSON(offerMsg); err != nil {
		t.Fatalf("Mobile app failed to send offer: %v", err)
	}

	// 4. Edge Daemon receives the Offer via Valkey routing
	edgeConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var receivedOffer map[string]interface{}
	if err := edgeConn.ReadJSON(&receivedOffer); err != nil {
		t.Fatalf("Edge Daemon failed to receive routed offer: %v", err)
	}

	if receivedOffer["type"] != "offer" || receivedOffer["session_id"] != testSessionID {
		t.Fatalf("Edge Daemon received invalid offer: %+v", receivedOffer)
	}

	// 5. Edge Daemon sends an Answer
	answerMsg := map[string]string{
		"type":       "answer",
		"session_id": testSessionID,
		"sdp":        `{"type":"answer","sdp":"mock_answer_data"}`,
	}
	if err := edgeConn.WriteJSON(answerMsg); err != nil {
		t.Fatalf("Edge Daemon failed to send answer: %v", err)
	}

	// 6. Mobile App receives the Answer via Valkey routing
	mobileConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var receivedAnswer map[string]interface{}
	if err := mobileConn.ReadJSON(&receivedAnswer); err != nil {
		t.Fatalf("Mobile App failed to receive routed answer: %v", err)
	}

	if receivedAnswer["type"] != "answer" {
		t.Fatalf("Mobile App received invalid answer: %+v", receivedAnswer)
	}

	// Validation Successful! The SDP successfully hopped through the PostgreSQL Authz check,
	// entered the Cloud Hub, published onto Valkey, and was pulled down into the separate WebSocket connection!
	t.Logf("End-to-End WebSocket -> Valkey -> WebSocket Routing passed successfully!")
}
