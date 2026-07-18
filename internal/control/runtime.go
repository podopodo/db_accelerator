package control

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"time"

	"github.com/podopodo/db_accelerator/internal/benchmark"
	"github.com/podopodo/db_accelerator/internal/buildinfo"
	"github.com/podopodo/db_accelerator/internal/config"
	"github.com/podopodo/db_accelerator/internal/lifecycle"
	"github.com/podopodo/db_accelerator/internal/relay"
	"github.com/podopodo/db_accelerator/internal/upstream"
)

type Runtime struct {
	state     *lifecycle.Manager
	config    config.Config
	relay     RelaySnapshotter
	upstream  *upstream.Connector
	startedAt time.Time

	mu          sync.RWMutex
	probeStatus string
	probeReport upstream.Report
	probeError  string
	probeKind   upstream.ErrorKind
}

type RelaySnapshotter interface {
	Snapshot() relay.Snapshot
}

type StatusResponse struct {
	Experimental bool               `json:"experimental"`
	ObservedAt   time.Time          `json:"observed_at"`
	UptimeSecs   int64              `json:"uptime_seconds"`
	Lifecycle    lifecycle.Snapshot `json:"lifecycle"`
	Build        buildinfo.Info     `json:"build"`
	Relay        relay.Snapshot     `json:"relay"`
	Upstream     UpstreamStatus     `json:"upstream"`
	Pressure     PressureStatus     `json:"pressure"`
	Limits       LimitsStatus       `json:"limits"`
	Acceleration AccelerationStatus `json:"acceleration"`
	Benchmark    benchmark.Status   `json:"benchmark"`
}

type UpstreamStatus struct {
	Status    string             `json:"status"`
	Address   string             `json:"address"`
	CheckedAt time.Time          `json:"checked_at,omitempty"`
	LatencyMS float64            `json:"latency_ms"`
	ErrorKind upstream.ErrorKind `json:"error_kind,omitempty"`
	Error     string             `json:"error,omitempty"`
	Metadata  upstream.Metadata  `json:"metadata"`
}

type PressureStatus struct {
	LogicalClients int64   `json:"logical_clients"`
	WaitingWork    int64   `json:"waiting_work"`
	ActivePool     int64   `json:"active_pool"`
	PinnedWork     int64   `json:"pinned_work"`
	DatabaseLinks  int64   `json:"database_links"`
	SafeLimit      int     `json:"safe_limit"`
	Percent        float64 `json:"percent"`
	Dominant       string  `json:"dominant_constraint"`
	SafeAction     string  `json:"safe_action"`
}

type LimitsStatus struct {
	LogicalConnections  int   `json:"logical_connections"`
	UpstreamConnections int   `json:"upstream_connections"`
	QueuedRequests      int   `json:"queued_requests"`
	QueuedBytes         int64 `json:"queued_bytes"`
}

type AccelerationStatus struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	Reason  string `json:"reason"`
}

func NewRuntime(state *lifecycle.Manager, cfg config.Config, relayServer RelaySnapshotter, connector *upstream.Connector, startedAt time.Time) *Runtime {
	return &Runtime{
		state:       state,
		config:      cfg,
		relay:       relayServer,
		upstream:    connector,
		startedAt:   startedAt,
		probeStatus: "checking",
	}
}

func (r *Runtime) Start(ctx context.Context) {
	go func() {
		r.probe(ctx)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.probe(ctx)
			}
		}
	}()
}

func (r *Runtime) probe(ctx context.Context) {
	if r.upstream == nil {
		r.mu.Lock()
		r.probeStatus = "disabled"
		r.probeError = "upstream diagnostic connector is disabled"
		r.probeKind = upstream.KindConfiguration
		r.mu.Unlock()
		return
	}
	report, err := r.upstream.Probe(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.probeStatus = "error"
		r.probeError = err.Error()
		var typed *upstream.Error
		if errors.As(err, &typed) {
			r.probeKind = typed.Kind
		} else {
			r.probeKind = upstream.KindServer
		}
		return
	}
	r.probeStatus = "ok"
	r.probeReport = report
	r.probeError = ""
	r.probeKind = ""
}

