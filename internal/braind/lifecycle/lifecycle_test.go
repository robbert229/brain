package lifecycle

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestLifecycleTransitions(t *testing.T) {
	now := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.FixedZone("test", -7*60*60))
	current := now
	lifecycle := newWithClock(func() time.Time { return current })

	assertSnapshot(t, lifecycle.Snapshot(), Starting, false, now.UTC())
	current = current.Add(time.Second)
	if err := lifecycle.MarkRunning(); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	assertSnapshot(t, lifecycle.Snapshot(), Running, true, current.UTC())

	current = current.Add(time.Second)
	if err := lifecycle.MarkDegraded(true); err != nil {
		t.Fatalf("mark ready degradation: %v", err)
	}
	assertSnapshot(t, lifecycle.Snapshot(), Degraded, true, current.UTC())

	current = current.Add(time.Second)
	if err := lifecycle.MarkDegraded(false); err != nil {
		t.Fatalf("mark unready degradation: %v", err)
	}
	assertSnapshot(t, lifecycle.Snapshot(), Degraded, false, current.UTC())

	current = current.Add(time.Second)
	if changed := lifecycle.BeginShutdown(); !changed {
		t.Fatal("first shutdown transition did not change state")
	}
	assertSnapshot(t, lifecycle.Snapshot(), ShuttingDown, false, current.UTC())
	if changed := lifecycle.BeginShutdown(); changed {
		t.Fatal("second shutdown transition changed state")
	}
	if err := lifecycle.MarkRunning(); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("expected terminal shutdown state, got %v", err)
	}
}

func TestLifecycleSupportsConcurrentReaders(t *testing.T) {
	lifecycle := New()
	const readers = 16
	const iterations = 100

	var wait sync.WaitGroup
	for range readers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range iterations {
				snapshot := lifecycle.Snapshot()
				if snapshot.State == "" {
					t.Error("observed empty lifecycle state")
				}
			}
		}()
	}
	for range iterations {
		if err := lifecycle.MarkRunning(); err != nil {
			t.Fatalf("mark running: %v", err)
		}
		if err := lifecycle.MarkDegraded(true); err != nil {
			t.Fatalf("mark degraded: %v", err)
		}
	}
	wait.Wait()
}

func assertSnapshot(t *testing.T, snapshot Snapshot, state State, ready bool, changedAt time.Time) {
	t.Helper()
	if snapshot.State != state || snapshot.Ready != ready || !snapshot.ChangedAt.Equal(changedAt) {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}
