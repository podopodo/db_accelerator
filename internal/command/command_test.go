package command

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := Run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "commit=") {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestConfigInitAndValidate(t *testing.T) {
	t.Setenv("DBA_ADMIN_TOKEN", "test-admin-token-12345")
	path := filepath.Join(t.TempDir(), "accelerator.yaml")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run([]string{"config", "init", "--output", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("init code = %d, stderr = %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"config", "validate", "--config", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("validate code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "configuration valid") {
		t.Fatalf("validate output = %q", stdout.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := Run([]string{"definitely-unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("unknown exit code = %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("unknown stderr = %q", stderr.String())
	}
}

func TestDoctorReportsDisabledUpstream(t *testing.T) {
	t.Setenv("DBA_ADMIN_TOKEN", "test-admin-token-12345")
	path := filepath.Join(t.TempDir(), "accelerator.yaml")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run([]string{"config", "init", "--output", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("init code = %d, stderr = %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"doctor", "--config", path}, &stdout, &stderr); code != 1 {
		t.Fatalf("doctor code = %d", code)
	}
	if !strings.Contains(stderr.String(), "upstream is disabled") {
		t.Fatalf("doctor stderr = %q", stderr.String())
	}
}
