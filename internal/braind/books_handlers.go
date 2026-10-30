package braind

// HTTP surface for the books shelf (spec 010). All domain rules live in the
// books package; this file is the thin transport: policy-gated routes, the
// shelf page, cover serving from the attachments folder, and the review form.

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/books"
	"github.com/johnrowl/brain/internal/braind/config"
)

// booksSurface carries the configured vault locations the shelf reads and the
// review form writes. It is wired only in run(); the unwired Handler() export
// serves no books routes, mirroring the auth surface's bootstrap posture.
type booksSurface struct {
	auth         *auth.Handler
	booksDir     string
	coversDir    string
	readOnly     bool
	maxNoteBytes int64
}

func newBooksSurface(vault config.Vault) *booksSurface {
	return &booksSurface{
		booksDir:     filepath.Join(vault.DataPath, vault.BooksFolder),
		coversDir:    filepath.Join(vault.DataPath, vault.CoversFolder),
		readOnly:     vault.ReadOnly,
		maxNoteBytes: vault.MaxNoteBytes,
	}
}

var (
	booksTemplate      = template.Must(template.ParseFS(webAssets, "webassets/books.html.tmpl"))
	reviewFormTemplate = template.Must(template.ParseFS(webAssets, "webassets/review_form.html.tmpl"))
	reviewNoticePage   = template.Must(template.ParseFS(webAssets, "webassets/review_notice.html.tmpl"))
)

// register mounts the books routes behind the shared authentication decision
// (contracts/http.md). Reads accept any verified session; the form and the
// create route require contributor rights and ride RequireAuth's CSRF check.
func (s *booksSurface) register(mux *http.ServeMux, authenticator *auth.Handler) {
	s.auth = authenticator
	mux.Handle("GET /books", authenticator.RequireAuth(auth.Policy{}, http.HandlerFunc(s.shelf)))
	mux.Handle("GET /books/covers/{filename}", authenticator.RequireAuth(auth.Policy{}, http.HandlerFunc(s.cover)))
	mux.Handle("GET /books/new", authenticator.RequireAuth(auth.Policy{Minimum: auth.RoleUser}, http.HandlerFunc(s.form)))
	mux.Handle("POST /books", authenticator.RequireAuth(auth.Policy{Minimum: auth.RoleUser}, http.HandlerFunc(s.create)))
}

// bookCard is one fully formatted shelf card; every display rule is applied
// here so the template only renders (Principle II).
type bookCard struct {
	Title      string
	Review     string
	Byline     string
	Interest   string
	Status     string
	Cover      string
	CoverFound bool
}

type booksPage struct {
	ProductName string
	Entries     []bookCard
	CanCreate   bool
	Skipped     int
	Truncated   bool
}

// shelf re-derives the catalog from the vault on every request (research R6).
func (s *booksSurface) shelf(w http.ResponseWriter, r *http.Request) {
	catalog, err := books.Scan(s.booksDir, s.coversDir)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	identity, verified := auth.FromContext(r.Context())
	page := booksPage{
		ProductName: productName,
		Skipped:     catalog.Skipped,
		Truncated:   catalog.Truncated,
		CanCreate:   verified && identity.Role.Meets(auth.RoleUser) && !s.readOnly,
	}
	for _, entry := range catalog.Entries {
		page.Entries = append(page.Entries, bookCard{
			Title:      entry.Title,
			Review:     entry.Review,
			Byline:     bookByline(entry),
			Interest:   entry.Interest,
			Status:     entry.Status,
			Cover:      entry.Cover,
			CoverFound: entry.CoverFound,
		})
	}
	writePageHeaders(w)
	if err := booksTemplate.Execute(w, page); err != nil {
		// Headers are already sent; the parse happened at init, so a failure
		// here means rendering itself failed mid-stream — nothing to recover.
		return
	}
}

