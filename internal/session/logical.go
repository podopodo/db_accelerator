package session

import (
	"errors"
	"fmt"

	"github.com/podopodo/db_accelerator/internal/engine"
)

type ReuseState string

const (
	Multiplexable ReuseState = "multiplexable"
	Replayable    ReuseState = "replayable"
	Pinned        ReuseState = "pinned"
	Poisoned      ReuseState = "poisoned"
)

type PinReason string

const (
	PinNone          PinReason = ""
	PinTransaction   PinReason = "transaction"
	PinAutocommitOff PinReason = "autocommit_disabled"
	PinPrepared      PinReason = "prepared_statement"
	PinUserVariable  PinReason = "user_variable"
	PinTemporary     PinReason = "temporary_object"
	PinLock          PinReason = "connection_lock"
	PinPoisoned      PinReason = "uncertain_state"
)

var ErrIllegalTransition = errors.New("illegal logical session transition")

type Baseline struct {
	Database  string
	Isolation string
	Charset   string
	Collation string
	TimeZone  string
	SQLMode   string
}

// Logical stores only state needed to decide whether an upstream connection
// can be shared. It never stores SQL text, parameter values, or user-variable
// values.
type Logical struct {
	baseline Baseline

	Database       string
	Autocommit     bool
	InTransaction  bool
	Isolation      string
	ReadOnly       bool
	Charset        string
	Collation      string
	TimeZone       string
	SQLMode        string
	Warnings       uint16
	LastInsertID   uint64
	Prepared       map[uint32]struct{}
	UserVariables  bool
	TemporaryState bool
	LockRisk       bool
	poisoned       bool
}

func NewLogical(baseline Baseline) *Logical {
	state := &Logical{baseline: baseline, Prepared: make(map[uint32]struct{})}
	state.Reset()
	return state
}

func (s *Logical) Reset() {
	s.Database = s.baseline.Database
	s.Autocommit = true
	s.InTransaction = false
	s.Isolation = s.baseline.Isolation
	s.ReadOnly = false
	s.Charset = s.baseline.Charset
	s.Collation = s.baseline.Collation
	s.TimeZone = s.baseline.TimeZone
	s.SQLMode = s.baseline.SQLMode
	s.Warnings = 0
	s.LastInsertID = 0
	clear(s.Prepared)
	s.UserVariables = false
	s.TemporaryState = false
	s.LockRisk = false
	s.poisoned = false
}

func (s *Logical) Apply(statement engine.Statement) error {
	s.Warnings = 0
	s.LastInsertID = 0
	switch statement.Kind {
	case engine.StatementRead, engine.StatementWrite, engine.StatementDDL, engine.StatementWarningCount:
		return nil
	case engine.StatementBegin:
		if s.InTransaction {
			return fmt.Errorf("%w: transaction already active", ErrIllegalTransition)
		}
		s.InTransaction = true
		return nil
	case engine.StatementCommit, engine.StatementRollback:
		s.InTransaction = false
		return nil
	case engine.StatementSavepoint:
		if !s.InTransaction {
			return fmt.Errorf("%w: savepoint outside transaction", ErrIllegalTransition)
		}
		return nil
	case engine.StatementAutocommitOff:
		s.Autocommit = false
		return nil
	case engine.StatementAutocommitOn:
		s.Autocommit = true
		s.InTransaction = false
		return nil
	case engine.StatementSetNames:
		s.Charset = "utf8mb4"
		return nil
	case engine.StatementUseDatabase:
		return nil
	case engine.StatementEmpty, engine.StatementUnsupported:
		return fmt.Errorf("%w: %s", ErrIllegalTransition, statement.Kind)
	default:
		s.poisoned = true
		return fmt.Errorf("%w: unknown statement kind %q", ErrIllegalTransition, statement.Kind)
	}
}

func (s *Logical) SelectDatabase(database string) error {
	if database == "" {
		return fmt.Errorf("%w: database is empty", ErrIllegalTransition)
	}
	s.Database = database
	return nil
}

func (s *Logical) SetResult(warnings uint16, lastInsertID uint64) {
	s.Warnings = warnings
	s.LastInsertID = lastInsertID
}

func (s *Logical) AddPrepared(id uint32) error {
	if id == 0 {
		return fmt.Errorf("%w: prepared statement id is zero", ErrIllegalTransition)
	}
	if _, exists := s.Prepared[id]; exists {
		return fmt.Errorf("%w: prepared statement id already exists", ErrIllegalTransition)
	}
	s.Prepared[id] = struct{}{}
	return nil
}

func (s *Logical) RemovePrepared(id uint32) bool {
	if _, exists := s.Prepared[id]; !exists {
		return false
	}
	delete(s.Prepared, id)
	return true
}

func (s *Logical) Poison() { s.poisoned = true }

func (s *Logical) Reuse() (ReuseState, PinReason) {
	switch {
	case s.poisoned:
		return Poisoned, PinPoisoned
	case s.InTransaction:
		return Pinned, PinTransaction
	case !s.Autocommit:
		return Pinned, PinAutocommitOff
	case len(s.Prepared) > 0:
		return Pinned, PinPrepared
	case s.UserVariables:
		return Pinned, PinUserVariable
	case s.TemporaryState:
		return Pinned, PinTemporary
	case s.LockRisk:
		return Pinned, PinLock
	case s.Database != s.baseline.Database || s.Isolation != s.baseline.Isolation || s.ReadOnly || s.Charset != s.baseline.Charset || s.Collation != s.baseline.Collation || s.TimeZone != s.baseline.TimeZone || s.SQLMode != s.baseline.SQLMode:
		return Replayable, PinNone
	default:
		return Multiplexable, PinNone
	}
}

func (s *Logical) Disconnect() {
	s.Reset()
}
