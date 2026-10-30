# Tasks: Fallback Book Covers

**Input**: Design documents from `specs/012-fallback-cover/`

## Phase 1: Shelf fallback

- [X] T001 Add a designed local fallback cover asset in `internal/braind/webassets/static/book-cover-placeholder.svg`.
- [X] T002 Update `internal/braind/webassets/books.html.tmpl` to render the fallback asset for unavailable covers with accessible alternative text.
- [X] T003 Update `internal/braind/books/catalog.go` so a cover is considered available only when it is a readable supported image.
- [X] T004 Confirm the shared cover sizing in `internal/braind/webassets/static/style.css` applies to the fallback image as well as real covers.
