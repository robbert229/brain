package braind

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/auth/authtest"
	"github.com/johnrowl/brain/internal/braind/config"
)

// booksFixture builds a data directory with the spec-010 shelf layout under
// the configured default folders and returns the wired handler plus the
// fixture paths.
type booksFixture struct {
	handler   http.Handler
	dataDir   string
	booksDir  string
	coversDir string
	session   *http.Cookie
	csrf      string
}

func newBooksFixture(t *testing.T, mutate func(*booksFixture)) *booksFixture {
	t.Helper()
	simulator := authtest.New(t)
	surface := enableOIDC(t, simulator)
	dataDir := t.TempDir()
	fixture := &booksFixture{dataDir: dataDir}
	fixture.booksDir = filepath.Join(dataDir, "01 - Note Box/Hobby/Books")
	fixture.coversDir = filepath.Join(dataDir, "06 - Meta/Attachments")
	fixture.handler = newHandler(handlerState{
		auth: surface,
		books: newBooksSurface(config.Vault{
			DataPath:     dataDir,
			BooksFolder:  "01 - Note Box/Hobby/Books",
			CoversFolder: "06 - Meta/Attachments",
		}),
	})
	if mutate != nil {
		mutate(fixture)
	}
	fixture.session = signIntoDaemon(t, fixture.handler)
	session, outcome := surface.sessions.Lookup(fixture.session.Value)
	if outcome != auth.OutcomeActive {
		t.Fatalf("fixture session = %s, want active", outcome)
	}
	fixture.csrf = session.CSRFToken
	return fixture
}

// signIntoDaemonAs signs in after replacing the provider claims, so tests can
// walk the shelf as a viewer or administrator.
func signIntoDaemonAs(t *testing.T, handler http.Handler, simulator *authtest.Provider, group string) *http.Cookie {
	t.Helper()
	simulator.SetClaims(map[string]any{"email": "member@example.test", "groups": []string{group}})
	return signIntoDaemon(t, handler)
}

func (f *booksFixture) writeNote(t *testing.T, rel, content string) {
	t.Helper()
	path := filepath.Join(f.booksDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
}

func (f *booksFixture) writeCover(t *testing.T, name string) {
	t.Helper()
	if err := os.MkdirAll(f.coversDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	var content bytes.Buffer
	if err := jpeg.Encode(&content, img, nil); err != nil {
		t.Fatalf("encode cover: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f.coversDir, name), content.Bytes(), 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}
}

// notebook renders one corpus-shaped book note.
func notebook(title, cover, finished, rating string) string {
	note := "---\ntags:\n  - Hobby/Books\ntype: Zettel\n"
	if title != "" {
		note += "author: Tao Wong\n"
	}
	if cover != "" {
		note += "cover: " + cover + "\n"
	}
	note += "interest: 🔥🔥\nstatus (Media): Finished\n"
	if finished != "" {
		note += "finished: " + finished + "\n"
	}
	if rating != "" {
		note += "rating: " + rating + "\n"
	}
	note += "---\n\n# " + title + "\nA densely packed opener to a promising series.\n"
	return note
}

func populateShelf(f *booksFixture, t *testing.T) {
	t.Helper()
	f.writeNote(t, "A Thousand Li.md", notebook("A Thousand Li 01 - The First Step", "book-cover-li.jpg", "2025-05-31", "4.5/5"))
	f.writeNote(t, "Series/Dungeon Crawler Carl 6.md", notebook("Dungeon Crawler Carl 6", "dcc6.jpg", "2025-03-12", "5/5"))
	f.writeNote(t, "No Cover.md", notebook("Immortality Starts With Generosity", "", "2024-12-25", "3.5/5"))
	f.writeNote(t, "Books.md", "---\ntags:\n  - Hobby/Books\n---\n# Books Index Note\n")
	f.writeNote(t, "Broken.md", "---\ncover: broken.jpg\n\tbad: true\n---\n# Broken\n")
	f.writeCover(t, "book-cover-li.jpg")
	f.writeCover(t, "dcc6.jpg")
}

func TestBooksShelfRequiresSession(t *testing.T) {
	fixture := newBooksFixture(t, nil)
	recorder := httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/books", nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("anonymous GET /books = %d, want 302", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); location != "/login?next=%2Fbooks" {
		t.Fatalf("anonymous redirect = %q, want sign-in with next=/books (SC-004)", location)
	}

	recorder = httptest.NewRecorder()
	api := httptest.NewRequest(http.MethodGet, "/books", nil)
	api.Header.Set("Accept", "application/json")
	fixture.handler.ServeHTTP(recorder, api)
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "unauthenticated") {
		t.Fatalf("API GET /books = %d %s, want 401 unauthenticated", recorder.Code, recorder.Body.String())
	}
}

func TestBooksShelfListsEveryBookNote(t *testing.T) {
	fixture := newBooksFixture(t, nil)
	populateShelf(fixture, t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/books", nil)
	request.AddCookie(fixture.session)
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /books = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); got != "default-src 'self'" {
		t.Errorf("CSP = %q", got)
	}
	for _, want := range []string{
		"A Thousand Li 01 - The First Step",
		"Tao Wong",
		"2025-05-31",
		"4.5/5",
		"Dungeon Crawler Carl 6",
		"Immortality Starts With Generosity",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("shelf lacks %q", want)
		}
	}
	// US3 fields ride along on the corpus-shaped notes.
	for _, want := range []string{"🔥🔥", "Finished"} {
		if !strings.Contains(body, want) {
			t.Errorf("shelf lacks %q", want)
		}
	}
	// FR-003: index and skipped notes are not books; the skip count is surfaced.
	if strings.Contains(body, "Books Index Note") {
		t.Error("index note listed as a book")
	}
	if !strings.Contains(body, "2 notes skipped") {
		t.Errorf("skipped notes not surfaced: %s", body)
	}
	// FR-005/SC-003: resolved covers are images; unresolved ones are placeholders.
	if !strings.Contains(body, `src="/books/covers/dcc6.jpg"`) {
		t.Error("resolved cover must render as an img")
	}
	if strings.Contains(body, `src="/books/covers/broken.jpg"`) || !strings.Contains(body, `src="/static/book-cover-placeholder.svg" alt="No cover available for Immortality Starts With Generosity"`) {
		t.Error("missing cover must render the accessible fallback, never a broken img")
	}
}

func TestBooksShelfSortsNewestFirstWithUndatedLast(t *testing.T) {
	fixture := newBooksFixture(t, nil)
	fixture.writeNote(t, "Undated.md", notebook("Undated Book", "", "", "4/5"))
	fixture.writeNote(t, "Old.md", notebook("Old Book", "", "2020-01-01", "4/5"))
	fixture.writeNote(t, "New.md", notebook("New Book", "", "2026-02-02", "4/5"))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/books", nil)
	request.AddCookie(fixture.session)
	fixture.handler.ServeHTTP(recorder, request)
	body := recorder.Body.String()
	newer, older, undated := strings.Index(body, "New Book"), strings.Index(body, "Old Book"), strings.Index(body, "Undated Book")
	if newer < 0 || older < 0 || undated < 0 {
		t.Fatalf("missing an entry: %s", body)
	}
	if !(newer < older && older < undated) {
		t.Fatalf("order violates FR-006: newest-first, undated last")
	}
}

func TestShelfShowsInterestAndStatusAtAGlance(t *testing.T) {
	fixture := newBooksFixture(t, nil)
	fixture.writeNote(t, "Chipy.md", notebook("Chipy Book", "", "2025-06-01", "4/5"))
	fixture.writeNote(t, "Plain.md", "---\ncover: plain.jpg\nrating: 3/5\n---\n\n# Plain Book\nNo chips on this one.\n")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/books", nil)
	request.AddCookie(fixture.session)
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /books = %d", recorder.Code)
	}
	cards := strings.Split(recorder.Body.String(), "<article")
	cardWith := func(needle string) string {
		for _, card := range cards {
			if strings.Contains(card, needle) {
				return card
			}
		}
		t.Fatalf("no card contains %q", needle)
		return ""
	}
	chipy := cardWith("Chipy Book")
	if !strings.Contains(chipy, "🔥🔥") || !strings.Contains(chipy, "Finished") {
		t.Errorf("recorded interest and status must appear on the entry (US3): %s", chipy)
	}
	plain := cardWith("Plain Book")
	if strings.Contains(plain, `class="chips"`) {
		t.Errorf("an entry without interest/status must show nothing for them (US3 scenario 2): %s", plain)
	}
}

