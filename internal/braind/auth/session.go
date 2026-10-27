package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// SessionOutcome is what SessionStore.Lookup reports for a cookie value.
type SessionOutcome string

const (
	// OutcomeActive: the session is usable and LastUsedAt was advanced.
	OutcomeActive SessionOutcome = "active"
	// OutcomeExpired: idle or absolute expiry reached; the record is destroyed.
	OutcomeExpired SessionOutcome = "expired"
	// OutcomeReauthRequired: the reauthentication interval elapsed while the
	// record survives so a completed sign-in can rotate it (FR-012).
	OutcomeReauthRequired SessionOutcome = "reauth_required"
)

// Identity is a provider-verified person as captured at a successful callback
// (data-model.md). It is never serialized to the browser.
type Identity struct {
	Subject    string
	UserClaim  string
	Groups     []string
	VerifiedAt time.Time
}

// Session is an opaque, time-limited reference to an authenticated identity.
type Session struct {
	ID             string
	Subject        string
	UserClaim      string
	Groups         []string
	Role           Role
	CSRFToken      string
	CreatedAt      time.Time
	LastUsedAt     time.Time
	LastVerifiedAt time.Time
}

// SessionPolicy mirrors the three configured timers.
type SessionPolicy struct {
	IdleTimeout      time.Duration
	AbsoluteLifetime time.Duration
	ReauthInterval   time.Duration
}

// SessionStore holds sessions in memory for the process lifetime. Sessions do
// not survive restart (research D2), which is acceptable because operational
// state is outside the Markdown-source principle and single-writer ownership
// already serializes restarts.
type SessionStore struct {
	policy   SessionPolicy
	now      func() time.Time
	mu       sync.Mutex
	sessions map[string]*Session

	cleanupOnce sync.Once
	stop        chan struct{}
}

// NewSessionStore creates an empty store. now may be nil for wall-clock tests.
func NewSessionStore(policy SessionPolicy, now func() time.Time) *SessionStore {
	if now == nil {
		now = time.Now
	}
	return &SessionStore{
		policy:   policy,
		now:      now,
		sessions: make(map[string]*Session),
		stop:     make(chan struct{}),
	}
}

// StartCleanup reaps expired sessions on an interval derived from the idle
// timeout, bounded so tests and short timeouts stay responsive.
func (s *SessionStore) StartCleanup() {
	s.cleanupOnce.Do(func() {
		interval := s.policy.IdleTimeout / 10
		if interval < time.Millisecond {
			interval = time.Millisecond
		}
		if interval > time.Minute {
			interval = time.Minute
		}
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-s.stop:
					return
				case <-ticker.C:
					s.Prune()
				}
			}
		}()
	})
}

// StopCleanup ends the background sweeper. Safe to call more than once.
func (s *SessionStore) StopCleanup() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
}

// Create issues a session for a verified identity. The ID is 32 crypto-random
// bytes: opaque, cookie-safe, and unrelated to any claim (FR-003).
func (s *SessionStore) Create(identity Identity, role Role) (Session, error) {
	id, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	created := s.now()
	session := &Session{
		ID:             id,
		Subject:        identity.Subject,
		UserClaim:      identity.UserClaim,
		Groups:         append([]string(nil), identity.Groups...),
		Role:           role,
		CSRFToken:      csrfToken,
		CreatedAt:      created,
		LastUsedAt:     created,
		LastVerifiedAt: identity.VerifiedAt,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(created)
	s.sessions[id] = session
	return session.copy(), nil
}

// Lookup resolves a cookie value and applies the expiry rules. An active session
// has LastUsedAt advanced; expired sessions are destroyed here so they can never
// be revived by a later request (FR-007).
func (s *SessionStore) Lookup(id string) (Session, SessionOutcome) {
	if id == "" {
		return Session{}, OutcomeExpired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.sessions[id]
	if !ok {
		return Session{}, OutcomeExpired
	}
	now := s.now()
	if now.Sub(record.CreatedAt) >= s.policy.AbsoluteLifetime {
		delete(s.sessions, id)
		return Session{}, OutcomeExpired
	}
	if now.Sub(record.LastUsedAt) >= s.policy.IdleTimeout {
		delete(s.sessions, id)
		return Session{}, OutcomeExpired
	}
	session := record.copy()
	if s.policy.ReauthInterval > 0 && now.Sub(record.LastVerifiedAt) >= s.policy.ReauthInterval {
		// Reauth is due: LastUsedAt is deliberately NOT advanced so an overdue
		// session cannot be extended while the middleware fails closed (FR-012).
		return session, OutcomeReauthRequired
	}
	record.LastUsedAt = now
	return session, OutcomeActive
}

// Rotate creates a fresh session for the same identity, destroying oldID so the
// previous cookie dies immediately. Used after a completed reauthentication
// (data-model.md transitions) so intervals cannot accumulate.
func (s *SessionStore) Rotate(oldID string, identity Identity, role Role) (Session, error) {
	session, err := s.Create(identity, role)
	if err != nil {
		return Session{}, err
	}
	if oldID != "" && oldID != session.ID {
		s.mu.Lock()
		delete(s.sessions, oldID)
		s.mu.Unlock()
	}
	return session, nil
}

// Destroy invalidates a session (sign-out).
func (s *SessionStore) Destroy(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// Prune removes sessions past either expiry limit. Count and status paths call
// this so active_sessions stays truthful without touching request paths.
func (s *SessionStore) Prune() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now())
}

// Count reports sessions that are not expired.
func (s *SessionStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now())
	return len(s.sessions)
}

func (s *SessionStore) pruneLocked(now time.Time) {
	for id, record := range s.sessions {
		if now.Sub(record.CreatedAt) >= s.policy.AbsoluteLifetime || now.Sub(record.LastUsedAt) >= s.policy.IdleTimeout {
			delete(s.sessions, id)
		}
	}
}

func (s *Session) copy() Session {
	session := *s
	session.Groups = append([]string(nil), s.Groups...)
	return session
}

// randomToken returns size cryptographically random bytes encoded for cookies.
func randomToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
