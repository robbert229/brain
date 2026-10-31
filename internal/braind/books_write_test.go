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

func postBookEdit(t *testing.T, f *booksWriteFixture, cookie *http.Cookie, form url.Values, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/books/edit", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if csrf != "" {
		request.Header.Set(auth.CSRFHeader, csrf)
	}
	request.AddCookie(cookie)
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func postBookDelete(t *testing.T, f *booksWriteFixture, cookie *http.Cookie, form url.Values, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/books/delete", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if csrf != "" {
		request.Header.Set(auth.CSRFHeader, csrf)
	}
	request.AddCookie(cookie)
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func TestBookEditFlowUpdatesNestedNoteAndRequiresRights(t *testing.T) {
	fixture := newBooksWriteFixture(t, false)
	const notePath = "Series/Book.md"
	fixture.booksFixture.writeNote(t, notePath, "---\ntags:\n  - Hobby/Books\ntype: Zettel\nstatus (Media): Finished\ninterest: 🔥🔥\ncustom: keep\nauthor: Old Author\nrating: 4/5\n---\n\n# Old Title\nOld review.\n")
	user := fixture.signIn(t, "access-brain-user")
	viewer := fixture.signIn(t, "access-brain-viewer")

	request := httptest.NewRequest(http.MethodGet, "/books/edit?file=Series%2FBook.md", nil)
	request.AddCookie(user)
	recorder := httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("user GET edit form = %d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{`action="/books/edit"`, `name="file" value="Series/Book.md"`, `value="Old Title"`, `value="Old Author"`, `>Old review.</textarea>`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Errorf("edit form lacks %q: %s", want, recorder.Body.String())
		}
	}

	form := url.Values{"file": {notePath}, "title": {"Updated Title"}, "author": {"New Author"}, "rating": {"4.5/5"}, "review": {"Updated review."}}
	if recorder := postBookEdit(t, fixture, user, form, ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("edit POST without CSRF = %d, want 403", recorder.Code)
	}
	if recorder := postBookEdit(t, fixture, viewer, form, fixture.csrfFor(t, viewer)); recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer edit POST = %d, want 403", recorder.Code)
	}
	if recorder := postBookEdit(t, fixture, user, form, fixture.csrfFor(t, user)); recorder.Code != http.StatusSeeOther {
		t.Fatalf("user edit POST = %d body=%s, want 303", recorder.Code, recorder.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(fixture.booksDir, notePath))
	if err != nil {
		t.Fatalf("read edited note in original nested path: %v", err)
	}
	for _, want := range []string{"tags:", "Hobby/Books", "status (Media): Finished", `interest: "\U0001F525\U0001F525"`, "custom: keep", "author: New Author", "rating: 4.5/5", "# Updated Title", "Updated review."} {
		if !strings.Contains(string(content), want) {
			t.Errorf("edited note lacks %q:\n%s", want, content)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.booksDir, "updated-title.md")); !os.IsNotExist(err) {
		t.Errorf("edit created duplicate note: %v", err)
	}

	request = httptest.NewRequest(http.MethodGet, "/books", nil)
	request.AddCookie(user)
	recorder = httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), "Updated Title") {
		t.Error("edited title is not visible on the shelf")
	}
}

