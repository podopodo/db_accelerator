package upstream

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/podopodo/db_accelerator/internal/config"
)

func TestNewDoesNotExposePasswordOrDSN(t *testing.T) {
	cfg := config.Default()
	cfg.Upstream.Enabled = true
	cfg.Upstream.TLSMode = "disabled"
	secrets, err := config.ResolveSecrets(cfg, func(string) (string, bool) { return "secret-canary", true })
	if err != nil {
		t.Fatalf("resolve secrets: %v", err)
	}
	connector, err := New(cfg, secrets)
	if err != nil {
		t.Fatalf("new connector: %v", err)
	}
	printed := fmt.Sprintf("%+v %#v", connector, connector)
	if strings.Contains(printed, "secret-canary") || strings.Contains(printed, cfg.Upstream.User+":") {
		t.Fatalf("connector formatting leaked credentials or DSN: %s", printed)
	}
}

func TestOpenReturnsFreshBoundedHandles(t *testing.T) {
	cfg := config.Default()
	cfg.Upstream.Enabled = true
	cfg.Upstream.TLSMode = "disabled"
	secrets, err := config.ResolveSecrets(cfg, func(string) (string, bool) { return "test", true })
	if err != nil {
		t.Fatal(err)
	}
	connector, err := New(cfg, secrets)
	if err != nil {
		t.Fatal(err)
	}
	first, err := connector.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := connector.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if first == second {
		t.Fatal("open reused a database handle")
	}
	if first.Stats().MaxOpenConnections != 1 || second.Stats().MaxOpenConnections != 1 {
		t.Fatal("connector handle is not bounded to one physical connection")
	}
}

func TestProbeClassifiesUnavailableServerWithoutSecretLeak(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Upstream.Enabled = true
	cfg.Upstream.TLSMode = "disabled"
	cfg.Upstream.Port = port
	cfg.Upstream.ConnectTimeout = "500ms"
	secrets, err := config.ResolveSecrets(cfg, func(string) (string, bool) { return "secret-canary", true })
	if err != nil {
		t.Fatal(err)
	}
	connector, err := New(cfg, secrets)
	if err != nil {
		t.Fatal(err)
	}
	_, err = connector.Probe(context.Background())
	var upstreamError *Error
	if !errors.As(err, &upstreamError) || upstreamError.Kind != KindNetwork {
		t.Fatalf("unavailable server error = %v", err)
	}
	if strings.Contains(err.Error(), "secret-canary") {
		t.Fatalf("error leaked secret: %v", err)
	}
}

func TestBuildTLSConfigModes(t *testing.T) {
	cfg := config.Default().Upstream
	for _, test := range []struct {
		mode         string
		wantTLS      bool
		wantFallback bool
		wantInsecure bool
	}{
		{mode: "disabled"},
		{mode: "preferred", wantTLS: true, wantFallback: true, wantInsecure: true},
		{mode: "required", wantTLS: true, wantInsecure: true},
	} {
		cfg.TLSMode = test.mode
		tlsConfig, fallback, err := buildTLSConfig(cfg)
		if err != nil {
			t.Fatalf("mode %s: %v", test.mode, err)
		}
		if (tlsConfig != nil) != test.wantTLS || fallback != test.wantFallback {
			t.Fatalf("mode %s: tls=%v fallback=%v", test.mode, tlsConfig != nil, fallback)
		}
		if tlsConfig != nil && tlsConfig.InsecureSkipVerify != test.wantInsecure {
			t.Fatalf("mode %s: insecure=%v", test.mode, tlsConfig.InsecureSkipVerify)
		}
	}
}

func TestBuildTLSConfigRejectsBadCA(t *testing.T) {
	cfg := config.Default().Upstream
	cfg.TLSMode = "verify-full"
	cfg.TLSCAFile = filepath.Join(t.TempDir(), "bad-ca.pem")
	if err := os.WriteFile(cfg.TLSCAFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := buildTLSConfig(cfg); err == nil {
		t.Fatal("invalid CA was accepted")
	}
}

func TestClassifyStableKinds(t *testing.T) {
	tests := []struct {
		name string
		err  error
		kind ErrorKind
	}{
		{name: "deadline", err: context.DeadlineExceeded, kind: KindTimeout},
		{name: "network", err: &net.DNSError{Err: "no such host", Name: "db.invalid"}, kind: KindNetwork},
		{name: "network timeout", err: timeoutError{}, kind: KindTimeout},
		{name: "auth", err: &driver.MySQLError{Number: 1045, Message: "access denied"}, kind: KindAuthentication},
		{name: "server", err: &driver.MySQLError{Number: 1064, Message: "syntax"}, kind: KindServer},
		{name: "tls", err: x509.UnknownAuthorityError{}, kind: KindTLS},
		{name: "no tls", err: driver.ErrNoTLS, kind: KindTLS},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classified := Classify("test", test.err)
			var upstreamError *Error
			if !errors.As(classified, &upstreamError) || upstreamError.Kind != test.kind {
				t.Fatalf("classified %T %v, want %s", classified, classified, test.kind)
			}
		})
	}
}

func TestDetectVendor(t *testing.T) {
	for _, test := range []struct{ version, comment, want string }{
		{"8.4.10", "MySQL Community Server - GPL", "mysql"},
		{"11.4.12-MariaDB", "mariadb.org binary distribution", "mariadb"},
		{"8.0.11-TiDB", "TiDB Server", "unsupported"},
		{"unknown", "custom", "unsupported"},
	} {
		if got := detectVendor(test.version, test.comment); got != test.want {
			t.Fatalf("detectVendor(%q, %q)=%q want %q", test.version, test.comment, got, test.want)
		}
	}
}

func TestBoundedContextKeepsEarlierDeadline(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelParent()
	child, cancelChild := boundedContext(parent, time.Second)
	defer cancelChild()
	parentDeadline, _ := parent.Deadline()
	childDeadline, _ := child.Deadline()
	if !parentDeadline.Equal(childDeadline) {
		t.Fatalf("child deadline %v differs from earlier parent %v", childDeadline, parentDeadline)
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}
