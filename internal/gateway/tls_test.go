package gateway

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/podopodo/db_accelerator/internal/config"
)

func TestClientTLSCertificateReloadAndExpiry(t *testing.T) {
	now := time.Date(2026, time.July, 19, 0, 0, 0, 0, time.UTC)
	directory := t.TempDir()
	certificatePath := filepath.Join(directory, "server.crt")
	keyPath := filepath.Join(directory, "server.key")
	firstCertificate := writeGatewayCertificate(t, certificatePath, keyPath, now.Add(-time.Hour), now.Add(24*time.Hour), 1)
	server := config.Default().Server
	server.MySQLTLSMode = "required"
	server.MySQLTLSCertFile = certificatePath
	server.MySQLTLSKeyFile = keyPath
	tlsConfig, store, err := newClientTLS(server)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	loaded, err := tlsConfig.GetCertificate(nil)
	if err != nil || loaded.Leaf.SerialNumber.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("first certificate = %+v err=%v", loaded.Leaf, err)
	}
	if expiry := store.expiry(); expiry == nil || !expiry.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("certificate expiry = %v", expiry)
	}

	secondCertificate := writeGatewayCertificate(t, certificatePath, keyPath, now.Add(-time.Hour), now.Add(48*time.Hour), 2)
	loaded, err = tlsConfig.GetCertificate(nil)
	if err != nil || loaded.Leaf.SerialNumber.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("rotated certificate = %+v err=%v", loaded.Leaf, err)
	}
	if string(firstCertificate) == string(secondCertificate) {
		t.Fatal("certificate fixture did not rotate")
	}

	writeGatewayCertificate(t, certificatePath, keyPath, now.Add(-48*time.Hour), now.Add(-time.Hour), 3)
	if _, err := tlsConfig.GetCertificate(nil); err == nil {
		t.Fatal("expired rotated certificate was accepted")
	}
}

func TestClientTLSDisabledHasNoCertificate(t *testing.T) {
	tlsConfig, store, err := newClientTLS(config.Default().Server)
	if err != nil || tlsConfig != nil || store != nil {
		t.Fatalf("disabled TLS config=%v store=%v err=%v", tlsConfig, store, err)
	}
}

func configureGatewayTestTLS(t *testing.T, cfg *config.Config) {
	t.Helper()
	now := time.Now().UTC()
	directory := t.TempDir()
	certificatePath := filepath.Join(directory, "server.crt")
	keyPath := filepath.Join(directory, "server.key")
	writeGatewayCertificate(t, certificatePath, keyPath, now.Add(-time.Hour), now.Add(24*time.Hour), 21188)
	cfg.Server.MySQLTLSMode = "required"
	cfg.Server.MySQLTLSCertFile = certificatePath
	cfg.Server.MySQLTLSKeyFile = keyPath
}

func gatewayTestClientTLS(t *testing.T, cfg config.Config) *tls.Config {
	t.Helper()
	certificate, err := os.ReadFile(cfg.Server.MySQLTLSCertFile)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificate) {
		t.Fatal("test certificate was not added to roots")
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "127.0.0.1"}
}

func gatewayUntrustedClientTLS() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: x509.NewCertPool(), ServerName: "127.0.0.1"}
}

func writeGatewayCertificate(t *testing.T, certificatePath, keyPath string, notBefore, notAfter time.Time, serial int64) []byte {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: "Database Accelerator test"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	key := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	if err := os.WriteFile(certificatePath, certificate, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	return certificate
}
