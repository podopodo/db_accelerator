package gateway

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/podopodo/db_accelerator/internal/config"
	"github.com/podopodo/db_accelerator/internal/testkit"
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
	configureGatewayTestTLS(t, &cfg)
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
	if snapshot := service.Snapshot(); snapshot.ClientTLSMode != "required" || snapshot.ClientTLSExpires == nil {
		t.Fatalf("client TLS snapshot = %+v", snapshot)
	}
	downgradeConfig := driver.NewConfig()
	downgradeConfig.User = cfg.Server.MySQLClientUser
	downgradeConfig.Passwd = secrets.ClientPassword.Reveal()
	downgradeConfig.Net = "tcp"
	downgradeConfig.Addr = service.Address()
	downgradeConfig.DBName = cfg.Upstream.Database
	downgradeConfig.Timeout = 3 * time.Second
	downgradeConnector, err := driver.NewConnector(downgradeConfig)
	if err != nil {
		t.Fatal(err)
	}
	downgradeClient := sql.OpenDB(downgradeConnector)
	if err := downgradeClient.Ping(); err == nil {
		downgradeClient.Close()
		t.Fatal("plaintext client bypassed required TLS")
	}
	_ = downgradeClient.Close()
	untrustedConfig := driver.NewConfig()
	untrustedConfig.User = cfg.Server.MySQLClientUser
	untrustedConfig.Passwd = secrets.ClientPassword.Reveal()
	untrustedConfig.Net = "tcp"
	untrustedConfig.Addr = service.Address()
	untrustedConfig.DBName = cfg.Upstream.Database
	untrustedConfig.Timeout = 3 * time.Second
	untrustedConfig.TLS = gatewayUntrustedClientTLS()
	untrustedConnector, err := driver.NewConnector(untrustedConfig)
	if err != nil {
		t.Fatal(err)
	}
	untrustedClient := sql.OpenDB(untrustedConnector)
	if err := untrustedClient.Ping(); err == nil {
		untrustedClient.Close()
		t.Fatal("client accepted an untrusted TLS certificate")
	}
	_ = untrustedClient.Close()
	for _, credentials := range []struct {
		name     string
		user     string
		password string
	}{
		{name: "upstream identity", user: cfg.Upstream.User, password: secrets.UpstreamPassword.Reveal()},
		{name: "wrong client password", user: cfg.Server.MySQLClientUser, password: "wrong-client-password"},
	} {
		t.Run("reject "+credentials.name, func(t *testing.T) {
			authConfig := driver.NewConfig()
			authConfig.User = credentials.user
			authConfig.Passwd = credentials.password
			authConfig.Net = "tcp"
			authConfig.Addr = service.Address()
			authConfig.DBName = cfg.Upstream.Database
			authConfig.Timeout = 3 * time.Second
			authConfig.TLS = gatewayTestClientTLS(t, cfg)
			authConnector, err := driver.NewConnector(authConfig)
			if err != nil {
				t.Fatal(err)
			}
			authClient := sql.OpenDB(authConnector)
			defer authClient.Close()
			err = authClient.Ping()
			var mysqlError *driver.MySQLError
			leaked := credentials.password != "" && strings.Contains(fmt.Sprint(err), credentials.password)
			if !errors.As(err, &mysqlError) || mysqlError.Number != 1045 || leaked {
				t.Fatalf("authentication error = %v", err)
			}
		})
	}
	multiStatementConfig := driver.NewConfig()
	multiStatementConfig.User = cfg.Server.MySQLClientUser
	multiStatementConfig.Passwd = secrets.ClientPassword.Reveal()
	multiStatementConfig.Net = "tcp"
	multiStatementConfig.Addr = service.Address()
	multiStatementConfig.DBName = cfg.Upstream.Database
	multiStatementConfig.MultiStatements = true
	multiStatementConfig.Timeout = 3 * time.Second
	multiStatementConfig.TLS = gatewayTestClientTLS(t, cfg)
	multiStatementConnector, err := driver.NewConnector(multiStatementConfig)
	if err != nil {
		t.Fatal(err)
	}
	multiStatementClient := sql.OpenDB(multiStatementConnector)
	if err := multiStatementClient.Ping(); err == nil {
		multiStatementClient.Close()
		t.Fatal("unsupported multi-statement client completed handshake")
	}
	_ = multiStatementClient.Close()

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
	var directEmpty, acceleratedEmpty string
	if err := admin.QueryRow("SELECT ''").Scan(&directEmpty); err != nil {
		t.Fatal(err)
	}
	if err := client.QueryRow("SELECT ''").Scan(&acceleratedEmpty); err != nil {
		t.Fatal(err)
	}
	if directEmpty != acceleratedEmpty {
		t.Fatalf("empty string direct=%q accelerated=%q", directEmpty, acceleratedEmpty)
	}
	metadataQuery := "SELECT CAST(12.34 AS DECIMAL(10,2)) AS price, CAST(42 AS SIGNED) AS quantity, CAST('hello' AS CHAR(12)) AS label"
	directShape := readColumnShape(t, admin, metadataQuery)
	acceleratedShape := readColumnShape(t, client, metadataQuery)
	if fmt.Sprint(directShape) != fmt.Sprint(acceleratedShape) {
		t.Fatalf("column metadata direct=%+v accelerated=%+v", directShape, acceleratedShape)
	}

	if _, err := client.Exec(`CREATE TABLE differential_values (
		id INT PRIMARY KEY,
		text_value VARCHAR(64) NOT NULL,
		unicode_value VARCHAR(64) NOT NULL,
		decimal_value DECIMAL(10,2) NOT NULL,
		date_value DATE NOT NULL,
		time_value TIME(6) NOT NULL,
		json_value JSON NOT NULL,
		blob_value BLOB NOT NULL,
		nullable_value VARCHAR(64) NULL,
		signed_value BIGINT NOT NULL,
		unsigned_value BIGINT UNSIGNED NOT NULL
	) ENGINE=InnoDB`); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Exec(`INSERT INTO differential_values VALUES (
		1, 'plain', 'བཀྲ་ཤིས', -12.34, '2026-07-19', '12:34:56.123456',
		'{"a":1}', X'00FF', NULL, -9223372036854775808, 18446744073709551615
	)`); err != nil {
		t.Fatal(err)
	}
	differentialQuery := "SELECT * FROM " + databaseName + ".differential_values ORDER BY id"
	for run := 1; run <= 3; run++ {
		direct := testkit.CaptureQuery(context.Background(), admin, differentialQuery)
		accelerated := testkit.CaptureQuery(context.Background(), client, differentialQuery)
		assertDifferentialMatch(t, prefix, run, differentialQuery, direct, accelerated)
	}
	missingQuery := "SELECT * FROM " + databaseName + ".differential_missing"
	assertDifferentialMatch(t, prefix, 1, missingQuery,
		testkit.CaptureQuery(context.Background(), admin, missingQuery),
		testkit.CaptureQuery(context.Background(), client, missingQuery),
	)

	directConnection, err := admin.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var directCast, acceleratedCast uint64
	if err := directConnection.QueryRowContext(context.Background(), "SELECT CAST('not-a-number' AS UNSIGNED)").Scan(&directCast); err != nil {
		t.Fatal(err)
	}
	var directWarnings uint16
	if err := directConnection.QueryRowContext(context.Background(), "SHOW COUNT(*) WARNINGS").Scan(&directWarnings); err != nil {
		t.Fatal(err)
	}
	if err := directConnection.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.QueryRow("SELECT CAST('not-a-number' AS UNSIGNED)").Scan(&acceleratedCast); err != nil {
		t.Fatal(err)
	}
	var acceleratedWarnings uint16
	if err := client.QueryRow("SHOW COUNT(*) WARNINGS").Scan(&acceleratedWarnings); err != nil {
		t.Fatal(err)
	}
	if directCast != acceleratedCast || directWarnings == 0 || acceleratedWarnings != directWarnings {
		t.Fatalf("warning result direct=%d/%d accelerated=%d/%d", directCast, directWarnings, acceleratedCast, acceleratedWarnings)
	}
	var largeValue string
	if err := client.QueryRow("SELECT REPEAT('x', 1048576)").Scan(&largeValue); err != nil || len(largeValue) != 1048576 {
		t.Fatalf("large row bytes=%d err=%v", len(largeValue), err)
	}
	if _, err := client.Exec("CREATE TABLE stream_rows (id INT PRIMARY KEY) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	values := make([]string, 48)
	for index := range values {
		values[index] = fmt.Sprintf("(%d)", index+1)
	}
	if _, err := client.Exec("INSERT INTO stream_rows (id) VALUES " + strings.Join(values, ",")); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	monitorDone := make(chan struct{})
	peakResult := make(chan uint64, 1)
	go monitorHeap(monitorDone, peakResult)
	stream, err := client.Query("SELECT REPEAT('x', 1048576) FROM stream_rows ORDER BY id")
	if err != nil {
		close(monitorDone)
		<-peakResult
		t.Fatal(err)
	}
	streamedRows := 0
	for stream.Next() {
		var chunk []byte
		if err := stream.Scan(&chunk); err != nil {
			stream.Close()
			close(monitorDone)
			<-peakResult
			t.Fatal(err)
		}
		if len(chunk) != 1048576 {
			t.Fatalf("streamed row bytes=%d", len(chunk))
		}
		streamedRows++
		if streamedRows%4 == 0 {
			runtime.GC()
		}
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	close(monitorDone)
	peakHeap := <-peakResult
	if streamedRows != len(values) {
		t.Fatalf("streamed rows=%d", streamedRows)
	}
	if growth := peakHeap - min(peakHeap, before.HeapAlloc); growth > 40<<20 {
		t.Fatalf("streaming heap growth=%d bytes; result may be fully buffered", growth)
	}
	if _, err := client.Exec("CREATE TABLE ledger (id BIGINT PRIMARY KEY, balance BIGINT NOT NULL) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Exec("INSERT INTO ledger (id, balance) VALUES (1, 100)"); err != nil {
		t.Fatal(err)
	}
	_, directDuplicate := admin.Exec("INSERT INTO " + databaseName + ".ledger (id, balance) VALUES (1, 100)")
	_, acceleratedDuplicate := client.Exec("INSERT INTO ledger (id, balance) VALUES (1, 100)")
	var directMySQL, acceleratedMySQL *driver.MySQLError
	if !errors.As(directDuplicate, &directMySQL) || !errors.As(acceleratedDuplicate, &acceleratedMySQL) || directMySQL.Number != acceleratedMySQL.Number || directMySQL.SQLState != acceleratedMySQL.SQLState {
		t.Fatalf("duplicate errors direct=%v accelerated=%v", directDuplicate, acceleratedDuplicate)
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

func assertDifferentialMatch(t *testing.T, server string, run int, operation string, direct, accelerated testkit.DifferentialSnapshot) {
	t.Helper()
	mismatches := testkit.CompareDifferential(direct, accelerated)
	if len(mismatches) == 0 {
		return
	}
	path := filepath.Join(t.TempDir(), fmt.Sprintf("differential-run-%d.json", run))
	err := testkit.SaveDifferentialReproduction(path, testkit.DifferentialReproduction{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		Seed:          21188,
		Server:        server,
		Driver:        "github.com/go-sql-driver/mysql v1.9.3",
		Operation:     operation,
		Direct:        direct,
		Proxy:         accelerated,
		Mismatches:    mismatches,
	})
	if err != nil {
		t.Fatalf("differential mismatch=%+v; save reproduction: %v", mismatches, err)
	}
	t.Fatalf("differential mismatch=%+v; reproduction=%s", mismatches, path)
}

func monitorHeap(done <-chan struct{}, result chan<- uint64) {
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	var peak uint64
	for {
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		peak = max(peak, memory.HeapAlloc)
		select {
		case <-done:
			result <- peak
			return
		case <-ticker.C:
		}
	}
}

type columnShape struct {
	Name       string
	Type       string
	Length     int64
	HasLength  bool
	Nullable   bool
	HasNull    bool
	Precision  int64
	Scale      int64
	HasDecimal bool
}

func readColumnShape(t *testing.T, database *sql.DB, query string) []columnShape {
	t.Helper()
	rows, err := database.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	types, err := rows.ColumnTypes()
	if err != nil {
		t.Fatal(err)
	}
	shape := make([]columnShape, len(types))
	for index, column := range types {
		shape[index].Name = column.Name()
		shape[index].Type = column.DatabaseTypeName()
		shape[index].Length, shape[index].HasLength = column.Length()
		shape[index].Nullable, shape[index].HasNull = column.Nullable()
		shape[index].Precision, shape[index].Scale, shape[index].HasDecimal = column.DecimalSize()
	}
	return shape
}

func openGatewayClient(t *testing.T, cfg config.Config, secrets config.Secrets, address string) *sql.DB {
	t.Helper()
	client := driver.NewConfig()
	client.User = cfg.Server.MySQLClientUser
	client.Passwd = secrets.ClientPassword.Reveal()
	client.Net = "tcp"
	client.Addr = address
	client.DBName = cfg.Upstream.Database
	client.Timeout = 3 * time.Second
	client.ReadTimeout = 5 * time.Second
	client.WriteTimeout = 5 * time.Second
	client.ParseTime = true
	client.TLS = gatewayTestClientTLS(t, cfg)
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
	cfg.Server.MySQLClientUser = "accelerator-test-client"
	cfg.Upstream.Database = ""
	cfg.Upstream.TLSMode = "disabled"
	cfg.Upstream.AllowEmptyPassword = os.Getenv(prefix+"_ALLOW_EMPTY_PASSWORD") == "true"
	password := os.Getenv(prefix + "_PASSWORD")
	secrets, err := config.ResolveSecrets(cfg, func(name string) (string, bool) {
		switch name {
		case cfg.Upstream.PasswordEnv:
			return password, true
		case cfg.Server.MySQLClientPasswordEnv:
			return "accelerator-test-password", true
		default:
			return "", false
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg, secrets
}
