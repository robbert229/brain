package auth

import (
	"sync"
	"time"
)

// DefaultTransactionTTL bounds how long a visitor may complete the provider
// round trip (research D3).
const DefaultTransactionTTL = 10 * time.Minute

// Transaction is the single-use record binding a sign-in request to its
// callback: one-time state, the nonce carried into the ID token, the S256 PKCE
// verifier, and where to land afterwards (data-model.md).
type Transaction struct {
	State             string
	Nonce             string
	Verifier          string
	PostLoginRedirect string
	CreatedAt         time.Time
}

// TransactionStore hands out and redeems single-use transactions. Deletion on
// Redeem makes replays fail by construction, even under concurrent callbacks.
type TransactionStore struct {
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	byState map[string]*Transaction
}

func NewTransactionStore(ttl time.Duration, now func() time.Time) *TransactionStore {
	if ttl <= 0 {
		ttl = DefaultTransactionTTL
	}
	if now == nil {
		now = time.Now
	}
	return &TransactionStore{ttl: ttl, now: now, byState: make(map[string]*Transaction)}
}

// Issue creates state, nonce, and verifier triples plus a sanitized redirect.
func (t *TransactionStore) Issue(redirect string) (Transaction, bool) {
	state, err := randomToken(32)
	if err != nil {
		return Transaction{}, false
	}
	nonce, err := randomToken(32)
	if err != nil {
		return Transaction{}, false
	}
	verifier, err := randomToken(32)
	if err != nil {
		return Transaction{}, false
	}
	transaction := &Transaction{
		State:             state,
		Nonce:             nonce,
		Verifier:          verifier,
		PostLoginRedirect: SafeRedirect(redirect),
		CreatedAt:         t.now(),
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(transaction.CreatedAt)
	t.byState[state] = transaction
	return *transaction, true
}

// Redeem consumes the transaction named by state. It returns false for unknown,
// expired, or already-redeemed values (SC-002).
func (t *TransactionStore) Redeem(state string) (Transaction, bool) {
	if state == "" {
		return Transaction{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	transaction, ok := t.byState[state]
	if !ok {
		return Transaction{}, false
	}
	delete(t.byState, state)
	if t.now().Sub(transaction.CreatedAt) >= t.ttl {
		return Transaction{}, false
	}
	return *transaction, true
}

// Prune reaps transactions whose TTL expired while the visitor never returned.
func (t *TransactionStore) Prune() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(t.now())
}

// Len reports live transactions, for tests and diagnostics.
func (t *TransactionStore) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(t.now())
	return len(t.byState)
}

func (t *TransactionStore) pruneLocked(now time.Time) {
	for state, transaction := range t.byState {
		if now.Sub(transaction.CreatedAt) >= t.ttl {
			delete(t.byState, state)
		}
	}
}

// SafeRedirect keeps the post-login target on this origin: a root-relative path
// without slashes or control characters, defaulting to "/" (FR-001 must not turn
// login into an open redirect).
func SafeRedirect(candidate string) string {
	if candidate == "" || len(candidate) < 2 {
		return "/"
	}
	if candidate[0] != '/' || candidate[1] == '/' {
		return "/"
	}
	for _, character := range candidate {
		if character < 0x20 || character == 0x7f {
			return "/"
		}
	}
	return candidate
}
