---

description: "Task list for the brain web interface landing page (009-landing-page)"
---

# Tasks: Landing Page for the Brain Web Interface

**Input**: Design documents from `/specs/009-landing-page/`

**Prerequisites**: [plan.md](./plan.md) (required), [spec.md](./spec.md) (required — user stories), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/http.md](./contracts/http.md)

**Tests**: The project constitution (`.specify/memory/constitution.md`) Principles I and II REQUIRE tests with every feature — "Every feature MUST land together with tests that demonstrate its independent testability" and "Changes to business logic MUST include direct tests of the changed rules." Test tasks are therefore included in every user-story phase (write-first, expect FAIL before implementation).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

**Code paths** (from plan.md): This feature extends the existing Go module `github.com/johnrowl/brain`. New files: `internal/braind/landing.go`, `internal/braind/landing_test.go`, `internal/braind/webassets/` (embedded assets). Modified: `internal/braind/run.go` (`Handler()`). All routes documented in [contracts/http.md](./contracts/http.md).

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and shared assets

- [X] T001 Vendor pinned htmx `2.0.11` into `internal/braind/webassets/static/htmx.min.js` (create `internal/braind/webassets/` and `static/` directories; add a short header comment noting version 2.0.11 and BSD license provenance). Rationale: research.md §1 — no CDN, offline/private-deployment safe.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 Create `internal/braind/landing.go` with a `//go:embed webassets` directive (recursively embeds `index.html.tmpl`, `static/style.css`, `static/htmx.min.js`) and explicit content constants: product name `brain`, tagline exactly `a second brain, powered by markdown` (per `data-model.md` — the tagline MUST render exactly, preserving the lowercase spelling of "markdown"), description, and the under-construction notice. No runtime, operational, vault, or configuration values are passed to the template (FR-005).
- [X] T003 [P] Register `GET /static/` in `Handler()` in `internal/braind/run.go` serving the embedded assets (stdlib `http.FileServer` over `http.FS` or equivalent) with correct content types (`text/css; charset=utf-8`, `text/javascript; charset=utf-8`) and `Cache-Control: public, max-age=3600`, per `contracts/http.md`. `/healthz` (200, `ok\n`, `Cache-Control: no-store`) and 404 semantics for unknown paths MUST remain unchanged.

**Checkpoint**: Foundation ready - embedded assets exist, `/static/` serves them, constants defined; user story implementation can now begin.

---

## Phase 3: User Story 1 - First-time visitor learns what brain is (Priority: P1) 🎯 MVP

**Goal**: `GET /` returns a pleasant HTML page naming the product, showing the exact tagline "a second brain, powered by markdown", and stating the web interface is under construction.

**Independent Test**: `go test ./internal/braind/ -run 'Landing' -v` asserts `GET /` → 200 `text/html` containing the exact copy and correct headers, and `POST /` → 405. Browser check: open the daemon root in a browser and read the copy.

### Tests for User Story 1 (write FIRST, expect FAIL) ⚠️

- [X] T004 [P] [US1] Add contract tests in `internal/braind/landing_test.go` (httptest, table-driven, matching `run_test.go` style): `GET /` → 200 with `Content-Type: text/html; charset=utf-8`, body containing `brain`, the exact tagline `a second brain, powered by markdown`, and the under-construction notice; headers `Cache-Control: public, max-age=3600` and `Content-Security-Policy: default-src 'self'` (CSP is safe because the page references only `/static/` assets — `contracts/http.md`); `POST /` → 405.

### Implementation for User Story 1

- [X] T005 [US1] Create `internal/braind/webassets/index.html.tmpl`: semantic HTML5 page (`<!DOCTYPE html>`, `lang="en"`, `viewport` meta) with the product name in a single `h1`, the exact tagline and description as plain text, the under-construction notice, and relative references to `/static/style.css` and `/static/htmx.min.js`. ALL meaningful content MUST be in the initial server-rendered response — no JavaScript-rendered content, no `<noscript>` duplication (FR-003, research.md §3).
- [X] T006 [US1] Implement the landing page render in `internal/braind/landing.go`: parse `webassets/index.html.tmpl` once with `html/template` (`template.Must`), execute it against only the content constants (auto-escaping on), and register `GET /` in `Handler()` in `internal/braind/run.go` setting the CSP and Cache-Control headers from T004's contract. After this task, T004's tests must pass.

