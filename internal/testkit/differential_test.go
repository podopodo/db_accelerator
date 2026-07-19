package testkit

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDifferentialDetectsStatusAndErrorChanges(t *testing.T) {
	direct := DifferentialSnapshot{Status: 2, Error: &DifferentialError{Code: 1062, SQLState: "23000", Message: "duplicate"}}
	proxy := direct
	proxy.Status = 3
	proxy.Error = &DifferentialError{Code: 1105, SQLState: "HY000", Message: "duplicate"}
	mismatches := CompareDifferential(direct, proxy)
	if len(mismatches) != 2 || mismatches[0].Field != "status" || mismatches[1].Field != "error" {
		t.Fatalf("mismatches = %+v", mismatches)
	}
	path := filepath.Join(t.TempDir(), "reproduction.json")
	err := SaveDifferentialReproduction(path, DifferentialReproduction{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		Seed:          21188,
		Server:        "test",
		Driver:        "test",
		Operation:     "INSERT",
		Direct:        direct,
		Proxy:         proxy,
		Mismatches:    mismatches,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDifferentialCleanSnapshotHasNoMismatch(t *testing.T) {
	snapshot := DifferentialSnapshot{Status: differentialStatusAutocommit, Warnings: 1, Rows: [][]DifferentialCell{{{Base64: "NDI="}}}}
	if mismatches := CompareDifferential(snapshot, snapshot); len(mismatches) != 0 {
		t.Fatalf("mismatches = %+v", mismatches)
	}
	if err := SaveDifferentialReproduction(filepath.Join(t.TempDir(), "invalid.json"), DifferentialReproduction{}); err == nil {
		t.Fatal("empty reproduction was saved")
	}
}
