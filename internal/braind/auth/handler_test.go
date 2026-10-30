package auth_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/auth/authtest"
)

type fixture struct {
	handler      http.Handler
	signer       *auth.Handler
	simulator    *authtest.Provider
	sessions     *auth.SessionStore
	transactions *auth.TransactionStore
	provider     *auth.Provider
	clock        *fakeClock
}

// fakeClock lets transaction-expiry tests move time without sleeping.
type fakeClock struct {
	current time.Time
}

func (c *fakeClock) Now() time.Time { return c.current }

func (c *fakeClock) Advance(by time.Duration) { c.current = c.current.Add(by) }

// authorizeLink extracts the provider authorization URL from the prompt's
// sign-in anchor. It is anchored on the anchor itself because the pages'
// heads now also carry the shared stylesheet href (see
// TestAuthPagesAlignWithLanding).
var authorizeLink = regexp.MustCompile(`<a role="button" class="sign-in" href="([^"]+)">Sign in</a>`)

// TestAuthPagesAlignWithLanding pins the presentation cleanup: the sign-in
// and failure pages render from embedded templates (like the landing page),
// are styled by the shared stylesheet, and carry the landing page's CSP.
func TestAuthPagesAlignWithLanding(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	for _, page := range []struct {
		name string
		path string
	}{
		{"sign-in prompt", "/login"},
		{"failure page", "/oauth2/callback"}, // missing state renders the HTML failure page
	} {
		recorder := httptest.NewRecorder()
		fixture.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, page.path, nil))
		if got := recorder.Header().Get("Content-Security-Policy"); got != "default-src 'self'" {
			t.Errorf("%s CSP = %q, want default-src 'self'", page.name, got)
		}
		if body := recorder.Body.String(); !strings.Contains(body, `href="/static/pico.min.css"`) || !strings.Contains(body, `href="/static/style.css"`) {
			t.Errorf("%s does not link the shared stylesheets (Pico + brand): %s", page.name, body)
		}
	}
}

// TestSignedInVisitorLeavesLogin verifies /login sends an authenticated
// visitor onward instead of offering a second sign-in: they arrived via a
// stale bookmark, the back button, or a leftover ?login_error, and they are
// headed to their (sanitized) intended destination.
func TestSignedInVisitorLeavesLogin(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := fixture.signIn(t)
	visit := func(target string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.AddCookie(cookie)
		fixture.handler.ServeHTTP(recorder, request)
		return recorder
	}
	if recorder := visit("/login?next=/notes/readme&login_error=provider_error"); recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "/notes/readme" {
		t.Fatalf("signed-in /login = %d -> %q, want 302 -> /notes/readme", recorder.Code, recorder.Header().Get("Location"))
	}
	if recorder := visit("/login?next=%2F%2Fevil.test"); recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "/" {
		t.Fatalf("foreign next = %d -> %q, want 302 -> /", recorder.Code, recorder.Header().Get("Location"))
	}
	anonymous := httptest.NewRecorder()
	fixture.handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/login", nil))
	if anonymous.Code != http.StatusOK || !strings.Contains(anonymous.Body.String(), "Sign in to Brain") {
		t.Fatalf("anonymous /login must still render the prompt, got %d", anonymous.Code)
	}
}

