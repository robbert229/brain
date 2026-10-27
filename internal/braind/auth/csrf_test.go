package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johnrowl/brain/internal/braind/auth"
)

// State-changing requests carrying a session must prove the sender is the same
// site that issued it (FR-008). GET and friends stay exempt, and an anonymous
// caller is rejected before the CSRF question is ever asked.
func TestStateChangingRequestRequiresCSRFToken(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})
	probe, handler := fixture.protected(t, auth.RoleViewer)

	for _, caseName := range []string{"missing", "mismatched"} {
		t.Run(caseName, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/vault/notes", strings.NewReader("title=Release"))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Accept", "application/json")
			if caseName == "mismatched" {
				request.Header.Set(auth.CSRFHeader, "not-the-session-token")
			}
			request.AddCookie(cookie)
			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("%s token = %d, want 403", caseName, recorder.Code)
			}
			if body := recorder.Body.String(); body != `{"error":"csrf_check_failed"}` {
				t.Fatalf("unexpected outcome %q", body)
			}
			if reached, _ := probe.decisions(); reached {
				t.Fatalf("%s token request reached the handler", caseName)
			}
		})
	}
}

func TestValidCSRFTokenAllowsTheRequest(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})
	probe, handler := fixture.protected(t, auth.RoleViewer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/vault/notes", nil)
	request.Header.Set(auth.CSRFHeader, fixture.csrfToken(t, cookie.Value))
	request.Header.Set("Accept", "application/json")
	request.AddCookie(cookie)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("header token = %d, want the handler to run", recorder.Code)
	}
	if reached, _ := probe.decisions(); !reached {
		t.Fatal("valid token did not reach the handler")
	}
}

func TestCSRFFormFieldIsAccepted(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})
	_, handler := fixture.protected(t, auth.RoleViewer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/vault/notes", strings.NewReader("title=Release&"+auth.CSRFFormField+"="+fixture.csrfToken(t, cookie.Value)))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.AddCookie(cookie)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("form field token = %d, want the handler to run", recorder.Code)
	}
}

func TestSafeMethodsSkipTheCSRFCheck(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-user"})
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(method, "/vault/notes", nil)
			request.Header.Set("Accept", "application/json")
			request.AddCookie(cookie)
			fixture.signer.RequireAuth(auth.Policy{Minimum: auth.RoleViewer}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})).ServeHTTP(recorder, request)

			if recorder.Code == http.StatusForbidden {
				t.Fatalf("%s was rejected by the csrf check", method)
			}
			if recorder.Code != http.StatusAccepted && method != http.MethodHead {
				t.Fatalf("%s = %d, want the handler to run", method, recorder.Code)
			}
		})
	}
}

func TestAnonymousStateChangingRequestIsUnauthenticatedFirst(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	_, handler := fixture.protected(t, auth.RoleViewer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/vault/notes", strings.NewReader("title=Release"))
	request.Header.Set("Accept", "application/json")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous write = %d, want 401", recorder.Code)
	}
	if body := recorder.Body.String(); body != `{"error":"unauthenticated"}` {
		t.Fatalf("unexpected outcome %q", body)
	}
}

func TestLogoutThroughTheMountedPolicyUsesTheSessionToken(t *testing.T) {
	fixture := newFixture(t, nil, nil)
	cookie := signedInSession(t, fixture, []string{"access-brain-viewer"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/logout", nil)
	request.Header.Set(auth.CSRFHeader, fixture.csrfToken(t, cookie.Value))
	request.Header.Set("Accept", "application/json")
	request.AddCookie(cookie)
	fixture.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", recorder.Code)
	}
	if _, outcome := fixture.sessions.Lookup(cookie.Value); outcome != auth.OutcomeExpired {
		t.Fatalf("session survived sign-out: %s", outcome)
	}
}
