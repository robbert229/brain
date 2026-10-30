package braind

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/auth/authtest"
	"github.com/johnrowl/brain/internal/braind/config"
	"github.com/johnrowl/brain/internal/braind/lifecycle"
	"github.com/johnrowl/brain/internal/braind/logging"
	"github.com/johnrowl/brain/internal/braind/status"
)

// TestUnwiredHandlerOmitsSignInRoutes keeps the 001 bootstrap contract: the
// exported unoperational Handler() has no auth surface wired, so a missing
// sign-in page is not a new route to probe. run() always wires OIDC.
func TestUnwiredHandlerOmitsSignInRoutes(t *testing.T) {
	handler := newHandler(handlerState{})
	for _, route := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/login"},
		{method: http.MethodGet, path: "/oauth2/callback"},
		{method: http.MethodPost, path: "/logout"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(route.method, route.path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s %s = %d, want 404 when no auth surface is wired", route.method, route.path, recorder.Code)
		}
	}
}

func TestAuthRoutesServeProvider(t *testing.T) {
	simulator := authtest.New(t)
	surface := enableOIDC(t, simulator)
	handler := newHandler(handlerState{auth: surface})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /login = %d, want 200 body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, required := range []string{"response_type=code", "code_challenge_method=S256", "state="} {
		if !strings.Contains(body, required) {
			t.Fatalf("sign-in prompt lacks %q: %s", required, body)
		}
	}
	if strings.Contains(body, authtest.ClientSecret) {
		t.Fatal("sign-in prompt disclosed the client secret")
	}

	// The public surface from spec 001 is unchanged.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("healthz = %d with OIDC enabled", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d with OIDC enabled", recorder.Code)
	}

	// Sign-out is a protected route: without a session it sends the browser to
	// sign in, and it still discloses nothing beyond the redirect.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/logout", nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("logout without a session = %d, want 302", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "braind_session") {
		t.Fatal("denied logout disclosed session material")
	}
}

// TestProviderOutageLeavesReadinessAlone is the FR-013 guarantee in daemon form:
// an unreachable identity provider makes sign-in unavailable and never readiness.
func TestProviderOutageLeavesReadinessAlone(t *testing.T) {
	simulator := authtest.New(t)
	surface := enableOIDC(t, simulator)
	simulator.SetEndpointFailure("discovery", true)
	process := lifecycle.New()
	if err := process.MarkRunning(); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	handler := newHandler(handlerState{auth: surface, process: process, vaultPath: t.TempDir()})

	recorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodGet, "/login", nil)
	loginRequest.Header.Set("Accept", "application/json")
	handler.ServeHTTP(recorder, loginRequest)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /login during provider outage = %d, want 503", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "authentication_unavailable") {
		t.Fatalf("json outcome not used: %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("readiness followed the provider outage: %d", recorder.Code)
	}
}

func TestProtectedRoutesApplyPolicyAtTheDaemon(t *testing.T) {
	simulator := authtest.New(t)
	surface := enableOIDC(t, simulator)
	handler := newHandler(handlerState{auth: surface})

	// Sign-out sits behind the shared policy, so an anonymous attempt is denied
	// the same way on either surface rather than silently doing nothing.
	for _, surfaceRequest := range []struct {
		accept  string
		request int
	}{
		{accept: "text/html", request: http.StatusFound},
		{accept: "application/json", request: http.StatusUnauthorized},
	} {
		recorder := httptest.NewRecorder()
		logout := httptest.NewRequest(http.MethodPost, "/logout", nil)
		logout.Header.Set("Accept", surfaceRequest.accept)
		handler.ServeHTTP(recorder, logout)
		if recorder.Code != surfaceRequest.request {
			t.Fatalf("%s logout = %d, want %d", surfaceRequest.accept, recorder.Code, surfaceRequest.request)
		}
	}

	// Unknown paths keep returning 404: the protected surface must not swallow
	// the public 001 routing contract behind an authentication redirect.
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown path = %d, want 404", recorder.Code)
	}
}

