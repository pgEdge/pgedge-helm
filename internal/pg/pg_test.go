// internal/pg/pg_test.go
package pg

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildConnConfig(t *testing.T) {
	certPath, keyPath := generateTempCerts(t)

	cfg, err := buildConnConfig("pgedge-n1-rw", "app", "admin", certPath, keyPath)
	if err != nil {
		t.Fatalf("buildConnConfig: %v", err)
	}
	if cfg.Host != "pgedge-n1-rw" {
		t.Errorf("Host: got %q", cfg.Host)
	}
	if cfg.Database != "app" {
		t.Errorf("Database: got %q", cfg.Database)
	}
	if cfg.User != "admin" {
		t.Errorf("User: got %q", cfg.User)
	}
	if cfg.Port != 5432 {
		t.Errorf("Port: got %d", cfg.Port)
	}
	if cfg.TLSConfig == nil {
		t.Error("expected TLS config to be set")
	}
}

func TestBuildConnConfigMissingCerts(t *testing.T) {
	_, err := buildConnConfig("host", "db", "user",
		"/nonexistent/tls.crt", "/nonexistent/tls.key")
	if err == nil {
		t.Error("expected error for missing cert files")
	}
}

func TestConnectHost(t *testing.T) {
	host := connectHost("external", "internal")
	if host != "internal" {
		t.Errorf("expected internal, got %q", host)
	}

	host = connectHost("external", "")
	if host != "external" {
		t.Errorf("expected external, got %q", host)
	}
}

// generateTempCerts creates a temporary self-signed certificate and key for testing.
func generateTempCerts(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath = filepath.Join(dir, "tls.crt")
	keyPath = filepath.Join(dir, "tls.key")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}
