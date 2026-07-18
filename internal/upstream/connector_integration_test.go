package upstream

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/podopodo/db_accelerator/internal/config"
)

// These tests are dormant in the unit lane. The compatibility lane supplies
// one prefix at a time, for example DBA_TEST_MYSQL_* or DBA_TEST_MARIADB_*.
func TestIntegrationProbe(t *testing.T) {
	prefix := os.Getenv("DBA_TEST_SERVER_PREFIX")
	if prefix == "" {
		t.Skip("set DBA_TEST_SERVER_PREFIX to DBA_TEST_MYSQL or DBA_TEST_MARIADB")
	}
	cfg, secrets := integrationConfiguration(t, prefix)
	connector, err := New(cfg, secrets)
	if err != nil {
		t.Fatalf("new connector: %v", err)
	}
	report, err := connector.Probe(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if report.Status != "ok" || report.Metadata.Vendor == "unsupported" || report.Metadata.Version == "" {
		t.Fatalf("invalid report: %+v", report)
	}
}

func TestIntegrationBadPassword(t *testing.T) {
	prefix := os.Getenv("DBA_TEST_SERVER_PREFIX")
	if prefix == "" {
		t.Skip("set DBA_TEST_SERVER_PREFIX to enable integration tests")
	}
	cfg, _ := integrationConfiguration(t, prefix)
	secrets, err := config.ResolveSecrets(cfg, func(string) (string, bool) { return "definitely-wrong-password", true })
	if err != nil {
		t.Fatal(err)
	}
	connector, err := New(cfg, secrets)
	if err != nil {
		t.Fatal(err)
	}
	_, err = connector.Probe(context.Background())
	var upstreamError *Error
	if !errors.As(err, &upstreamError) || upstreamError.Kind != KindAuthentication {
		t.Fatalf("bad password error = %v", err)
	}
}

func integrationConfiguration(t *testing.T, prefix string) (config.Config, config.Secrets) {
	t.Helper()
	required := func(suffix string) string {
		value := os.Getenv(prefix + "_" + suffix)
		if value == "" {
			t.Fatalf("%s_%s is required", prefix, suffix)
		}
		return value
	}
	port, err := strconv.Atoi(required("PORT"))
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	cfg := config.Default()
	cfg.Upstream.Enabled = true
	cfg.Upstream.Host = required("HOST")
	cfg.Upstream.Port = port
	cfg.Upstream.User = required("USER")
	cfg.Upstream.Database = os.Getenv(prefix + "_DATABASE")
	cfg.Upstream.TLSMode = envDefault(prefix+"_TLS_MODE", "disabled")
	cfg.Upstream.TLSCAFile = os.Getenv(prefix + "_TLS_CA_FILE")
	cfg.Upstream.TLSServerName = os.Getenv(prefix + "_TLS_SERVER_NAME")
	allowEmptyPassword := os.Getenv(prefix+"_ALLOW_EMPTY_PASSWORD") == "true"
	cfg.Upstream.AllowEmptyPassword = allowEmptyPassword
	password := os.Getenv(prefix + "_PASSWORD")
	if password == "" && !allowEmptyPassword {
		t.Fatalf("%s_PASSWORD is required unless %s_ALLOW_EMPTY_PASSWORD=true", prefix, prefix)
	}
	secrets, err := config.ResolveSecrets(cfg, func(string) (string, bool) { return password, true })
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	return cfg, secrets
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
