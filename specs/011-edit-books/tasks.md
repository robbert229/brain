# Tasks: Edit Book Reviews

**Input**: Design documents from `specs/011-edit-books/`

## Phase 1: Domain operations

- [X] T001 Extend `internal/braind/books/book.go` and `internal/braind/books/catalog.go` to retain a safe relative note path for each shelf entry.
- [X] T002 Add safe read and atomic update operations in `internal/braind/books/writer.go`, preserving unrelated frontmatter and validating size and fields.

## Phase 2: Edit flow

- [X] T003 Add contributor-only edit links and route registration in `internal/braind/books_handlers.go`.
- [X] T004 Add a prefilled edit form in `internal/braind/webassets/review_form.html.tmpl` and wire successful edits back to `/books`.
