// Package benchmark runs a bounded, isolated comparison against the configured
// database. Results are local evidence, never a universal performance claim.
package benchmark

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/podopodo/db_accelerator/internal/buildinfo"
	"github.com/podopodo/db_accelerator/internal/config"
	"github.com/podopodo/db_accelerator/internal/gateway"
	"github.com/podopodo/db_accelerator/internal/upstream"
)

const SchemaVersion = 1

type Options struct {
	Config      config.Config
	Secrets     config.Secrets
	Clients     int
	Concurrency int
	Operations  int
	Rows        int
	OutputPath  string
}

type Report struct {
	SchemaVersion int          `json:"schema_version"`
	RunID         string       `json:"run_id"`
	StartedAt     time.Time    `json:"started_at"`
	CompletedAt   time.Time    `json:"completed_at"`
	Confidence    string       `json:"confidence"`
	Environment   Environment  `json:"environment"`
	Workload      Workload     `json:"workload"`
	Direct        PathMetrics  `json:"direct"`
	Accelerator   PathMetrics  `json:"accelerator"`
	Gains         Gains        `json:"gains"`
	Evidence      EvidenceNote `json:"evidence"`
}

type Environment struct {
	ServerProduct string `json:"server_product"`
	ServerVersion string `json:"server_version"`
	Address       string `json:"address"`
	OperatingSys  string `json:"os"`
	Architecture  string `json:"arch"`
	LogicalCPUs   int    `json:"logical_cpus"`
	Driver        string `json:"driver"`
	DriverVersion string `json:"driver_version"`
	Accelerator   string `json:"accelerator_version"`
	Commit        string `json:"accelerator_commit"`
}

type Workload struct {
	Name        string `json:"name"`
	OpenClients int    `json:"open_clients"`
	Concurrency int    `json:"active_concurrency"`
	Operations  int    `json:"operations_per_path"`
	Rows        int    `json:"dataset_rows"`
	PayloadSize int    `json:"payload_bytes"`
	DirectRuns  int    `json:"direct_runs"`
	QueryShape  string `json:"query_shape"`
}

type PathMetrics struct {
	ClientReadyMS           float64 `json:"client_ready_ms"`
	PeakDatabaseConnections int     `json:"peak_database_connections"`
	Operations              int     `json:"operations"`
	Errors                  int     `json:"errors"`
	DurationMS              float64 `json:"duration_ms"`
	ThroughputPerSecond     float64 `json:"throughput_per_second"`
	P50MS                   float64 `json:"p50_ms"`
	P95MS                   float64 `json:"p95_ms"`
	P99MS                   float64 `json:"p99_ms"`
	MaxMS                   float64 `json:"max_ms"`
}

type Gains struct {
	ConnectionsSaved           int     `json:"connections_saved"`
	ConnectionReductionPercent float64 `json:"connection_reduction_percent"`
	FanInRatio                 float64 `json:"fan_in_ratio"`
	ClientReadySpeedup         float64 `json:"client_ready_speedup"`
	ThroughputChangePercent    float64 `json:"throughput_change_percent"`
	P95LatencyChangePercent    float64 `json:"p95_latency_change_percent"`
}

type EvidenceNote struct {
	Measured     bool   `json:"measured"`
	Experimental bool   `json:"experimental"`
	Scope        string `json:"scope"`
	Caveat       string `json:"caveat"`
}

type Status struct {
	Available bool    `json:"available"`
	Report    *Report `json:"report,omitempty"`
	Error     string  `json:"error,omitempty"`
}