func newFixture(t *testing.T, configure func(*authtest.Provider), mutate func(*auth.HandlerConfig)) *fixture {
	t.Helper()
	simulator := authtest.New(t)
	if configure != nil {
		configure(simulator)
	}
	provider, err := auth.NewProvider(auth.ProviderConfig{
		ConfigurationURL: simulator.URL(),
		ClientID:         authtest.ClientID,
		ClientSecret:     authtest.ClientSecret,
		RedirectURL:      "/oauth2/callback",
		HTTPClient:       &http.Client{},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := provider.Refresh(context.Background()); err != nil {
		t.Fatalf("discover provider: %v", err)
	}
	clock := &fakeClock{current: time.Now()}
	sessions := auth.NewSessionStore(auth.SessionPolicy{
		IdleTimeout:      8 * time.Hour,
		AbsoluteLifetime: 8 * time.Hour,
		ReauthInterval:   time.Hour,
	}, clock.Now)
	transactions := auth.NewTransactionStore(auth.DefaultTransactionTTL, clock.Now)

	config := auth.HandlerConfig{
		Provider:     provider,
		Sessions:     sessions,
		Transactions: transactions,
		Roles: auth.RoleConfig{
			AdminGroup:  "access-brain-admin",
			UserGroup:   "access-brain-user",
			ViewerGroup: "access-brain-viewer",
		},
		GroupsClaim:  "groups",
		UserClaim:    "email",
		GroupsSource: "id_token",
		BaseURL:      simulator.Issuer(),
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		// The handler must stamp verification with the same clock the session
		// store reads, otherwise time travel tests disagree about "now".
		Clock: clock.Now,
	}
	if mutate != nil {
		mutate(&config)
	}
	handler, err := auth.NewHandler(config)
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	return &fixture{handler: mux, signer: handler, simulator: simulator, sessions: sessions, transactions: transactions, provider: provider, clock: clock}
}

// startSignIn requests the prompt and returns the provider authorization URL as
// a browser would follow it.
func (f *fixture) startSignIn(t *testing.T, next string) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	if next != "" {
		request.URL.RawQuery = "next=" + next
	}
	f.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login prompt status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	match := authorizeLink.FindStringSubmatch(recorder.Body.String())
	if match == nil {
		t.Fatalf("prompt has no sign-in link: %s", recorder.Body.String())
	}
	return strings.ReplaceAll(match[1], "&amp;", "&")
}

// authorize asks the provider to authorize the browser and returns the callback
// URL it redirects to.
func (f *fixture) authorize(t *testing.T, authorizeURL string) string {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(authorizeURL)
	if err != nil {
		t.Fatalf("authorize request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("authorize status = %d", response.StatusCode)
	}
	return response.Header.Get("Location")
}

func (f *fixture) callback(t *testing.T, callbackURL string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

// signIn performs one complete sign-in and returns the issued session cookie.
func (f *fixture) signIn(t *testing.T) *http.Cookie {
	t.Helper()
	recorder := f.callback(t, f.authorize(t, f.startSignIn(t, "")))
	if recorder.Code != http.StatusFound {
		t.Fatalf("callback status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	cookie := sessionCookie(t, recorder)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("session cookie attributes wrong: %+v", cookie)
	}
	return cookie
}

func (f *fixture) csrfToken(t *testing.T, sessionID string) string {
	t.Helper()
	session, outcome := f.sessions.Lookup(sessionID)
	if outcome != auth.OutcomeActive {
		t.Fatalf("expected an active session, got %s", outcome)
	}
	return session.CSRFToken
}

func sessionCookie(t *testing.T, recorder *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == auth.CookieName {
			return cookie
		}
	}
	t.Fatalf("no session cookie in response: %s", recorder.Body.String())
	return nil
}

func noSessionCookie(recorder *httptest.ResponseRecorder) bool {
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == auth.CookieName && cookie.Value != "" {
			return false
		}
	}
	return true
}

func TestSignInCreatesSessionWithResolvedRole(t *testing.T) {
	fixture := newFixture(t, func(simulator *authtest.Provider) {
		simulator.SetClaims(map[string]any{"groups": []string{"access-brain-admin"}, "email": "admin@example.test"})
	}, nil)

	cookie := fixture.signIn(t)
	if cookie.Value == "" {
		t.Fatal("expected an opaque session value")
	}
	session, outcome := fixture.sessions.Lookup(cookie.Value)
	if outcome != auth.OutcomeActive {
		t.Fatalf("expected an active session, got %s", outcome)
	}
	if session.Role != auth.RoleAdministrator {
		t.Fatalf("expected administrator role, got %q", session.Role)
	}
	if session.Subject == "" {
		t.Fatal("session must remember the verified subject")
	}
	// US1-AC1: the cookie must be opaque, so it must not encode claims.
	for _, marker := range []string{".", "eyJ", "admin@example"} {
		if strings.Contains(cookie.Value, marker) {
			t.Fatalf("session cookie leaks claim material: %q", cookie.Value)
		}
	}
}

func TestSignInLandsOnRequestedPage(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	recorder := fixture.callback(t, fixture.authorize(t, fixture.startSignIn(t, "/notes/readme")))
	if recorder.Code != http.StatusFound {
		t.Fatalf("callback status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Location"); got != "/notes/readme" {
		t.Fatalf("post-login redirect = %q", got)
	}
}

func TestSignInIgnoresForeignNextTarget(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	recorder := fixture.callback(t, fixture.authorize(t, fixture.startSignIn(t, "//evil.test")))
	if got := recorder.Header().Get("Location"); got != "/" {
		t.Fatalf("next target must be confined to this origin, got %q", got)
	}
}

func TestCallbackRejectsInvalidTransactions(t *testing.T) {
	t.Run("unknown state", func(t *testing.T) {
		fixture := newFixture(t, nil, nil)
		location := fixture.authorize(t, fixture.startSignIn(t, ""))
		recorder := fixture.callback(t, strings.Replace(location, "state=", "state=forged-", 1))
		if recorder.Code != http.StatusBadRequest || !noSessionCookie(recorder) {
			t.Fatalf("expected 400 without session, got %d cookie=%v", recorder.Code, sessionCookieOrEmpty(recorder))
		}
	})

	t.Run("missing state", func(t *testing.T) {
		fixture := newFixture(t, nil, nil)
		location := fixture.authorize(t, fixture.startSignIn(t, ""))
		index := strings.Index(location, "&state=")
		if index < 0 {
			t.Fatalf("provider omitted state: %q", location)
		}
		recorder := fixture.callback(t, location[:index])
		if recorder.Code != http.StatusBadRequest || !noSessionCookie(recorder) {
			t.Fatalf("expected 400 without session, got %d", recorder.Code)
		}
	})

	t.Run("missing code", func(t *testing.T) {
		fixture := newFixture(t, nil, nil)
		recorder := fixture.callback(t, "/oauth2/callback?state=anything")
		if recorder.Code != http.StatusBadRequest || !noSessionCookie(recorder) {
			t.Fatalf("expected 400 without session, got %d", recorder.Code)
		}
	})

	t.Run("expired transaction", func(t *testing.T) {
		fixture := newFixture(t, nil, nil)
		location := fixture.authorize(t, fixture.startSignIn(t, ""))
		fixture.clock.Advance(auth.DefaultTransactionTTL)
		recorder := fixture.callback(t, location)
		if recorder.Code != http.StatusBadRequest || !noSessionCookie(recorder) {
			t.Fatalf("expected 400 without session, got %d", recorder.Code)
		}
	})
}

func TestCallbackRejectsReplay(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	location := fixture.authorize(t, fixture.startSignIn(t, ""))

	first := fixture.callback(t, location)
	if first.Code != http.StatusFound {
		t.Fatalf("first callback status = %d body=%s", first.Code, first.Body.String())
	}
	second := fixture.callback(t, location)
	if second.Code != http.StatusBadRequest {
		t.Fatalf("replay status = %d, want 400", second.Code)
	}
	if !noSessionCookie(second) {
		t.Fatal("replayed callback issued a session")
	}
}

func TestCallbackHonorsProviderErrorRedirect(t *testing.T) {
	fixture := newFixture(t, func(simulator *authtest.Provider) {
		simulator.SetAuthorizeResponse(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/oauth2/callback?error=access_denied&state="+r.URL.Query().Get("state"), http.StatusFound)
		})
	}, nil)
	recorder := fixture.callback(t, fixture.authorize(t, fixture.startSignIn(t, "")))
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if location := recorder.Header().Get("Location"); !strings.HasPrefix(location, "/login?login_error=access_denied") {
		t.Fatalf("unexpected redirect %q", location)
	}
	if !noSessionCookie(recorder) {
		t.Fatal("provider error must not create a session")
	}
}

func TestCallbackStopsWhenProviderUnavailable(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	location := fixture.authorize(t, fixture.startSignIn(t, ""))
	fixture.simulator.SetEndpointFailure("token", true)

	recorder := fixture.callback(t, location)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 during provider outage, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Header().Get("Location"), "login_error=provider_unavailable") {
		t.Fatalf("unexpected redirect %q", recorder.Header().Get("Location"))
	}
	if !noSessionCookie(recorder) {
		t.Fatal("failed exchange must not create a session")
	}
}

func TestCallbackRejectsExpiredIDToken(t *testing.T) {
	fixture := newFixture(t, func(simulator *authtest.Provider) {
		simulator.SetTokenTTL(-time.Minute)
	}, nil)
	recorder := fixture.callback(t, fixture.authorize(t, fixture.startSignIn(t, "")))
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 back to login for an expired token, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !noSessionCookie(recorder) {
		t.Fatal("expired token must not create a session")
	}
}

func TestCallbackDeniesIdentitiesWithoutRoles(t *testing.T) {
	cases := map[string]map[string]any{
		"unmapped group": {"groups": []string{"someone-else"}, "email": "visitor@example.test"},
		"absent claim":   {"email": "person@example.test"},
		"non-string":     {"groups": []any{"ok", 3}, "email": "person@example.test"},
		"object":         {"groups": map[string]any{"x": 1}, "email": "person@example.test"},
		"partial match":  {"groups": []string{"access-brain-us"}, "email": "person@example.test"},
	}
	for name, claims := range cases {
		claims := claims
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t, func(simulator *authtest.Provider) {
				simulator.SetClaims(claims)
			}, nil)
			recorder := fixture.callback(t, fixture.authorize(t, fixture.startSignIn(t, "")))
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d body=%s", recorder.Code, recorder.Body.String())
			}
			if !noSessionCookie(recorder) {
				t.Fatal("denied identity must not receive a session")
			}
			if sessionCookieOrEmpty(recorder) != "" {
				t.Fatal("denied identity must not receive a session cookie")
			}
		})
	}
}

