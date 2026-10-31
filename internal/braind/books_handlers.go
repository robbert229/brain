package braind

// HTTP surface for the books shelf (spec 010). All domain rules live in the
// books package; this file is the thin transport: policy-gated routes, the
// shelf page, cover serving from the attachments folder, and the review form.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/books"
	"github.com/johnrowl/brain/internal/braind/config"
)

const maxBookCoverUpload = 8 << 20

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
	deleteTemplate     = template.Must(template.ParseFS(webAssets, "webassets/delete_confirm.html.tmpl"))
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
	mux.Handle("GET /books/edit", authenticator.RequireAuth(auth.Policy{Minimum: auth.RoleUser}, http.HandlerFunc(s.editForm)))
	mux.Handle("POST /books/edit", authenticator.RequireAuth(auth.Policy{Minimum: auth.RoleUser}, http.HandlerFunc(s.update)))
	mux.Handle("GET /books/delete", authenticator.RequireAuth(auth.Policy{Minimum: auth.RoleUser}, http.HandlerFunc(s.deleteConfirmation)))
	mux.Handle("POST /books/delete", authenticator.RequireAuth(auth.Policy{Minimum: auth.RoleUser}, http.HandlerFunc(s.deleteReview)))
}

// bookCard carries the formatted shelf-row values; the template only renders
// these values and omits fields that are absent (Principle II).
type bookCard struct {
	Title          string
	Review         string
	Byline         string
	Author         string
	Interest       string
	Status         string
	Finished       string
	Rating         string
	RatingStars    string
	Cover          string
	CoverFound     bool
	EditURL        string
	DeleteURL      string
	Series         string
	SeriesPosition string
}

type booksPage struct {
	ProductName string
	Entries     []bookCard
	CanWrite    bool
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
		CanWrite:    verified && identity.Role.Meets(auth.RoleUser) && !s.readOnly,
	}
	for _, entry := range catalog.Entries {
		editURL := ""
		deleteURL := ""
		if page.CanWrite {
			editURL = "/books/edit?file=" + url.QueryEscape(entry.Path)
			deleteURL = "/books/delete?file=" + url.QueryEscape(entry.Path)
		}
		position := ""
		if entry.HasSeriesNumber {
			position = "Book " + strconv.FormatFloat(entry.SeriesNumber, 'f', -1, 64)
		}
		page.Entries = append(page.Entries, bookCard{
			Title:          entry.Title,
			Review:         shelfReview(entry),
			Byline:         bookByline(entry),
			Author:         entry.Author,
			Interest:       entry.Interest,
			Status:         entry.Status,
			Finished:       finishedDate(entry.Finished),
			Rating:         books.FormatRating(entry.Rating, entry.RatingText),
			RatingStars:    ratingStars(entry.Rating, entry.RatingText),
			Cover:          entry.Cover,
			CoverFound:     entry.CoverFound,
			EditURL:        editURL,
			DeleteURL:      deleteURL,
			Series:         entry.Series,
			SeriesPosition: position,
		})
	}
	writePageHeaders(w)
	if err := booksTemplate.Execute(w, page); err != nil {
		// Headers are already sent; the parse happened at init, so a failure
		// here means rendering itself failed mid-stream — nothing to recover.
		return
	}
}

// bookByline joins the recorded author, finished date, and rating values.
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

