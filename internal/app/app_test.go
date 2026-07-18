package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/podopodo/db_accelerator/internal/config"
	"github.com/podopodo/db_accelerator/internal/lifecycle"
)

func TestRunBecomesReadyAndStops(t *testing.T) {
	cfg := config.Default()
	cfg.Server.AdminListen = "127.0.0.1:0"
	cfg.Server.AdminTokenEnv = "DBA_TEST_ADMIN_TOKEN"
	cfg.Server.DataDir = t.TempDir()
	cfg.Server.ShutdownTimeout = "2s"
	secrets, err := config.ResolveSecrets(cfg, func(name string) (string, bool) {
		return "test-admin-token-12345", name == "DBA_TEST_ADMIN_TOKEN"
	})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(cfg, secrets, logger)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- application.Run(ctx) }()

	select {
	case <-application.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("application did not become ready")
	}

	response, err := http.Get("http://" + application.AdminAddress() + "/readyz")
	if err != nil {
		t.Fatalf("ready request: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ready status = %d", response.StatusCode)
	}
	dashboard, err := http.Get("http://" + application.AdminAddress() + "/")
	if err != nil {
		t.Fatalf("dashboard request: %v", err)
	}
	defer dashboard.Body.Close()
	body, err := io.ReadAll(dashboard.Body)
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	if dashboard.StatusCode != http.StatusOK || !strings.Contains(string(body), "Connection Pressure Map") {
		t.Fatalf("dashboard status=%d", dashboard.StatusCode)
	}

	statusURL := "http://" + application.AdminAddress() + "/api/v1/status"
	status, err := http.Get(statusURL)
	if err != nil {
		t.Fatal(err)
	}
	_ = status.Body.Close()
	if status.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status API = %d", status.StatusCode)
	}
	login, err := http.Post("http://"+application.AdminAddress()+"/api/v1/session", "application/json", strings.NewReader(`{"token":"test-admin-token-12345"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = login.Body.Close()
	if login.StatusCode != http.StatusOK || len(login.Cookies()) != 1 {
		t.Fatalf("login status=%d cookies=%+v", login.StatusCode, login.Cookies())
	}
	statusRequest, err := http.NewRequest(http.MethodGet, statusURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	statusRequest.AddCookie(login.Cookies()[0])
	status, err = http.DefaultClient.Do(statusRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = status.Body.Close()
	if status.StatusCode != http.StatusOK {
		t.Fatalf("authenticated status API = %d", status.StatusCode)
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("application did not stop")
	}
	if got := application.State().Snapshot().State; got != lifecycle.Stopped {
		t.Fatalf("state = %s, want stopped", got)
	}
}