func (r *Runtime) Snapshot() StatusResponse {
	now := time.Now().UTC()
	relaySnapshot := relay.Snapshot{
		Mode:            "disabled",
		ListenAddress:   r.config.Server.MySQLListen,
		UpstreamAddress: fmt.Sprintf("%s:%d", r.config.Upstream.Host, r.config.Upstream.Port),
		MaxConnections:  r.config.Limits.MaxUpstreamConnections,
	}
	if r.relay != nil {
		relaySnapshot = r.relay.Snapshot()
	}
	r.mu.RLock()
	upstreamStatus := UpstreamStatus{
		Status:    r.probeStatus,
		Address:   relaySnapshot.UpstreamAddress,
		CheckedAt: r.probeReport.CheckedAt,
		LatencyMS: float64(r.probeReport.Latency) / float64(time.Millisecond),
		ErrorKind: r.probeKind,
		Error:     r.probeError,
		Metadata:  r.probeReport.Metadata,
	}
	r.mu.RUnlock()

	active := relaySnapshot.Active
	databaseLinks := relaySnapshot.DatabaseLinks
	waiting := relaySnapshot.WaitingWork
	pinned := relaySnapshot.PinnedWork
	limit := r.config.Limits.MaxUpstreamConnections
	percent := 0.0
	if limit > 0 {
		percent = math.Min(100, float64(databaseLinks)/float64(limit)*100)
	}
	dominant := "Compatibility relay is below its connection limit."
	safeAction := "No action needed. Keep observing real client traffic."
	if relaySnapshot.Mode == "protocol-pooled" {
		dominant = "The pooled gateway is below its upstream connection limit."
		safeAction = "No action needed. Pool capacity has headroom."
	}
	if upstreamStatus.Status == "error" {
		dominant = "The upstream database probe is failing."
		safeAction = "Check Laragon, port, credentials, and TLS policy."
	} else if percent >= 90 {
		dominant = "The upstream connection pool is at its safety limit."
		safeAction = "Reduce query concurrency or raise the verified database connection budget."
	} else if waiting > 0 {
		dominant = fmt.Sprintf("%d request(s) are waiting for an upstream connection.", waiting)
		safeAction = "Inspect slow queries before changing the upstream connection limit."
	} else if relaySnapshot.Mode == "protocol-pooled" && active > databaseLinks {
		dominant = fmt.Sprintf("%d logical clients share %d active database link(s).", active, databaseLinks)
		safeAction = "No action needed. Connection pooling is reducing idle database pressure."
	} else if active > 0 {
		dominant = fmt.Sprintf("%d direct database link(s) are active.", active)
		safeAction = "Traffic is passing 1:1; do not claim connection reduction yet."
	}

	return StatusResponse{
		Experimental: true,
		ObservedAt:   now,
		UptimeSecs:   int64(now.Sub(r.startedAt).Seconds()),
		Lifecycle:    r.state.Snapshot(),
		Build:        buildinfo.Current(),
		Relay:        relaySnapshot,
		Upstream:     upstreamStatus,
		Pressure: PressureStatus{
			LogicalClients: active,
			WaitingWork:    waiting,
			ActivePool:     databaseLinks,
			PinnedWork:     pinned,
			DatabaseLinks:  databaseLinks,
			SafeLimit:      limit,
			Percent:        percent,
			Dominant:       dominant,
			SafeAction:     safeAction,
		},
		Limits: LimitsStatus{
			LogicalConnections:  r.config.Limits.MaxLogicalConnections,
			UpstreamConnections: r.config.Limits.MaxUpstreamConnections,
			QueuedRequests:      r.config.Limits.MaxQueuedRequests,
			QueuedBytes:         r.config.Limits.MaxQueuedBytes,
		},
		Acceleration: accelerationStatus(relaySnapshot.Mode),
		Benchmark:    benchmark.LoadStatus(filepath.Join(r.config.Server.DataDir, "benchmark-latest.json")),
	}
}

func accelerationStatus(mode string) AccelerationStatus {
	if mode == "protocol-pooled" {
		return AccelerationStatus{
			Enabled: true,
			Mode:    "protocol-aware connection pooling",
			Reason:  "Safe text queries share a bounded upstream pool; transactions remain pinned. Unsupported stateful commands are refused.",
		}
	}
	return AccelerationStatus{
		Enabled: false,
		Mode:    "transparent compatibility relay",
		Reason:  "Protocol-aware pooling and cache remain locked behind correctness gates.",
	}
}
