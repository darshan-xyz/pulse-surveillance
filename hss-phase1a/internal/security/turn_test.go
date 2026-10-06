package security

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGenerateTURNCredentials(t *testing.T) {
	secret := "test-secret-12345"
	userID := "alice"
	ttl := 15 * time.Minute
	turnServer := "turn:coturn.pulse.com:3478"

	creds, err := GenerateTURNCredentials(secret, userID, ttl, turnServer)
	if err != nil {
		t.Fatalf("GenerateTURNCredentials failed: %v", err)
	}

	if !strings.Contains(creds.Username, ":alice") {
		t.Fatalf("Expected username to contain :alice, got: %s", creds.Username)
	}
	if creds.Password == "" {
		t.Fatal("Password is empty")
	}
	if creds.TurnServer != turnServer {
		t.Fatalf("Expected turn server %s, got %s", turnServer, creds.TurnServer)
	}
	if len(creds.URIs) < 2 {
		t.Fatalf("Expected at least 2 URIs (udp, tcp), got: %v", creds.URIs)
	}

	parts := strings.Split(creds.Username, ":")
	if len(parts) != 2 {
		t.Fatalf("Expected username format timestamp:userID, got: %s", creds.Username)
	}

	expiryUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		t.Fatalf("Invalid timestamp in username: %v", err)
	}

	now := time.Now().Unix()
	if expiryUnix <= now || expiryUnix > now+int64(ttl.Seconds())+10 {
		t.Fatalf("Expiry %d out of expected range for TTL %v", expiryUnix, ttl)
	}
}

func TestVerifyTURNCredentialsValid(t *testing.T) {
	secret := "secure-key-999"
	userID := "bob"
	creds, err := GenerateTURNCredentials(secret, userID, 10*time.Minute, "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	valid, err := VerifyTURNCredentials(secret, creds.Username, creds.Password)
	if err != nil || !valid {
		t.Fatalf("Expected valid credentials to verify, got valid=%v, err=%v", valid, err)
	}
	t.Log("PASS: Dynamic TURN credentials verified successfully")
}

func TestVerifyTURNCredentialsExpired(t *testing.T) {
	secret := "secure-key-999"
	// Generate with negative TTL (already expired)
	creds, err := GenerateTURNCredentials(secret, "charlie", -10*time.Second, "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	valid, err := VerifyTURNCredentials(secret, creds.Username, creds.Password)
	if valid || err == nil {
		t.Fatal("Expected expired TURN credentials to be rejected")
	}
	t.Logf("PASS: Expired TURN credentials correctly rejected: %v", err)
}

func TestVerifyTURNCredentialsInvalidSecret(t *testing.T) {
	secret := "secure-key-999"
	attackerSecret := "wrong-secret-666"

	creds, err := GenerateTURNCredentials(secret, "david", 10*time.Minute, "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	valid, err := VerifyTURNCredentials(attackerSecret, creds.Username, creds.Password)
	if valid || err == nil {
		t.Fatal("Expected mismatched secret to be rejected")
	}
	t.Logf("PASS: TURN credentials with wrong secret correctly rejected: %v", err)
}

func TestVerifyTURNCredentialsTampered(t *testing.T) {
	secret := "secure-key-999"
	creds, err := GenerateTURNCredentials(secret, "eve", 10*time.Minute, "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Tamper with username
	tamperedUser := creds.Username + "_tampered"
	valid, err := VerifyTURNCredentials(secret, tamperedUser, creds.Password)
	if valid || err == nil {
		t.Fatal("Expected tampered username to be rejected")
	}

	// Tamper with password
	valid2, err2 := VerifyTURNCredentials(secret, creds.Username, "TAMPERED_BASE64_PASSWORD==")
	if valid2 || err2 == nil {
		t.Fatal("Expected tampered password to be rejected")
	}

	t.Log("PASS: Tampered TURN credentials correctly rejected")
}
