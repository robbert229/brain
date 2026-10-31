---
description: "Task list for Book Reviews (spec 010)"
---

# Tasks: Book Reviews

**Input**: Design documents from `specs/010-books/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/http.md](./contracts/http.md), [quickstart.md](./quickstart.md)

**Tests**: INCLUDED — the constitution (Principles I & II) mandates automated direct tests for every feature and business rule, so each story writes its tests first and they MUST fail before implementation.

**Organization**: Tasks grouped by user story; each story is independently implementable and testable once the Foundational phase is done.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 = browse shelf, US2 = write review, US3 = glance polish, US4 = edit existing review, US5 = delete review
- Paths are repository-relative.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Dependencies and configuration that every story needs

- [X] T001 Add `gopkg.in/yaml.v3` as the sole new dependency: `go get gopkg.in/yaml.v3@latest && go mod tidy` (touches `go.mod`, `go.sum`) — research R1
- [X] T002 [P] Write `docs/adr/0012-accept-yaml-v3-for-frontmatter-parsing.md` recording the decision per ADR-0010's pure-mechanics precedent, and register it in `docs/adr/README.md`
- [X] T003 Add config keys in `internal/braind/config/config.go`: `EnvBooksFolder = "BRAIND_BOOKS_FOLDER"` (default `01 - Note Box/Hobby/Books`) and `EnvCoversFolder = "BRAIND_COVERS_FOLDER"` (default `06 - Meta/Attachments`), exposed on the `Vault` struct; validation (extend `internal/braind/config/config_test.go` with cases): "must be relative sub-path of BRAIND_DATA_PATH; no `..` segments, not absolute" — research R7. Missing folders are NOT config errors

**Checkpoint**: Dependency + config ready; nothing user-visible yet.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The `books` domain package (parse + scan) and daemon wiring — no HTTP routes yet

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T004 Implement `internal/braind/books/book.go`: `Book` struct with fields `Title, Review, Author, Cover, Interest, Status, Finished time.Time, Rating float64, RatingText, Filename` and `ParseNote(filename string, content []byte) (Book, bool)` implementing data-model.md's normalization table verbatim: frontmatter must start with `---` line + closing `---`; YAML must decode to a mapping; book iff mapping has ≥1 of `cover`, `rating`, `status (Media)`; title = first `# ` heading else filename base; `finished` accepts YAML date or `2006-01-02`/`2006-1-2`; `rating` accepts `n/5` (`4.5/5`), bare number, unparseable → Rating 0 + raw text in RatingText; `cover` must be "a single clean path segment with image extension (.jpg .jpeg .png .webp .gif) or be treated as absent"
- [X] T005 [P] Unit tests in `internal/braind/books/book_test.go`: table-driven over the data-model.md rules using real corpus shapes (the 130-file sample frontmatter from spec.md input: tags block sequence, `status (Media)`, `interest: 🔥🔥`, `finished: 2025-05-31`, `rating: 4.5/5`, `![[cover]]` embed) — MUST fail before T004 passes
- [X] T006 Implement `internal/braind/books/catalog.go`: `Scan(booksDir, coversDir string) (Catalog, error)` with `Catalog{Books []Book; Skipped int; Truncated bool}` — recursive `fs.WalkDir`; non-book/`---`-less/non-mapping/IO-error `.md` files increment `Skipped` and never abort (FR-011); "visit budget: files ≤2000; on exhaustion Truncated=true, render what was found" (research R6); cover resolution validates readable supported image data under the configured covers root; absent, corrupt, empty, unreadable, and escaping symlink targets are missing (FR-005, FR-012); sort: "Finished descending (newest first) → zero-Finished after all dated → Title ascending (case-insensitive)" (FR-006); missing `booksDir` → empty Catalog, nil error
- [X] T007 [P] Unit tests in `internal/braind/books/catalog_test.go` over `t.TempDir()` fixtures mirroring the real layout: nested folders found once at any depth; `Books.md` Waypoint note and series-folder note with only `tags` excluded; malformed-frontmatter note skipped-but-counted while siblings still list; valid readable image vs absent, missing, empty, corrupt, unreadable, and symlinked-outside attachments; sort order incl. undated-last and case-insensitive title tiebreak; 2001-file budget truncation
- [X] T008 Wire the books surface into `internal/braind/run.go`: extend `handlerState` with a `books *booksSurface` field (holds `dataPath, booksFolder, coversFolder string; readOnly bool; maxNoteBytes int64` sourced from `config.Vault()`) populated in `newHandler(handlerState{...})` when the auth surface is wired; the `Handler()` unwired export keeps it nil (routes 404-safe until wired — decide in-task, mirror how `auth` surface is threaded)

