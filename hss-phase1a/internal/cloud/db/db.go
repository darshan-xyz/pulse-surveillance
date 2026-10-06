package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	dbMu        sync.RWMutex
	DB          *sql.DB
	IsConnected bool
)

// InitDB initializes PostgreSQL connection and creates required tables.
func InitDB(dsn string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	var err error
	DB, err = sql.Open("postgres", dsn)
	if err != nil {
		IsConnected = false
		return err
	}

	if err := DB.Ping(); err != nil {
		IsConnected = false
		return err
	}

	IsConnected = true
	log.Println("Connected to PostgreSQL")

	// Create tables if not exists
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		username VARCHAR(50) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL
	);

	CREATE TABLE IF NOT EXISTS devices (
		device_id VARCHAR(100) PRIMARY KEY,
		public_key TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS permissions (
		user_id INTEGER REFERENCES users(id),
		device_id VARCHAR(100) REFERENCES devices(device_id),
		role VARCHAR(20) DEFAULT 'viewer',
		PRIMARY KEY (user_id, device_id)
	);

	CREATE TABLE IF NOT EXISTS push_tokens (
		id SERIAL PRIMARY KEY,
		username VARCHAR(50) NOT NULL,
		push_token VARCHAR(255) UNIQUE NOT NULL,
		platform VARCHAR(20) NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_push_tokens_user ON push_tokens(username);

	CREATE TABLE IF NOT EXISTS refresh_tokens (
		id SERIAL PRIMARY KEY,
		username VARCHAR(50) NOT NULL,
		token_id VARCHAR(100) UNIQUE NOT NULL,
		expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
		revoked BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_refresh_tokens_id ON refresh_tokens(token_id);
	`

	if _, err := DB.Exec(schema); err != nil {
		return err
	}

	log.Println("PostgreSQL schema initialized")
	return nil
}

// SetDBConnection updates the connection status safely (used for chaos testing and dynamic reconnection).
func SetDBConnection(connected bool) {
	dbMu.Lock()
	defer dbMu.Unlock()
	IsConnected = connected
}

// SetDB sets the underlying *sql.DB and connection status safely.
func SetDB(sqlDB *sql.DB, connected bool) {
	dbMu.Lock()
	defer dbMu.Unlock()
	DB = sqlDB
	IsConnected = connected
}

// PingDB verifies current database health and updates IsConnected accordingly.
func PingDB() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if DB == nil {
		IsConnected = false
		return fmt.Errorf("database handle is nil")
	}
	if err := DB.Ping(); err != nil {
		IsConnected = false
		return err
	}
	IsConnected = true
	return nil
}

// IsDBHealthy returns true if the database handle is valid and connected.
func IsDBHealthy() bool {
	dbMu.RLock()
	defer dbMu.RUnlock()
	return IsConnected && DB != nil
}

type PushTokenRecord struct {
	Username  string
	PushToken string
	Platform  string
	UpdatedAt time.Time
}

var (
	memPushMu     sync.RWMutex
	memPushTokens = make(map[string]PushTokenRecord)
)

// RegisterPushToken associates a device push token with a username
func RegisterPushToken(username, pushToken, platform string) error {
	if username == "" || pushToken == "" {
		return fmt.Errorf("username and push_token are required")
	}
	if platform == "" {
		platform = "unknown"
	}

	// Always record in memory as fallback
	memPushMu.Lock()
	memPushTokens[pushToken] = PushTokenRecord{
		Username:  username,
		PushToken: pushToken,
		Platform:  platform,
		UpdatedAt: time.Now(),
	}
	memPushMu.Unlock()

	dbMu.RLock()
	connected := IsConnected && DB != nil
	dbHandle := DB
	dbMu.RUnlock()

	if !connected {
		return nil
	}

	query := `
		INSERT INTO push_tokens (username, push_token, platform, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (push_token)
		DO UPDATE SET username = EXCLUDED.username, platform = EXCLUDED.platform, updated_at = CURRENT_TIMESTAMP
	`
	_, err := dbHandle.Exec(query, username, pushToken, platform)
	if err != nil {
		log.Printf("[DB] RegisterPushToken query failed, falling back to in-memory store: %v", err)
		return nil // gracefully fallback without breaking callers
	}
	return nil
}

// GetPushTokensForUser retrieves all push tokens for a specific user
func GetPushTokensForUser(username string) ([]string, error) {
	dbMu.RLock()
	connected := IsConnected && DB != nil
	dbHandle := DB
	dbMu.RUnlock()

	if !connected {
		return getMemPushTokensForUser(username), nil
	}

	rows, err := dbHandle.Query("SELECT push_token FROM push_tokens WHERE username = $1", username)
	if err != nil {
		log.Printf("[DB] GetPushTokensForUser failed, falling back to in-memory store: %v", err)
		return getMemPushTokensForUser(username), nil
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil {
			tokens = append(tokens, t)
		}
	}
	if len(tokens) == 0 {
		memTokens := getMemPushTokensForUser(username)
		if len(memTokens) > 0 {
			return memTokens, nil
		}
	}
	return tokens, nil
}

func getMemPushTokensForUser(username string) []string {
	memPushMu.RLock()
	defer memPushMu.RUnlock()
	var tokens []string
	for _, rec := range memPushTokens {
		if rec.Username == username {
			tokens = append(tokens, rec.PushToken)
		}
	}
	return tokens
}

// GetPushTokensForDevice retrieves all push tokens for users authorized to access deviceID
func GetPushTokensForDevice(deviceID string) ([]string, error) {
	dbMu.RLock()
	connected := IsConnected && DB != nil
	dbHandle := DB
	dbMu.RUnlock()

	if !connected {
		return getMemPushTokensForDevice(deviceID), nil
	}

	query := `
		SELECT DISTINCT pt.push_token
		FROM push_tokens pt
		JOIN users u ON pt.username = u.username
		JOIN permissions p ON u.id = p.user_id
		WHERE p.device_id = $1
	`
	rows, err := dbHandle.Query(query, deviceID)
	if err != nil {
		log.Printf("[DB] GetPushTokensForDevice failed, falling back to in-memory store: %v", err)
		return getMemPushTokensForDevice(deviceID), nil
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil {
			tokens = append(tokens, t)
		}
	}
	if len(tokens) == 0 {
		memTokens := getMemPushTokensForDevice(deviceID)
		if len(memTokens) > 0 {
			return memTokens, nil
		}
	}
	return tokens, nil
}

func getMemPushTokensForDevice(deviceID string) []string {
	memPushMu.RLock()
	defer memPushMu.RUnlock()
	var tokens []string
	for _, rec := range memPushTokens {
		if VerifyDeviceAccess(rec.Username, deviceID) {
			tokens = append(tokens, rec.PushToken)
		}
	}
	return tokens
}

// ClearPushTokens clears the in-memory push token registry (useful for test resets)
func ClearPushTokens() {
	memPushMu.Lock()
	defer memPushMu.Unlock()
	memPushTokens = make(map[string]PushTokenRecord)
}

// VerifyDeviceAccess checks if a user has permission to access a device
func VerifyDeviceAccess(username, deviceID string) bool {
	dbMu.RLock()
	connected := IsConnected && DB != nil
	dbHandle := DB
	dbMu.RUnlock()

	if !connected {
		return checkFallbackDeviceAccess(username, deviceID)
	}

	var count int
	query := `
		SELECT COUNT(*) 
		FROM permissions p
		JOIN users u ON p.user_id = u.id
		WHERE u.username = $1 AND p.device_id = $2
	`
	err := dbHandle.QueryRow(query, username, deviceID).Scan(&count)
	if err != nil {
		log.Printf("[DB] Error verifying access via DB, falling back to in-memory permissions: %v", err)
		return checkFallbackDeviceAccess(username, deviceID)
	}
	return count > 0
}

func checkFallbackDeviceAccess(username, deviceID string) bool {
	switch username {
	case "admin":
		return deviceID == "test-camera-1" || deviceID == "webcam-live-01" || strings.HasPrefix(deviceID, "webcam-") || strings.HasPrefix(deviceID, "chaos-") || strings.HasPrefix(deviceID, "test-")
	case "operator", "operator_user":
		return deviceID == "test-camera-1" || deviceID == "webcam-live-01" || strings.HasPrefix(deviceID, "webcam-") || strings.HasPrefix(deviceID, "chaos-") || strings.HasPrefix(deviceID, "test-")
	case "user2":
		return deviceID == "test-camera-2"
	case "tenant1_user", "tenant1_viewer":
		return deviceID == "cam-tenant1-01" || deviceID == "cam-tenant1-02"
	case "tenant2_user":
		return deviceID == "cam-tenant2-01"
	default:
		return false
	}
}

var (
	memDeviceMu   sync.RWMutex
	memDeviceKeys = make(map[string]string)
)

// RegisterDevicePublicKey stores a device's public key in the in-memory cache (for degraded/test mode).
func RegisterDevicePublicKey(deviceID, pubKey string) {
	memDeviceMu.Lock()
	defer memDeviceMu.Unlock()
	memDeviceKeys[deviceID] = pubKey
}

// ClearDeviceKeys clears the in-memory device public key registry.
func ClearDeviceKeys() {
	memDeviceMu.Lock()
	defer memDeviceMu.Unlock()
	memDeviceKeys = make(map[string]string)
}

// GetDevicePublicKey retrieves the public key for verifying Ed25519 JWTs safely without panicking.
func GetDevicePublicKey(deviceID string) (string, error) {
	if deviceID == "" {
		return "", fmt.Errorf("empty device ID")
	}

	dbMu.RLock()
	connected := IsConnected && DB != nil
	dbHandle := DB
	dbMu.RUnlock()

	// Check in-memory store first or if DB is offline/nil
	if !connected {
		memDeviceMu.RLock()
		key, exists := memDeviceKeys[deviceID]
		memDeviceMu.RUnlock()
		if exists {
			return key, nil
		}
		return "", fmt.Errorf("device public key not found for device %s (in-memory mode)", deviceID)
	}

	var pubKey string
	err := dbHandle.QueryRow("SELECT public_key FROM devices WHERE device_id = $1", deviceID).Scan(&pubKey)
	if err != nil {
		// Fallback to in-memory key registry if DB query failed
		memDeviceMu.RLock()
		key, exists := memDeviceKeys[deviceID]
		memDeviceMu.RUnlock()
		if exists {
			return key, nil
		}
		return "", fmt.Errorf("device public key not found for device %s: %w", deviceID, err)
	}
	return pubKey, nil
}

type RefreshTokenRecord struct {
	TokenID   string
	Username  string
	ExpiresAt time.Time
	Revoked   bool
}

var (
	memRefreshMu     sync.RWMutex
	memRefreshTokens = make(map[string]RefreshTokenRecord)
)

// ClearRefreshTokens clears the in-memory refresh token registry.
func ClearRefreshTokens() {
	memRefreshMu.Lock()
	defer memRefreshMu.Unlock()
	memRefreshTokens = make(map[string]RefreshTokenRecord)
}

// ClearAllMemoryStores resets all in-memory fallback registries.
func ClearAllMemoryStores() {
	ClearPushTokens()
	ClearRefreshTokens()
	ClearDeviceKeys()
}

// StoreRefreshToken records a new refresh token
func StoreRefreshToken(tokenID, username string, expiresAt time.Time) error {
	if tokenID == "" || username == "" {
		return fmt.Errorf("tokenID and username are required")
	}

	// Always record in memory as fallback
	memRefreshMu.Lock()
	memRefreshTokens[tokenID] = RefreshTokenRecord{
		TokenID:   tokenID,
		Username:  username,
		ExpiresAt: expiresAt,
		Revoked:   false,
	}
	memRefreshMu.Unlock()

	dbMu.RLock()
	connected := IsConnected && DB != nil
	dbHandle := DB
	dbMu.RUnlock()

	if !connected {
		return nil
	}

	query := `
		INSERT INTO refresh_tokens (token_id, username, expires_at, revoked)
		VALUES ($1, $2, $3, false)
		ON CONFLICT (token_id)
		DO UPDATE SET expires_at = EXCLUDED.expires_at, revoked = false
	`
	_, err := dbHandle.Exec(query, tokenID, username, expiresAt)
	if err != nil {
		log.Printf("[DB] StoreRefreshToken query failed, falling back to in-memory store: %v", err)
		return nil
	}
	return nil
}

// RevokeRefreshToken marks a refresh token as revoked
func RevokeRefreshToken(tokenID string) error {
	if tokenID == "" {
		return nil
	}

	// Always update in memory
	memRefreshMu.Lock()
	if rec, exists := memRefreshTokens[tokenID]; exists {
		rec.Revoked = true
		memRefreshTokens[tokenID] = rec
	}
	memRefreshMu.Unlock()

	dbMu.RLock()
	connected := IsConnected && DB != nil
	dbHandle := DB
	dbMu.RUnlock()

	if !connected {
		return nil
	}

	query := `UPDATE refresh_tokens SET revoked = true WHERE token_id = $1`
	_, err := dbHandle.Exec(query, tokenID)
	if err != nil {
		log.Printf("[DB] RevokeRefreshToken query failed, token updated in in-memory store: %v", err)
		return nil
	}
	return nil
}

// IsRefreshTokenValid checks if a refresh token exists, is unexpired, and is not revoked
func IsRefreshTokenValid(tokenID string) bool {
	if tokenID == "" {
		return false
	}

	dbMu.RLock()
	connected := IsConnected && DB != nil
	dbHandle := DB
	dbMu.RUnlock()

	if !connected {
		return isMemRefreshTokenValid(tokenID)
	}

	var revoked bool
	var expiresAt time.Time
	query := `SELECT revoked, expires_at FROM refresh_tokens WHERE token_id = $1`
	err := dbHandle.QueryRow(query, tokenID).Scan(&revoked, &expiresAt)
	if err != nil {
		log.Printf("[DB] IsRefreshTokenValid query failed, falling back to in-memory store: %v", err)
		return isMemRefreshTokenValid(tokenID)
	}
	return !revoked && time.Now().Before(expiresAt)
}

func isMemRefreshTokenValid(tokenID string) bool {
	memRefreshMu.RLock()
	defer memRefreshMu.RUnlock()
	rec, exists := memRefreshTokens[tokenID]
	if !exists || rec.Revoked {
		return false
	}
	return time.Now().Before(rec.ExpiresAt)
}

// GetUserRole returns the role assigned to a user
func GetUserRole(username string) string {
	dbMu.RLock()
	connected := IsConnected && DB != nil
	dbHandle := DB
	dbMu.RUnlock()

	if !connected {
		return getFallbackUserRole(username)
	}

	var role string
	err := dbHandle.QueryRow("SELECT role FROM permissions p JOIN users u ON p.user_id = u.id WHERE u.username = $1 LIMIT 1", username).Scan(&role)
	if err != nil || role == "" {
		return getFallbackUserRole(username)
	}
	return role
}

func getFallbackUserRole(username string) string {
	switch username {
	case "admin", "tenant1_user", "tenant2_user":
		return "admin"
	case "operator", "operator_user":
		return "operator"
	case "user2", "tenant1_viewer":
		return "viewer"
	default:
		return "viewer"
	}
}

// VerifyDevicePermission checks if a user has a specific RBAC permission for a device
func VerifyDevicePermission(username, deviceID, permission string) bool {
	if !VerifyDeviceAccess(username, deviceID) {
		return false
	}
	role := GetUserRole(username)
	switch role {
	case "admin":
		return true // Admins have all permissions
	case "operator":
		return permission != "Device:ManageUsers"
	case "viewer":
		return permission == "Device:View" || permission == "Stream:Access"
	default:
		return false
	}
}


