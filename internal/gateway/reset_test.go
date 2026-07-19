package gateway

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/podopodo/db_accelerator/internal/upstream"
)

func TestResetIsolationAndQuoting(t *testing.T) {
	for input, want := range map[string]string{
		"REPEATABLE-READ": "REPEATABLE READ",
		"read committed":  "READ COMMITTED",
		" SERIALIZABLE ":  "SERIALIZABLE",
	} {
		got, err := resetIsolation(input)
		if err != nil || got != want {
			t.Fatalf("resetIsolation(%q)=%q,%v want %q", input, got, err, want)
		}
	}
	if _, err := resetIsolation("CHAOS"); err == nil {
		t.Fatal("unsupported isolation accepted")
	}
	if !safeResetIdentifier("utf8mb4_0900_ai_ci") || safeResetIdentifier("utf8mb4;DROP") {
		t.Fatal("reset identifier validation failed")
	}
	if got := quoteResetIdentifier("a`b"); got != "`a``b`" {
		t.Fatalf("identifier quote=%q", got)
	}
	if got := quoteResetString("a'b"); got != "'a''b'" {
		t.Fatalf("string quote=%q", got)
	}
}

func TestCheckoutDestroysConnectionWhenRollbackFails(t *testing.T) {
	connector := &failingResetConnector{}
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	service := &Service{
		database:     database,
		resetTimeout: time.Second,
		baseline: upstream.Metadata{
			Database:             "app",
			CharacterSet:         "utf8mb4",
			Collation:            "utf8mb4_general_ci",
			SQLMode:              "STRICT_TRANS_TABLES",
			TimeZone:             "SYSTEM",
			TransactionIsolation: "REPEATABLE-READ",
		},
	}
	if connection, err := service.checkout(context.Background()); err == nil || connection != nil {
		t.Fatalf("checkout connection=%v error=%v", connection, err)
	}
	if got := service.resetDiscards.Load(); got != 2 {
		t.Fatalf("discard count=%d want 2", got)
	}
	if got := connector.connections.Load(); got != 2 {
		t.Fatalf("physical connections=%d want 2", got)
	}
}

type failingResetConnector struct{ connections atomic.Int64 }

func (c *failingResetConnector) Connect(context.Context) (driver.Conn, error) {
	c.connections.Add(1)
	return &failingResetConnection{}, nil
}

func (c *failingResetConnector) Driver() driver.Driver { return failingResetDriver{} }

type failingResetDriver struct{}

func (failingResetDriver) Open(string) (driver.Conn, error) { return &failingResetConnection{}, nil }

type failingResetConnection struct{}

func (*failingResetConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (*failingResetConnection) Close() error { return nil }
func (*failingResetConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("begin is not supported")
}
func (*failingResetConnection) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.EqualFold(strings.TrimSpace(query), "ROLLBACK") {
		return nil, errors.New("injected rollback failure")
	}
	return driver.RowsAffected(0), nil
}
func (*failingResetConnection) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "@@in_transaction") {
		return &singleValueRows{value: int64(1)}, nil
	}
	return nil, errors.New("unexpected query")
}

type singleValueRows struct {
	value int64
	done  bool
}

func (*singleValueRows) Columns() []string { return []string{"value"} }
func (*singleValueRows) Close() error      { return nil }
func (r *singleValueRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	values[0] = r.value
	return nil
}