**Checkpoint**: User Story 1 is fully functional and testable independently — visiting the daemon root shows the required copy (MVP).

---

## Phase 4: User Story 2 - The page works for every visitor (Priority: P2)

**Goal**: The page is readable with scripting disabled and renders without layout breakage across 320 px – 1920 px viewports, with accessible semantics.

**Independent Test**: `go test ./internal/braind/ -run 'Landing' -v` includes the no-script render assertion and the stylesheet contract; manual/browser check at 320/768/1024/1920 px shows no horizontal scrolling or clipping (quickstart.md §5).

### Tests for User Story 2 (write FIRST, expect FAIL) ⚠️

- [X] T007 [P] [US2] Add no-script and asset tests in `internal/braind/landing_test.go`: strip all `<script>...</script>` blocks from the `GET /` body and assert the product name, tagline, description, and notice remain (proves server-rendered content — FR-003/SC-003); assert `GET /static/style.css` → 200, `Content-Type: text/css; charset=utf-8`, non-empty body (contract).

### Implementation for User Story 2

- [X] T008 [US2] Create `internal/braind/webassets/static/style.css` (hand-written, dependency-free per research.md §4): fluid responsive layout with media queries covering 320 px, 768 px, 1024 px, and 1920 px with NO horizontal overflow or overlapping content at any width (FR-004/SC-002); sufficient text contrast for readability (SC-003).
- [X] T009 [P] [US2] Add accessible semantics to `internal/braind/webassets/index.html.tmpl`: landmark elements (`<main>` wrapping the copy, proper heading hierarchy), text-content-only structure reachable without a pointer (acceptance scenario US2-3), no external fonts/icons (research.md §4). Parallel with T008 — disjoint files (`index.html.tmpl` vs `static/style.css`).

**Checkpoint**: User Stories 1 AND 2 both work independently — page renders fully with scripting disabled and stays clean across viewports.

---

## Phase 5: User Story 3 - The page stays private and reachable (Priority: P3)

**Goal**: The page contains no sensitive content and keeps rendering correctly even when the vault or optional subsystems are unavailable, with no broken internal references.

**Independent Test**: `go test ./internal/braind/ -run 'Landing' -v` covers the sensitive-content scan, the subsystem-unavailable structural test, and link-integrity checks; quickstart.md §6 scan shows no sensitive matches.

### Tests for User Story 3 (write FIRST, expect FAIL) ⚠️

- [X] T010 [P] [US3] Add privacy/availability tests in `internal/braind/landing_test.go`: (a) the rendered `GET /` body contains NONE of a probe set of sensitive strings (e.g., `client_secret`, secret-like token patterns, vault-path-like `/notes/`, `BEGIN `) — SC-005/FR-005; (b) `GET /` and `GET /static/*` respond correctly with no configuration/state passed to the handler, proving the page is static and independent of the vault and optional subsystems — FR-006 (subsystems-down scenario).
- [X] T011 [US3] Add link-integrity test in `internal/braind/landing_test.go`: extract every `src`/`href` from the rendered `GET /` body and assert each relative reference resolves with 200 through `Handler()` via httptest (US3 edge case — no broken or unfinished surfaces; SC-001 covers content).

### Implementation for User Story 3

- [X] T012 [US3] Harden `internal/braind/landing.go` per the US3 contract: confirm the template context contains ONLY the four content constants from T002 (no runtime/operational values), that no secret-like or vault-path-like strings exist in the served HTML, and that unknown paths still return 404 and only `GET` is accepted (contracts/http.md; existing `run_test.go` rejection cases remain green).

