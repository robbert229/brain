package braind

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

// webAssets holds the landing page template and static assets. Embedding keeps
// the binary self-contained and offline-capable (research.md §1, §2).
//
//go:embed webassets
var webAssets embed.FS

// Landing page content constants (data-model.md). The page is fully static:
// the template context receives ONLY these values and never runtime state,
// configuration, or vault data (FR-005).
const (
	productName        = "brain"
	tagline            = "a second brain, powered by markdown"
	pageDescription    = "brain is a private second brain: your notes live in an Obsidian vault as plain Markdown, readable and searchable through the web."
	constructionNotice = "The brain web interface is under construction."
)

// landingContent is the complete data context for the landing page template:
// the only values the template may render (data-model.md, FR-005).
type landingContent struct {
	ProductName        string
	Tagline            string
	Description        string
	ConstructionNotice string
}

// landingTemplate is parsed once from the embedded template file.
var landingTemplate = template.Must(template.ParseFS(webAssets, "webassets/index.html.tmpl"))

// serveLanding renders the landing page at the root address. The page is
// entirely server-rendered, so it is complete when scripting is disabled
// (FR-003). It is served only to verified sessions (run.go), so the cache
// directive keeps the page out of shared caches. Headers follow
// contracts/http.md.
func serveLanding(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	if err := landingTemplate.Execute(w, landingContent{
		ProductName:        productName,
		Tagline:            tagline,
		Description:        pageDescription,
		ConstructionNotice: constructionNotice,
	}); err != nil {
		// The template is static; a render failure is a programming error.
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// staticAssetsHandler serves the embedded assets under /static/ with a short
// public cache lifetime per contracts/http.md. Requests for files that do not
// exist are handled by the file server as 404.
func staticAssetsHandler() http.Handler {
	static, err := fs.Sub(webAssets, "webassets/static")
	if err != nil {
		panic(err) // the embedded layout is fixed at compile time
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.FileServer(http.FS(static)).ServeHTTP(w, r)
	})
}
