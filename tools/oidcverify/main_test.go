package main

import (
	"encoding/base64"
	"testing"
)

func TestSummarizeClaimsDoesNotReturnUnexpectedGroupNames(t *testing.T) {
	claims := map[string]any{
		"sub":            "private-subject",
		"email":          "private@example.test",
		"email_verified": true,
		"groups":         []any{"access-brain-admin", "private-unrelated-group"},
	}

	evidence, groups := summarizeClaims(claims, []string{"access-brain-admin", "access-brain-user"})
	if !evidence.SubjectPresent || !evidence.EmailPresent || !evidence.EmailIsString || !evidence.EmailVerifiedPresent {
		t.Fatalf("unexpected identity evidence: %+v", evidence)
	}
	if !evidence.GroupsPresent || !evidence.GroupsAreStrings || evidence.GroupCount != 2 {
		t.Fatalf("unexpected group evidence: %+v", evidence)
	}
	if !evidence.ExpectedMembership["access-brain-admin"] || evidence.ExpectedMembership["access-brain-user"] {
		t.Fatalf("unexpected expected-group membership: %+v", evidence.ExpectedMembership)
	}
	if len(groups) != 2 {
		t.Fatalf("internal comparison needs both groups, got %d", len(groups))
	}
}

func TestSummarizeClaimsRejectsMixedGroupArray(t *testing.T) {
	evidence, _ := summarizeClaims(map[string]any{"groups": []any{"one", 2.0}}, nil)
	if !evidence.GroupsPresent || evidence.GroupsAreStrings {
		t.Fatalf("mixed group array should not validate: %+v", evidence)
	}
}

func TestAudienceContains(t *testing.T) {
	if !audienceContains("client", "client") {
		t.Fatal("string audience did not match")
	}
	if !audienceContains([]any{"other", "client"}, "client") {
		t.Fatal("array audience did not match")
	}
	if audienceContains([]any{"other"}, "client") {
		t.Fatal("unexpected audience match")
	}
}

func TestFindRSAKey(t *testing.T) {
	modulus := base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3})
	exponent := base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1})
	key, err := findRSAKey([]jwk{{KTY: "RSA", KID: "key", N: modulus, E: exponent}}, "key")
	if err != nil {
		t.Fatalf("findRSAKey: %v", err)
	}
	if key.E != 65537 {
		t.Fatalf("unexpected exponent: %d", key.E)
	}
}

func TestNormalizeIssuer(t *testing.T) {
	got, err := normalizeIssuer("https://example.test/")
	if err != nil || got != "https://example.test" {
		t.Fatalf("normalizeIssuer = %q, %v", got, err)
	}
	if _, err := normalizeIssuer("http://example.test"); err == nil {
		t.Fatal("insecure issuer was accepted")
	}
}