// shelfReview removes the cover embed from the prose: the cover is already
// represented by the adjacent image or fallback tile on the shelf.
func shelfReview(entry books.Entry) string {
	if entry.Cover == "" {
		return entry.Review
	}
	embed := "![[" + entry.Cover + "]]"
	lines := strings.Split(entry.Review, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) == embed {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func finishedDate(finished time.Time) string {
	if finished.IsZero() {
		return ""
	}
	return finished.Format("2006-01-02")
}

// ratingStars converts the normalized rating into a compact, accessible visual
// scale for the shelf. Unparseable ratings remain visible as their source text.
func ratingStars(rating float64, raw string) string {
	if raw != "" || rating <= 0 || rating > 5 {
		return ""
	}
	full := int(rating)
	half := 0
	if rating-float64(full) >= 0.5 {
		half = 1
	}
	return strings.Repeat("★", full) + strings.Repeat("½", half) + strings.Repeat("☆", 5-full-half)
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
	root, err := os.OpenRoot(s.coversDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, name, info.ModTime(), file)
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
	File        string
	Edit        bool
}

func (s *booksSurface) editForm(w http.ResponseWriter, r *http.Request) {
	if s.readOnly {
		s.noticeReadonly(w)
		return
	}
	file := r.URL.Query().Get("file")
	draft, err := books.ReadReview(s.booksDir, file, s.maxNoteBytes)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if draft.Finished == "" {
		draft.Finished = currentDate()
	}
	writePageHeaders(w)
	_ = reviewFormTemplate.Execute(w, reviewFormPage{ProductName: productName, Values: draft, CSRFToken: s.csrfToken(r), File: file, Edit: true})
}

func (s *booksSurface) update(w http.ResponseWriter, r *http.Request) {
	if s.readOnly {
		s.noticeReadonly(w)
		return
	}
	draft, invalid, err := s.reviewSubmission(w, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if invalid != nil {
		file := r.FormValue("file")
		writePageHeaders(w)
		w.WriteHeader(http.StatusUnprocessableEntity)
		page := s.formPage(r, draft, invalid)
		page.File, page.Edit = file, true
		_ = reviewFormTemplate.Execute(w, page)
		return
	}
	file := r.FormValue("file")
	if err := books.UpdateReview(s.booksDir, file, draft, s.maxNoteBytes, time.Now().UTC()); err != nil {
		var invalid *books.ValidationError
		if !errors.As(err, &invalid) {
			http.NotFound(w, r)
			return
		}
		writePageHeaders(w)
		w.WriteHeader(http.StatusUnprocessableEntity)
		page := s.formPage(r, draft, invalid)
		page.File, page.Edit = file, true
		_ = reviewFormTemplate.Execute(w, page)
		return
	}
	http.Redirect(w, r, "/books", http.StatusSeeOther)
}

type deleteConfirmationPage struct {
	ProductName string
	Title       string
	File        string
	CSRFToken   string
}

// deleteConfirmation is deliberately read-only: displaying the confirmation
// page never changes the vault. Deletion happens only on the explicit POST.
func (s *booksSurface) deleteConfirmation(w http.ResponseWriter, r *http.Request) {
	if s.readOnly {
		s.noticeReadonly(w)
		return
	}
	file := r.URL.Query().Get("file")
	draft, err := books.ReadReview(s.booksDir, file, s.maxNoteBytes)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writePageHeaders(w)
	_ = deleteTemplate.Execute(w, deleteConfirmationPage{
		ProductName: productName,
		Title:       draft.Title,
		File:        file,
		CSRFToken:   s.csrfToken(r),
	})
}

// deleteReview removes only the selected Markdown note after the confirmation
// form supplies both an explicit confirmation value and the session's CSRF token.
func (s *booksSurface) deleteReview(w http.ResponseWriter, r *http.Request) {
	if s.readOnly {
		s.noticeReadonly(w)
		return
	}
	if r.FormValue("confirm") != "yes" {
		http.Error(w, "confirmation required", http.StatusBadRequest)
		return
	}
	if err := books.DeleteReview(s.booksDir, r.FormValue("file"), s.maxNoteBytes); err != nil {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/books", http.StatusSeeOther)
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
		Values:      books.ReviewDraft{Finished: currentDate()},
		CSRFToken:   s.csrfToken(r),
	}); err != nil {
		return
	}
}

func currentDate() string {
	return time.Now().Format("2006-01-02")
}

// create files one new note on success, re-renders the form with preserved
// values on validation failure, and refuses everything in read-only mode.
// Authentication, role, and CSRF arrived through RequireAuth before this
// handler ran (contracts/http.md).
func (s *booksSurface) create(w http.ResponseWriter, r *http.Request) {
	if s.readOnly {
		s.noticeReadonly(w)
		return
	}
	draft, invalid, err := s.reviewSubmission(w, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if invalid != nil {
		writePageHeaders(w)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = reviewFormTemplate.Execute(w, s.formPage(r, draft, invalid))
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

// reviewSubmission accepts an optional uploaded cover, validates its actual
// bytes, and stores it in the configured attachment folder before returning
// its generated filename for note serialization.
func (s *booksSurface) reviewSubmission(w http.ResponseWriter, r *http.Request) (books.ReviewDraft, *books.ValidationError, error) {
	isMultipart := strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data")
	var parseErr error
	if isMultipart && r.MultipartForm == nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxBookCoverUpload+s.maxNoteBytes+(1<<20))
		parseErr = r.ParseMultipartForm(maxBookCoverUpload + (1 << 20))
	} else if !isMultipart {
		parseErr = r.ParseForm()
	}
	if parseErr != nil {
		draft := reviewDraftFromRequest(r)
		return draft, &books.ValidationError{Fields: map[string]string{"cover_upload": "The cover upload is too large or the form could not be read."}}, nil
	}
	draft := reviewDraftFromRequest(r)
	if !isMultipart {
		return draft, nil, nil
	}
	file, header, err := r.FormFile("cover_upload")
	if errors.Is(err, http.ErrMissingFile) {
		return draft, nil, nil
	}
	if err != nil {
		return draft, &books.ValidationError{Fields: map[string]string{"cover_upload": "Choose a supported image file."}}, nil
	}
	defer file.Close()
	if header.Size > maxBookCoverUpload {
		return draft, &books.ValidationError{Fields: map[string]string{"cover_upload": "The cover image must be 8 MiB or smaller."}}, nil
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBookCoverUpload+1))
	if err != nil {
		return draft, nil, err
	}
	if len(data) == 0 || len(data) > maxBookCoverUpload {
		return draft, &books.ValidationError{Fields: map[string]string{"cover_upload": "The cover image must be non-empty and 8 MiB or smaller."}}, nil
	}
	ext := coverExtension(data)
	if ext == "" || books.ValidateImageData("cover"+ext, data) != nil {
		return draft, &books.ValidationError{Fields: map[string]string{"cover_upload": "Use a valid JPG, PNG, GIF, or WebP image."}}, nil
	}
	name, err := s.storeUploadedCover(data, ext)
	if err != nil {
		return draft, nil, err
	}
	draft.Cover = name
	return draft, nil, nil
}

func coverExtension(data []byte) string {
	switch {
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return ".jpg"
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return ".png"
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return ".gif"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return ".webp"
	default:
		return ""
	}
}

func (s *booksSurface) storeUploadedCover(data []byte, ext string) (string, error) {
	if err := os.MkdirAll(s.coversDir, 0o755); err != nil {
		return "", fmt.Errorf("create covers folder: %w", err)
	}
	digest := sha256.Sum256(data)
	name := "book-cover-" + hex.EncodeToString(digest[:]) + ext
	temp, err := os.CreateTemp(s.coversDir, ".braind-cover-*")
	if err != nil {
		return "", fmt.Errorf("create temporary cover: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	target := filepath.Join(s.coversDir, name)
	if err := os.Link(tempName, target); err != nil {
		if !os.IsExist(err) {
			return "", fmt.Errorf("publish cover: %w", err)
		}
		root, openErr := os.OpenRoot(s.coversDir)
		if openErr != nil {
			return "", openErr
		}
		defer root.Close()
		existing, openErr := root.Open(name)
		if openErr != nil {
			return "", fmt.Errorf("cover name already exists but is not a readable image")
		}
		defer existing.Close()
		info, statErr := existing.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != int64(len(data)) {
			return "", fmt.Errorf("cover name already exists but is not the same image")
		}
		stored, readErr := io.ReadAll(io.LimitReader(existing, maxBookCoverUpload+1))
		if readErr != nil || !bytes.Equal(stored, data) {
			return "", fmt.Errorf("cover name already exists but is not the same image")
		}
	}
	if err := syncCoverDirectory(s.coversDir); err != nil {
		return "", err
	}
	return name, nil
}

func syncCoverDirectory(dir string) error {
	folder, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer folder.Close()
	return folder.Sync()
}

func reviewDraftFromRequest(r *http.Request) books.ReviewDraft {
	return books.ReviewDraft{
		Title: r.FormValue("title"), Author: r.FormValue("author"),
		Rating: r.FormValue("rating"), Cover: r.FormValue("cover"),
		Finished: r.FormValue("finished"), Review: r.FormValue("review"),
		Series: r.FormValue("series"), SeriesNumber: r.FormValue("series_number"),
		SeriesSet: r.Form.Has("series") || r.Form.Has("series_number"),
	}
}

// formPage assembles the re-render context: sorted field messages so the
// error list is deterministic.
func (s *booksSurface) formPage(r *http.Request, draft books.ReviewDraft, invalid *books.ValidationError) reviewFormPage {
	page := reviewFormPage{ProductName: productName, Values: draft, CSRFToken: s.csrfToken(r)}
	for _, field := range []string{"title", "author", "rating", "cover", "finished", "review", "series", "series_number"} {
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
