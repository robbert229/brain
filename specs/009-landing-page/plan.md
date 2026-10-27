# Implementation Plan: Landing Page for the Brain Web Interface

**Branch**: `009-landing-page` | **Date**: 2026-10-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/009-landing-page/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command; its definition describes the execution workflow.

## Summary

Serve a pleasant, accessible landing page at the root web address of the `braind`
service. The page tells a first-time visitor what brain is — "a second brain, powered
by markdown" — and that the web interface is under construction. All content is
server-rendered so the page is complete with scripting disabled; it is enhanced with
HTMX, and the HTMX script is served by `braind` itself (no CDN, no external network).

Technical approach: extend the existing Go 1.22 `http.ServeMux` in `internal/braind`
with `GET /` (an `html/template` rendering of an embedded template over explicit content
constants) and `GET /static/` (embedded, vendored htmx 2.0.11 plus a hand-written CSS
file). stdlib `httptest` coverage proves the contract; no new packages, frameworks, or
services. See [research.md](./research.md) for the resolved technology decisions.

## Technical Context

**Language/Version**: Go 1.22 (module `github.com/johnrowl/brain`) — already in repository

**Primary Dependencies**: stdlib only at runtime — `net/http` (Go 1.22 method+path
patterns already in use for `/healthz`), `html/template`, `embed`. Vendored htmx
`2.0.11` as an embedded static asset (not a Go dependency; no CDN).

**Storage**: None — fully static page; content is compile-time constants; assets are
`go:embed`ed files. No persistence, no vault access. See [data-model.md](./data-model.md).

**Testing**: stdlib `testing` + `net/http/httptest`, table-driven, consistent with
`internal/braind/run_test.go`; `go vet ./...` and `go test ./...` must pass in CI.

**Target Platform**: Linux server (`braind`); modern desktop and mobile browsers as clients.

**Project Type**: web-service (HTTP daemon) — `braind`

**Performance Goals**: trivial static page; main content visible in < 2 s for 95% of
loads on a typical local connection (SC-004). Per-request cost is a single template
render.

**Constraints**: no external network for page or assets (private deployment); no
JavaScript application tier (Constitution IV); page must not read the vault or optional
subsystems (FR-006); no sensitive content in any response (FR-005); tagline must render
exactly; responsive and accessible (SC-002/SC-003).

**Scale/Scope**: one page, two static assets, three route groups (`/`, `/static/`,
`/healthz`). No functional web UI in scope.

No `[NEEDS CLARIFICATION]` remain; all choices resolved in [research.md](./research.md).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Result | Basis |
| --- | --- | --- |
| I. Independently Testable Features | PASS | Handler-level `httptest` coverage for every route, status, and copy assertion; runs in CI; no external state or services. |
| II. Explicit Business Logic | PASS | The template is declarative markup only. Content values (product name, tagline, description, status notice) are explicit named Go constants rendered through an auto-escaping template; no logic is buried in HTML. |
| III. Simplicity Over Abstraction | PASS | Two routes, embedded assets, stdlib only. No framework, package boundary, or abstraction introduced; hand-written minimal CSS. Extraction of a `web` package is deferred until the M1 reader surface demonstrates repeated need. |
| IV. Go Backend, HTMX Frontend | PASS | Served by `braind` (Go); page is server-rendered HTML enhanced with HTMX; `htmx.min.js` is delivered by `braind` itself; no Node.js application server. |
| V. Markdown Single Source of Truth | PASS (not engaged) | No vault note content flows into the page; the page is static copy. No derived/canonical stores involved. |
| Data & Storage (mutation durability, derived data) | PASS (not engaged) | No mutations; nothing derived from vault content. |

No violations.

## Project Structure

### Documentation (this feature)

```text
specs/009-landing-page/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   └── http.md
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/braind/main.go                        # unchanged (entry point)
internal/braind/run.go                    # Handler() gains GET / and GET /static/ routes
internal/braind/landing.go                # NEW: go:embed of webassets, content constants,
                                          #      page render + route registration
internal/braind/landing_test.go           # NEW: httptest coverage (copy, assets, methods,
                                          #      404/405, /healthz regression, sensitive scan)
internal/braind/webassets/                # NEW: go:embed root
├── index.html.tmpl                       #     landing page template (server-rendered content)
└── static/
    ├── style.css                         #     hand-written responsive stylesheet
    └── htmx.min.js                       #     pinned htmx 2.0.11, vendored w/ version+license note
```

Existing packages (`config`, `datalock`, `lifecycle`, `logging`, `status`, `cli`,
`tnnlpcore`) are unchanged.

**Structure Decision**: Single existing Go module. The landing page slots into the
existing `braind` HTTP daemon — no new packages, services, or processes — consistent
with Simplicity Over Abstraction (Principle III) and the Go+HTMX mandate (Principle IV).
Embedded assets keep a single self-contained binary for private deployments. If the
roadmap's browser surface (M1 reader) grows, extraction into a dedicated `web` package
is a review-time decision with demonstrated need, not a present abstraction.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No Constitution violations; this table is intentionally empty.
