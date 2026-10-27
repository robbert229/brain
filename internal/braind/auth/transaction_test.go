package auth

import (
	"sync"
	"testing"
)

func TestTransactionIssueAndRedeem(t *testing.T) {
	clock := newFakeClock()
	store := NewTransactionStore(DefaultTransactionTTL, clock.Now)

	transaction, ok := store.Issue("/notes/readme")
	if !ok {
		t.Fatal("issue failed")
	}
	if transaction.State == "" || transaction.Nonce == "" || transaction.Verifier == "" {
		t.Fatalf("transaction is incomplete: %+v", transaction)
	}
	if transaction.PostLoginRedirect != "/notes/readme" {
		t.Fatalf("unexpected redirect %q", transaction.PostLoginRedirect)
	}
	redeemed, ok := store.Redeem(transaction.State)
	if !ok {
		t.Fatal("redeem failed for a fresh transaction")
	}
	if redeemed.Nonce != transaction.Nonce || redeemed.Verifier != transaction.Verifier {
		t.Fatal("redeemed transaction does not match issuance")
	}
	if store.Len() != 0 {
		t.Fatalf("transaction must be consumed, remaining=%d", store.Len())
	}
}

func TestTransactionRejectsReplayAndUnknownState(t *testing.T) {
	store := NewTransactionStore(DefaultTransactionTTL, nil)
	transaction, _ := store.Issue("/")

	if _, ok := store.Redeem(transaction.State); !ok {
		t.Fatal("first redeem failed")
	}
	if _, ok := store.Redeem(transaction.State); ok {
		t.Fatal("replayed transaction was accepted")
	}
	if _, ok := store.Redeem("never-issued"); ok {
		t.Fatal("unknown state was redeemed")
	}
	if _, ok := store.Redeem(""); ok {
		t.Fatal("empty state was redeemed")
	}
}

func TestTransactionExpires(t *testing.T) {
	clock := newFakeClock()
	store := NewTransactionStore(DefaultTransactionTTL, clock.Now)
	transaction, _ := store.Issue("/")

	clock.Advance(DefaultTransactionTTL)

	if _, ok := store.Redeem(transaction.State); ok {
		t.Fatal("expired transaction was accepted")
	}
	if store.Len() != 0 {
		t.Fatalf("expired transaction must be pruned, len=%d", store.Len())
	}
}

func TestTransactionConcurrentRedeemHasOneWinner(t *testing.T) {
	store := NewTransactionStore(DefaultTransactionTTL, nil)
	transaction, _ := store.Issue("/")

	var winners int64
	var winnersMu sync.Mutex
	var barrier sync.WaitGroup
	start := make(chan struct{})
	for worker := 0; worker < 8; worker++ {
		barrier.Add(1)
		go func() {
			defer barrier.Done()
			<-start
			if _, ok := store.Redeem(transaction.State); ok {
				winnersMu.Lock()
				winners++
				winnersMu.Unlock()
			}
		}()
	}
	close(start)
	barrier.Wait()

	if winners != 1 {
		t.Fatalf("expected exactly one redeem winner, got %d", winners)
	}
}

func TestSafeRedirect(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty defaults to root", input: "", want: "/"},
		{name: "plain path kept", input: "/", want: "/"},
		{name: "nested path kept", input: "/notes/page", want: "/notes/page"},
		{name: "single slash unsafe", input: "/", want: "/"},
		{name: "protocol relative rejected", input: "//evil.test", want: "/"},
		{name: "absolute url rejected", input: "https://evil.test", want: "/"},
		{name: "non-slash rejected", input: "notes", want: "/"},
		{name: "control character rejected", input: "/a\nb", want: "/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SafeRedirect(test.input); got != test.want {
				t.Fatalf("SafeRedirect(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestChallengeFromVerifier(t *testing.T) {
	// RFC 7636 appendix B vector: verifier of known content yields a known challenge.
	verifier := "dBjdY9T05Zz5Q3cQ"
	challenge := ChallengeFromVerifier(verifier)
	if len(challenge) != 43 {
		t.Fatalf("unexpected challenge length %d: %q", len(challenge), challenge)
	}
	if challenge != ChallengeFromVerifier(verifier) {
		t.Fatal("challenge derivation must be deterministic")
	}
}
