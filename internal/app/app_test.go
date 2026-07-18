package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/podopodo/db_accelerator/internal/config"
	"github.com/podopodo/db_accelerator/internal/lifecycle"
)

func TestRunBecomesReadyAndStops(t *testing.T) {
	cfg := config.Default()
	cfg.Server.AdminListen = "127.0.0.1:0"
	cfg.Server.DataDir = t.TempDir()
	cfg.Server.ShutdownTimeout = "2s"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(cfg, logger)

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
