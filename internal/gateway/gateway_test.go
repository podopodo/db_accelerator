package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/podopodo/db_accelerator/internal/config"
	protocol "github.com/podopodo/db_accelerator/internal/protocol/mysql"
)

func TestAcquireBoundsWaitingRequests(t *testing.T) {
	cfg := config.Default()
	cfg.Limits.MaxUpstreamConnections = 1
	cfg.Limits.MaxQueuedRequests = 1
	cfg.Limits.MaxQueuedBytes = 8
	service := &Service{
		cfg:       cfg,
		admission: make(chan struct{}, 1),
		queue:     make(chan struct{}, 1),
	}

	firstRelease, err := service.acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		release, acquireErr := service.acquire(ctx, 4)
		if release != nil {
			release()
		}
		result <- acquireErr
	}()

	deadline := time.Now().Add(time.Second)
	for service.waiting.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if service.waiting.Load() != 1 || service.queuedBytes.Load() != 4 {
		t.Fatalf("waiting=%d queued_bytes=%d", service.waiting.Load(), service.queuedBytes.Load())
	}
	if _, err := service.acquire(context.Background(), 1); !errors.Is(err, errQueueFull) {
		t.Fatalf("second queued request error = %v", err)
	}

	firstRelease()
	if err := <-result; err != nil {
		t.Fatalf("queued request was not admitted: %v", err)
	}
	if service.waiting.Load() != 0 || service.queuedBytes.Load() != 0 {
		t.Fatalf("queue leaked: waiting=%d bytes=%d", service.waiting.Load(), service.queuedBytes.Load())
	}
}

func TestAcquireRejectsQueuedByteOverflow(t *testing.T) {
	cfg := config.Default()
	cfg.Limits.MaxUpstreamConnections = 1
	cfg.Limits.MaxQueuedRequests = 2
	cfg.Limits.MaxQueuedBytes = 3
	service := &Service{
		cfg:       cfg,
		admission: make(chan struct{}, 1),
		queue:     make(chan struct{}, 2),
	}
	service.admission <- struct{}{}
	if _, err := service.acquire(context.Background(), 4); !errors.Is(err, errQueueFull) {
		t.Fatalf("oversized queued request error = %v", err)
	}
}

func TestMySQLColumnTypeMapping(t *testing.T) {
	tests := map[string]byte{
		"BIGINT UNSIGNED": protocol.ColumnTypeLongLong,
		"UNSIGNED BIGINT": protocol.ColumnTypeLongLong,
		"DECIMAL":         protocol.ColumnTypeNewDecimal,
		"DATETIME":        protocol.ColumnTypeDateTime,
		"VARBINARY":       protocol.ColumnTypeBlob,
		"LONGTEXT":        protocol.ColumnTypeBlob,
		"JSON":            protocol.ColumnTypeJSON,
		"unknown":         protocol.ColumnTypeVarString,
	}
	for input, want := range tests {
		if got := mysqlColumnType(input); got != want {
			t.Errorf("mysqlColumnType(%q)=%d want %d", input, got, want)
		}
	}
}

func TestTextValue(t *testing.T) {
	if value, null := textValue(nil, "VARCHAR"); !null || value != nil {
		t.Fatalf("nil encoded as value=%q null=%v", value, null)
	}
	input := []byte("hello")
	value, null := textValue(input, "BLOB")
	if null || string(value) != "hello" {
		t.Fatalf("bytes encoded as value=%q null=%v", value, null)
	}
	input[0] = 'x'
	if string(value) != "hello" {
		t.Fatal("byte value aliases driver storage")
	}
	date := time.Date(2026, time.July, 19, 13, 14, 15, 0, time.UTC)
	value, null = textValue(date, "DATE")
	if null || string(value) != "2026-07-19" {
		t.Fatalf("date encoded as %q null=%v", value, null)
	}
}

