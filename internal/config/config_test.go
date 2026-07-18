package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultValid(t *testing.T) {
	cfg := Default()
	if cfg.Server.MySQLMode != "transparent" {
		t.Fatalf("default mysql mode = %q", cfg.Server.MySQLMode)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestLoadMySQLModeFromEnvironment(t *testing.T) {
	cfg, err := Load(LoadOptions{LookupEnv: func(name string) (string, bool) {
		if name == "DBA_MYSQL_MODE" {
			return "pooled", true
		}
		return "", false
	}})
	if err != nil {
		t.Fatalf("load pooled mode: %v", err)
	}
	if cfg.Server.MySQLMode != "pooled" {
		t.Fatalf("mysql mode = %q", cfg.Server.MySQLMode)
	}
}

func TestValidateRejectsInvalidMySQLMode(t *testing.T) {
	cfg := Default()
	cfg.Server.MySQLMode = "magical"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "server.mysql_mode") {
		t.Fatalf("invalid mysql mode error = %v", err)
	}
}

func TestLoadPrecedenceAndRelativeDataDir(t *testing.T) {
	directory := t.TempDir()
	basePath := filepath.Join(directory, "accelerator.yaml")
	managedPath := filepath.Join(directory, "managed.yaml")
	writeTestFile(t, basePath, `version: 1
server:
  admin_listen: 127.0.0.1:9100
  data_dir: state
logging:
  level: warn
`)
	writeTestFile(t, managedPath, `logging:
  level: error
`)
	flagLevel := "debug"
	cfg, err := Load(LoadOptions{
		Path:        basePath,
		ManagedPath: managedPath,
		Overrides:   Overrides{LogLevel: &flagLevel},
		LookupEnv: func(name string) (string, bool) {
			if name == "DBA_LOG_LEVEL" {
				return "info", true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Logging.Level != "error" {
		t.Fatalf("managed overlay did not win: %q", cfg.Logging.Level)
	}
	wantDir := filepath.Join(directory, "state")
	if cfg.Server.DataDir != wantDir {
		t.Fatalf("data dir = %q, want %q", cfg.Server.DataDir, wantDir)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	writeTestFile(t, path, "version: 1\nunknown_field: true\n")
	if _, err := Load(LoadOptions{Path: path, LookupEnv: noEnvironment}); err == nil {
		t.Fatal("unknown field was accepted")
	}
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	_, err := Load(LoadOptions{LookupEnv: func(name string) (string, bool) {
		if name == "DBA_MAX_LOGICAL_CONNECTIONS" {
			return "not-a-number", true
		}
		return "", false
	}})
	if err == nil || !strings.Contains(err.Error(), "DBA_MAX_LOGICAL_CONNECTIONS") {
		t.Fatalf("invalid environment error = %v", err)
	}
}

func TestLoadNormalizesRelativeUpstreamCA(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "accelerator.yaml")
	writeTestFile(t, path, `version: 1
upstream:
  enabled: true
  tls_mode: verify-full
  tls_ca_file: certs/ca.pem
  tls_server_name: database.internal
`)
	cfg, err := Load(LoadOptions{Path: path, LookupEnv: noEnvironment})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := filepath.Join(directory, "certs", "ca.pem")
	if cfg.Upstream.TLSCAFile != want {
		t.Fatalf("CA path = %q want %q", cfg.Upstream.TLSCAFile, want)
	}
}

func TestValidateRejectsUnsafeUpstreamLimits(t *testing.T) {
	cfg := Default()
	cfg.Upstream.Enabled = true
	cfg.Upstream.ConnectTimeout = "0s"
	cfg.Upstream.TLSMode = "verify-full"
	cfg.Upstream.Host = "127.0.0.1"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "connect_timeout") || !strings.Contains(err.Error(), "tls_ca_file") || !strings.Contains(err.Error(), "tls_server_name") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestSecretCannotBeFormattedOrMarshaled(t *testing.T) {
	cfg := Default()
	cfg.Upstream.Enabled = true
	secrets, err := ResolveSecrets(cfg, func(name string) (string, bool) {
		return "secret-canary", name == cfg.Upstream.PasswordEnv
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if secrets.UpstreamPassword.Reveal() != "secret-canary" {
		t.Fatal("secret reveal failed")
	}
	formatted := fmt.Sprintf("%+v", secrets)
	encoded, marshalErr := json.Marshal(secrets.UpstreamPassword)
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	if strings.Contains(formatted, "secret-canary") || strings.Contains(string(encoded), "secret-canary") {
		t.Fatalf("secret leaked: formatted=%q json=%q", formatted, encoded)
	}
}

func TestAdminTokenIsResolvedAndRedacted(t *testing.T) {
	cfg := Default()
	cfg.Server.AdminTokenEnv = "DBA_ADMIN_TOKEN"
	secrets, err := ResolveSecrets(cfg, func(name string) (string, bool) {
		return "admin-secret-canary", name == "DBA_ADMIN_TOKEN"
	})
	if err != nil {
		t.Fatalf("resolve admin token: %v", err)
	}
	if secrets.AdminToken.Reveal() != "admin-secret-canary" {
		t.Fatal("admin token was not resolved")
	}
	if strings.Contains(fmt.Sprintf("%+v", secrets), "admin-secret-canary") {
		t.Fatal("admin token leaked through formatting")
	}
	cfg.Server.AdminTokenEnv = ""
	if _, err := ResolveSecrets(cfg, noEnvironment); err != nil {
		t.Fatalf("optional admin auth: %v", err)
	}
}

func TestAdminTokenRejectsMissingOrShortValue(t *testing.T) {
	cfg := Default()
	cfg.Server.AdminTokenEnv = "DBA_ADMIN_TOKEN"
	if _, err := ResolveSecrets(cfg, noEnvironment); err == nil {
		t.Fatal("missing admin token was accepted")
	}
	if _, err := ResolveSecrets(cfg, func(string) (string, bool) { return "too-short", true }); err == nil {
		t.Fatal("short admin token was accepted")
	}
}

func TestEmptyPasswordRequiresExplicitOptIn(t *testing.T) {
	cfg := Default()
	cfg.Upstream.Enabled = true
	lookup := func(string) (string, bool) { return "", true }
	if _, err := ResolveSecrets(cfg, lookup); err == nil || !strings.Contains(err.Error(), "allow_empty_password") {
		t.Fatalf("empty password error = %v", err)
	}
	cfg.Upstream.AllowEmptyPassword = true
	secrets, err := ResolveSecrets(cfg, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("explicit passwordless resolve: %v", err)
	}
	if secrets.UpstreamPassword.Reveal() != "" {
		t.Fatal("passwordless secret is not empty")
	}
}

func TestWriteDefaultRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accelerator.yaml")
	if err := WriteDefault(path, false); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := WriteDefault(path, false); err == nil {
		t.Fatal("second write unexpectedly succeeded")
	}
	loaded, err := Load(LoadOptions{Path: path, LookupEnv: noEnvironment})
	if err != nil {
		t.Fatalf("generated config invalid: %v", err)
	}
	if loaded.Version != CurrentVersion {
		t.Fatalf("generated version = %d", loaded.Version)
	}
}

func noEnvironment(string) (string, bool) { return "", false }

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