func (o Options) validate() error {
	var problems []string
	if o.Clients < 2 || o.Clients > 256 {
		problems = append(problems, "clients must be between 2 and 256")
	}
	if o.Concurrency < 1 || o.Concurrency > 32 || o.Concurrency > o.Clients {
		problems = append(problems, "concurrency must be between 1 and min(clients, 32)")
	}
	if o.Operations < 100 || o.Operations > 100_000 {
		problems = append(problems, "operations must be between 100 and 100000")
	}
	if o.Rows < 100 || o.Rows > 10_000 {
		problems = append(problems, "rows must be between 100 and 10000")
	}
	if !o.Config.Upstream.Enabled {
		problems = append(problems, "upstream must be enabled")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func Run(ctx context.Context, options Options) (_ Report, runErr error) {
	if err := options.validate(); err != nil {
		return Report{}, err
	}
	startedAt := time.Now().UTC()
	runID, databaseName, err := runIdentity(startedAt)
	if err != nil {
		return Report{}, err
	}
	connector, err := upstream.New(options.Config, options.Secrets)
	if err != nil {
		return Report{}, err
	}
	admin, err := connector.OpenPool(max(2, options.Concurrency))
	if err != nil {
		return Report{}, err
	}
	defer admin.Close()
	if err := admin.PingContext(ctx); err != nil {
		return Report{}, upstream.Classify("benchmark ping", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+databaseName+"`"); err != nil {
		return Report{}, fmt.Errorf("create isolated benchmark database: %w", err)
	}
	defer func() {
		if safeBenchmarkDatabase(databaseName) {
			if _, err := admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`"); err != nil && runErr == nil {
				runErr = fmt.Errorf("drop isolated benchmark database: %w", err)
			}
		}
	}()
	if err := createDataset(ctx, admin, databaseName, options.Rows); err != nil {
		return Report{}, err
	}

	var version, comment string
	if err := admin.QueryRowContext(ctx, "SELECT VERSION(), @@version_comment").Scan(&version, &comment); err != nil {
		return Report{}, fmt.Errorf("read benchmark server identity: %w", err)
	}
	product := "mysql"
	if strings.Contains(strings.ToLower(version+" "+comment), "mariadb") {
		product = "mariadb"
	}

	directReady, directPeak, err := measureDirectReadiness(ctx, connector, options.Clients)
	if err != nil {
		return Report{}, err
	}
	directDatabase, err := connector.OpenPool(options.Concurrency)
	if err != nil {
		return Report{}, err
	}
	defer directDatabase.Close()
	warmQuery := pointQuery(databaseName, 1)
	if err := warm(ctx, directDatabase, warmQuery, options.Concurrency); err != nil {
		return Report{}, fmt.Errorf("warm direct path: %w", err)
	}
	directA := runWorkload(ctx, options.Operations, options.Concurrency, options.Rows, func(_ int) *sql.DB { return directDatabase }, databaseName)

	gatewayConfig := options.Config
	gatewayConfig.Server.MySQLMode = "pooled"
	gatewayConfig.Server.MySQLListen = "127.0.0.1:0"
	gatewayConfig.Limits.MaxLogicalConnections = max(options.Clients+8, gatewayConfig.Limits.MaxLogicalConnections)
	gatewayConfig.Limits.MaxUpstreamConnections = options.Concurrency
	gatewayConfig.Limits.MaxQueuedRequests = max(options.Clients, gatewayConfig.Limits.MaxQueuedRequests)
	gatewayConnector, err := upstream.New(gatewayConfig, options.Secrets)
	if err != nil {
		return Report{}, err
	}
	service, err := gateway.New(gatewayConfig, options.Secrets, gatewayConnector, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		return Report{}, err
	}
	serviceContext, serviceCancel := context.WithCancel(ctx)
	if err := service.Start(serviceContext); err != nil {
		serviceCancel()
		return Report{}, err
	}
	defer func() {
		serviceCancel()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := service.Shutdown(shutdownContext); err != nil && runErr == nil {
			runErr = fmt.Errorf("shutdown benchmark gateway: %w", err)
		}
	}()

	logicalClients, acceleratedReady, err := openGatewayClients(ctx, gatewayConfig, options.Secrets, service.Address(), options.Clients)
	if err != nil {
		return Report{}, err
	}
	defer closeDatabases(logicalClients)
	if err := warm(ctx, logicalClients[0], warmQuery, options.Concurrency); err != nil {
		return Report{}, fmt.Errorf("warm accelerator path: %w", err)
	}
	peakConnections := atomic.Int64{}
	monitorDone := make(chan struct{})
	go monitorGateway(service, &peakConnections, monitorDone)
	accelerated := runWorkload(ctx, options.Operations, options.Concurrency, options.Rows, func(worker int) *sql.DB { return logicalClients[worker%len(logicalClients)] }, databaseName)
	close(monitorDone)
	if total := service.Snapshot().DatabaseLinks + service.Snapshot().IdleDatabaseLinks; total > peakConnections.Load() {
		peakConnections.Store(total)
	}
	accelerated.ClientReadyMS = milliseconds(acceleratedReady)
	accelerated.PeakDatabaseConnections = int(peakConnections.Load())

	if err := warm(ctx, directDatabase, warmQuery, options.Concurrency); err != nil {
		return Report{}, fmt.Errorf("rewarm direct path: %w", err)
	}
	directB := runWorkload(ctx, options.Operations, options.Concurrency, options.Rows, func(_ int) *sql.DB { return directDatabase }, databaseName)
	direct := medianPathMetrics(directA, directB)
	direct.ClientReadyMS = milliseconds(directReady)
	direct.PeakDatabaseConnections = directPeak

	if direct.Errors > 0 || accelerated.Errors > 0 {
		return Report{}, fmt.Errorf("benchmark invalid: direct errors=%d accelerator errors=%d", direct.Errors, accelerated.Errors)
	}
	report := Report{
		SchemaVersion: SchemaVersion,
		RunID:         runID,
		StartedAt:     startedAt,
		CompletedAt:   time.Now().UTC(),
		Confidence:    "local-measured-experimental",
		Environment: Environment{
			ServerProduct: product,
			ServerVersion: version,
			Address:       net.JoinHostPort(options.Config.Upstream.Host, strconv.Itoa(options.Config.Upstream.Port)),
			OperatingSys:  runtime.GOOS,
			Architecture:  runtime.GOARCH,
			LogicalCPUs:   runtime.NumCPU(),
			Driver:        "github.com/go-sql-driver/mysql",
			DriverVersion: "v1.9.3",
			Accelerator:   buildinfo.Version,
			Commit:        buildinfo.Commit,
		},
		Workload: Workload{
			Name:        "bounded-point-read-comparison",
			OpenClients: options.Clients,
			Concurrency: options.Concurrency,
			Operations:  options.Operations,
			Rows:        options.Rows,
			PayloadSize: 128,
			DirectRuns:  2,
			QueryShape:  "single-row primary-key SELECT; no cache; local TCP",
		},
		Direct:      direct,
		Accelerator: accelerated,
		Evidence: EvidenceNote{
			Measured:     true,
			Experimental: true,
			Scope:        "This machine, this database, this binary, and the recorded bounded workload only.",
			Caveat:       "Connection savings are the product claim. Throughput and latency may improve or regress; both are reported without filtering.",
		},
	}
	report.Gains = calculateGains(report.Direct, report.Accelerator, options.Clients)
	if err := Save(options.OutputPath, report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func measureDirectReadiness(ctx context.Context, connector *upstream.Connector, clients int) (time.Duration, int, error) {
	databases := make([]*sql.DB, clients)
	for index := range databases {
		database, err := connector.OpenPool(1)
		if err != nil {
			closeDatabases(databases)
			return 0, 0, err
		}
		databases[index] = database
	}
	started := time.Now()
	err := pingAll(ctx, databases)
	duration := time.Since(started)
	peak := 0
	for _, database := range databases {
		peak += database.Stats().OpenConnections
	}
	closeDatabases(databases)
	if err != nil {
		return 0, peak, fmt.Errorf("open direct clients: %w", err)
	}
	return duration, peak, nil
}

func openGatewayClients(ctx context.Context, cfg config.Config, secrets config.Secrets, address string, count int) ([]*sql.DB, time.Duration, error) {
	databases := make([]*sql.DB, count)
	for index := range databases {
		client := driver.NewConfig()
		client.User = cfg.Upstream.User
		client.Passwd = secrets.UpstreamPassword.Reveal()
		client.Net = "tcp"
		client.Addr = address
		client.DBName = cfg.Upstream.Database
		client.Timeout = 5 * time.Second
		client.ReadTimeout = 30 * time.Second
		client.WriteTimeout = 30 * time.Second
		connector, err := driver.NewConnector(client)
		if err != nil {
			closeDatabases(databases)
			return nil, 0, err
		}
		database := sql.OpenDB(connector)
		database.SetMaxOpenConns(1)
		database.SetMaxIdleConns(1)
		databases[index] = database
	}
	started := time.Now()
	if err := pingAll(ctx, databases); err != nil {
		closeDatabases(databases)
		return nil, 0, fmt.Errorf("open accelerator clients: %w", err)
	}
	return databases, time.Since(started), nil
}

func pingAll(ctx context.Context, databases []*sql.DB) error {
	errorsFound := make(chan error, len(databases))
	var wait sync.WaitGroup
	for _, database := range databases {
		if database == nil {
			continue
		}
		wait.Add(1)
		go func(db *sql.DB) {
			defer wait.Done()
			if err := db.PingContext(ctx); err != nil {
				errorsFound <- err
			}
		}(database)
	}
	wait.Wait()
	close(errorsFound)
	return <-errorsFound
}

func runWorkload(ctx context.Context, operations, concurrency, rows int, database func(int) *sql.DB, databaseName string) PathMetrics {
	latencies := make([]time.Duration, operations)
	var next atomic.Int64
	var errorsFound atomic.Int64
	started := time.Now()
	var wait sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wait.Add(1)
		go func(workerID int) {
			defer wait.Done()
			for {
				index := int(next.Add(1) - 1)
				if index >= operations {
					return
				}
				query := pointQuery(databaseName, index%rows+1)
				operationStarted := time.Now()
				var payload string
				var counter int64
				err := database(workerID).QueryRowContext(ctx, query).Scan(&payload, &counter)
				latencies[index] = time.Since(operationStarted)
				if err != nil || payload == "" || counter < 0 {
					errorsFound.Add(1)
				}
			}
		}(worker)
	}
	wait.Wait()
	duration := time.Since(started)
	slices.Sort(latencies)
	successful := operations - int(errorsFound.Load())
	return PathMetrics{
		Operations:          operations,
		Errors:              int(errorsFound.Load()),
		DurationMS:          milliseconds(duration),
		ThroughputPerSecond: float64(successful) / duration.Seconds(),
		P50MS:               percentile(latencies, 0.50),
		P95MS:               percentile(latencies, 0.95),
		P99MS:               percentile(latencies, 0.99),
		MaxMS:               milliseconds(latencies[len(latencies)-1]),
	}
}

func warm(ctx context.Context, database *sql.DB, query string, concurrency int) error {
	var wait sync.WaitGroup
	errorsFound := make(chan error, concurrency)
	for index := 0; index < concurrency; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for operation := 0; operation < 8; operation++ {
				var payload string
				var counter int64
				if err := database.QueryRowContext(ctx, query).Scan(&payload, &counter); err != nil {
					errorsFound <- err
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	return <-errorsFound
}

func monitorGateway(service *gateway.Service, peak *atomic.Int64, done <-chan struct{}) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot := service.Snapshot()
		total := snapshot.DatabaseLinks + snapshot.IdleDatabaseLinks
		for current := peak.Load(); total > current && !peak.CompareAndSwap(current, total); current = peak.Load() {
		}
		select {
		case <-done:
			return
		case <-ticker.C:
		}
	}
}

func createDataset(ctx context.Context, database *sql.DB, name string, rows int) error {
	table := "`" + name + "`.`bench_items`"
	if _, err := database.ExecContext(ctx, "CREATE TABLE "+table+" (id INT PRIMARY KEY, payload VARCHAR(160) NOT NULL, counter BIGINT NOT NULL) ENGINE=InnoDB"); err != nil {
		return fmt.Errorf("create benchmark table: %w", err)
	}
	digits := "(SELECT 0 n UNION ALL SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5 UNION ALL SELECT 6 UNION ALL SELECT 7 UNION ALL SELECT 8 UNION ALL SELECT 9)"
	statement := fmt.Sprintf("INSERT INTO %s (id, payload, counter) SELECT number, RPAD(CONCAT('row-', number, '-'), 128, 'x'), 0 FROM (SELECT a.n + b.n*10 + c.n*100 + d.n*1000 + 1 number FROM %s a CROSS JOIN %s b CROSS JOIN %s c CROSS JOIN %s d) generated WHERE number <= %d", table, digits, digits, digits, digits, rows)
	if _, err := database.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("populate benchmark table: %w", err)
	}
	return nil
}

func runIdentity(started time.Time) (string, string, error) {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "", "", err
	}
	suffix := hex.EncodeToString(random)
	runID := started.Format("20060102T150405Z") + "-" + suffix
	return runID, "dba_benchmark_" + suffix, nil
}

func safeBenchmarkDatabase(name string) bool {
	if !strings.HasPrefix(name, "dba_benchmark_") || len(name) != len("dba_benchmark_")+8 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(name, "dba_benchmark_"))
	return err == nil
}

func pointQuery(database string, id int) string {
	return fmt.Sprintf("SELECT payload, counter FROM `%s`.`bench_items` WHERE id = %d", database, id)
}

func closeDatabases(databases []*sql.DB) {
	for _, database := range databases {
		if database != nil {
			_ = database.Close()
		}
	}
}

func medianPathMetrics(first, second PathMetrics) PathMetrics {
	return PathMetrics{
		Operations:          first.Operations,
		Errors:              max(first.Errors, second.Errors),
		DurationMS:          median(first.DurationMS, second.DurationMS),
		ThroughputPerSecond: median(first.ThroughputPerSecond, second.ThroughputPerSecond),
		P50MS:               median(first.P50MS, second.P50MS),
		P95MS:               median(first.P95MS, second.P95MS),
		P99MS:               median(first.P99MS, second.P99MS),
		MaxMS:               median(first.MaxMS, second.MaxMS),
	}
}

func calculateGains(direct, accelerated PathMetrics, clients int) Gains {
	connectionsSaved := max(0, direct.PeakDatabaseConnections-accelerated.PeakDatabaseConnections)
	return Gains{
		ConnectionsSaved:           connectionsSaved,
		ConnectionReductionPercent: percent(float64(connectionsSaved), float64(max(1, direct.PeakDatabaseConnections))),
		FanInRatio:                 round(float64(clients)/float64(max(1, accelerated.PeakDatabaseConnections)), 2),
		ClientReadySpeedup:         round(direct.ClientReadyMS/maxFloat(0.0001, accelerated.ClientReadyMS), 2),
		ThroughputChangePercent:    percent(accelerated.ThroughputPerSecond-direct.ThroughputPerSecond, direct.ThroughputPerSecond),
		P95LatencyChangePercent:    percent(direct.P95MS-accelerated.P95MS, direct.P95MS),
	}
}

func percentile(values []time.Duration, quantile float64) float64 {
	index := int(math.Ceil(float64(len(values))*quantile)) - 1
	index = min(max(index, 0), len(values)-1)
	return milliseconds(values[index])
}

func milliseconds(value time.Duration) float64 {
	return round(float64(value)/float64(time.Millisecond), 6)
}

func percent(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return round(numerator/denominator*100, 2)
}

func median(first, second float64) float64 { return round((first+second)/2, 3) }
func maxFloat(first, second float64) float64 {
	if first > second {
		return first
	}
	return second
}
func round(value float64, places int) float64 {
	unit := math.Pow10(places)
	return math.Round(value*unit) / unit
}

func Save(path string, report Report) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("benchmark output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create benchmark output directory: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode benchmark report: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write benchmark report: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("publish benchmark report: %w", err)
	}
	return nil
}

func LoadStatus(path string) Status {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Status{}
	}
	if err != nil {
		return Status{Error: "benchmark evidence could not be read"}
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil || report.SchemaVersion != SchemaVersion || !report.Evidence.Measured {
		return Status{Error: "benchmark evidence is invalid or unsupported"}
	}
	return Status{Available: true, Report: &report}
}