**Checkpoint**: `go test ./internal/braind/books/...` green. Domain logic done; shelf can now be built.

---

## Phase 3: User Story 1 — Browse the bookshelf (Priority: P1) 🎯 MVP

**Goal**: Signed-in visitor reaches `/books` from the landing page and sees every book note as a card (cover or placeholder, title, author, rating, finished, review) newest-first.

**Independent Test**: Fixture data dir with nested folders, an index note, a book without a usable cover, and a malformed note → `GET /books` (authenticated) lists exactly the books with their own fields, bundled fallback cover for unusable attachments, index note absent, malformed note counted; anonymous request redirected to sign-in. (quickstart.md automated table, rows 2–4 + httptest row.)

### Tests for User Story 1 ⚠️ write FIRST, must fail

- [X] T009 [P] [US1] Shelf contract tests in `internal/braind/books_test.go` (httptest + fixture dir + auth surface with a fake verified session, matching `auth_wiring_test.go` style): anonymous `GET /books` → 302 `/login?next=%2Fbooks`; signed-in → 200 `text/html; charset=utf-8` containing every fixture book title exactly once, `no books yet` for empty dir, `N notes skipped` footer when Skipped>0, `<img src="/books/covers/…">` for resolved cover and bundled accessible fallback-cover image for absent or invalid covers, headers `Cache-Control: no-store` and `Content-Security-Policy: default-src 'self'` — contracts/http.md
- [X] T010 [P] [US1] Cover-endpoint tests in `internal/braind/books_covers_test.go`: anonymous → 302; signed-in GET real jpg fixture → 200 with extension content type and `Cache-Control: private, max-age=3600`, `If-Modified-Since` → 304; traversal names (`..%2F`, encoded slashes, absolute), non-image extension (`.svg`, `.txt`), missing files, and symlink targets outside `coversDir` → 404 (FR-012)

### Implementation for User Story 1

- [X] T011 [US1] Shelf template `internal/braind/webassets/books.html.tmpl` (Pico dialect like `index.html.tmpl`): card = real cover `<img>` or bundled accessible fallback cover `<img>` → `h3` title → byline (author · finished YYYY-MM-DD · rating) → review paragraphs; empty state; skipped/truncated footers; renders only pre-escaped Book fields (Principle II: templates carry no business rules)
- [X] T012 [US1] Implement `internal/braind/books_handlers.go`: `shelfHandler` (RequireAuth any role, `books.Scan`, render, `Cache-Control: no-store`) and `coversHandler` (RequireAuth; "reject unless the path param is a single clean path segment with an image extension (.jpg .jpeg .png .webp .gif); serve via rooted filesystem access that cannot follow symlinks outside `coversDir`; unsafe or missing → plain 404") and register `GET /books` + `GET /books/covers/{filename}` in `newHandler` (run.go) when `state.books != nil`
- [X] T013 [US1] Add the Books card link to `internal/braind/webassets/index.html.tmpl` and extend `internal/braind/landing_test.go` to pin the `/books` link (FR-001)

**Checkpoint**: `go test ./...` green; US1 deployable — shelf works end-to-end without US2/US3.

---

## Phase 4: User Story 2 — Write a new book review (Priority: P2)

