# Implementation Plan: Book Reviews

**Branch**: `010-books` | **Date**: 2026-10-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/010-books/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command; its definition describes the execution workflow.

## Summary

A server-rendered "books" shelf that recursively scans the configured Books folder
under the service data directory, renders one card per book note (cover, title,
author, rating, finished date, interest, status, review text) in one global
newest-first list, with series name and volume shown inside applicable entries, and offers
a plain HTML form that durably creates a new Obsidian-convention book note in that
folder. Implemented as a new `internal/braind/books` package (parser, catalog, writer)
wired into the existing `newHandler` mux behind the spec 002 auth surface, with page
templates in the existing embedded `webassets` and covers served from the configured
attachments folder. Supported covers are checked for readable image data; unavailable covers use a bundled accessible fallback image with the same dimensions. Contributor and administrator sessions can edit existing book notes through a prefilled form; edits preserve unrelated frontmatter and use the same validation and size limits as note creation. Changing series assignment moves a note to its generated path, while unchanged assignment fields preserve the original path.

The shelf renders one flat list in global newest-finished-first order. Entries from
first-level subfolders show the series folder name and a recognized numeric volume
position (including fractional volumes and optional `Volume`/`Book` labels) inside
the book entry. Series membership does not split the list or change its order.
Create/edit forms can assign a series and optional numeric position; changed
assignments move notes into a generated folder and filename while unchanged
assignments preserve the original path.

## Technical Context

**Language/Version**: Go 1.24 (existing `go.mod` toolchain; stdlib `net/http` ServeMux with method+pattern routing)

**Primary Dependencies**: stdlib (`net/http`, `html/template`, `os`, `io/fs`) plus `gopkg.in/yaml.v3` for frontmatter parsing (approved; ADR-0010 precedent for pure-mechanics libraries). Existing: Pico CSS 2.1.1 and htmx 2.0.11 (vendored, ADR-0011).

**Storage**: The vault filesystem (markdown files) is the only store — book notes under `<data>/<books folder>`, cover images under `<data>/<covers folder>`. No application database (Principle V). Derived data (the shelf listing) is computed per request and never persisted.

**Testing**: `go test` with `httptest` and `t.TempDir()` fixture vaults, matching `landing_test.go` / `run_test.go` style; the books package gets direct unit tests for parsing, sorting, validation, and filename safety (Principle II).

**Target Platform**: Linux container (ko-built distroless image) on the dev Kubernetes StatefulSet, same as the rest of `braind`.

**Project Type**: Web service (single daemon), extension of the existing single binary.

**Performance Goals**: Shelf of the current corpus (130 books) renders in <250 ms server-side; full scan of up to 500 notes completes <500 ms; cover fetch is a single file open. No pagination or caching until a demonstrated need (Principle III).

**Constraints**: `BRAIND_READ_ONLY` disables creation, editing, and deletion; note size bounded by `BRAIND_MAX_NOTE_BYTES`; cover uploads are limited to 8 MiB and supported image formats; edit/delete targets stay under the configured Books folder; delete confirmation GET is side-effect-free and confirmed removal retains cover attachments; single-writer data-lock (spec 001) already held by the daemon; review saves and delete confirmation MUST work without JavaScript (Principle IV).

**Scale/Scope**: ~130 book notes today, hundreds of folders; one books package, 4 page templates, progressive shelf enhancement, 9 routes, 4 configuration keys, and 1 landing-page link.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Principle | Verdict |
| --- | --- | --- |
| Independently testable without other features or live services | I | **PASS** — `books` package exercised by unit tests over fixture directories; handlers tested with `httptest` and the real `auth` surface; CI-runnable, no shared mutable state. |
| Business logic explicit, named, typed, directly tested | II | **PASS** — book detection, field parsing, sort order, form validation, filename safety are named functions in `books` with table-driven tests; templates only render. |
| Simplest solution; no unproven abstraction | III | **PASS** — scan-per-request (no cache/index layer), rooted filesystem access for covers to prevent symlink escape, one template per page, no interfaces/repository pattern. Single new dependency (`yaml.v3`) justified as pure mechanics per ADR-0010 precedent. |
| Go backend; server-rendered HTML + HTMX; core flows work without JS | IV | **PASS** — shelf, forms, and delete confirmation are server-rendered HTML; create/edit/delete submissions are plain form POSTs with CSRF fields; htmx not required for any flow. |
| Markdown is the single source of truth for note content | V | **PASS** — reviews are stored as ordinary Obsidian-convention `.md` files in the Books folder; nothing written to any other store; shelf is derived and recomputable. |
| Data & Storage: durable writes, derived data never authoritative | — | **PASS** — atomic create and in-place edit (temp write + fsync + replacement + dir fsync), plus confirmed note removal with containing-directory fsync; shelf never persisted; cover attachments are retained on deletion. |

No violations — Complexity Tracking not needed.

**Post-Phase-1 re-check**: gates re-evaluated against `research.md`, `data-model.md`,
and `contracts/http.md` — all still pass. The design adds no abstraction beyond the
single `books` package (III), keeps every rule in directly-tested functions (II),
exercises through fixture-driven `httptest` with no live services (I), submits the
review as an ordinary form POST (IV), and persists exactly one Obsidian-convention
markdown file per review with derived data recomputed per request (V).

## Project Structure

### Documentation (this feature)

```text
specs/010-books/
├── spec.md              # approved feature spec
├── plan.md              # this file
├── research.md          # Phase 0 decisions (yaml.v3, detection rules, cover strategy, safe create)
├── data-model.md        # Phase 1 entities: Book, ShelfEntry, ReviewDraft, validation rules
├── quickstart.md        # Phase 1 validation guide
├── contracts/
│   └── http.md          # Phase 1 route contract: shelf, create/edit/delete, covers
├── checklists/
│   └── requirements.md  # spec quality checklist (passed)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/braind/
├── books/                    # NEW package — all books business logic
│   ├── book.go               #   Book entity, frontmatter split + YAML parse, field normalization
│   ├── catalog.go            #   recursive scan, book detection, skip accounting, sort order
│   ├── writer.go             #   ReviewDraft validation, safe read/update/delete, atomic create and edit
│   ├── book_test.go
│   ├── catalog_test.go
│   └── writer_test.go
├── books_handlers.go         # route handlers: shelf, create/edit/delete, cover serving
├── books_test.go             # httptest contract tests against a fixture data dir + auth surface
├── webassets/
│   ├── index.html.tmpl       # EDIT: add "Books" link card
│   ├── books.html.tmpl       # NEW: shelf page
│   ├── review_form.html.tmpl # NEW: create/edit form page
│   ├── review_notice.html.tmpl # NEW: read-only refusal page
│   ├── delete_confirm.html.tmpl # NEW: explicit review deletion confirmation
│   └── static/books.js       # NEW: progressive shelf search and status filters
├── landing.go                # EDIT: template context gains books link (no behavior change)
├── run.go                    # EDIT: mount books routes in newHandler with handlerState wiring
└── config/config.go          # EDIT: BRAIND_BOOKS_FOLDER, BRAIND_COVERS_FOLDER (+ validation)
docs/adr/0012-*.md            # NEW: accept yaml.v3 (precedent ADR-0010)
```

**Structure Decision**: Single Go module, single daemon — everything joins the
existing `internal/braind` layout. Domain logic (parse/scan/validate/write) lives in
the `books` package with no HTTP imports; `books_handlers.go` is the thin HTTP layer,
mirroring how `auth` separates `handler.go` from policy. Templates join the existing
`webassets` embed so the binary stays self-contained.

## Complexity Tracking

> No constitution violations; this section is unused.
