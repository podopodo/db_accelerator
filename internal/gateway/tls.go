package gateway

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/podopodo/db_accelerator/internal/config"
)

type clientCertificateStore struct {
	certFile string
	keyFile  string
	now      func() time.Time

	mu       sync.RWMutex
	notAfter time.Time
}

func newClientTLS(server config.ServerConfig) (*tls.Config, *clientCertificateStore, error) {
	if server.MySQLTLSMode == "disabled" {
		return nil, nil, nil
	}
	if server.MySQLTLSMode != "required" {
		return nil, nil, fmt.Errorf("unsupported client TLS mode %q", server.MySQLTLSMode)
	}
	store := &clientCertificateStore{certFile: server.MySQLTLSCertFile, keyFile: server.MySQLTLSKeyFile, now: time.Now}
	if _, err := store.load(); err != nil {
		return nil, nil, err
	}
	configuration := &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: store.getCertificate,
	}
	return configuration, store, nil
}

func (s *clientCertificateStore) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return s.load()
}

func (s *clientCertificateStore) load() (*tls.Certificate, error) {
	certificate, err := tls.LoadX509KeyPair(s.certFile, s.keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client TLS certificate: %w", err)
	}
	if len(certificate.Certificate) == 0 {
		return nil, errors.New("client TLS certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse client TLS certificate: %w", err)
	}
	now := s.now()
	if now.Before(leaf.NotBefore) {
		return nil, fmt.Errorf("client TLS certificate is not valid before %s", leaf.NotBefore.UTC().Format(time.RFC3339))
	}
	if !now.Before(leaf.NotAfter) {
		return nil, fmt.Errorf("client TLS certificate expired at %s", leaf.NotAfter.UTC().Format(time.RFC3339))
	}
	certificate.Leaf = leaf
	s.mu.Lock()
	s.notAfter = leaf.NotAfter.UTC()
	s.mu.Unlock()
	return &certificate, nil
}

func (s *clientCertificateStore) expiry() *time.Time {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.notAfter.IsZero() {
		return nil
	}
	value := s.notAfter
	return &value
}
