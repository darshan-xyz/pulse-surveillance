package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"hss/internal/database"
	"log"
	"os"
	"path/filepath"
)

var DeviceID string
var PublicKeyPEM string

func InitIdentity() error {
	secretsDir := filepath.Join(".", "data", "secrets")
	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return err
	}

	var devID, pubKey string
	err := database.DB.QueryRow("SELECT value FROM settings WHERE key = 'device_id'").Scan(&devID)
	if err == nil {
		database.DB.QueryRow("SELECT value FROM settings WHERE key = 'public_key'").Scan(&pubKey)
		DeviceID = devID
		PublicKeyPEM = pubKey
		log.Printf("Loaded existing Device ID: %s\n", DeviceID)
		return nil
	}

	log.Println("No identity found, generating new Ed25519 keypair...")
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	devIDBytes := make([]byte, 12)
	rand.Read(devIDBytes)
	DeviceID = "hss_" + hex.EncodeToString(devIDBytes)
	PublicKeyPEM = string(pubPEM)

	_, err = database.DB.Exec("INSERT INTO settings (key, value) VALUES ('device_id', ?)", DeviceID)
	if err != nil {
		return err
	}
	_, err = database.DB.Exec("INSERT INTO settings (key, value) VALUES ('public_key', ?)", PublicKeyPEM)
	if err != nil {
		return err
	}

	privPath := filepath.Join(secretsDir, "private.pem")
	if err := os.WriteFile(privPath, privPEM, 0600); err != nil {
		return err
	}

	log.Printf("Generated new Device ID: %s\n", DeviceID)
	return nil
}
