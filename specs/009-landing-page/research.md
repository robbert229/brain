# Research: Landing Page for the Brain Web Interface

**Feature**: [spec.md](./spec.md) | **Date**: 2026-10-26

## Purpose

Resolve the technical choices needed to plan a static marketing landing page served at
the root of the `braind` web service: HTMX-powered, HTMX assets served by `braind`
itself, no external network, works without JavaScript, responsive and accessible.
No `[NEEDS CLARIFICATION]` markers remained in the specification; the questions below
are technology-selection and best-practice questions.

## 1. HTMX version and sourcing

- **Decision**: Pin **htmx 2.x (`2.0.11`)** and vendor the minified distribution into a
  Go-embedded assets directory, served by `braind` at `/static/htmx.min.js`.
- **Rationale**: htmx 2.x is the `latest` line, remains the default for CDN-loading
  sites, and is supported indefinitely. htmx 4.0.0 (released 2026-08-28) is a
  fetch()-based rewrite with intentional breaking changes (explicit attribute
  inheritance, renamed attributes/events, real requests for back-button and error
  responses) and is deliberately tagged `next` on npm until early 2027; nothing in a
  placeholder landing page needs 4.x features. Vendoring via `go:embed` keeps the
  artifact in the binary: reproducible builds, offline builds, and private deployments.
- **Alternatives considered**:
  - *htmx 4.0.0*: stable release but marked `next`; breaking changes offer no benefit
    here and would force migration work later for zero current value.
  - *CDN link (jsdelivr/unpkg)*: runtime external network dependency — conflicts with
    the private-deployment assumption and breaks offline operation.
  - *htmx 1.x*: legacy line retained for IE support; unnecessary.

## 2. Serving the page and assets from the daemon

- **Decision**: Embed the page template, stylesheet, and htmx asset into the binary with
  `go:embed`; register `GET /` and `GET /static/` on the existing
  `http.NewServeMux()` already used for `GET /healthz` (Go 1.22 method+path patterns).
- **Rationale**: Matches the current `braind` structure (single mux in
  `internal/braind/run.go`), keeps one self-contained binary, and uses stdlib only.
  No new process, port, or service.
- **Alternatives considered**:
  - *Reading assets from disk at runtime*: breaks the single-binary/self-contained
    deployment and adds a filesystem dependency.
  - *Separate static server or Node.js app tier*: explicitly forbidden by the
    constitution (Principle IV).
  - *Third-party Go web framework (chi, gin, echo)*: unjustified for two routes.

## 3. Page templating

- **Decision**: `html/template` with an explicit, minimal data context (product name,
  tagline, description, construction notice). All meaningful content is rendered into
  the initial response so the page is complete without JavaScript.
- **Rationale**: `html/template` auto-escapes and keeps the page declarative; content
  values are explicit Go constants so no business logic hides in markup (Principle II).
  Server-rendered content structurally satisfies the "scripting disabled" requirement —
  no `<noscript>` duplication is needed because nothing depends on scripts.
- **Alternatives considered**:
  - *Hand-assembled HTML strings*: no escaping, error-prone, harder to review.
  - *Client-side rendering of content*: violates the no-JavaScript requirement.

## 4. Styling and design

- **Decision**: One hand-written embedded `style.css`: semantic HTML5 landmarks, a
  fluid responsive layout, sufficient color contrast, no external fonts/icons.
- **Rationale**: Zero external requests, tiny footprint, accessible by construction
  (SC-002/SC-003). A CSS framework is not justified for a single page (Principle III).
- **Alternatives considered**:
  - *Tailwind/Bootstrap*: build-step or CDN dependency and abstraction far beyond the
    need.
  - *Web fonts/CDN icons*: external network at runtime; rejected per assumptions.

## 5. Security and caching headers

- **Decision**: Landing page: `Content-Security-Policy: default-src 'self'` (page has no
  external resources) and `Cache-Control: public, max-age=3600`. Static assets:
  short public `max-age` with correct content types (`text/css`, `text/javascript`).
  `/healthz` remains `Cache-Control: no-store` (operational, unchanged).
- **Rationale**: Static marketing copy is safe to cache briefly; `default-src 'self'`
  is free hardening given zero third-party content; health semantics are preserved for
  operational correctness (no regressions, SC-006).
- **Alternatives considered**:
  - *`no-store` everywhere*: unnecessarily discards caching for static copy.
  - *No CSP*: weaker posture without benefit.

## 6. Testing strategy

- **Decision**: Stdlib table-driven `httptest` tests in package `braind` (matching
  `run_test.go` style): root returns 200 `text/html` containing product name, exact
  tagline, and construction notice; assets served with correct content types; `POST /`
  rejected; unknown paths 404; `/healthz` unchanged; rendered body contains none of a
  probe set of sensitive strings (secret-like values, vault-path-like values).
- **Rationale**: Independently testable (Principle I), no external services, fast,
  CI-friendly.
- **Alternatives considered**:
  - *Browser automation (Playwright/ChromeDriver)*: heavy for a static page; adds a
    node/JS toolchain the project forbids.
  - *Golden-file HTML comparison*: brittle; string assertions on required copy are
    sufficient and stable.

## Resolved inputs from the specification

- Landing page is public (no authentication) — contains only marketing copy.
- Tagline must render exactly: "a second brain, powered by markdown".
- Page must remain available when optional subsystems or the vault are unavailable —
  guaranteed because the page is static and reads nothing at request time.
- Scope is the landing page only; no functional web UI in this feature.
