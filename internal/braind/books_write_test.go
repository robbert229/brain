package braind

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/auth/authtest"
	"github.com/johnrowl/brain/internal/braind/config"
)

// booksWriteFixture builds the same data-dir wiring as newBooksFixture with
// knobs for read-only mode and per-role sign-in.
type booksWriteFixture struct {
	booksFixture
	simulator *authtest.Provider
	surface   *authSurface
}

func newBooksWriteFixture(t *testing.T, readOnly bool) *booksWriteFixture {
	t.Helper()
	simulator := authtest.New(t)
	surface := enableOIDC(t, simulator)
	dataDir := t.TempDir()
	fixture := &booksWriteFixture{simulator: simulator, surface: surface}
	fixture.dataDir = dataDir
	fixture.booksDir = filepath.Join(dataDir, "01 - Note Box/Hobby/Books")
	fixture.coversDir = filepath.Join(dataDir, "06 - Meta/Attachments")
	fixture.handler = newHandler(handlerState{
		auth: surface,
		books: newBooksSurface(config.Vault{
			DataPath:     dataDir,
			BooksFolder:  "01 - Note Box/Hobby/Books",
			CoversFolder: "06 - Meta/Attachments",
			ReadOnly:     readOnly,
			MaxNoteBytes: 2 << 20,
		}),
	})
	return fixture
}

func (f *booksWriteFixture) signIn(t *testing.T, group string) *http.Cookie {
	t.Helper()
	return signIntoDaemonAs(t, f.handler, f.simulator, group)
}

func (f *booksWriteFixture) csrfFor(t *testing.T, cookie *http.Cookie) string {
	t.Helper()
	session, outcome := f.surface.sessions.Lookup(cookie.Value)
	if outcome != auth.OutcomeActive || session.CSRFToken == "" {
		t.Fatalf("session for CSRF token = %s", outcome)
	}
	return session.CSRFToken
}

func postReview(t *testing.T, f *booksWriteFixture, cookie *http.Cookie, form url.Values, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/books", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if csrf != "" {
		request.Header.Set(auth.CSRFHeader, csrf)
	}
	request.AddCookie(cookie)
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func TestReviewFormGatedByRole(t *testing.T) {
	fixture := newBooksWriteFixture(t, false)
	viewer := fixture.signIn(t, "access-brain-viewer")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/books/new", nil)
	request.AddCookie(viewer)
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer GET /books/new = %d, want 403 (FR-009)", recorder.Code)
	}

	member := fixture.signIn(t, "access-brain-user")
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/books/new", nil)
	request.AddCookie(member)
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("member GET /books/new = %d, want 200 body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{`method="post"`, `action="/books"`, `name="csrf_token"`, "Save review", `name="title"`, `name="review"`} {
		if !strings.Contains(body, want) {
			t.Errorf("review form lacks %q", want)
		}
	}
	// Principle IV: an ordinary form, no htmx dependency.
	if strings.Contains(body, "hx-post") {
		t.Error("form must work as an ordinary POST (Principle IV)")
	}

	// The shelf offers the link to contributors, not to viewers.
	shelfFor := func(cookie *http.Cookie) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/books", nil)
		req.AddCookie(cookie)
		fixture.handler.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	if !strings.Contains(shelfFor(member), `href="/books/new"`) {
		t.Error("contributor shelf must offer Add a review")
	}
	if strings.Contains(shelfFor(viewer), `href="/books/new"`) {
		t.Error("viewer shelf must not offer Add a review (SC-004)")
	}
}

func TestReviewSubmissionRoundTrip(t *testing.T) {
	fixture := newBooksWriteFixture(t, false)
	member := fixture.signIn(t, "access-brain-user")
	csrf := fixture.csrfFor(t, member)

	form := url.Values{
		"title":    {"Dungeon Crawler Carl 7"},
		"author":   {"Matt Dinniman"},
		"rating":   {"5/5"},
		"cover":    {"dcc7.jpg"},
		"finished": {"2026-10-01"},
		"review":   {"Crazier than ever. Jeff delivers."},
	}
	recorder := postReview(t, fixture, member, form, csrf)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("POST /books = %d body=%s, want 303", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Location") != "/books" {
		t.Fatalf("Location = %q, want /books", recorder.Header().Get("Location"))
	}
	content, err := os.ReadFile(filepath.Join(fixture.booksDir, "dungeon-crawler-carl-7.md"))
	if err != nil {
		t.Fatalf("note not filed: %v", err)
	}
	for _, want := range []string{"author: Matt Dinniman", "rating: 5/5", "cover: dcc7.jpg", "finished: 2026-10-01", "# Dungeon Crawler Carl 7", "![[dcc7.jpg]]"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("stored note lacks %q:\n%s", want, content)
		}
	}
	// The shelf shows it (SC-002).
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/books", nil)
	request.AddCookie(member)
	fixture.handler.ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), "Dungeon Crawler Carl 7") {
		t.Error("new review must appear on the shelf")
	}
}

func TestReviewValidationReRendersForm(t *testing.T) {
	fixture := newBooksWriteFixture(t, false)
	member := fixture.signIn(t, "access-brain-user")
	recorder := postReview(t, fixture, member, url.Values{
		"title":  {"Kept Value"},
		"review": {"   "},
	}, fixture.csrfFor(t, member))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty review = %d, want 422 body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `value="Kept Value"`) {
		t.Error("form must preserve the submitted title")
	}
	if !strings.Contains(body, "A review is required") {
		t.Error("form must show the field error")
	}
	entries, _ := os.ReadDir(fixture.booksDir)
	if len(entries) != 0 {
		t.Fatalf("invalid submission wrote files: %v", entries)
	}
}

func TestReviewCreateRefusedWithoutCSRFOrRights(t *testing.T) {
	fixture := newBooksWriteFixture(t, false)
	member := fixture.signIn(t, "access-brain-user")
	form := url.Values{"title": {"T"}, "review": {"R"}}
	if recorder := postReview(t, fixture, member, form, ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF = %d, want 403", recorder.Code)
	}
	viewer := fixture.signIn(t, "access-brain-viewer")
	if recorder := postReview(t, fixture, viewer, form, fixture.csrfFor(t, viewer)); recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer POST = %d, want 403 (FR-009)", recorder.Code)
	}
}

func TestReadOnlyModeDisablesCreate(t *testing.T) {
	fixture := newBooksWriteFixture(t, true)
	member := fixture.signIn(t, "access-brain-user")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/books/new", nil)
	request.AddCookie(member)
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("read-only GET /books/new = %d, want 403 (FR-009)", recorder.Code)
	}
	if recorder := postReview(t, fixture, member, url.Values{"title": {"T"}, "review": {"R"}}, fixture.csrfFor(t, member)); recorder.Code != http.StatusForbidden {
		t.Fatalf("read-only POST = %d, want 403", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/books", nil)
	request.AddCookie(member)
	fixture.handler.ServeHTTP(recorder, request)
	if strings.Contains(recorder.Body.String(), `href="/books/new"`) {
		t.Error("read-only shelf must hide the form link (US2 scenario 5)")
	}
	entries, err := os.ReadDir(fixture.booksDir)
	if err == nil && len(entries) != 0 {
		t.Fatalf("read-only mode wrote files: %v", entries)
	}
}