**Goal**: Contributor fills the form (title and review required; author, rating, cover, finished, series, and volume optional) and submits; one Obsidian-convention note appears at its standalone or series path and on the shelf.

**Independent Test**: Submit standalone and series forms against a fixture data dir → new `.md` notes with the data-model.md frontmatter/body/`![[cover]]` layout exist at their generated paths; shelf includes them; unsafe inputs write nothing; viewer/read-only refused.

### Tests for User Story 2 ⚠️ write FIRST, must fail

- [X] T014 [P] [US2] Writer tests in `internal/braind/books/writer_test.go`: `TestCreateReview` asserts generated bytes equal the data-model.md serialization block exactly (key order `tags,type Zettel,author,cover,status (Media),finished,rating`; empty optionals omitted; `![[cover]]` only when set); slug collisions produce `slug-2.md`, `slug-3.md`, never overwrite an existing file (pre-create a same-name file, assert content untouched); validation table quoting data-model.md: "Title required, ≤200 runes, no control characters"; "Review required, rendered note ≤ MaxNoteBytes"; "Rating empty or 0–5 halves (n/5 or bare number)"; "Cover empty or single clean segment with image extension"; "Finished empty or YYYY-MM-DD, not in the future"; every refusal leaves no file or temp file behind (FR-008, FR-010)
- [X] T015 [P] [US2] Form/create contract tests in `internal/braind/books_write_test.go`: viewer `GET /books/new` → 403; `BRAIND_READ_ONLY=true` → 403 on both `/books/new` and `POST /books`, and shelf HTML omits the Add-a-review link; missing/bad CSRF token → 403; signed-in `POST /books` valid → `303` `Location: /books` and the note exists on disk then appears on next `GET /books`; invalid submission → `422` re-render preserving entered values with per-field errors (contracts/http.md)

### Implementation for User Story 2

- [X] T016 [US2] Implement `internal/braind/books/writer.go`: `ReviewDraft` validation and `CreateReview`; generate collision-safe standalone slugs or series paths from the optional folder and volume fields; atomically publish notes without overwriting existing files.
- [X] T017 [US2] Form template `internal/braind/webassets/review_form.html.tmpl`: plain `<form method="post" action="/books">` (works without JS — Principle IV) with hidden CSRF field, labelled fields in data-model.md order, `Save review` submit, error list above fields on re-render
- [X] T018 [US2] Implement `formHandler` (GET `/books/new`) and `createHandler` (POST `/books`) in `internal/braind/books_handlers.go`: role gate `Role.Meets(auth.RoleUser)` + read-only 403 + CSRF via the existing session check; success `303` → `/books`; 422 re-render; register both routes in `newHandler`; add the conditional "Add a review" link to `books.html.tmpl` (shown only when writable + contributor)

**Checkpoint**: US1 + US2 both functional; reviews capture to vault.

---

## Phase 5: User Story 3 — See who reviewed what, at a glance (Priority: P3)

**Goal**: Shelf cards surface `interest` and `status` recorded on notes, and degrade cleanly when absent.

**Independent Test**: Same US1 fixture dir — notes with `interest: 🔥🔥` / `status (Media): Finished` show both; notes omitting them show neither and the card stays aligned. (quickstart.md row 2, field-level assertions.)

### Tests for User Story 3 ⚠️ write FIRST, must fail

- [X] T019 [P] [US3] Extend `internal/braind/books_test.go` with a fixture pair asserting the interest and status values render on their card and are fully absent (no empty chip element) when the note omits them (FR-004 graceful degradation)

### Implementation for User Story 3

- [X] T020 [US3] Render interest and status as inline chips in `internal/braind/webassets/books.html.tmpl` (Pico `<small>`/badge style consistent with the byline line), conditional on non-empty values

**Checkpoint**: All three stories independently functional.

---

## Phase 6: User Story 4 — Edit an existing book review (Priority: P2)

**Goal**: A contributor or administrator updates an existing shelf note without losing unrelated frontmatter; changing series assignment moves it to a generated path.

