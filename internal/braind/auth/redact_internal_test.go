package auth

import (
	"errors"
	"strings"
	"testing"
)

// TestRedactedErrorKeepsCoarseDetail guards the diagnostic path that made the
// token-endpoint failure invisible: colon-prefixed "oidc: reason" messages
// must survive redaction intact, while JWT- and secret-shaped text never
// reaches the log (FR-009).
func TestRedactedErrorKeepsCoarseDetail(t *testing.T) {
	safe := errors.New("oidc: token endpoint returned HTTP 200 (invalid_grant)")
	if got := redactedError(safe); got != safe.Error() {
		t.Fatalf("coarse detail lost: %q", got)
	}

	token := "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJqb2huIn0.dGhpcy1pcy1hLXNpZ25hdHVyZQ"
	leaky := errors.New("oidc: validation failed for token " + token + " secret abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKL")
	got := redactedError(leaky)
	if strings.Contains(got, token) || strings.Contains(got, "abcdefghijklmnopqrstuvwxyz0123456789") {
		t.Fatalf("secret material leaked into log text: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("expected scrubbing marker: %q", got)
	}

	long := errors.New("oidc: " + strings.Repeat("x", 300))
	if len(redactedError(long)) > 200 {
		t.Fatal("error text not truncated")
	}

	if got := redactedError(nil); got != "" {
		t.Fatalf("nil error = %q, want empty", got)
	}
}
