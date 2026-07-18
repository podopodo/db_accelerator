// Package lifecycle owns observable process readiness and transition safety.
package lifecycle

import (
	"errors"
	"sync"
	"time"
)

type State string

const (
	Starting State = "starting"
	Ready    State = "ready"
	Draining State = "draining"
	Stopped  State = "stopped"
	Failed   State = "failed"
)

type Snapshot struct {
	State  State     `json:"state"`
	Since  time.Time `json:"since"`
	Reason string    `json:"reason,omitempty"`
}

type Manager struct {
	mu       sync.RWMutex
	snapshot Snapshot
}

func New(now time.Time) *Manager {
	return &Manager{snapshot: Snapshot{State: Starting, Since: now.UTC()}}
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

func (m *Manager) IsLive() bool {
	return m.Snapshot().State != Stopped
}

func (m *Manager) IsReady() bool {
	return m.Snapshot().State == Ready
}

func (m *Manager) Transition(next State, reason string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !allowed(m.snapshot.State, next) {
		return errors.New("illegal lifecycle transition from " + string(m.snapshot.State) + " to " + string(next))
	}
	m.snapshot = Snapshot{State: next, Since: now.UTC(), Reason: reason}
	return nil
}

func allowed(current, next State) bool {
	if current == next {
		return true
	}
	switch current {
	case Starting:
		return next == Ready || next == Draining || next == Failed || next == Stopped
	case Ready:
		return next == Draining || next == Failed
	case Draining:
		return next == Stopped || next == Failed
	case Failed:
		return next == Draining || next == Stopped
	case Stopped:
		return false
	default:
		return false
	}
}