**Independent Test**: Edit a nested fixture note; unchanged assignments retain the path, changed assignments move safely, unrelated metadata is retained, unsafe paths are refused, and viewer/read-only requests cannot change files.

### Tests for User Story 4

- [X] T026 [US4] Add domain tests in `internal/braind/books/writer_test.go` for edit prefill, in-place updates, unrelated frontmatter preservation, and refusal of traversal, absolute, and symlinked paths.
- [X] T027 [US4] Add handler tests in `internal/braind/books_write_test.go` for edit form prefill, viewer and CSRF refusal, successful nested-note update, and shelf refresh.

### Implementation for User Story 4

- [X] T028 [US4] Carry each scanned note's relative path through `internal/braind/books/book.go` and `internal/braind/books/catalog.go`; implement bounded safe read and atomic update/move in `internal/braind/books/writer.go`, preserving unrelated frontmatter.
- [X] T029 [US4] Add role-gated GET/POST `/books/edit` handlers in `internal/braind/books_handlers.go` and a prefilled edit form/action in `internal/braind/webassets/review_form.html.tmpl` and `internal/braind/webassets/books.html.tmpl`; hide edit actions in read-only mode.

**Checkpoint**: Existing notes can be edited with authorization, CSRF, validation, and path confinement; assignment changes move notes safely without overwriting.

---

## Phase 7: User Story 5 — Delete a book review (Priority: P2)

**Goal**: A contributor removes an unwanted review only after confirming the identified book; cancellation and refused requests leave all files unchanged.

**Independent Test**: Open the confirmation page for a fixture note and verify it remains untouched; cancel and verify it remains visible; confirm a second time and verify only the note disappears while its cover attachment remains. Viewer, read-only, CSRF, and unsafe-path requests are refused.

### Tests and implementation for User Story 5 (vertical TDD slices)

- [X] T030 [US5] Add `DeleteReview` domain tests in `internal/braind/books/writer_test.go`: successful deletion of a nested valid book note and retention of its cover; refusal of traversal, absolute path, symlink escape, missing note, directory, and non-book note.
- [X] T031 [US5] Implement `books.DeleteReview` in `internal/braind/books/writer.go` using the existing safe relative-path resolution; verify the target is a regular book note, remove only that note, and sync its containing directory.
- [X] T032 [US5] Add HTTP tests in `internal/braind/books_write_test.go`: GET confirmation names the book and has no side effect; POST without confirmation or valid CSRF, as viewer, or in read-only mode refuses without mutation; valid contributor confirmation redirects and deletes the selected note only.
- [X] T033 [US5] Implement role-gated GET/POST `/books/delete` handlers, CSRF-protected confirmation form, a book-identifying confirmation page with explicit Delete review and Cancel actions, and writable/read-only behavior.
- [X] T034 [US5] Add Playwright coverage proving the shelf Delete review action opens confirmation, cancellation preserves the entry, and explicit confirmation removes it from the shelf after reload; viewer does not see delete actions and cannot directly open the confirmation route.
- [X] T035 [US5] Add a conditional Delete review action to each writable shelf entry; style the confirmation page and destructive action consistently with the Books palette and responsive form pages.

**Checkpoint**: Deletion requires an explicit confirmation, is authorized and CSRF-protected, is confined to valid book notes, and preserves cover attachments.

---

## Phase 8: Polish & Cross-Cutting Concerns

