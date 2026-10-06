package security

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateMTLSCredentials(t *testing.T) {
	creds, err := GenerateMTLSCredentials()
	if err != nil {
		t.Fatalf("Failed to generate mTLS credentials: %v", err)
	}

	if len(creds.CACertPEM) == 0 || len(creds.CAKeyPEM) == 0 {
		t.Fatal("CA credentials are empty")
	}
	if len(creds.ServerCertPEM) == 0 || len(creds.ServerKeyPEM) == 0 {
		t.Fatal("Server credentials are empty")
	}
	if len(creds.ClientCertPEM) == 0 || len(creds.ClientKeyPEM) == 0 {
		t.Fatal("Client credentials are empty")
	}
	if len(creds.UntrustedClientPEM) == 0 || len(creds.UntrustedKeyPEM) == 0 {
		t.Fatal("Untrusted credentials are empty")
	}
}

func TestServerMTLSConfiguration(t *testing.T) {
	creds, err := GenerateMTLSCredentials()
	if err != nil {
		t.Fatalf("Failed to generate credentials: %v", err)
	}

	serverConfig, err := NewServerMTLSConfig(creds.CACertPEM, creds.ServerCertPEM, creds.ServerKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create server mTLS config: %v", err)
	}

	if serverConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("Expected ClientAuth to be RequireAndVerifyClientCert, got: %v", serverConfig.ClientAuth)
	}
	if serverConfig.ClientCAs == nil {
		t.Fatal("Expected ClientCAs pool to be set")
	}
}

func TestMTLSHandshakeSuccess(t *testing.T) {
	creds, err := GenerateMTLSCredentials()
	if err != nil {
		t.Fatalf("Failed to generate credentials: %v", err)
	}

	serverConfig, err := NewServerMTLSConfig(creds.CACertPEM, creds.ServerCertPEM, creds.ServerKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create server mTLS config: %v", err)
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("mTLS Authenticated OK"))
	}))
	ts.TLS = serverConfig
	ts.StartTLS()
	defer ts.Close()

	// Configure client with valid cert
	clientConfig, err := NewClientMTLSConfig(creds.CACertPEM, creds.ClientCertPEM, creds.ClientKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create client mTLS config: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientConfig,
		},
	}

	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("mTLS client failed to connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got: %d", resp.StatusCode)
	}
	t.Log("PASS: Authorized mTLS client successfully connected and authenticated")
}

func TestMTLSHandshakeRejectsNoCert(t *testing.T) {
	creds, err := GenerateMTLSCredentials()
	if err != nil {
		t.Fatalf("Failed to generate credentials: %v", err)
	}

	serverConfig, err := NewServerMTLSConfig(creds.CACertPEM, creds.ServerCertPEM, creds.ServerKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create server mTLS config: %v", err)
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts.TLS = serverConfig
	ts.StartTLS()
	defer ts.Close()

	// Client with trusted CA but NO client certificate
	clientConfig, err := NewClientMTLSConfig(creds.CACertPEM, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create client config: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientConfig,
		},
	}

	_, err = client.Get(ts.URL)
	if err == nil {
		t.Fatal("SECURITY DEFECT: Server accepted connection lacking client certificate!")
	}
	t.Logf("PASS: Connection without client certificate correctly rejected by mTLS: %v", err)
}

func TestMTLSHandshakeRejectsRogueCert(t *testing.T) {
	creds, err := GenerateMTLSCredentials()
	if err != nil {
		t.Fatalf("Failed to generate credentials: %v", err)
	}

	serverConfig, err := NewServerMTLSConfig(creds.CACertPEM, creds.ServerCertPEM, creds.ServerKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create server mTLS config: %v", err)
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts.TLS = serverConfig
	ts.StartTLS()
	defer ts.Close()

	// Client presenting untrusted rogue certificate
	clientConfig, err := NewClientMTLSConfig(creds.CACertPEM, creds.UntrustedClientPEM, creds.UntrustedKeyPEM)
	if err != nil {
		t.Fatalf("Failed to create client config: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientConfig,
		},
	}

	_, err = client.Get(ts.URL)
	if err == nil {
		t.Fatal("SECURITY DEFECT: Server accepted untrusted/rogue client certificate!")
	}
	if !strings.Contains(err.Error(), "tls") && !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "remote error") {
		t.Logf("Handshake error: %v", err)
	}
	t.Logf("PASS: Rogue client certificate correctly rejected by mTLS: %v", err)
}
