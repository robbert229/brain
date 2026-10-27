package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/johnrowl/brain/internal/braind/auth"
)

// TestPolicyParityAcrossSurfaces drives the same role/requirement pairs through
// HTML-style and JSON-style requests and proves the decision is one value, not
// two. This is SC-003: a rule cannot drift between the pages and the API.
func TestPolicyParityAcrossSurfaces(t *testing.T) {
	subjects := []struct {
		name   string
		groups []string
		role   auth.Role
	}{
		{name: "viewer", groups: []string{"access-brain-viewer"}, role: auth.RoleViewer},
		{name: "user", groups: []string{"access-brain-user"}, role: auth.RoleUser},
		{name: "administrator", groups: []string{"access-brain-admin"}, role: auth.RoleAdministrator},
	}
	requirements := []struct {
		name    string
		minimum auth.Role
	}{
		{name: "public", minimum: auth.RoleNone},
		{name: "user", minimum: auth.RoleUser},
		{name: "administrator", minimum: auth.RoleAdministrator},
	}

	for _, subject := range subjects {
		for _, requirement := range requirements {
			subject, requirement := subject, requirement
			t.Run(subject.name+"/"+requirement.name, func(t *testing.T) {
				fixture := newFixture(t, nil, nil)
				cookie := signedInSession(t, fixture, subject.groups)

				var outcomes []surfaceOutcome
				for _, surface := range []string{"html", "json"} {
					probe, handler := fixture.protected(t, requirement.minimum)
					recorder := httptest.NewRecorder()
					request := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
					if surface == "json" {
						request.Header.Set("Accept", "application/json")
					} else {
						request.Header.Set("Accept", "text/html")
					}
					request.AddCookie(cookie)
					handler.ServeHTTP(recorder, request)
					reached, identity := probe.decisions()
					outcomes = append(outcomes, surfaceOutcome{
						status:      recorder.Code,
						granted:     reached,
						statusClass: classify(recorder.Code),
						subject:     identity.Subject,
					})
				}

				first, second := outcomes[0], outcomes[1]
				if first.granted != subject.role.Meets(requirement.minimum) {
					t.Fatalf("html granted = %v, want %v", first.granted, subject.role.Meets(requirement.minimum))
				}
				if first.statusClass != second.statusClass {
					t.Fatalf("surfaces disagree: html=%d(%s) json=%d(%s)", first.status, first.statusClass, second.status, second.statusClass)
				}
				if first.granted != second.granted {
					t.Fatalf("surfaces disagree on the decision: html=%v json=%v", first.granted, second.granted)
				}
				if first.granted && (first.subject == "" || second.subject != first.subject) {
					t.Fatalf("identity not shared across surfaces: %+v vs %+v", first, second)
				}
			})
		}
	}
}

type surfaceOutcome struct {
	status      int
	statusClass string
	granted     bool
	subject     string
}

// classify reduces an HTTP status to the decision it represents, so browser and
// JSON responses can be compared even though they are shaped differently.
func classify(status int) string {
	switch status {
	case http.StatusAccepted:
		return "granted"
	case http.StatusForbidden:
		return "denied"
	case http.StatusFound:
		return "sign-in"
	case http.StatusUnauthorized:
		return "unauthenticated"
	default:
		return "other"
	}
}

func TestAnonymousRequestsAreAlwaysRoutedToSignIn(t *testing.T) {
	for _, requirement := range []auth.Role{auth.RoleNone, auth.RoleUser, auth.RoleAdministrator} {
		fixture := newFixture(t, nil, nil)
		var classes []string
		for _, surface := range []string{"html", "json"} {
			probe, handler := fixture.protected(t, requirement)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/vault/notes", nil)
			if surface == "json" {
				request.Header.Set("Accept", "application/json")
			}
			handler.ServeHTTP(recorder, request)
			if reached, _ := probe.decisions(); reached {
				t.Fatalf("anonymous request reached the handler")
			}
			classes = append(classes, classify(recorder.Code))
		}
		if classes[0] != "sign-in" || classes[1] != "unauthenticated" {
			t.Fatalf("unexpected outcomes %v for minimum %q", classes, requirement)
		}
	}
}
