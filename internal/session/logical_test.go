package session

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/podopodo/db_accelerator/internal/engine"
)

func TestLogicalTransitions(t *testing.T) {
	state := NewLogical(testBaseline())
	if reuse, reason := state.Reuse(); reuse != Multiplexable || reason != PinNone {
		t.Fatalf("new state reuse=%s reason=%s", reuse, reason)
	}
	if err := state.Apply(engine.ClassifySQL("SET AUTOCOMMIT=0")); err != nil {
		t.Fatal(err)
	}
	if reuse, reason := state.Reuse(); reuse != Pinned || reason != PinAutocommitOff {
		t.Fatalf("autocommit state reuse=%s reason=%s", reuse, reason)
	}
	if err := state.Apply(engine.ClassifySQL("BEGIN")); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(engine.ClassifySQL("SAVEPOINT one")); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(engine.ClassifySQL("COMMIT")); err != nil {
		t.Fatal(err)
	}
	if state.InTransaction || state.Autocommit {
		t.Fatalf("commit state=%+v", state)
	}
	if err := state.Apply(engine.ClassifySQL("SET AUTOCOMMIT=1")); err != nil {
		t.Fatal(err)
	}
	if err := state.AddPrepared(7); err != nil {
		t.Fatal(err)
	}
	if reuse, reason := state.Reuse(); reuse != Pinned || reason != PinPrepared {
		t.Fatalf("prepared state reuse=%s reason=%s", reuse, reason)
	}
	if !state.RemovePrepared(7) || state.RemovePrepared(7) {
		t.Fatal("prepared handle removal was not isolated")
	}
}

func TestLogicalTransitionTable(t *testing.T) {
	tests := []struct {
		name            string
		setup           []string
		query           string
		wantAutocommit  bool
		wantTransaction bool
		wantError       bool
	}{
		{name: "read", query: "SELECT 1", wantAutocommit: true},
		{name: "warning count", query: "SHOW COUNT(*) WARNINGS", wantAutocommit: true},
		{name: "write", query: "INSERT INTO t VALUES (1)", wantAutocommit: true},
		{name: "ddl", query: "CREATE TABLE t (id INT)", wantAutocommit: true},
		{name: "begin", query: "BEGIN", wantAutocommit: true, wantTransaction: true},
		{name: "commit", setup: []string{"BEGIN"}, query: "COMMIT", wantAutocommit: true},
		{name: "rollback", setup: []string{"BEGIN"}, query: "ROLLBACK", wantAutocommit: true},
		{name: "savepoint", setup: []string{"BEGIN"}, query: "SAVEPOINT one", wantAutocommit: true, wantTransaction: true},
		{name: "autocommit off", query: "SET AUTOCOMMIT=0"},
		{name: "autocommit on", setup: []string{"SET AUTOCOMMIT=0", "BEGIN"}, query: "SET AUTOCOMMIT=1", wantAutocommit: true},
		{name: "set names", query: "SET NAMES utf8mb4", wantAutocommit: true},
		{name: "use", query: "USE app", wantAutocommit: true},
		{name: "unsupported", query: "SET sql_mode='ANSI'", wantAutocommit: true, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := NewLogical(testBaseline())
			for _, query := range test.setup {
				if err := state.Apply(engine.ClassifySQL(query)); err != nil {
					t.Fatalf("setup %q: %v", query, err)
				}
			}
			err := state.Apply(engine.ClassifySQL(test.query))
			if (err != nil) != test.wantError {
				t.Fatalf("error=%v want_error=%v", err, test.wantError)
			}
			if state.Autocommit != test.wantAutocommit || state.InTransaction != test.wantTransaction {
				t.Fatalf("state autocommit=%v transaction=%v", state.Autocommit, state.InTransaction)
			}
		})
	}
}

func TestLogicalIllegalTransitionsFailClosed(t *testing.T) {
	state := NewLogical(testBaseline())
	for _, query := range []string{"SAVEPOINT one", "SELECT 1; SELECT 2", ""} {
		if err := state.Apply(engine.ClassifySQL(query)); !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("query=%q error=%v", query, err)
		}
	}
	unknown := engine.Statement{Kind: engine.StatementKind("future-command")}
	if err := state.Apply(unknown); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("unknown transition error=%v", err)
	}
	if reuse, reason := state.Reuse(); reuse != Poisoned || reason != PinPoisoned {
		t.Fatalf("unknown transition reuse=%s reason=%s", reuse, reason)
	}
}

func TestLogicalRandomCommandSequencesKeepInvariants(t *testing.T) {
	random := rand.New(rand.NewSource(21188))
	commands := []string{"SELECT 1", "INSERT INTO t VALUES (1)", "BEGIN", "COMMIT", "ROLLBACK", "SAVEPOINT one", "SET AUTOCOMMIT=0", "SET AUTOCOMMIT=1", "SET NAMES utf8mb4", "USE app"}
	state := NewLogical(testBaseline())
	for step := 0; step < 5000; step++ {
		statement := engine.ClassifySQL(commands[random.Intn(len(commands))])
		_ = state.Apply(statement)
		reuse, reason := state.Reuse()
		if state.InTransaction && reuse != Pinned {
			t.Fatalf("step=%d transaction not pinned: reuse=%s reason=%s", step, reuse, reason)
		}
		if reuse == Multiplexable && (!state.Autocommit || state.InTransaction || len(state.Prepared) != 0) {
			t.Fatalf("step=%d unsafe multiplexable state=%+v", step, state)
		}
	}
}

func TestLogicalDisconnectClearsState(t *testing.T) {
	state := NewLogical(testBaseline())
	_ = state.Apply(engine.ClassifySQL("SET AUTOCOMMIT=0"))
	_ = state.Apply(engine.ClassifySQL("BEGIN"))
	_ = state.AddPrepared(9)
	state.UserVariables = true
	state.TemporaryState = true
	state.LockRisk = true
	state.SetResult(3, 44)
	state.Disconnect()
	fresh := NewLogical(testBaseline())
	if reuse, _ := state.Reuse(); reuse != Multiplexable {
		t.Fatalf("disconnect reuse=%s", reuse)
	}
	if state.Database != fresh.Database || state.Autocommit != fresh.Autocommit || state.InTransaction || len(state.Prepared) != 0 || state.Warnings != 0 || state.LastInsertID != 0 || state.UserVariables || state.TemporaryState || state.LockRisk {
		t.Fatalf("disconnect state=%+v", state)
	}
}

func testBaseline() Baseline {
	return Baseline{Database: "app", Isolation: "REPEATABLE-READ", Charset: "utf8mb4", Collation: "utf8mb4_0900_ai_ci", TimeZone: "SYSTEM", SQLMode: "STRICT_TRANS_TABLES"}
}
