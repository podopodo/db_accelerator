package control

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/podopodo/db_accelerator/internal/buildinfo"
	"github.com/podopodo/db_accelerator/internal/lifecycle"
)

func TestHealthReadiness(t *testing.T) {
	state := lifecycle.New(time.Now())
	handler := HealthHandler(state, buildinfo.Current())

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("starting readiness status = %d", response.Code)
	}

	if err := state.Transition(lifecycle.Ready, "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("ready status = %d", response.Code)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("health response can be cached")
	}
}
