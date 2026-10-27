package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/auth"
)

// ageSessionPastReauth moves the fixture clock beyond the reauthentication
// interval while keeping the session inside its idle window, which is the state
// FR-012 talks about: cached authorization is now stale.
func ageSessionPastReauth(t *testing.T, f *fixture, cookie *http.Cookie) {
	t.Helper()
	f.clock.Advance(59 * time.Minute)
	if _, outcome := f.sessions.Lookup(cookie.Value); outcome != auth.OutcomeActive {
		t.Fatalf("session should still be active before the interval, got %s", outcome)
	}
	f.clock.Advance(5 * time.Minute)
	if _, outcome := f.sessions.Lookup(cookie.Value); outcome != auth.OutcomeReauthRequired {
		t.Fatalf("expected reauth_required after the interval, got %s", outcome)
	}
}

// degradeProvider makes the next provider round trip fail, as an outage would.
func degradeProvider(t *testing.T, f *fixture) {
	t.Helper()
	f.simulator.SetEndpointFailure("discovery", true)
	if err := f.provider.Refresh(context.Background()); err == nil {
		t.Fatal("expected discovery to report the outage")
	}
	if f.provider.Health() != auth.HealthDegraded {
		t.Fatalf("provider health = %v, want degraded", f.provider.Health())
	}
}

// TestActiveSessionSurvivesTheProviderOutage is US3-AC1: verification happened at
// sign-in, so a live session keeps its last verified role while the provider is
// unreachable and request paths never wait on it.
func TestActiveSessionSurvivesTheProviderOutage(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})
	degradeProvider(t, fixture)

	probe, handler := fixture.protected(t, auth.RoleUser)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	request.Header.Set("Accept", "application/json")
	request.AddCookie(cookie)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("active session during outage = %d, want the request to continue", recorder.Code)
	}
	if reached, _ := probe.decisions(); !reached {
		t.Fatal("provider outage stopped a session that was still within its window")
	}

	// Sign-out still works locally while the provider is down.
	logout := httptest.NewRecorder()
	logoutRequest := httptest.NewRequest(http.MethodPost, "/logout", nil)
	logoutRequest.Header.Set(auth.CSRFHeader, fixture.csrfToken(t, cookie.Value))
	logoutRequest.Header.Set("Accept", "application/json")
	logoutRequest.AddCookie(cookie)
	fixture.handler.ServeHTTP(logout, logoutRequest)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("sign-out during outage = %d, want 204", logout.Code)
	}
}

// TestSessionPastReauthIntervalFailsClosedDuringOutage is US3-AC2 and SC-004:
// the interval bounds cached authorization, and an overdue session is neither
// refreshed nor extended while the provider cannot be consulted.
func TestSessionPastReauthIntervalFailsClosedDuringOutage(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})
	ageSessionPastReauth(t, fixture, cookie)
	degradeProvider(t, fixture)

	probe, handler := fixture.protected(t, auth.RoleUser)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	request.Header.Set("Accept", "application/json")
	request.AddCookie(cookie)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("overdue session during outage = %d, want 503", recorder.Code)
	}
	if body := recorder.Body.String(); body != `{"error":"authentication_unavailable"}` {
		t.Fatalf("unexpected outcome %q", body)
	}
	if reached, _ := probe.decisions(); reached {
		t.Fatal("stale cached authorization reached the handler")
	}

	// Repeated attempts must not extend the session: the gate holds until the
	// provider is reachable again.
	for attempt := 0; attempt < 3; attempt++ {
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
		request.Header.Set("Accept", "application/json")
		request.AddCookie(cookie)
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("attempt %d = %d, want 503 every time", attempt+1, recorder.Code)
		}
	}
	if _, outcome := fixture.sessions.Lookup(cookie.Value); outcome != auth.OutcomeReauthRequired {
		t.Fatalf("session state after the outage = %s, want reauth_required", outcome)
	}
	if fixture.sessions.Count() != 1 {
		t.Fatalf("the gate changed the session count: %d", fixture.sessions.Count())
	}
}

