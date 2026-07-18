package relay

import (
	"context"
	"database/sql"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

// TestIntegrationNativeDriver proves that an unmodified MySQL driver can
// authenticate and query through the transparent relay. The normal unit lane
// skips it; the compatibility lane supplies DBA_TEST_MYSQL_* or
// DBA_TEST_MARIADB_* variables.
func TestIntegrationNativeDriver(t *testing.T) {
	prefix := os.Getenv("DBA_TEST_SERVER_PREFIX")
	if prefix == "" {
		t.Skip("set DBA_TEST_SERVER_PREFIX to DBA_TEST_MYSQL or DBA_TEST_MARIADB")
	}
	required := func(suffix string) string {
		value := os.Getenv(prefix + "_" + suffix)
		if value == "" {
			t.Fatalf("%s_%s is required", prefix, suffix)
		}
		return value
	}
	port, err := strconv.Atoi(required("PORT"))
	if err != nil {
		t.Fatalf("port: %v", err)
	}

	server, err := New(Config{
		ListenAddress:   "127.0.0.1:0",
		UpstreamAddress: net.JoinHostPort(required("HOST"), strconv.Itoa(port)),
		MaxConnections:  4,
		DialTimeout:     3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := server.Start(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	client := driver.NewConfig()
	client.User = required("USER")
	client.Passwd = os.Getenv(prefix + "_PASSWORD")
	client.Net = "tcp"
	client.Addr = server.Address()
	client.DBName = os.Getenv(prefix + "_DATABASE")
	client.Timeout = 3 * time.Second
	client.ReadTimeout = 3 * time.Second
	client.WriteTimeout = 3 * time.Second
	client.TLS = nil

	connector, err := driver.NewConnector(client)
	if err != nil {
		t.Fatal(err)
	}
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(1)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()
	var version string
	var answer int
	if err := database.QueryRowContext(queryCtx, "SELECT VERSION(), 40 + 2").Scan(&version, &answer); err != nil {
		t.Fatalf("native query through relay: %v", err)
	}
	if version == "" || answer != 42 {
		t.Fatalf("version=%q answer=%d", version, answer)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for server.Snapshot().Active != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if snapshot := server.Snapshot(); snapshot.AcceptedTotal == 0 || snapshot.ClientToDBBytes == 0 || snapshot.DBToClientBytes == 0 {
		t.Fatalf("relay did not observe native traffic: %+v", snapshot)
	}
}
