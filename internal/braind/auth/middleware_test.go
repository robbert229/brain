package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/auth/authtest"
)

// protectedProbe records whether policy let a request through and which identity
// it carried, so tests can assert both the decision and what reached handlers.
type protectedProbe struct {
	mu       sync.Mutex
	reached  bool
	identity auth.Identity
}

func (p *protectedProbe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reached = true
	identity, _ := auth.FromContext(r.Context())
	p.identity = identity
	w.WriteHeader(http.StatusAccepted)
}

func (p *protectedProbe) decisions() (bool, auth.Identity) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reached, p.identity
}

func (f *fixture) protected(t *testing.T, minimum auth.Role) (*protectedProbe, http.Handler) {
	t.Helper()
	probe := &protectedProbe{}
	return probe, f.signer.RequireAuth(auth.Policy{Minimum: minimum}, probe)
}

func signedInSession(t *testing.T, f *fixture, groups []string) *http.Cookie {
	t.Helper()
	f.simulator.SetClaims(map[string]any{"groups": groups, "email": "person@example.test"})
	return f.signIn(t)
}

func TestAnonymousRequestRedirectsBrowsersToSignIn(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	probe, handler := fixture.protected(t, auth.RoleViewer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vault/notes/readme", nil)
	request.Header.Set("Accept", "text/html")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("anonymous browser request = %d, want 302", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); location != "/login?next=%2Fvault%2Fnotes%2Freadme" {
		t.Fatalf("unexpected redirect %q", location)
	}
	if reached, _ := probe.decisions(); reached {
		t.Fatal("anonymous request reached the handler")
	}
}

func TestAnonymousJSONRequestIsUnauthenticated(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	probe, handler := fixture.protected(t, auth.RoleViewer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	request.Header.Set("Accept", "application/json")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous json request = %d, want 401", recorder.Code)
	}
	if body := recorder.Body.String(); body != `{"error":"unauthenticated"}` {
		t.Fatalf("unexpected outcome %q", body)
	}
	if reached, _ := probe.decisions(); reached {
		t.Fatal("anonymous request reached the handler")
	}
}

func TestAPIShapedPathAlwaysGetsJSONOutcome(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	_, handler := fixture.protected(t, auth.RoleViewer)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("/api path = %d, want 401 without any Accept negotiation", recorder.Code)
	}
}

func TestExpiredSessionIsRejectedLikeAnAnonymousRequest(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})
	fixture.sessions.Destroy(cookie.Value)
	_, handler := fixture.protected(t, auth.RoleUser)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	request.Header.Set("Accept", "application/json")
	request.AddCookie(cookie)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("destroyed session = %d, want 401", recorder.Code)
	}
}

func TestActiveSessionReachesHandlerWithIdentity(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})
	probe, handler := fixture.protected(t, auth.RoleViewer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	request.AddCookie(cookie)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("authorized request = %d body=%s", recorder.Code, recorder.Body.String())
	}
	reached, identity := probe.decisions()
	if !reached {
		t.Fatal("handler was not reached")
	}
	if identity.Subject == "" {
		t.Fatal("handler reached without the verified identity in context")
	}
	if identity.UserClaim != "person@example.test" {
		t.Fatalf("unexpected identity %+v", identity)
	}
}

func TestRoleBelowRequirementIsForbidden(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-viewer"})
	probe, handler := fixture.protected(t, auth.RoleUser)

	for _, surface := range []struct {
		name   string
		accept string
	}{
		{name: "browser", accept: "text/html"},
		{name: "json", accept: "application/json"},
	} {
		t.Run(surface.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
			request.Header.Set("Accept", surface.accept)
			request.AddCookie(cookie)
			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("%s request = %d, want 403", surface.name, recorder.Code)
			}
			if !strings.Contains(recorder.Body.String(), "Access denied") && !strings.Contains(recorder.Body.String(), "forbidden") {
				t.Fatalf("unexpected body %q", recorder.Body.String())
			}
		})
	}
	if reached, _ := probe.decisions(); reached {
		t.Fatal("insufficient role reached the handler")
	}
}

func TestOverlappingGroupsResolveToHighestRole(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-viewer", "access-brain-user", "access-brain-admin"})

	probe, administratorOnly := fixture.protected(t, auth.RoleAdministrator)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vault/settings", nil)
	request.AddCookie(cookie)
	administratorOnly.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("multi-group session = %d, want access to the administrator route", recorder.Code)
	}
	if reached, _ := probe.decisions(); !reached {
		t.Fatal("highest matching group did not win")
	}
}

func TestUnmappedIdentityNeverReachesProtectedRoutes(t *testing.T) {
	// Sign-in denies first, so there is no session to present: the protected
	// surface treats the caller as unauthenticated (US2-AC3).
	fixture := newFixture(t, func(simulator *authtest.Provider) {
		simulator.SetClaims(map[string]any{"groups": []string{"someone-else"}, "email": "visitor@example.test"})
	}, nil)
	recorder := fixture.callback(t, fixture.authorize(t, fixture.startSignIn(t, "")))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("sign-in for an unmapped identity = %d, want 403", recorder.Code)
	}

	_, handler := fixture.protected(t, auth.RoleViewer)
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	request.Header.Set("Accept", "application/json")
	request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "stolen-or-forged"})
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("forged cookie = %d, want 401", recorder.Code)
	}
}

func TestProtectedRoutesAreMountedByRegister(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/logout", nil)
	request.AddCookie(cookie)
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("logout without a csrf token = %d, want 403 through the mounted policy", recorder.Code)
	}
}

func TestAnonymousSignoutFollowsTheProtectedPolicy(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/logout", nil)
	request.Header.Set("Accept", "application/json")
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous logout = %d, want 401", recorder.Code)
	}
}

func TestAnonymousRequestOutcomeIsSurfaceSpecificButDecisionsMatch(t *testing.T) {
	// US2-AC4: one policy answers both surfaces. Representation differs (redirect
	// versus outcome code), the decision to deny never does.
	fixture := newFixture(t, nil, nil)
	_, handler := fixture.protected(t, auth.RoleUser)

	htmlRecorder := httptest.NewRecorder()
	htmlRequest := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	htmlRequest.Header.Set("Accept", "text/html")
	handler.ServeHTTP(htmlRecorder, htmlRequest)

	jsonRecorder := httptest.NewRecorder()
	jsonRequest := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
	jsonRequest.Header.Set("Accept", "application/json")
	handler.ServeHTTP(jsonRecorder, jsonRequest)

	deniedBoth := htmlRecorder.Code != http.StatusOK && jsonRecorder.Code != http.StatusOK
	if !deniedBoth {
		t.Fatalf("surfaces disagree: html=%d json=%d", htmlRecorder.Code, jsonRecorder.Code)
	}
}
