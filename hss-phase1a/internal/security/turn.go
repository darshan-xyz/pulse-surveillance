package security

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultTURNSecret is the default secret for dynamic HMAC-SHA1 TURN authentication
	DefaultTURNSecret = "pulse-enterprise-turn-secret-key"
	// DefaultTURNServer is the default coturn endpoint
	DefaultTURNServer = "turn:coturn.pulse.com:3478"
	// DefaultTURNTTL is the default lifetime for ephemeral credentials (1 hour)
	DefaultTURNTTL = 1 * time.Hour
)

// TURNCredentials represents the RFC 5766 ephemeral credentials payload
type TURNCredentials struct {
	Username   string   `json:"username"`
	Password   string   `json:"password"`
	TurnServer string   `json:"turn_server"`
	URIs       []string `json:"uris"`
	TTL        int64    `json:"ttl"`
	ExpiresAt  int64    `json:"expires_at"`
}

// GetDefaultTURNSecret returns the active TURN HMAC secret
func GetDefaultTURNSecret() string {
	if s := os.Getenv("TURN_SECRET"); s != "" {
		return s
	}
	return DefaultTURNSecret
}

// GetDefaultTURNServer returns the active TURN server address
func GetDefaultTURNServer() string {
	if s := os.Getenv("TURN_SERVER"); s != "" {
		return s
	}
	return DefaultTURNServer
}

// GenerateTURNCredentials generates RFC 5766 compliant dynamic TURN credentials using HMAC-SHA1.
// Username format: timestamp:userID
// Password format: Base64(HMAC-SHA1(secret, username))
func GenerateTURNCredentials(secret, userID string, ttl time.Duration, turnServer string) (*TURNCredentials, error) {
	if secret == "" {
		secret = GetDefaultTURNSecret()
	}
	if turnServer == "" {
		turnServer = GetDefaultTURNServer()
	}
	if ttl == 0 {
		ttl = DefaultTURNTTL
	}
	if userID == "" {
		userID = "anonymous_user"
	}

	expiryUnix := time.Now().Add(ttl).Unix()
	username := fmt.Sprintf("%d:%s", expiryUnix, userID)

	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	password := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	hostPort := strings.TrimPrefix(turnServer, "turn:")
	uris := []string{
		fmt.Sprintf("turn:%s?transport=udp", hostPort),
		fmt.Sprintf("turn:%s?transport=tcp", hostPort),
	}

	return &TURNCredentials{
		Username:   username,
		Password:   password,
		TurnServer: turnServer,
		URIs:       uris,
		TTL:        int64(ttl.Seconds()),
		ExpiresAt:  expiryUnix,
	}, nil
}

// VerifyTURNCredentials validates the authenticity and expiration of ephemeral credentials
func VerifyTURNCredentials(secret, username, password string) (bool, error) {
	if secret == "" {
		secret = GetDefaultTURNSecret()
	}
	if username == "" || password == "" {
		return false, errors.New("empty credentials")
	}

	parts := strings.SplitN(username, ":", 2)
	if len(parts) != 2 {
		return false, errors.New("invalid username format (expected timestamp:userID)")
	}

	expiryUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid timestamp in username: %w", err)
	}

	// Check expiration
	if time.Now().Unix() > expiryUnix {
		return false, errors.New("turn credentials expired")
	}

	// Recompute expected password
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	expectedPassword := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(password), []byte(expectedPassword)) {
		return false, errors.New("invalid password signature")
	}

	return true, nil
}
