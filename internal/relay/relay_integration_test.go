package relay

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

// The compatibility lane runs this test once for Oracle MySQL and once for
// MariaDB. The transparent relay must remain semantically indistinguishable
// from a direct native-driver connection.
func TestIntegrationTransparentRelayMatchesDirectDriver(t *testing.T) {
	prefix := os.Getenv("DBA_TEST_SERVER_PREFIX")
	if prefix == "" {
		t.Skip("set DBA_TEST_SERVER_PREFIX to enable the transparent relay integration lane")
	}
	host := requiredRelayEnv(t, prefix, "HOST")
	port, err := strconv.Atoi(requiredRelayEnv(t, prefix, "PORT"))
	if err != nil {
		t.Fatal(err)
	}
	user := requiredRelayEnv(t, prefix, "USER")
	password := os.Getenv(prefix + "_PASSWORD")
	if password == "" && os.Getenv(prefix+"_ALLOW_EMPTY_PASSWORD") != "true" {
		t.Fatalf("%s_PASSWORD is required", prefix)
	}
	upstreamAddress := net.JoinHostPort(host, strconv.Itoa(port))

	server, err := New(Config{ListenAddress: "127.0.0.1:0", UpstreamAddress: upstreamAddress, MaxConnections: 16, DialTimeout: 3 * time.Second})
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
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			t.Errorf("shutdown relay: %v", err)
		}
	})

	admin := openRelayDatabase(t, upstreamAddress, user, password, "", false)
	defer admin.Close()
	const databaseName = "dba_accelerator_relay_test"
	if _, err := admin.Exec("DROP DATABASE IF EXISTS " + databaseName); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("CREATE DATABASE " + databaseName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS " + databaseName) })

	direct := openRelayDatabase(t, upstreamAddress, user, password, databaseName, true)
	defer direct.Close()
	proxied := openRelayDatabase(t, server.Address(), user, password, databaseName, true)

	query := "SELECT CAST(12.34 AS DECIMAL(10,2)), 'བཀྲ་ཤིས', NULL, X'00FF', ''"
	directRow := readRelayRow(t, direct, query)
	proxiedRow := readRelayRow(t, proxied, query)
	if fmt.Sprint(directRow) != fmt.Sprint(proxiedRow) {
		t.Fatalf("row direct=%+v proxied=%+v", directRow, proxiedRow)
	}

	if _, err := direct.Exec("CREATE TABLE ledger (id BIGINT AUTO_INCREMENT PRIMARY KEY, note VARCHAR(64) UNIQUE) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	result, err := proxied.Exec("INSERT INTO ledger (note) VALUES ('relay')")
	if err != nil {
		t.Fatal(err)
	}
	affected, _ := result.RowsAffected()
	insertID, _ := result.LastInsertId()
	if affected != 1 || insertID != 1 {
		t.Fatalf("insert affected=%d id=%d", affected, insertID)
	}
	_, directDuplicate := direct.Exec("INSERT INTO ledger (note) VALUES ('relay')")
	_, proxiedDuplicate := proxied.Exec("INSERT INTO ledger (note) VALUES ('relay')")
	var directError, proxiedError *driver.MySQLError
	if !errors.As(directDuplicate, &directError) || !errors.As(proxiedDuplicate, &proxiedError) || directError.Number != proxiedError.Number || directError.SQLState != proxiedError.SQLState {
		t.Fatalf("errors direct=%v proxied=%v", directDuplicate, proxiedDuplicate)
	}

	var large string
	if err := proxied.QueryRow("SELECT REPEAT('x', 4194304)").Scan(&large); err != nil || len(large) != 4194304 {
		t.Fatalf("large result bytes=%d err=%v", len(large), err)
	}
	if got := readMultiResults(t, proxied, "SELECT 11; SELECT 22"); fmt.Sprint(got) != "[11 22]" {
		t.Fatalf("multi results = %v", got)
	}
	if err := proxied.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for server.Snapshot().Active != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if snapshot := server.Snapshot(); snapshot.RelayErrorsTotal != 0 || snapshot.ClientToDBBytes == 0 || snapshot.DBToClientBytes == 0 {
		t.Fatalf("relay snapshot = %+v", snapshot)
	}
}

type relayRow struct {
	Decimal string
	Unicode string
	Null    sql.NullString
	Binary  []byte
	Empty   string
}

func readRelayRow(t *testing.T, database *sql.DB, query string) relayRow {
	t.Helper()
	var row relayRow
	if err := database.QueryRow(query).Scan(&row.Decimal, &row.Unicode, &row.Null, &row.Binary, &row.Empty); err != nil {
		t.Fatal(err)
	}
	return row
}

func readMultiResults(t *testing.T, database *sql.DB, query string) []int {
	t.Helper()
	rows, err := database.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []int
	for {
		for rows.Next() {
			var value int
			if err := rows.Scan(&value); err != nil {
				t.Fatal(err)
			}
			values = append(values, value)
		}
		if !rows.NextResultSet() {
			break
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}

func openRelayDatabase(t *testing.T, address, user, password, database string, multiStatements bool) *sql.DB {
	t.Helper()
	client := driver.NewConfig()
	client.User = user
	client.Passwd = password
	client.Net = "tcp"
	client.Addr = address
	client.DBName = database
	client.MultiStatements = multiStatements
	client.Timeout = 3 * time.Second
	client.ReadTimeout = 10 * time.Second
	client.WriteTimeout = 10 * time.Second
	connector, err := driver.NewConnector(client)
	if err != nil {
		t.Fatal(err)
	}
	databaseHandle := sql.OpenDB(connector)
	databaseHandle.SetMaxOpenConns(1)
	databaseHandle.SetMaxIdleConns(1)
	if err := databaseHandle.Ping(); err != nil {
		databaseHandle.Close()
		t.Fatal(err)
	}
	return databaseHandle
}

func requiredRelayEnv(t *testing.T, prefix, suffix string) string {
	t.Helper()
	value := os.Getenv(prefix + "_" + suffix)
	if value == "" {
		t.Fatalf("%s_%s is required", prefix, suffix)
	}
	return value
}