func TestMySQLErrorMapsGatewayFailures(t *testing.T) {
	code, state, message := mysqlError(errQueueFull)
	if code != 1040 || state != "08004" || message != errQueueFull.Error() {
		t.Fatalf("queue error mapped as code=%d state=%q message=%q", code, state, message)
	}
	code, state, _ = mysqlError(context.DeadlineExceeded)
	if code != 3024 || state != "HY000" {
		t.Fatalf("deadline mapped as code=%d state=%q", code, state)
	}
}

func TestAuthenticationLimiterLocksExpiresAndClears(t *testing.T) {
	now := time.Date(2026, time.July, 19, 0, 0, 0, 0, time.UTC)
	limiter := newAuthLimiter()
	limiter.now = func() time.Time { return now }
	remote := "192.0.2.10:12345"
	for attempt := 0; attempt < limiter.maxFailures; attempt++ {
		if !limiter.allowed(remote) {
			t.Fatalf("attempt %d locked early", attempt)
		}
		limiter.failure(remote)
	}
	if limiter.allowed("192.0.2.10:54321") {
		t.Fatal("source address was not locked across source ports")
	}
	now = now.Add(limiter.lockout + time.Second)
	if !limiter.allowed(remote) {
		t.Fatal("lockout did not expire")
	}
	limiter.failure(remote)
	limiter.success(remote)
	if !limiter.allowed(remote) {
		t.Fatal("successful authentication did not clear failures")
	}
}

func TestPinAndRejectionReasonsExposeOnlyStableCategories(t *testing.T) {
	service := &Service{rejectionReasons: make(map[string]uint64), pinReasons: make(map[string]uint64)}
	service.recordPin("transaction")
	service.recordPin("prepared_statement")
	service.recordRejection("temporary_object")
	snapshot := service.reasonSnapshot(service.pinReasons)
	if snapshot["transaction"] != 1 || snapshot["prepared_statement"] != 1 {
		t.Fatalf("pin reasons=%v", snapshot)
	}
	snapshot["transaction"] = 99
	if service.pinReasons["transaction"] != 1 {
		t.Fatal("reason snapshot aliases mutable metrics")
	}
	for input, want := range map[string]string{
		"multiple statements are not enabled":              "multi_statement",
		"temporary objects are unsafe for a shared pool":   "temporary_object",
		"user variables are unsafe for a shared pool":      "user_variable",
		"session-changing SET is unsafe for a shared pool": "session_setting",
		"statement type is not supported in pooled mode":   "unsupported_statement",
	} {
		if got := rejectionReason(input); got != want {
			t.Fatalf("rejectionReason(%q)=%q want %q", input, got, want)
		}
	}
}

func TestAuthenticationLimiterBoundsSourceInventory(t *testing.T) {
	limiter := newAuthLimiter()
	limiter.maxEntries = 2
	limiter.failure("192.0.2.1:1")
	limiter.failure("192.0.2.2:2")
	limiter.failure("192.0.2.3:3")
	if len(limiter.attempts) != 2 {
		t.Fatalf("attempt inventory size=%d", len(limiter.attempts))
	}
}

func TestPermissionIdentitySeparatesFuturePools(t *testing.T) {
	base := config.Default()
	base.Server.MySQLMode = "pooled"
	first := newPermissionIdentity(base)
	mutations := []func(*config.Config){
		func(cfg *config.Config) { cfg.Server.MySQLClientUser = "another-client" },
		func(cfg *config.Config) { cfg.Upstream.User = "another-upstream" },
		func(cfg *config.Config) { cfg.Upstream.Database = "another_database" },
		func(cfg *config.Config) { cfg.Upstream.TLSMode = "verify-full" },
		func(cfg *config.Config) { cfg.Server.MySQLTLSMode = "required" },
	}
	for index, mutate := range mutations {
		changed := base
		mutate(&changed)
		if first == newPermissionIdentity(changed) {
			t.Fatalf("mutation %d did not change permission identity", index)
		}
	}
}
