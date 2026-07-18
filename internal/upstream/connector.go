// Package upstream opens and verifies direct MySQL and MariaDB connections.
package upstream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/podopodo/db_accelerator/internal/config"
)

// ErrorKind is a stable operational category. Callers must not branch on
// driver text.
type ErrorKind string

const (
	KindConfiguration  ErrorKind = "configuration"
	KindNetwork        ErrorKind = "network"
	KindAuthentication ErrorKind = "authentication"
	KindTimeout        ErrorKind = "timeout"
	KindTLS            ErrorKind = "tls"
	KindServer         ErrorKind = "server"
)

// Error carries a stable category and keeps the original error for diagnosis.
type Error struct {
	Kind      ErrorKind
	Operation string
	Err       error
}

func (e *Error) Error() string {
	if e == nil {
		return "upstream operation failed"
	}
	if e.Err == nil {
		return fmt.Sprintf("upstream %s failed (%s)", e.Operation, e.Kind)
	}
	return fmt.Sprintf("upstream %s failed (%s): %v", e.Operation, e.Kind, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// Connector owns immutable, secret-bearing driver configuration. It does not
// expose or format the password or a DSN.
type Connector struct {
	driverConfig *driver.Config
	healthLimit  time.Duration
	address      string
}

func (c *Connector) String() string   { return "upstream connector " + c.address }
func (c *Connector) GoString() string { return c.String() }

// New creates a connector without opening a socket.
func New(cfg config.Config, secrets config.Secrets) (*Connector, error) {
	if !cfg.Upstream.Enabled {
		return nil, &Error{Kind: KindConfiguration, Operation: "configure", Err: errors.New("upstream is disabled")}
	}
	if err := cfg.Validate(); err != nil {
		return nil, &Error{Kind: KindConfiguration, Operation: "configure", Err: err}
	}
	connectLimit, readLimit, writeLimit, healthLimit, err := cfg.UpstreamDurations()
	if err != nil {
		return nil, &Error{Kind: KindConfiguration, Operation: "configure", Err: err}
	}

	tlsConfig, allowPlaintextFallback, err := buildTLSConfig(cfg.Upstream)
	if err != nil {
		return nil, &Error{Kind: KindConfiguration, Operation: "configure tls", Err: err}
	}
	address := net.JoinHostPort(cfg.Upstream.Host, strconv.Itoa(cfg.Upstream.Port))
	mysqlConfig := driver.NewConfig()
	mysqlConfig.User = cfg.Upstream.User
	mysqlConfig.Passwd = secrets.UpstreamPassword.Reveal()
	mysqlConfig.Net = "tcp"
	mysqlConfig.Addr = address
	mysqlConfig.DBName = cfg.Upstream.Database
	mysqlConfig.Timeout = connectLimit
	mysqlConfig.ReadTimeout = readLimit
	mysqlConfig.WriteTimeout = writeLimit
	mysqlConfig.TLS = tlsConfig
	mysqlConfig.AllowFallbackToPlaintext = allowPlaintextFallback
	mysqlConfig.ParseTime = true
	mysqlConfig.MultiStatements = false
	mysqlConfig.AllowAllFiles = false
	mysqlConfig.AllowCleartextPasswords = false
	mysqlConfig.AllowOldPasswords = false
	mysqlConfig.Logger = &driver.NopLogger{}

	return &Connector{driverConfig: mysqlConfig, healthLimit: healthLimit, address: address}, nil
}

func buildTLSConfig(cfg config.UpstreamConfig) (*tls.Config, bool, error) {
	mode := strings.ToLower(cfg.TLSMode)
	if mode == "disabled" {
		return nil, false, nil
	}
	serverName := strings.TrimSpace(cfg.TLSServerName)
	if serverName == "" {
		serverName = cfg.Host
	}
	base := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}

	switch mode {
	case "preferred":
		// Preferred means encryption when offered, but permits an old server to
		// fall back. Identity is not promised by this explicit policy mode.
		base.InsecureSkipVerify = true // #nosec G402 -- selected policy does not verify identity.
		return base, true, nil
	case "required":
		// Required guarantees encryption only. Use verify-full for identity.
		base.InsecureSkipVerify = true // #nosec G402 -- selected policy does not verify identity.
		return base, false, nil
	case "verify-ca", "verify-full":
		roots, err := loadRoots(cfg.TLSCAFile)
		if err != nil {
			return nil, false, err
		}
		base.RootCAs = roots
		if mode == "verify-full" {
			return base, false, nil
		}
		base.InsecureSkipVerify = true // #nosec G402 -- chain is verified below without hostname.
		base.VerifyConnection = verifyCertificateChain(roots)
		return base, false, nil
	default:
		return nil, false, fmt.Errorf("unsupported tls mode %q", cfg.TLSMode)
	}
}

func loadRoots(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("CA file contains no certificates")
	}
	return roots, nil
}

func verifyCertificateChain(roots *x509.CertPool) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("server supplied no certificate")
		}
		intermediates := x509.NewCertPool()
		for _, certificate := range state.PeerCertificates[1:] {
			intermediates.AddCert(certificate)
		}
		_, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{
			Roots:         roots,
			Intermediates: intermediates,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		return err
	}
}

// Open returns a one-connection database handle. Ping is performed by Probe,
// where one physical connection is retained for every metadata query.
func (c *Connector) Open() (*sql.DB, error) {
	return c.OpenPool(1)
}

// OpenPool returns a bounded database/sql pool for the protocol-aware gateway.
// Session-changing SQL must never be returned to this shared pool without a
// verified reset; the gateway therefore accepts only its safe statement set.
func (c *Connector) OpenPool(maxOpen int) (*sql.DB, error) {
	if maxOpen <= 0 {
		return nil, &Error{Kind: KindConfiguration, Operation: "configure pool", Err: errors.New("max open connections must be positive")}
	}
	connector, err := driver.NewConnector(c.driverConfig.Clone())
	if err != nil {
		return nil, classify("create connector", err)
	}
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(maxOpen)
	database.SetMaxIdleConns(maxOpen)
	database.SetConnMaxIdleTime(30 * time.Second)
	database.SetConnMaxLifetime(30 * time.Minute)
	return database, nil
}

// Classify converts a driver or network failure into a stable category.
func Classify(operation string, err error) error { return classify(operation, err) }

func classify(operation string, err error) error {
	if err == nil {
		return nil
	}
	var existing *Error
	if errors.As(err, &existing) {
		return existing
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &Error{Kind: KindTimeout, Operation: operation, Err: err}
	}
	var netError net.Error
	if errors.As(err, &netError) {
		kind := KindNetwork
		if netError.Timeout() {
			kind = KindTimeout
		}
		return &Error{Kind: kind, Operation: operation, Err: err}
	}
	var certificateVerification *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	var certificateInvalid x509.CertificateInvalidError
	var recordHeader tls.RecordHeaderError
	if errors.As(err, &certificateVerification) || errors.As(err, &unknownAuthority) || errors.As(err, &hostnameError) || errors.As(err, &certificateInvalid) || errors.As(err, &recordHeader) || errors.Is(err, driver.ErrNoTLS) {
		return &Error{Kind: KindTLS, Operation: operation, Err: err}
	}
	var mysqlError *driver.MySQLError
	if errors.As(err, &mysqlError) {
		kind := KindServer
		switch mysqlError.Number {
		case 1044, 1045, 1698:
			kind = KindAuthentication
		}
		return &Error{Kind: kind, Operation: operation, Err: err}
	}
	return &Error{Kind: KindServer, Operation: operation, Err: err}
}