// bookByline joins the recorded facts newest presentation: author · finished ·
// rating (FR-004; empty parts drop out cleanly).
func bookByline(entry books.Entry) string {
	parts := make([]string, 0, 3)
	if entry.Author != "" {
		parts = append(parts, entry.Author)
	}
	if !entry.Finished.IsZero() {
		parts = append(parts, entry.Finished.Format("2006-01-02"))
	}
	if rating := books.FormatRating(entry.Rating, entry.RatingText); rating != "" {
		parts = append(parts, rating)
	}
	return strings.Join(parts, " · ")
}

// cover serves one image from the covers folder and nothing else: the name
// must be a single clean image segment, and the file server is rooted at the
// folder, so no request can name a file outside it (FR-012). Unsafe names get
// the same 404 as missing ones.
func (s *booksSurface) cover(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("filename")
	if !books.IsImageName(name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFileFS(w, r, os.DirFS(s.coversDir), name)
}

// writePageHeaders applies the shared headers for session-gated pages
// (contracts/http.md): nothing cached, and CSP as safe as the landing page
// because the books pages reference only /static/ and /books/covers/.
func writePageHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
}

// reviewFormPage is the complete data context of the form template: submitted
// values for redisplay, sorted per-field errors, and the session CSRF token.
type reviewFormPage struct {
	ProductName string
	Values      books.ReviewDraft
	Errors      []string
	CSRFToken   string
}

// form serves the add-review page as an ordinary HTML form (Principle IV);
// read-only mode answers with the disabled notice instead (FR-009).
func (s *booksSurface) form(w http.ResponseWriter, r *http.Request) {
	if s.readOnly {
		s.noticeReadonly(w)
		return
	}
	writePageHeaders(w)
	if err := reviewFormTemplate.Execute(w, reviewFormPage{
		ProductName: productName,
		CSRFToken:   s.csrfToken(r),
	}); err != nil {
		return
	}
}

// create files one new note on success, re-renders the form with preserved
// values on validation failure, and refuses everything in read-only mode.
// Authentication, role, and CSRF arrived through RequireAuth before this
// handler ran (contracts/http.md).
func (s *booksSurface) create(w http.ResponseWriter, r *http.Request) {
	draft := books.ReviewDraft{
		Title:    r.FormValue("title"),
		Author:   r.FormValue("author"),
		Rating:   r.FormValue("rating"),
		Cover:    r.FormValue("cover"),
		Finished: r.FormValue("finished"),
		Review:   r.FormValue("review"),
	}
	if s.readOnly {
		s.noticeReadonly(w)
		return
	}
	if _, err := books.CreateReview(s.booksDir, draft, s.maxNoteBytes, time.Now().UTC()); err != nil {
		var invalid *books.ValidationError
		if !errors.As(err, &invalid) {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writePageHeaders(w)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = reviewFormTemplate.Execute(w, s.formPage(r, draft, invalid))
		return
	}
	http.Redirect(w, r, "/books", http.StatusSeeOther)
}

// formPage assembles the re-render context: sorted field messages so the
// error list is deterministic.
func (s *booksSurface) formPage(r *http.Request, draft books.ReviewDraft, invalid *books.ValidationError) reviewFormPage {
	page := reviewFormPage{ProductName: productName, Values: draft, CSRFToken: s.csrfToken(r)}
	for _, field := range []string{"title", "author", "rating", "cover", "finished", "review"} {
		if message, found := invalid.Fields[field]; found {
			page.Errors = append(page.Errors, fmt.Sprintf("%s: %s", field, message))
		}
	}
	return page
}

func (s *booksSurface) csrfToken(r *http.Request) string {
	session, _ := s.auth.LookupSession(r)
	return session.CSRFToken
}

// noticeReadonly is the writable-policy answer for the create flow: the route
// exists and is understood, the deployment policy refuses it (research R8).
func (s *booksSurface) noticeReadonly(w http.ResponseWriter) {
	writePageHeaders(w)
	w.WriteHeader(http.StatusForbidden)
	_ = reviewNoticePage.Execute(w, struct{ ProductName string }{ProductName: productName})
}
