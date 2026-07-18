package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/podopodo/db_accelerator/internal/buildinfo"
	"github.com/podopodo/db_accelerator/internal/config"
	"github.com/podopodo/db_accelerator/internal/lifecycle"
)

func TestRuntimeSnapshotTellsTransparentRelayTruth(t *testing.T) {
	cfg := config.Default()
	state := lifecycle.New(time.Now())
	runtime := NewRuntime(state, cfg, nil, nil, time.Now().Add(-3*time.Second))
	runtime.probe(context.Background())

	snapshot := runtime.Snapshot()
	if snapshot.Relay.Mode != "disabled" || snapshot.Acceleration.Enabled {
		t.Fatalf("unsafe capability claim: %+v", snapshot)
	}
	if snapshot.Upstream.Status != "disabled" || snapshot.UptimeSecs < 2 {
		t.Fatalf("unexpected runtime state: %+v", snapshot)
	}
}

func TestAppHandlerServesStatusAndSecurityHeaders(t *testing.T) {
	cfg := config.Default()
	state := lifecycle.New(time.Now())
	runtime := NewRuntime(state, cfg, nil, nil, time.Now())
	runtime.probe(context.Background())
	handler := AppHandler(state, buildinfo.Current(), runtime)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d", response.Code)
	}
	if response.Header().Get("X-Frame-Options") != "DENY" || response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("missing security headers: %v", response.Header())
	}
	if !strings.Contains(response.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("content security policy does not prevent framing")
	}
	var payload StatusResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Experimental || payload.Acceleration.Enabled {
		t.Fatalf("unsafe API payload: %+v", payload)
	}
}