func TestAuthSurfaceWatchPublishesDiscoveryState(t *testing.T) {
	simulator := authtest.New(t)
	surface := enableOIDC(t, simulator)
	store := status.New(status.Snapshot{OIDC: status.OIDC{Discovery: status.OIDCPending}}, authtest.ClientSecret, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go surface.watch(ctx, store)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if store.Load().OIDC.Discovery == status.OIDCReady {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := store.Load().OIDC.Discovery; got != status.OIDCReady {
		t.Fatalf("discovery state = %q, want ready", got)
	}

	simulator.SetEndpointFailure("discovery", true)
	store.Update(func(snapshot *status.Snapshot) { snapshot.OIDC.Discovery = status.OIDCReady })
	surface.refresh(ctx, store)
	if got := store.Load().OIDC.Discovery; got != status.OIDCDegraded {
		t.Fatalf("discovery state after outage = %q, want degraded", got)
	}
}

// TestStatusReportsActiveSessionsAfterSignIn is FR-014 at the daemon level: the
// whole sign-in flow runs through the mux and the operator-facing status
// document counts the sessions it produced. No request path writes the count, so
// a number that tracks this flow cannot be a stale guess.
func TestStatusReportsActiveSessionsAfterSignIn(t *testing.T) {
	simulator := authtest.New(t)
	surface := enableOIDC(t, simulator)
	store := status.New(status.Snapshot{OIDC: status.OIDC{Discovery: status.OIDCPending}}, authtest.ClientSecret, "")
	handler := newHandler(handlerState{auth: surface, statusStore: store})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go surface.watch(ctx, store)
	waitForDiscovery(t, store, status.OIDCReady)

	// A browser follows the single link the sign-in page offers, the provider
	// sends it back with a code, and the callback hands it a session cookie.
	sessionCookie := signIntoDaemon(t, handler)

	if sessions := activeSessions(t, handler, surface, store); sessions != 1 {
		t.Fatalf("active_sessions = %d after one sign-in, want 1", sessions)
	}

	// Sign-out is counted too, which is what makes the number trustworthy.
	session, outcome := surface.sessions.Lookup(sessionCookie.Value)
	if outcome != auth.OutcomeActive {
		t.Fatalf("session after callback = %s, want active", outcome)
	}
	recorder := httptest.NewRecorder()
	logout := httptest.NewRequest(http.MethodPost, "/logout", nil)
	logout.Header.Set(auth.CSRFHeader, session.CSRFToken)
	logout.Header.Set("Accept", "application/json")
	logout.AddCookie(sessionCookie)
	handler.ServeHTTP(recorder, logout)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", recorder.Code)
	}
	if sessions := activeSessions(t, handler, surface, store); sessions != 0 {
		t.Fatalf("active_sessions = %d after sign-out, want 0", sessions)
	}

	for _, field := range []string{"active_sessions", "discovery"} {
		if !strings.Contains(storeJSON(t, handler), field) {
			t.Fatalf("status document lacks %q: %s", field, storeJSON(t, handler))
		}
	}
}

// activeSessions refreshes the published view the way the watch loop would and
// returns what the daemon serves at /status.
func activeSessions(t *testing.T, handler http.Handler, surface *authSurface, store *status.Store) int {
	t.Helper()
	surface.refresh(context.Background(), store)
	var document struct {
		OIDC struct {
			ActiveSessions int `json:"active_sessions"`
		} `json:"oidc"`
	}
	if err := json.Unmarshal(storeJSONBytes(t, handler), &document); err != nil {
		t.Fatalf("decode status document: %v", err)
	}
	return document.OIDC.ActiveSessions
}

func storeJSON(t *testing.T, handler http.Handler) string {
	t.Helper()
	return string(storeJSONBytes(t, handler))
}

func storeJSONBytes(t *testing.T, handler http.Handler) []byte {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	return recorder.Body.Bytes()
}

func waitForDiscovery(t *testing.T, store *status.Store, want status.OIDCDiscoveryState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if store.Load().OIDC.Discovery == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("discovery state = %q, want %q", store.Load().OIDC.Discovery, want)
}

// TestLandingPageRequiresSession pins the policy that the index page shares
// the same authentication decision as every other protected route: an
// anonymous browser is routed to sign-in carrying where it wanted to go, an
// API client gets the JSON outcome, and a verified session is served the page.
func TestLandingPageRequiresSession(t *testing.T) {
	simulator := authtest.New(t)
	surface := enableOIDC(t, simulator)
	handler := newHandler(handlerState{auth: surface})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("anonymous GET / = %d, want 302", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); location != "/login?next=%2F" {
		t.Fatalf("anonymous GET / redirected to %q, want sign-in with next=/", location)
	}

	recorder = httptest.NewRecorder()
	apiRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	apiRequest.Header.Set("Accept", "application/json")
	handler.ServeHTTP(recorder, apiRequest)
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "unauthenticated") {
		t.Fatalf("API GET / = %d %s, want 401 unauthenticated", recorder.Code, recorder.Body.String())
	}

	sessionCookie := signIntoDaemon(t, handler)
	recorder = httptest.NewRecorder()
	page := httptest.NewRequest(http.MethodGet, "/", nil)
	page.AddCookie(sessionCookie)
	handler.ServeHTTP(recorder, page)
	if recorder.Code != http.StatusOK {
		t.Fatalf("authenticated GET / = %d, want 200", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "a second brain, powered by markdown") {
		t.Fatalf("authenticated landing page is missing the tagline: %s", body)
	}
	// A session-gated page must not be retained by shared caches.
	if cache := recorder.Header().Get("Cache-Control"); cache != "private, max-age=3600" {
		t.Fatalf("landing Cache-Control = %q, want private, max-age=3600", cache)
	}
}

// signIntoDaemon walks the browser path once — sign-in prompt, provider
// authorize redirect, callback — and returns the session cookie the daemon
// issued.
func signIntoDaemon(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	match := daemonAuthorizeLink.FindStringSubmatch(recorder.Body.String())
	if match == nil {
		t.Fatalf("prompt has no sign-in link: %s", recorder.Body.String())
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	authorizeResponse, err := client.Get(strings.ReplaceAll(match[1], "&amp;", "&"))
	if err != nil {
		t.Fatalf("authorize request: %v", err)
	}
	defer authorizeResponse.Body.Close()
	if authorizeResponse.StatusCode != http.StatusFound {
		t.Fatalf("authorize status = %d", authorizeResponse.StatusCode)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, authorizeResponse.Header.Get("Location"), nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("callback = %d, want 302 body=%s", recorder.Code, recorder.Body.String())
	}
	cookie, found := sessionCookie(recorder)
	if !found {
		t.Fatalf("callback set no session cookie: %+v", recorder.Result().Cookies())
	}
	return cookie
}

func sessionCookie(recorder *httptest.ResponseRecorder) (*http.Cookie, bool) {
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == auth.CookieName {
			return cookie, true
		}
	}
	return nil, false
}

// daemonAuthorizeLink finds the provider link the sign-in prompt hands the
// browser, which is the only sign-in path a real user has.
var daemonAuthorizeLink = regexp.MustCompile(`href="([^"]+)"`)

// enableOIDC configures a daemon surface against the simulated provider. The
// public origin stays HTTPS because that is what configuration validation
// accepts; the callback path is only exercised by a real browser.
func enableOIDC(t *testing.T, simulator *authtest.Provider) *authSurface {
	t.Helper()
	root := t.TempDir()
	environment := map[string]string{
		config.EnvListenAddr:           "127.0.0.1:0",
		config.EnvExternalURL:          "https://brain.test",
		config.EnvDataPath:             root,
		config.EnvVaultPath:            root + "/vault",
		config.EnvGitDir:               root + "/git/vault.git",
		config.EnvOIDCConfigurationURL: simulator.URL(),
		config.EnvOIDCClientID:         authtest.ClientID,
		config.EnvOIDCClientSecret:     authtest.ClientSecret,
		config.EnvOIDCAdminGroup:       "access-brain-admin",
		config.EnvOIDCUserGroup:        "access-brain-user",
		config.EnvOIDCViewerGroup:      "access-brain-viewer",
		config.EnvOIDCGroupsClaim:      "groups",
		config.EnvOIDCGroupsSource:     "id_token",
		config.EnvOIDCAutoRedirect:     "false",
		config.EnvGitBackupEnabled:     "false",
		config.EnvLogLevel:             "error",
	}
	processConfig, err := config.Load(mapLookup(environment))
	if err != nil {
		t.Fatalf("load oidc configuration: %v", err)
	}
	logger, err := logging.New(io.Discard, io.Discard, "error", authtest.ClientSecret)
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	surface, err := newAuthSurface(processConfig, logger)
	if err != nil {
		t.Fatalf("create auth surface: %v", err)
	}
	t.Cleanup(surface.Close)
	return surface
}
