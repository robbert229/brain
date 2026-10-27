package auth_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/auth/authtest"
)

// privacyFixture captures everything the handler writes to its logger so the
// tests can prove no token or claim material escapes into daemon logs (FR-009,
// SC-005). The daemon's own redacting handler is not involved here: if a value
// appears in this buffer, the auth package emitted it.
type privacyFixture struct {
	*fixture
	logs *bytes.Buffer
}

func newPrivacyFixture(t *testing.T, configure func(*authtest.Provider)) *privacyFixture {
	t.Helper()
	var logs bytes.Buffer
	result := &privacyFixture{logs: &logs}
	result.fixture = newFixture(t, configure, func(config *auth.HandlerConfig) {
		config.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	})
	return result
}

// sensitiveMaterial is everything a log line must never contain: tokens, the
// configured client secret, and raw claim values.
func sensitiveMaterial() []string {
	return []string{
		authtest.ClientSecret,
		"person@example.test",
		"admin@example.test",
		"access-brain-admin",
		"access-brain-user",
		"access-brain-viewer",
		"test-subject-1",
		"eyJ", // JWT header prefix, so any token material would surface here
	}
}

func (f *privacyFixture) assertClean(t *testing.T, scenario string) {
	t.Helper()
	output := f.logs.String()
	for _, secret := range sensitiveMaterial() {
		if strings.Contains(output, secret) {
			t.Fatalf("%s: log output contains claim or credential material %q: %s", scenario, secret, output)
		}
	}
	if strings.Contains(output, "id_token") || strings.Contains(output, "access_token") {
		t.Fatalf("%s: log output names a token field: %s", scenario, output)
	}
}

func TestPrivacyAcrossSignInFailures(t *testing.T) {
	scenarios := []struct {
		name      string
		configure func(*authtest.Provider)
		adjust    func(t *testing.T, f *privacyFixture, callbackURL string) string
		expect    int
	}{
		{
			name: "valid sign-in",
			configure: func(simulator *authtest.Provider) {
				simulator.SetClaims(map[string]any{"groups": []string{"access-brain-admin"}, "email": "admin@example.test"})
			},
			expect: http.StatusFound,
		},
		{
			name: "unknown state",
			adjust: func(t *testing.T, f *privacyFixture, callbackURL string) string {
				return strings.Replace(callbackURL, "state=", "state=forged-", 1)
			},
			expect: http.StatusBadRequest,
		},
		{
			name: "provider denial",
			configure: func(simulator *authtest.Provider) {
				simulator.SetAuthorizeResponse(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, "/oauth2/callback?error=access_denied&state="+r.URL.Query().Get("state"), http.StatusFound)
				})
			},
			expect: http.StatusSeeOther,
		},
		{
			name:   "provider token outage",
			expect: http.StatusSeeOther,
			adjust: func(t *testing.T, f *privacyFixture, callbackURL string) string {
				f.simulator.SetEndpointFailure("token", true)
				return callbackURL
			},
		},
		{
			name: "expired id token",
			configure: func(simulator *authtest.Provider) {
				simulator.SetTokenTTL(-time.Minute)
			},
			expect: http.StatusSeeOther,
		},
		{
			name: "unmapped groups",
			configure: func(simulator *authtest.Provider) {
				simulator.SetClaims(map[string]any{"groups": []string{"access-brain-viewer-extra"}, "email": "person@example.test"})
			},
			expect: http.StatusForbidden,
		},
		{
			name: "malformed groups",
			configure: func(simulator *authtest.Provider) {
				simulator.SetClaims(map[string]any{"groups": []any{"access-brain-user", 3}, "email": "person@example.test"})
			},
			expect: http.StatusForbidden,
		},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			test := newPrivacyFixture(t, scenario.configure)
			callbackURL := test.authorize(t, test.startSignIn(t, ""))
			if scenario.adjust != nil {
				callbackURL = scenario.adjust(t, test, callbackURL)
			}
			recorder := test.callback(t, callbackURL)
			if recorder.Code != scenario.expect {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, scenario.expect, recorder.Body.String())
			}
			test.assertClean(t, scenario.name)

			// Sign-out must also stay silent about material it knows internally.
			issued := sessionCookieOrEmpty(recorder)
			if issued == "" {
				return
			}
			cookie := &http.Cookie{Name: auth.CookieName, Value: issued}
			logout := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/logout", nil)
			request.Header.Set(auth.CSRFHeader, test.csrfToken(t, issued))
			request.AddCookie(cookie)
			test.handler.ServeHTTP(logout, request)
			test.assertClean(t, scenario.name+" sign-out")
		})
	}
}

func TestPrivacySessionCookiesAreOpaque(t *testing.T) {
	test := newPrivacyFixture(t, func(simulator *authtest.Provider) {
		simulator.SetClaims(map[string]any{"groups": []string{"access-brain-user"}, "email": "person@example.test"})
	})
	recorder := test.callback(t, test.authorize(t, test.startSignIn(t, "")))
	cookie := sessionCookie(t, recorder)
	for _, marker := range []string{".", "=", "eyJ", "example.test", "access-brain"} {
		if strings.Contains(cookie.Value, marker) {
			t.Fatalf("session cookie is not opaque: %q", cookie.Value)
		}
	}
	if _, err := base64.RawURLEncoding.DecodeString(cookie.Value); err != nil {
		t.Fatalf("session cookie is not base64url of random bytes: %v", err)
	}
	if len(cookie.Value) < 43 {
		t.Fatalf("session cookie looks too short to be 32 random bytes: %d", len(cookie.Value))
	}
	if !strings.Contains(test.logs.String(), "outcome=granted") {
		t.Fatalf("expected only the granted outcome in logs: %s", test.logs.String())
	}
	test.assertClean(t, "session cookie")
}

func TestPrivacyLoginPromptLeaksNothing(t *testing.T) {
	test := newPrivacyFixture(t, nil)
	// Discovery fails only from now on, so the fixture could still be built.
	test.simulator.SetEndpointFailure("discovery", true)
	if err := test.provider.Refresh(context.Background()); err == nil {
		t.Fatal("expected discovery failure")
	}
	recorder := httptest.NewRecorder()
	test.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), authtest.ClientID) {
		t.Fatalf("login prompt leaked the client id: %s", recorder.Body.String())
	}
	test.assertClean(t, "login prompt")
}
