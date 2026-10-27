package auth

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock lets expiry tests move time without sleeping.
type fakeClock struct {
	mu      sync.Mutex
	current time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{current: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *fakeClock) Advance(by time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = c.current.Add(by)
}

func sessionPolicy() SessionPolicy {
	return SessionPolicy{IdleTimeout: time.Hour, AbsoluteLifetime: 8 * time.Hour, ReauthInterval: time.Hour}
}

// identityAt builds a verified identity anchored at the supplied instant so
// fake-clock tests control the reauth interval.
func identityAt(verifiedAt time.Time) Identity {
	return Identity{Subject: "subject-1", UserClaim: "person@example.test", Groups: []string{"access-brain-user"}, VerifiedAt: verifiedAt}
}

func identity() Identity { return identityAt(time.Now()) }

func TestSessionCreateAndLookup(t *testing.T) {
	clock := newFakeClock()
	store := NewSessionStore(sessionPolicy(), clock.Now)

	session, err := store.Create(identityAt(clock.Now()), RoleUser)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.ID == "" || session.CSRFToken == "" {
		t.Fatal("session must carry an ID and CSRF token")
	}
	if strings.ContainsAny(session.ID, ".=") {
		t.Fatalf("session id is not cookie-safe: %q", session.ID)
	}
	if session.ID == session.Subject || session.ID == session.CSRFToken {
		t.Fatal("session id must not reuse any other value")
	}
	if session.Role != RoleUser {
		t.Fatalf("unexpected role %q", session.Role)
	}
	if store.Count() != 1 {
		t.Fatalf("expected one active session, got %d", store.Count())
	}

	lookup, outcome := store.Lookup(session.ID)
	if outcome != OutcomeActive {
		t.Fatalf("expected active session, got %s", outcome)
	}
	if lookup.Subject != "subject-1" {
		t.Fatalf("unexpected subject %q", lookup.Subject)
	}

	if _, outcome := store.Lookup("unknown-id"); outcome != OutcomeExpired {
		t.Fatalf("unknown id returned %s", outcome)
	}
	if _, outcome := store.Lookup(""); outcome != OutcomeExpired {
		t.Fatalf("empty id returned %s", outcome)
	}
}

func TestSessionExpires(t *testing.T) {
	t.Run("idle timeout", func(t *testing.T) {
		clock := newFakeClock()
		store := NewSessionStore(sessionPolicy(), clock.Now)
		session, err := store.Create(identityAt(clock.Now()), RoleUser)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		clock.Advance(time.Hour)
		if _, outcome := store.Lookup(session.ID); outcome != OutcomeExpired {
			t.Fatalf("expected idle expiry, got %s", outcome)
		}
		if store.Count() != 0 {
			t.Fatalf("expired session must be removed, count=%d", store.Count())
		}
	})

	t.Run("absolute lifetime ends activity", func(t *testing.T) {
		clock := newFakeClock()
		policy := SessionPolicy{IdleTimeout: 2 * time.Hour, AbsoluteLifetime: 3 * time.Hour, ReauthInterval: 3 * time.Hour}
		store := NewSessionStore(policy, clock.Now)
		session, err := store.Create(identityAt(clock.Now()), RoleUser)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		for step := 0; step < 3; step++ {
			clock.Advance(time.Hour)
			store.Lookup(session.ID) // keep refreshing idle
		}
		if _, outcome := store.Lookup(session.ID); outcome != OutcomeExpired {
			t.Fatalf("absolute lifetime must end the session, got %s", outcome)
		}
	})

	t.Run("idle refresh keeps session alive", func(t *testing.T) {
		clock := newFakeClock()
		policy := SessionPolicy{IdleTimeout: time.Hour, AbsoluteLifetime: 8 * time.Hour, ReauthInterval: 8 * time.Hour}
		store := NewSessionStore(policy, clock.Now)
		session, err := store.Create(identityAt(clock.Now()), RoleUser)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		clock.Advance(59 * time.Minute)
		if _, outcome := store.Lookup(session.ID); outcome != OutcomeActive {
			t.Fatalf("expected active, got %s", outcome)
		}
		clock.Advance(59 * time.Minute)
		if _, outcome := store.Lookup(session.ID); outcome != OutcomeActive {
			t.Fatalf("usage must refresh idle expiry, got %s", outcome)
		}
	})
}

func TestSessionReauthIntervalMarksOutcome(t *testing.T) {
	clock := newFakeClock()
	policy := SessionPolicy{IdleTimeout: 2 * time.Hour, AbsoluteLifetime: 8 * time.Hour, ReauthInterval: time.Hour}
	store := NewSessionStore(policy, clock.Now)
	session, err := store.Create(identityAt(clock.Now()), RoleUser)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	clock.Advance(time.Hour)
	if _, outcome := store.Lookup(session.ID); outcome != OutcomeReauthRequired {
		t.Fatalf("expected reauth_required, got %s", outcome)
	}
	// A reauth-required session still counts until destroyed: it must not be
	// silently extended, and must not report active either.
	if store.Count() != 1 {
		t.Fatalf("reauth-required session should remain tracked, count=%d", store.Count())
	}
}

func TestSessionDestroyAndRotate(t *testing.T) {
	store := NewSessionStore(sessionPolicy(), nil)
	session, err := store.Create(identity(), RoleViewer)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	store.Destroy(session.ID)
	if _, outcome := store.Lookup(session.ID); outcome != OutcomeExpired {
		t.Fatalf("destroyed session still resolves: %s", outcome)
	}

	rotated, err := store.Create(identity(), RoleViewer)
	if err != nil {
		t.Fatalf("create for rotation: %v", err)
	}
	replacement, err := store.Rotate(rotated.ID, identity(), RoleUser)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if replacement.ID == rotated.ID {
		t.Fatal("rotation must issue a new session id")
	}
	if _, outcome := store.Lookup(rotated.ID); outcome != OutcomeExpired {
		t.Fatal("rotation must destroy the previous session id")
	}
	if lookup, outcome := store.Lookup(replacement.ID); outcome != OutcomeActive || lookup.Role != RoleUser {
		t.Fatalf("rotated session wrong: %s %+v", outcome, lookup)
	}
	if store.Count() != 1 {
		t.Fatalf("expected exactly one session, got %d", store.Count())
	}
}

func TestSessionConcurrentAccess(t *testing.T) {
	store := NewSessionStore(sessionPolicy(), nil)
	session, err := store.Create(identity(), RoleUser)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var workers sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			if worker%4 == 0 {
				store.Prune()
				store.Count()
				return
			}
			store.Lookup(session.ID)
		}(worker)
	}
	workers.Wait()
	if _, outcome := store.Lookup(session.ID); outcome != OutcomeActive {
		t.Fatalf("session lost under concurrency: %s", outcome)
	}
}

func TestSessionCleanupReapsExpired(t *testing.T) {
	clock := newFakeClock()
	store := NewSessionStore(SessionPolicy{IdleTimeout: 1, AbsoluteLifetime: 1, ReauthInterval: 1}, clock.Now)
	session, err := store.Create(identity(), RoleUser)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	store.StartCleanup()
	defer store.StopCleanup()
	clock.Advance(2)
	for attempt := 0; attempt < 50 && store.Count() > 0; attempt++ {
		time.Sleep(10 * time.Millisecond)
	}
	if store.Count() != 0 {
		t.Fatalf("cleanup did not reap expired session (%s)", session.ID)
	}
}