func TestCallbackReadsGroupsFromUserinfo(t *testing.T) {
	fixture := newFixture(t, func(simulator *authtest.Provider) {
		// The ID token carries no groups; the userinfo endpoint is authoritative.
		simulator.SetClaims(map[string]any{"email": "person@example.test"})
		simulator.SetUserinfoClaims(map[string]any{"groups": []string{"access-brain-user"}})
	}, func(config *auth.HandlerConfig) {
		config.GroupsSource = "userinfo"
	})
	cookie := fixture.signIn(t)
	session, outcome := fixture.sessions.Lookup(cookie.Value)
	if outcome != auth.OutcomeActive {
		t.Fatalf("expected active session, got %s", outcome)
	}
	if session.Role != auth.RoleUser {
		t.Fatalf("expected userinfo groups to resolve user, got %q", session.Role)
	}
}

func TestCallbackReauthRotatesSession(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	first := fixture.signIn(t)

	recorder := fixture.callback(t, fixture.authorize(t, fixture.startSignIn(t, "")), first)
	if recorder.Code != http.StatusFound {
		t.Fatalf("reauth status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	replacement := sessionCookie(t, recorder)
	if replacement.Value == first.Value {
		t.Fatal("reauthentication must rotate the session identifier")
	}
	if _, outcome := fixture.sessions.Lookup(first.Value); outcome != auth.OutcomeExpired {
		t.Fatalf("previous session must be destroyed, got %s", outcome)
	}
	if _, outcome := fixture.sessions.Lookup(replacement.Value); outcome == auth.OutcomeExpired {
		t.Fatal("rotated session must be usable")
	}
}

func TestSignoutClearsLocalSession(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := fixture.signIn(t)

	recorder := postLogout(t, fixture, cookie, fixture.csrfToken(t, cookie.Value))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, outcome := fixture.sessions.Lookup(cookie.Value); outcome != auth.OutcomeExpired {
		t.Fatalf("session survived sign-out: %s", outcome)
	}
	for _, issued := range recorder.Result().Cookies() {
		if issued.Name == auth.CookieName && issued.MaxAge >= 0 {
			t.Fatalf("cookie not expired: %+v", issued)
		}
	}
}

func TestSignoutUsesProviderLogoutWhenAdvertised(t *testing.T) {
	fixture := newFixture(t, func(simulator *authtest.Provider) {
		simulator.EnableLogout()
	}, nil)
	cookie := fixture.signIn(t)

	recorder := postLogout(t, fixture, cookie, fixture.csrfToken(t, cookie.Value))
	if recorder.Code != http.StatusFound {
		t.Fatalf("expected provider logout redirect, got %d", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); !strings.Contains(location, "/logout") {
		t.Fatalf("expected end_session_endpoint redirect, got %q", location)
	}
}

func TestSignoutWithProviderDownStillClearsLocalSession(t *testing.T) {
	fixture := newFixture(t, func(simulator *authtest.Provider) {
		simulator.EnableLogout()
	}, nil)
	cookie := fixture.signIn(t)
	fixture.simulator.SetEndpointFailure("logout", true)

	recorder := postLogout(t, fixture, cookie, fixture.csrfToken(t, cookie.Value))
	if recorder.Code != http.StatusNoContent && recorder.Code != http.StatusFound {
		t.Fatalf("unexpected logout status %d", recorder.Code)
	}
	if _, outcome := fixture.sessions.Lookup(cookie.Value); outcome != auth.OutcomeExpired {
		t.Fatal("provider outage must not keep the local session alive")
	}
}

func TestSignoutRequiresCSRF(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := fixture.signIn(t)

	recorder := postLogout(t, fixture, cookie, "")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing csrf token, got %d", recorder.Code)
	}
	if _, outcome := fixture.sessions.Lookup(cookie.Value); outcome == auth.OutcomeExpired {
		t.Fatal("failed csrf must not destroy the session")
	}
}

func TestSignoutAcceptsHeaderToken(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := fixture.signIn(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/logout", nil)
	request.Header.Set(auth.CSRFHeader, fixture.csrfToken(t, cookie.Value))
	request.AddCookie(cookie)
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSignoutWithoutSessionIsNotAuthorized(t *testing.T) {
	fixture := newFixture(t, nil, nil)

	// Sign-out is a protected route, so an anonymous visitor recovers by signing
	// in, and a client asking for JSON gets the same decision as an outcome code.
	recorder := httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/logout", nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("browser sign-out without a session = %d, want 302", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); location != "/login?next=%2Flogout" {
		t.Fatalf("unexpected redirect %q", location)
	}

	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/logout", nil)
	request.Header.Set("Accept", "application/json")
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("json sign-out without a session = %d, want 401", recorder.Code)
	}
	if body := recorder.Body.String(); body != `{"error":"unauthenticated"}` {
		t.Fatalf("unexpected json outcome %q", body)
	}
}

func TestLoginServesPromptOrRedirect(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	authorizeURL := fixture.startSignIn(t, "")
	for _, required := range []string{"response_type=code", "code_challenge_method=S256", "state=", "nonce="} {
		if !strings.Contains(authorizeURL, required) {
			t.Fatalf("authorize url lacks %q: %s", required, authorizeURL)
		}
	}
}

func TestLoginReportsAuthenticationUnavailable(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	fixture.simulator.SetEndpointFailure("discovery", true)
	if err := fixture.provider.Refresh(context.Background()); err == nil {
		t.Fatal("expected discovery refresh to fail")
	}
	recorder := httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when discovery is down, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	request.Header.Set("Accept", "application/json")
	fixture.handler.ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), "authentication_unavailable") {
		t.Fatalf("json outcome missing: %s", recorder.Body.String())
	}
}

func TestAutoRedirectSendsTheBrowserToTheProvider(t *testing.T) {
	fixture := newFixture(t, nil, func(config *auth.HandlerConfig) {
		config.AutoRedirect = true
	})
	recorder := httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("auto-redirect status = %d", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); !strings.HasPrefix(location, fixture.simulator.Issuer()+"/authorize") {
		t.Fatalf("unexpected location %q", location)
	}
}

func postLogout(t *testing.T, fixture *fixture, cookie *http.Cookie, csrfToken string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader("csrf_token="+csrfToken))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(cookie)
	fixture.handler.ServeHTTP(recorder, request)
	return recorder
}

func sessionCookieOrEmpty(recorder *httptest.ResponseRecorder) string {
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == auth.CookieName {
			return cookie.Value
		}
	}
	return ""
}
