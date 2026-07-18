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
	"github.com/podopodo/db_accelerator/internal/relay"
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

type fixedRelay struct{ snapshot relay.Snapshot }

func (f fixedRelay) Snapshot() relay.Snapshot { return f.snapshot }

func TestRuntimeMapsPooledPressureAndCapability(t *testing.T) {
	cfg := config.Default()
	cfg.Server.MySQLMode = "pooled"
	cfg.Limits.MaxUpstreamConnections = 10
	state := lifecycle.New(time.Now())
	runtime := NewRuntime(state, cfg, fixedRelay{snapshot: relay.Snapshot{
		Mode: "protocol-pooled", Active: 50, DatabaseLinks: 4, WaitingWork: 2, PinnedWork: 1, MaxConnections: 10,
	}}, nil, time.Now())
	runtime.probe(context.Background())

	snapshot := runtime.Snapshot()
	if !snapshot.Acceleration.Enabled || snapshot.Pressure.Percent != 40 || snapshot.Pressure.LogicalClients != 50 || snapshot.Pressure.DatabaseLinks != 4 || snapshot.Pressure.WaitingWork != 2 || snapshot.Pressure.PinnedWork != 1 {
		t.Fatalf("pooled runtime mapping: %+v", snapshot)
	}
}

func TestAppHandlerServesStatusAndSecurityHeaders(t *testing.T) {
	cfg := config.Default()
	state := lifecycle.New(time.Now())
	runtime := NewRuntime(state, cfg, nil, nil, time.Now())
	runtime.probe(context.Background())
	handler := AppHandler(state, buildinfo.Current(), runtime, "")

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
