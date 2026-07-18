package benchmark

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSafeBenchmarkDatabase(t *testing.T) {
	for _, valid := range []string{"dba_benchmark_0123abcd", "dba_benchmark_ffffffff"} {
		if !safeBenchmarkDatabase(valid) {
			t.Fatalf("valid benchmark database refused: %s", valid)
		}
	}
	for _, invalid := range []string{"app", "dba_benchmark_", "dba_benchmark_zzzzzzzz", "dba_benchmark_0123abcd_extra"} {
		if safeBenchmarkDatabase(invalid) {
			t.Fatalf("unsafe benchmark database accepted: %s", invalid)
		}
	}
}

func TestCalculateGains(t *testing.T) {
	gains := calculateGains(
		PathMetrics{ClientReadyMS: 100, PeakDatabaseConnections: 64, ThroughputPerSecond: 1000, P95MS: 2},
		PathMetrics{ClientReadyMS: 25, PeakDatabaseConnections: 8, ThroughputPerSecond: 900, P95MS: 2.5},
		64,
	)
	if gains.ConnectionsSaved != 56 || gains.ConnectionReductionPercent != 87.5 || gains.FanInRatio != 8 || gains.ClientReadySpeedup != 4 || gains.ThroughputChangePercent != -10 || gains.P95LatencyChangePercent != -25 {
		t.Fatalf("gains = %+v", gains)
	}
}

func TestSaveAndLoadStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "benchmark.json")
	report := Report{SchemaVersion: SchemaVersion, RunID: "test", CompletedAt: time.Now(), Evidence: EvidenceNote{Measured: true}}
	if err := Save(path, report); err != nil {
		t.Fatal(err)
	}
	status := LoadStatus(path)
	if !status.Available || status.Report == nil || status.Report.RunID != "test" {
		t.Fatalf("status = %+v", status)
	}
}

func TestOptionsValidation(t *testing.T) {
	options := Options{Clients: 1, Concurrency: 33, Operations: 2, Rows: 2}
	if err := options.validate(); err == nil {
		t.Fatal("unsafe benchmark options were accepted")
	}
}