func TestBooksShelfEmptyState(t *testing.T) {
	fixture := newBooksFixture(t, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/books", nil)
	request.AddCookie(fixture.session)
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /books on empty vault = %d", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "no books yet") {
		t.Fatalf("missing-books state not rendered: %s", body)
	}
}

func TestCoverServing(t *testing.T) {
	fixture := newBooksFixture(t, nil)
	populateShelf(fixture, t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/books/covers/dcc6.jpg", nil)
	request.AddCookie(fixture.session)
	fixture.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET cover = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("cover Content-Type = %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, max-age=3600" {
		t.Errorf("cover Cache-Control = %q", got)
	}
	if recorder.Header().Get("Last-Modified") == "" {
		t.Error("covers should revalidate via Last-Modified")
	}
	revalidation := httptest.NewRecorder()
	revalidate := httptest.NewRequest(http.MethodGet, "/books/covers/dcc6.jpg", nil)
	revalidate.Header.Set("If-Modified-Since", recorder.Header().Get("Last-Modified"))
	revalidate.AddCookie(fixture.session)
	fixture.handler.ServeHTTP(revalidation, revalidate)
	if revalidation.Code != http.StatusNotModified {
		t.Fatalf("If-Modified-Since = %d, want 304", revalidation.Code)
	}
}

func TestCoverServingRejectsUnsafeNames(t *testing.T) {
	fixture := newBooksFixture(t, nil)
	fixture.writeCover(t, "dcc6.jpg")
	outside := filepath.Join(t.TempDir(), "outside.jpg")
	if err := os.WriteFile(outside, []byte("outside attachment root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(fixture.coversDir, "outside.jpg")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	for _, target := range []string{
		"/books/covers/..%2Fsecret.jpg",
		"/books/covers/..%2F..%2F%2E%2E%2Fetc%2Fpasswd",
		"/books/covers/%2E%2E/dcc6.jpg",
		"/books/covers/notes.txt",
		"/books/covers/diagram.svg",
		"/books/covers/missing.jpg",
		"/books/covers/outside.jpg",
		"/books/covers/",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.AddCookie(fixture.session)
		fixture.handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 (FR-012)", target, recorder.Code)
		}
	}
	// Anonymous requests never reach cover bytes either.
	recorder := httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/books/covers/dcc6.jpg", nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("anonymous cover = %d, want 302", recorder.Code)
	}
}
