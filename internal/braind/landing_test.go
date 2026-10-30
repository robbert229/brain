package braind

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

const (
	wantTagline            = "a second brain, powered by markdown"
	wantConstructionNotice = "The brain web interface is under construction."
)

// getLandingBody renders the root page through Handler() and returns its body.
func getLandingBody(t *testing.T) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET / expected status 200, got %d", recorder.Code)
	}
	return recorder.Body.String()
}

// T004 [US1] Contract tests: root page copy, headers, and method rejection.
func TestLandingPageCopy(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET / expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("unexpected Content-Type: %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, max-age=3600" {
		t.Errorf("unexpected Cache-Control: %q", got)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); got != "default-src 'self'" {
		t.Errorf("unexpected Content-Security-Policy: %q", got)
	}
	body := recorder.Body.String()
	for _, want := range []string{"brain", wantTagline, wantConstructionNotice} {
		if !strings.Contains(body, want) {
			t.Errorf("landing page body missing %q", want)
		}
	}
}

func TestLandingPageRejectsNonGET(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST / expected status %d, got %d", http.StatusMethodNotAllowed, recorder.Code)
	}
}

// T007 [US2] The page content survives scripting being disabled: strip script
// blocks and require the full copy to remain (server-rendered content).
func TestLandingPageContentIndependentOfScripts(t *testing.T) {
	stripScripts := regexp.MustCompile(`(?s)<script\b[^>]*>.*?</script>`)
	body := stripScripts.ReplaceAllString(getLandingBody(t), "")
	for _, want := range []string{"brain", wantTagline, wantConstructionNotice} {
		if !strings.Contains(body, want) {
			t.Errorf("server-rendered content missing %q after stripping scripts", want)
		}
	}
}

func TestStaticStylesheetServed(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/static/style.css", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /static/style.css expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Errorf("unexpected Content-Type: %q", got)
	}
	if strings.TrimSpace(recorder.Body.String()) == "" {
		t.Error("style.css body is empty")
	}
}

// T010 [US3] Privacy and availability: the rendered page leaks no sensitive or
// vault-shaped values, and the page serves with no configuration or state.
func TestLandingPageSensitiveContentScan(t *testing.T) {
	body := getLandingBody(t)
	probes := []string{
		"client_secret", "secret", "token", "session",
		"/notes/", "vault/", "BEGIN ", "PRIVATE KEY",
	}
	for _, probe := range probes {
		if strings.Contains(body, probe) {
			t.Errorf("landing page body contains sensitive/operational value %q", probe)
		}
	}
}

func TestLandingPageIsStatelessAndReachable(t *testing.T) {
	handler := Handler() // no config, no vault, no subsystem state
	for _, path := range []string{"/", "/static/style.css", "/static/htmx.min.js"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("GET %s expected status %d, got %d", path, http.StatusOK, recorder.Code)
		}
	}
}

// T011 [US3] Link integrity: every internal reference in the page resolves.
func TestLandingPageReferencesResolve(t *testing.T) {
	body := getLandingBody(t)
	reference := regexp.MustCompile(`(?:src|href)="(/[^"]+)"`)
	handler := Handler()
	for _, match := range reference.FindAllStringSubmatch(body, -1) {
		target := match[1]
		if !strings.HasPrefix(target, "/static/") {
			continue // only the page's own assets are in scope
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("page references %s which resolves to status %d", target, recorder.Code)
		}
	}
}
