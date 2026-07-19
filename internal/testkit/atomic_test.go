package testkit

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAtomicReproductionRequiresFailureAndUsesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atomic.json")
	if err := SaveAtomicReproduction(path, AtomicReproduction{}); err == nil {
		t.Fatal("empty atomic reproduction was saved")
	}
	err := SaveAtomicReproduction(path, AtomicReproduction{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		Seed:          21188,
		Server:        "test",
		Isolation:     "REPEATABLE READ",
		Error:         "injected",
		Operations:    []AtomicOperation{{Index: 1, From: 1, To: 2, Amount: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
}
