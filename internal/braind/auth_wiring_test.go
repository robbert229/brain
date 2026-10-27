package braind

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/auth/authtest"
	"github.com/johnrowl/brain/internal/braind/config"
	"github.com/johnrowl/brain/internal/braind/lifecycle"
	"github.com/johnrowl/brain/internal/braind/logging"
	"github.com/johnrowl/brain/internal/braind/status"
)

// TestAuthRoutesAreAbsentWhenOIDCDisabled keeps the 001 contract: with no
// identity provider configured the daemon exposes the same surface it always
// did, so a missing sign-in page is not a new route to probe.
func TestAuthRoutesAreAbsentWhenOIDCDisabled(t *testing.T) {
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
			t.Fatalf("%s %s = %d, want 404 when OIDC is disabled", route.method, route.path, recorder.Code)
		}
	}
}

func TestAuthRoutesServeProviderWhenOIDCEnabled(t *testing.T) {
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

	// Sign-out without a session clears nothing and leaks nothing.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/logout", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("logout without a session = %d, want 204", recorder.Code)
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

func TestAuthSurfaceWatchPublishesDiscoveryState(t *testing.T) {
	simulator := authtest.New(t)
	surface := enableOIDC(t, simulator)
	store := status.New(status.Snapshot{OIDC: status.OIDC{Enabled: true, Discovery: status.OIDCPending}}, authtest.ClientSecret, "")

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
		config.EnvOIDCEnabled:          "true",
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
		t.Fatalf("load oidc-enabled configuration: %v", err)
	}
	logger, err := logging.New(io.Discard, io.Discard, "error", authtest.ClientSecret)
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	surface, err := newAuthSurface(processConfig, logger)
	if err != nil {
		t.Fatalf("create auth surface: %v", err)
	}
	if surface == nil {
		t.Fatal("expected an auth surface for an enabled provider")
	}
	t.Cleanup(surface.Close)
	return surface
}