// TestBrowserGetsSignInRedirectWhenProviderIsReachable proves the same overdue
// session produces a recoverable outcome rather than an error page when the
// provider is healthy.
func TestBrowserGetsSignInRedirectWhenProviderIsReachable(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})
	ageSessionPastReauth(t, fixture, cookie)

	_, htmlHandler := fixture.protected(t, auth.RoleUser)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vault/notes/readme", nil)
	request.Header.Set("Accept", "text/html")
	request.AddCookie(cookie)
	htmlHandler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("browser reauth = %d, want 302", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); !strings.HasPrefix(location, "/login?next=") {
		t.Fatalf("unexpected redirect %q", location)
	}

	_, jsonHandler := fixture.protected(t, auth.RoleUser)

	jsonRecorder := httptest.NewRecorder()
	jsonRequest := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	jsonRequest.Header.Set("Accept", "application/json")
	jsonRequest.AddCookie(cookie)
	jsonHandler.ServeHTTP(jsonRecorder, jsonRequest)
	if jsonRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("json reauth = %d, want 401", jsonRecorder.Code)
	}
	if body := jsonRecorder.Body.String(); body != `{"error":"reauthentication_required"}` {
		t.Fatalf("unexpected outcome %q", body)
	}
}

// TestProviderOutageCreatesNoSession is US3-AC3: during an outage nobody becomes
// authenticated, so the protected surface cannot be reached with a fresh cookie.
func TestProviderOutageCreatesNoSession(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	degradeProvider(t, fixture)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	request.Header.Set("Accept", "application/json")
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("sign-in prompt during outage = %d, want 503", recorder.Code)
	}
	if fixture.sessions.Count() != 0 {
		t.Fatalf("sign-in during outage created sessions: %d", fixture.sessions.Count())
	}

	// A visitor who returns with an authorization code gets no session either.
	returning := newFixture(t, nil, nil)
	location := returning.authorize(t, returning.startSignIn(t, ""))
	returning.simulator.SetEndpointFailure("token", true)
	recorder = returning.callback(t, location)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("callback during outage = %d, want 303", recorder.Code)
	}
	if returning.sessions.Count() != 0 {
		t.Fatalf("callback during outage created sessions: %d", returning.sessions.Count())
	}
}

// TestReauthRotatesTheSessionOnceTheProviderRecovers ties US3 back to US1: after
// an outage the next completed sign-in replaces the stale session, so reauth
// intervals cannot accumulate.
func TestReauthRotatesTheSessionOnceTheProviderRecovers(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})
	ageSessionPastReauth(t, fixture, cookie)
	degradeProvider(t, fixture)

	probe, handler := fixture.protected(t, auth.RoleUser)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	request.Header.Set("Accept", "application/json")
	request.AddCookie(cookie)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("during outage = %d, want 503", recorder.Code)
	}

	fixture.simulator.SetEndpointFailure("discovery", false)
	if err := fixture.provider.Refresh(context.Background()); err != nil {
		t.Fatalf("provider should recover: %v", err)
	}

	recorder = fixture.callback(t, fixture.authorize(t, fixture.startSignIn(t, "")), cookie)
	if recorder.Code != http.StatusFound {
		t.Fatalf("reauth callback = %d, want 302 body=%s", recorder.Code, recorder.Body.String())
	}
	replacement := sessionCookie(t, recorder)
	if replacement.Value == cookie.Value {
		t.Fatal("reauthentication must issue a new session identifier")
	}
	if _, outcome := fixture.sessions.Lookup(cookie.Value); outcome != auth.OutcomeExpired {
		t.Fatalf("stale session survived: %s", outcome)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	request.Header.Set("Accept", "application/json")
	request.AddCookie(replacement)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("request with the rotated session = %d, want access", recorder.Code)
	}
	if reached, _ := probe.decisions(); !reached {
		t.Fatal("rotated session did not reach the handler")
	}
	if fixture.sessions.Count() != 1 {
		t.Fatalf("expected exactly one live session, got %d", fixture.sessions.Count())
	}
}
