package security

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
)

// MTLSCredentials holds PEM encoded certificates and keys for mTLS setup
type MTLSCredentials struct {
	CACertPEM          []byte
	CAKeyPEM           []byte
	ServerCertPEM      []byte
	ServerKeyPEM       []byte
	ClientCertPEM      []byte
	ClientKeyPEM       []byte
	UntrustedClientPEM []byte
	UntrustedKeyPEM    []byte
}

// NewServerMTLSConfig creates a *tls.Config enforcing strict mutual TLS (ClientAuth: tls.RequireAndVerifyClientCert)
func NewServerMTLSConfig(caCertPEM, serverCertPEM, serverKeyPEM []byte) (*tls.Config, error) {
	if len(caCertPEM) == 0 {
		return nil, errors.New("caCertPEM is required for mTLS")
	}
	if len(serverCertPEM) == 0 || len(serverKeyPEM) == 0 {
		return nil, errors.New("server certificate and key are required")
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		return nil, errors.New("failed to parse CA certificate PEM")
	}

	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load server keypair: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
	}

	return tlsConfig, nil
}

// NewClientMTLSConfig creates a *tls.Config for an Edge client presenting a certificate to the server
func NewClientMTLSConfig(caCertPEM, clientCertPEM, clientKeyPEM []byte) (*tls.Config, error) {
	if len(caCertPEM) == 0 {
		return nil, errors.New("caCertPEM is required for mTLS")
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		return nil, errors.New("failed to parse CA certificate PEM")
	}

	var certs []tls.Certificate
	if len(clientCertPEM) > 0 && len(clientKeyPEM) > 0 {
		clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to load client keypair: %w", err)
		}
		certs = append(certs, clientCert)
	}

	tlsConfig := &tls.Config{
		Certificates: certs,
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	return tlsConfig, nil
}

// GenerateMTLSCredentials creates a complete set of test/production certificates
// including CA, Server certificate, authorized Client certificate, and an untrusted Client certificate.
func GenerateMTLSCredentials() (*MTLSCredentials, error) {
	// 1. Generate Root CA
	caPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1000),
		Subject: pkix.Name{
			Organization:  []string{"Pulse Cloud Security"},
			CommonName:    "Pulse Root CA",
			Country:       []string{"US"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	caCertBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, err
	}

	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertBytes})
	caPrivKeyBytes, err := x509.MarshalECPrivateKey(caPrivKey)
	if err != nil {
		return nil, err
	}
	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caPrivKeyBytes})

	// 2. Generate Server Certificate signed by CA
	serverPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2000),
		Subject: pkix.Name{
			Organization: []string{"Pulse Cloud"},
			CommonName:   "localhost",
		},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
		DNSNames:     []string{"localhost", "pulse-cloud", "pulse.local"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	serverCertBytes, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, err
	}

	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertBytes})
	serverKeyBytes, err := x509.MarshalECPrivateKey(serverPrivKey)
	if err != nil {
		return nil, err
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyBytes})

	// 3. Generate Valid Edge Client Certificate signed by CA
	clientPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3000),
		Subject: pkix.Name{
			Organization: []string{"Pulse Edge Devices"},
			CommonName:   "edge-daemon-01",
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientCertBytes, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, err
	}

	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertBytes})
	clientKeyBytes, err := x509.MarshalECPrivateKey(clientPrivKey)
	if err != nil {
		return nil, err
	}
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyBytes})

	// 4. Generate Untrusted Client Certificate signed by a Rogue/Different CA
	roguePrivKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rogueCATemplate := &x509.Certificate{
		SerialNumber: big.NewInt(9999),
		Subject: pkix.Name{
			Organization: []string{"Rogue Attacker CA"},
			CommonName:   "Rogue Root CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	rogueCACertBytes, _ := x509.CreateCertificate(rand.Reader, rogueCATemplate, rogueCATemplate, &roguePrivKey.PublicKey, roguePrivKey)

	rogueClientPrivKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rogueClientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(9998),
		Subject: pkix.Name{
			Organization: []string{"Attacker Device"},
			CommonName:   "malicious-edge-daemon",
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	rogueCertBytes, _ := x509.CreateCertificate(rand.Reader, rogueClientTemplate, rogueCATemplate, &rogueClientPrivKey.PublicKey, roguePrivKey)
	untrustedClientPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rogueCertBytes})
	rogueClientKeyBytes, _ := x509.MarshalECPrivateKey(rogueClientPrivKey)
	untrustedKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: rogueClientKeyBytes})
	_ = rogueCACertBytes

	return &MTLSCredentials{
		CACertPEM:          caCertPEM,
		CAKeyPEM:           caKeyPEM,
		ServerCertPEM:      serverCertPEM,
		ServerKeyPEM:       serverKeyPEM,
		ClientCertPEM:      clientCertPEM,
		ClientKeyPEM:       clientKeyPEM,
		UntrustedClientPEM: untrustedClientPEM,
		UntrustedKeyPEM:    untrustedKeyPEM,
	}, nil
}

// LoadMTLSCredentialsFromFiles loads CA and keypair PEMs from given file paths
func LoadMTLSCredentialsFromFiles(caPath, certPath, keyPath string) ([]byte, []byte, []byte, error) {
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read CA file: %w", err)
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read Cert file: %w", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read Key file: %w", err)
	}
	return caPEM, certPEM, keyPEM, nil
}

// VerifyClientCertificate checks if a client certificate was verified by the trusted CA
func VerifyClientCertificate(r *tls.ConnectionState, caCertPEM []byte) error {
	if r == nil || len(r.PeerCertificates) == 0 {
		return errors.New("mTLS error: no client certificate provided")
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		return errors.New("mTLS error: invalid CA certificate pool")
	}

	opts := x509.VerifyOptions{
		Roots:         caCertPool,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		CurrentTime:   time.Now(),
		Intermediates: x509.NewCertPool(),
	}

	for _, cert := range r.PeerCertificates[1:] {
		opts.Intermediates.AddCert(cert)
	}

	clientCert := r.PeerCertificates[0]
	if _, err := clientCert.Verify(opts); err != nil {
		return fmt.Errorf("mTLS error: client certificate verification failed: %w", err)
	}

	return nil
}

// Ensure bytes buffer isn't unused
var _ = bytes.NewBuffer
