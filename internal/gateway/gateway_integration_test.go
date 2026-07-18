package gateway

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/podopodo/db_accelerator/internal/config"
	"github.com/podopodo/db_accelerator/internal/upstream"
)

func TestIntegrationPooledGatewayTransactionsAndFanIn(t *testing.T) {
	prefix := os.Getenv("DBA_TEST_SERVER_PREFIX")
	if prefix == "" {
		t.Skip("set DBA_TEST_SERVER_PREFIX to enable the pooled gateway integration lane")
	}
	cfg, secrets := gatewayIntegrationConfiguration(t, prefix)
	const databaseName = "dba_accelerator_gateway_test"

	adminConnector, err := upstream.New(cfg, secrets)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := adminConnector.Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("DROP DATABASE IF EXISTS " + databaseName); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("CREATE DATABASE " + databaseName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + databaseName)
		_ = admin.Close()
	})

	cfg.Upstream.Database = databaseName
	cfg.Server.MySQLMode = "pooled"
	cfg.Server.MySQLListen = "127.0.0.1:0"
	cfg.Limits.MaxLogicalConnections = 100
	cfg.Limits.MaxUpstreamConnections = 1
	connector, err := upstream.New(cfg, secrets)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(cfg, secrets, connector, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := service.Start(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		if err := service.Shutdown(shutdownCtx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	client := openGatewayClient(t, cfg, secrets, service.Address())
	defer client.Close()
	var version string
	var answer int
	if err := client.QueryRow("SELECT VERSION(), CAST(42 AS SIGNED)").Scan(&version, &answer); err != nil {
		t.Fatalf("query through pooled gateway: %v", err)
	}
	if version == "" || answer != 42 {
		t.Fatalf("version=%q answer=%d", version, answer)
	}
	var decimalValue string
	var unicodeValue string
	var nullableValue sql.NullString
	var binaryValue []byte
	if err := client.QueryRow("SELECT CAST(12.34 AS DECIMAL(10,2)), 'བཀྲ་ཤིས', NULL, X'00FF'").Scan(&decimalValue, &unicodeValue, &nullableValue, &binaryValue); err != nil {
		t.Fatalf("datatype query: %v", err)
	}
	if decimalValue != "12.34" || unicodeValue != "བཀྲ་ཤིས" || nullableValue.Valid || len(binaryValue) != 2 || binaryValue[0] != 0 || binaryValue[1] != 0xff {
		t.Fatalf("datatype values decimal=%q unicode=%q null=%+v binary=%x", decimalValue, unicodeValue, nullableValue, binaryValue)
	}
	var largeValue string
	if err := client.QueryRow("SELECT REPEAT('x', 1048576)").Scan(&largeValue); err != nil || len(largeValue) != 1048576 {
		t.Fatalf("large row bytes=%d err=%v", len(largeValue), err)
	}
	if _, err := client.Exec("CREATE TABLE ledger (id BIGINT PRIMARY KEY, balance BIGINT NOT NULL) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Exec("INSERT INTO ledger (id, balance) VALUES (1, 100)"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Exec("CREATE TABLE generated_ids (id BIGINT AUTO_INCREMENT PRIMARY KEY, note VARCHAR(32)) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	result, err := client.Exec("INSERT INTO generated_ids (note) VALUES ('first')")
	if err != nil {
		t.Fatal(err)
	}
	affected, _ := result.RowsAffected()
	insertID, _ := result.LastInsertId()
	if affected != 1 || insertID != 1 {
		t.Fatalf("insert result affected=%d insert_id=%d", affected, insertID)
	}

	tx, err := client.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE ledger SET balance = 50 WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := client.QueryRow("SELECT balance FROM ledger WHERE id = 1").Scan(&answer); err != nil || answer != 100 {
		t.Fatalf("rollback balance=%d err=%v", answer, err)
	}

	tx, err = client.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE ledger SET balance = 75 WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := client.QueryRow("SELECT balance FROM ledger WHERE id = 1").Scan(&answer); err != nil || answer != 75 {
		t.Fatalf("commit balance=%d err=%v", answer, err)
	}

	const logicalClients = 64
	clients := make([]*sql.DB, logicalClients)
	var wait sync.WaitGroup
	errorsFound := make(chan error, logicalClients)
	for index := range clients {
		clients[index] = openGatewayClient(t, cfg, secrets, service.Address())
		wait.Add(1)
		go func(database *sql.DB) {
			defer wait.Done()
			var slept, value int
			if err := database.QueryRow("SELECT SLEEP(0.01), 1").Scan(&slept, &value); err != nil || value != 1 {
				errorsFound <- fmt.Errorf("fan-in query value=%d: %v", value, err)
			}
		}(clients[index])
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	snapshot := service.Snapshot()
	if snapshot.AcceptedTotal < logicalClients || snapshot.Active < logicalClients || snapshot.IdleDatabaseLinks > int64(cfg.Limits.MaxUpstreamConnections) {
		t.Fatalf("fan-in snapshot: %+v", snapshot)
	}
	for _, database := range clients {
		_ = database.Close()
	}
}

func openGatewayClient(t *testing.T, cfg config.Config, secrets config.Secrets, address string) *sql.DB {
	t.Helper()
	client := driver.NewConfig()
	client.User = cfg.Upstream.User
	client.Passwd = secrets.UpstreamPassword.Reveal()
	client.Net = "tcp"
	client.Addr = address
	client.DBName = cfg.Upstream.Database
	client.Timeout = 3 * time.Second
	client.ReadTimeout = 5 * time.Second
	client.WriteTimeout = 5 * time.Second
	client.TLS = nil
	connector, err := driver.NewConnector(client)
	if err != nil {
		t.Fatal(err)
	}
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if err := database.Ping(); err != nil {
		database.Close()
		t.Fatalf("gateway ping: %v", err)
	}
	return database
}

func gatewayIntegrationConfiguration(t *testing.T, prefix string) (config.Config, config.Secrets) {
	t.Helper()
	required := func(suffix string) string {
		value := os.Getenv(prefix + "_" + suffix)
		if value == "" {
			t.Fatalf("%s_%s is required", prefix, suffix)
		}
		return value
	}
	port, err := strconv.Atoi(required("PORT"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Upstream.Enabled = true
	cfg.Upstream.Host = required("HOST")
	cfg.Upstream.Port = port
	cfg.Upstream.User = required("USER")
	cfg.Upstream.Database = ""
	cfg.Upstream.TLSMode = "disabled"
	cfg.Upstream.AllowEmptyPassword = os.Getenv(prefix+"_ALLOW_EMPTY_PASSWORD") == "true"
	password := os.Getenv(prefix + "_PASSWORD")
	secrets, err := config.ResolveSecrets(cfg, func(string) (string, bool) { return password, true })
	if err != nil {
		t.Fatal(err)
	}
	return cfg, secrets
}
