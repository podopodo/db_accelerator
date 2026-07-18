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
		"DECIMAL":         protocol.ColumnTypeNewDecimal,
		"DATETIME":        protocol.ColumnTypeDateTime,
		"VARBINARY":       protocol.ColumnTypeBlob,
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