- [X] T021 [P] Add `BRAIND_BOOKS_FOLDER` and `BRAIND_COVERS_FOLDER` env entries (with the defaults above) to `deploy/install.yaml` next to the existing `BRAIND_DATA_PATH` block
- [X] T022 [P] Full gate: `gofmt -l internal/braind` empty, `go vet ./...`, `go test ./...` green — record results
- [X] T023 Run `quickstart.md` dev-cluster validation (image-publish → kubectl rollout → live checks: landing link, 130-book shelf with covers, one test review round-trip, anonymous 302, cover traversal 404)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: start immediately; T002 parallel with T001/T003
- **Phase 2 (Foundational)**: needs T001 (yaml.v3 for T004/T006); BLOCKS all stories. Within: T004 → {T005, T006, T007}; T008 independent of T004–T007
- **Phase 3 (US1)**: needs Phase 2. T009/T010 first (fail), then T011/T012/T013
- **Phase 4 (US2)**: needs Phase 2 only for the writer path; the 303→shelf test reuses US1's shelf. T014/T015 first, then T016–T018
- **Phase 5 (US3)**: needs US1 (T009's fixture harness); T019 first, then T020
- **Phase 6 (US4)**: needs the catalog path from Phase 2; implement safe domain read/update before the edit routes
- **Phase 7 (US5)**: needs the catalog-relative safe path logic from the domain package and existing auth/CSRF wiring
- **Phase 8 (Polish)**: all desired stories complete

### Cross-story notes

- US2's writer is independent of US1's reader (opposite directions on the vault) — it could ship without the shelf rendering, but its acceptance includes the shelf, so deliver after US1.
- US3 is a pure presentation delta on US1's cards.
- US4 uses catalog-relative paths and can begin after the domain/catalog phase; edit tests precede the handlers.
- US5 uses the same catalog-relative path rules; the confirmation page is side-effect-free and the confirmed delete is a separate CSRF-protected POST.

### Parallel Opportunities

- T002 ∥ T001/T003; T005/T007 ∥ each other (different test files) after T004; T009 ∥ T010; T014 ∥ T015; T021 ∥ T022
- Single implementer: strict T001→T023 order is safe; [P] only means "no hidden ordering" for future parallel staffing

---

## Parallel Example: User Story 1

```bash
# Tests first, both files at once:
"Shelf contract tests in internal/braind/books_test.go"        (T009)
"Cover-endpoint tests in internal/braind/books_covers_test.go" (T010)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 → 2 → 3. **STOP and VALIDATE**: `go test ./internal/braind/...` green + quickstart automated table rows for US1.
2. Deploy/demo: shelf live, read-only value delivered.

### Incremental Delivery

1. Setup + Foundational → domain tested.
2. US1 → shelf live (MVP). 3. US2 → capture live. 4. US3 → polish. 5. US4 → edit existing notes. 6. US5/US6 → series shelf and assignment flows. 7. Phase 7 → ship PR.

---

## Notes

- [P] = different files, no dependency. Commit after each checkpoint (Conventional Commits).
- Tests precede implementation per story (constitution Principle I); verify they fail first.
- File-conflict watch: `books_handlers.go` (T012, T018, T029, T033, T035) and `run.go` (T008, T012, T018) are sequential by design.


## Fallback Cover Completion (FR-005)

- [X] T024 Add `internal/braind/webassets/static/book-cover-placeholder.svg` and render it with accessible alternative text whenever a configured cover is absent, unreadable, empty, corrupt, or unsupported.
- [X] T025 Validate supported cover data and add unit tests in `internal/braind/books/catalog_test.go` for valid, absent, missing, invalid, unreadable, and path-escaping cover files.

## Series Support (FR-019–FR-029)

- [ ] T030 Add catalog tests for folder-derived membership, filename positions (including decimal and optional `Volume`/`Book` prefixes), unnumbered members, global newest-first ordering, and excluded overview notes; add a shelf contract test for per-entry series indicators and no series headings.
- [X] T031 Derive first-level series names and filename positions during catalog scanning while retaining one flat list in the existing newest-first order.
- [X] T032 Render one shelf list and show each series name and recognized volume position inside its book entry; do not add series headings or sublists.
- [ ] T033 Add writer and HTTP tests for series creation, prefilled assignment fields, moving between series and standalone paths, invalid folder/number inputs, and collision-safe edits.
- [X] T034 Add series and volume fields to create/edit forms; derive current values from paths and filenames; generate collision-safe series paths on create and move notes only when edit assignments change.
