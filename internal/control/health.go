package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/podopodo/db_accelerator/internal/buildinfo"
	"github.com/podopodo/db_accelerator/internal/lifecycle"
	"github.com/podopodo/db_accelerator/internal/ui"
)

var requestSequence atomic.Uint64

func HealthHandler(state *lifecycle.Manager, info buildinfo.Info) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		status := http.StatusOK
		if !state.IsLive() {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, state.Snapshot())
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		status := http.StatusOK
		if !state.IsReady() {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, state.Snapshot())
	})
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, info)
	})
	return securityHeaders(mux)
}

// AppHandler serves the embedded operations cockpit and versioned read-only
// demo API. Authentication remains a later gate; callers must bind locally.
func AppHandler(state *lifecycle.Manager, info buildinfo.Info, runtime *Runtime) http.Handler {
	mux := http.NewServeMux()
	health := HealthHandler(state, info)
	mux.Handle("/livez", health)
	mux.Handle("/readyz", health)
	mux.Handle("/api/v1/version", health)
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, runtime.Snapshot())
	})
	mux.HandleFunc("GET /api/v1/upstream", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, runtime.Snapshot().Upstream)
	})
	mux.HandleFunc("GET /api/v1/relay", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, runtime.Snapshot().Relay)
	})
	mux.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, _ *http.Request) {
		snapshot := runtime.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"limits": snapshot.Limits,
			"relay": map[string]any{
				"listen_address":   snapshot.Relay.ListenAddress,
				"upstream_address": snapshot.Relay.UpstreamAddress,
				"mode":             snapshot.Relay.Mode,
			},
			"read_only": true,
		})
	})
	mux.Handle("/", ui.Handler())
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("X-Request-ID", fmt.Sprintf("r-%x-%x", time.Now().UnixMilli(), requestSequence.Add(1)))
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
