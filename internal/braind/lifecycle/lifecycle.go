// Package lifecycle tracks braind's process lifecycle and readiness.
package lifecycle

import (
	"errors"
	"sync"
	"time"
)

// State is a process-wide lifecycle state.
type State string

const (
	Starting     State = "starting"
	Running      State = "running"
	Degraded     State = "degraded"
	ShuttingDown State = "shutting-down"
)

var ErrShuttingDown = errors.New("lifecycle is shutting down")

// Snapshot is an immutable point-in-time lifecycle view for probes and status
// handlers. Degraded processes may be ready when only an upstream is impaired.
type Snapshot struct {
	State     State
	Ready     bool
	ChangedAt time.Time
}

// Lifecycle safely coordinates process state across concurrent components.
type Lifecycle struct {
	mu       sync.RWMutex
	snapshot Snapshot
	now      func() time.Time
}

// New returns a lifecycle in the unready starting state.
func New() *Lifecycle {
	return newWithClock(time.Now)
}

func newWithClock(now func() time.Time) *Lifecycle {
	return &Lifecycle{
		snapshot: Snapshot{State: Starting, ChangedAt: now().UTC()},
		now:      now,
	}
}

// Snapshot returns a copy of the current lifecycle view.
func (l *Lifecycle) Snapshot() Snapshot {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.snapshot
}

// MarkRunning records that the process can serve its configured mode.
func (l *Lifecycle) MarkRunning() error {
	return l.transition(Running, true)
}

// MarkDegraded records partial impairment. ready states whether the local
// service can still satisfy its configured read/write mode.
func (l *Lifecycle) MarkDegraded(ready bool) error {
	return l.transition(Degraded, ready)
}

// BeginShutdown atomically makes readiness false before shutdown work starts.
// It is idempotent and reports whether this call changed the state.
func (l *Lifecycle) BeginShutdown() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.snapshot.State == ShuttingDown {
		return false
	}
	l.snapshot = Snapshot{
		State:     ShuttingDown,
		Ready:     false,
		ChangedAt: l.now().UTC(),
	}
	return true
}

func (l *Lifecycle) transition(state State, ready bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.snapshot.State == ShuttingDown {
		return ErrShuttingDown
	}
	if l.snapshot.State == state && l.snapshot.Ready == ready {
		return nil
	}
	l.snapshot = Snapshot{
		State:     state,
		Ready:     ready,
		ChangedAt: l.now().UTC(),
	}
	return nil
}
