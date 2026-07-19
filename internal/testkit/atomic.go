package testkit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type AtomicOperation struct {
	Index  int   `json:"index"`
	From   int   `json:"from"`
	To     int   `json:"to"`
	Amount int64 `json:"amount"`
}

type AtomicReproduction struct {
	SchemaVersion int               `json:"schema_version"`
	CreatedAt     time.Time         `json:"created_at"`
	Seed          int64             `json:"seed"`
	Server        string            `json:"server"`
	Isolation     string            `json:"isolation"`
	Error         string            `json:"error"`
	Operations    []AtomicOperation `json:"operations"`
}

func SaveAtomicReproduction(path string, reproduction AtomicReproduction) error {
	if reproduction.Seed == 0 || len(reproduction.Operations) == 0 || reproduction.Error == "" {
		return fmt.Errorf("atomic reproduction requires seed, operations, and error")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(reproduction, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o600)
}
