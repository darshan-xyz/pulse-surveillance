package security

import (
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// DefaultJWTSecret is the default symmetric secret key used for development / demo mode.
const DefaultJWTSecret = "super-secret-key-for-dev"

// GetJWTSecret returns the secret key used for signing and verifying viewer JWT tokens.
func GetJWTSecret() []byte {
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		return []byte(secret)
	}
	return []byte(DefaultJWTSecret)
}

// GenerateDeviceToken creates a short-lived JWT proving the device's identity
// to the Central Control Plane. It uses the device's local Ed25519 private key.
func GenerateDeviceToken() (string, error) {
	// Create claims
	claims := jwt.RegisteredClaims{
		Subject:   DeviceID,
		Issuer:    DeviceID,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
	}

	// Sign the token with Ed25519
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	
	// PrivateKey was loaded in InitIdentity()
	tokenString, err := token.SignedString(PrivateKey)
	if err != nil {
		return "", err
	}
	
	return tokenString, nil
}