**Checkpoint**: All user stories are independently functional — private, reachable, no dead references.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [X] T013 [P] Run full validation per `specs/009-landing-page/quickstart.md` and record outcomes: `go vet ./...`, `go test ./...` (full suite — landing + existing braind tests), local daemon run + curl checks (`/`, `/static/htmx.min.js`, `/static/style.css`, `/healthz` regression, `POST /` → 405, unknown path → 404), no-script text check (§4), responsive check at 320/768/1024/1920 (§5), sensitive-content scan (§6), build smoke `go build ./cmd/braind` (§7).
- [X] T014 [P] Update `docs/roadmap.md` sequencing/status index to reference this feature (`009-landing-page`) as the initial web surface, keeping the existing table format.
- [ ] T015 Verify the final commit bundle follows Conventional Commits per `CONTRIBUTING.md` (feature and fix commits use `feat:`/`fix:` scopes; each task group committed with green tests).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately (T001 vendors the only shared external artifact).
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories (T002 constants, T003 `/static/` serving).
- **User Stories (Phase 3+)**: All depend on Foundational completion.
- **Polish (Final Phase)**: Depends on all desired user stories being complete.

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational — no dependency on other stories. **MVP.**
- **User Story 2 (P2)**: Can start after Foundational — touches `index.html.tmpl` (with US1's T005) and adds `style.css`; sequenced after US1 because both edit the template.
- **User Story 3 (P3)**: Can start after Foundational — uses the rendered page for its tests; sequenced after US1 for the same shared-file reason.

> Note: US2 and US3 share files with US1 (`index.html.tmpl`, `landing.go`, `landing_test.go`), so stories are sequenced P1 → P2 → P3 rather than truly parallel. Within and across stories, tasks marked `[P]` touch disjoint files and can run in parallel.

### Within Each User Story

- Tests MUST be written and FAIL before implementation (T004 before T005/T006; T007 before T008/T009; T010/T011 before T012).
- Implementation before integration; story complete before moving to next priority.

### Parallel Opportunities

- T001 (Phase 1) and T002/T003 (Phase 2) are independent of each other's content.
- T003 `[P]` (static serving) can run in parallel with T002 (constants/embed) — different files (`run.go` vs `landing.go`).
- T004 `[P]` is a standalone test file add.
- T008 `[P]` (`style.css`) and T009 `[P]` (`index.html.tmpl`) — disjoint files, can run together; both build on T005's template which is complete once US1 lands.
- T010 `[P]` and T011 (US3 tests) — but both append to `landing_test.go`, so run sequentially within the story if a single implementer does both.
- T013/T014/T015 (Polish) are mutually independent.

---

## Parallel Example: User Story 1

```bash
# Foundational: run together (different files)
Task: "T002 Create internal/braind/landing.go with go:embed directive and content constants"
Task: "T003 Register GET /static/ in Handler() in internal/braind/run.go serving embedded assets"

# US1: write the contract test, then implement to satisfy it
Task: "T004 Add contract tests in internal/braind/landing_test.go (GET / copy + headers, POST / 405)"
Task: "T005 Create internal/braind/webassets/index.html.tmpl (server-rendered content)"
Task: "T006 Implement page render + register GET / in internal/braind/landing.go and run.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001 — vendor htmx 2.0.11)
2. Complete Phase 2: Foundational (T002 embed+constants, T003 `/static/` serving) — CRITICAL, blocks all stories
3. Complete Phase 3: User Story 1 (T004 tests → T005 template → T006 render+route)
4. **STOP and VALIDATE**: run the quickstart §1 tests and the manual §3 curl/browser checks — root shows name + exact tagline + under-construction notice
5. Deploy/demo if ready (MVP is a page at the daemon root; htmx already served for future tiers)

### Incremental Delivery

1. Setup + Foundational → assets embedded and `/static/` live
2. Add User Story 1 → independently tested → **MVP**
3. Add User Story 2 → responsive stylesheet + no-script/accessibility tests
4. Add User Story 3 → privacy/availability/link-integrity hardening
5. Polish → full quickstart validation + roadmap update

### Parallel Team Strategy

With multiple developers: Developer A owns Foundational; Developer B takes US1 immediately after (test-first); Developer C prepares T008 `style.css` in parallel (disjoint file) and merges when the template lands. US2/US3 integration touches shared files and is sequenced after US1 merges.

---

## Notes

- `[P]` tasks = different files, no dependencies; stories US2/US3 are sequenced (not parallel) because they share `index.html.tmpl`, `landing.go`, and `landing_test.go` with US1.
- `[Story]` label maps each task to its user story from `spec.md` for traceability.
- Tests are mandated by the constitution (Principles I & II) — write them first and verify they fail before implementation.
- Exact copy constraints (tagline spelling, under-construction notice) come from `data-model.md` and `spec.md` FR-002; do not paraphrase.
- Headers and status codes MUST match `contracts/http.md` exactly (CSP `default-src 'self'`, `Cache-Control: public, max-age=3600`, `/healthz` `no-store`).
- Commit after each task or logical group; Conventional Commits per `CONTRIBUTING.md`.