func TestReviewFormGatedByRole(t *testing.T) {
	fixture := newBooksWriteFixture(t, false)
	fixture.booksFixture.writeNote(t, "Book.md", "---\ntype: Zettel\nstatus (Media): Finished\n---\n\n# Gated Book\nReview.\n")
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
	if !strings.Contains(shelfFor(member), `href="/books/delete?file=Book.md"`) {
		t.Error("contributor shelf must offer Delete review")
	}
	if strings.Contains(shelfFor(viewer), `href="/books/delete?`) {
		t.Error("viewer shelf must not offer Delete review")
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

func TestBookDeleteRequiresExplicitConfirmationAndRights(t *testing.T) {
	fixture := newBooksWriteFixture(t, false)
	const notePath = "Series/Delete Me.md"
	fixture.booksFixture.writeNote(t, notePath, "---\ntype: Zettel\nstatus (Media): Finished\ncover: shared.jpg\n---\n\n# Delete Me\nA review to delete.\n")
	fixture.booksFixture.writeCover(t, "shared.jpg")
	user := fixture.signIn(t, "access-brain-user")
	viewer := fixture.signIn(t, "access-brain-viewer")

	request := httptest.NewRequest(http.MethodGet, "/books/delete?file=Series%2FDelete%20Me.md", nil)
	request.AddCookie(user)
	recorder := httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("user GET delete confirmation = %d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{"Delete Me", "permanently", `action="/books/delete"`, `name="confirm" value="yes"`, `href="/books"`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Errorf("confirmation page lacks %q: %s", want, recorder.Body.String())
		}
	}
	noteFilename := filepath.Join(fixture.booksDir, filepath.FromSlash(notePath))
	if _, err := os.Stat(noteFilename); err != nil {
		t.Fatalf("GET confirmation removed the note: %v", err)
	}
	anonymous := httptest.NewRecorder()
	fixture.handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/books/delete?file=Series%2FDelete%20Me.md", nil))
	if anonymous.Code != http.StatusFound {
		t.Fatalf("anonymous GET confirmation = %d, want login redirect", anonymous.Code)
	}
	unsafe := httptest.NewRecorder()
	unsafeRequest := httptest.NewRequest(http.MethodGet, "/books/delete?file=..%2Foutside.md", nil)
	unsafeRequest.AddCookie(user)
	fixture.handler.ServeHTTP(unsafe, unsafeRequest)
	if unsafe.Code != http.StatusNotFound {
		t.Fatalf("unsafe GET confirmation = %d, want 404", unsafe.Code)
	}

	form := url.Values{"file": {notePath}, "confirm": {"yes"}}
	if recorder := postBookDelete(t, fixture, user, form, ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("delete POST without CSRF = %d, want 403", recorder.Code)
	}
	if recorder := postBookDelete(t, fixture, user, form, "invalid-token"); recorder.Code != http.StatusForbidden {
		t.Fatalf("delete POST with invalid CSRF = %d, want 403", recorder.Code)
	}
	if recorder := postBookDelete(t, fixture, viewer, form, fixture.csrfFor(t, viewer)); recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer delete POST = %d, want 403", recorder.Code)
	}
	if recorder := postBookDelete(t, fixture, user, url.Values{"file": {notePath}}, fixture.csrfFor(t, user)); recorder.Code != http.StatusBadRequest {
		t.Fatalf("delete POST without confirmation = %d, want 400", recorder.Code)
	}
	if _, err := os.Stat(noteFilename); err != nil {
		t.Fatalf("refused delete removed the note: %v", err)
	}

	readOnly := newBooksWriteFixture(t, true)
	readOnly.booksFixture.writeNote(t, notePath, "---\ntype: Zettel\nstatus (Media): Finished\n---\n\n# Delete Me\nKeep me in read-only mode.\n")
	readOnlyUser := readOnly.signIn(t, "access-brain-user")
	request = httptest.NewRequest(http.MethodGet, "/books/delete?file=Series%2FDelete%20Me.md", nil)
	request.AddCookie(readOnlyUser)
	recorder = httptest.NewRecorder()
	readOnly.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("read-only GET confirmation = %d, want 403", recorder.Code)
	}
	if recorder := postBookDelete(t, readOnly, readOnlyUser, form, readOnly.csrfFor(t, readOnlyUser)); recorder.Code != http.StatusForbidden {
		t.Fatalf("read-only delete POST = %d, want 403", recorder.Code)
	}
	readOnlyShelf := httptest.NewRecorder()
	readOnlyRequest := httptest.NewRequest(http.MethodGet, "/books", nil)
	readOnlyRequest.AddCookie(readOnlyUser)
	readOnly.handler.ServeHTTP(readOnlyShelf, readOnlyRequest)
	if strings.Contains(readOnlyShelf.Body.String(), `href="/books/delete?`) {
		t.Error("read-only shelf must hide delete actions")
	}
	if _, err := os.Stat(filepath.Join(readOnly.booksDir, filepath.FromSlash(notePath))); err != nil {
		t.Errorf("read-only delete removed note: %v", err)
	}

	recorder = postBookDelete(t, fixture, user, form, fixture.csrfFor(t, user))
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("confirmed delete POST = %d body=%s, want 303", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Location") != "/books" {
		t.Fatalf("delete Location = %q, want /books", recorder.Header().Get("Location"))
	}
	if _, err := os.Stat(noteFilename); !os.IsNotExist(err) {
		t.Errorf("confirmed delete left note on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.coversDir, "shared.jpg")); err != nil {
		t.Errorf("confirmed delete removed shared cover: %v", err)
	}
}
