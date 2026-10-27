# Data Model: Landing Page for the Brain Web Interface

**Feature**: [spec.md](./spec.md) | **Date**: 2026-10-26

## Overview

The landing page is fully static. There is **no persistent storage, no runtime data
input, and no state**. Markdown-as-source-of-truth (Constitution Principle V) is not
engaged: no vault note content flows into the page, and nothing is derived from the
vault. This document models the only two "data" shapes involved: the page content
rendered into HTML, and the embedded web assets.

## Entity: Landing Page Content

Static, compile-time values rendered into the page at request time.

| Field | Value | Constraint |
| --- | --- | --- |
| Product name | `brain` | Rendered as the page title/heading (FR-002). |
| Tagline | `a second brain, powered by markdown` | Must render **exactly**, preserving the lowercase spelling of "markdown" (FR-002; edge case). |
| Description | Short plain-language statement of what brain is | Static text only. |
| Construction notice | Statement that the web interface is under construction | Static text only (FR-002). |

**Validation rules** (from specification):

- The tagline constant is the single source of that string; automated tests assert the
  rendered HTML contains the exact value (FR-002, SC-001).
- The template context contains **only** the fields above. No runtime, operational,
  vault, or configuration values are ever passed to the template (FR-005; sensitive
  content, SC-005).
- Rendering must not depend on the vault or any optional subsystem (FR-006).

**State transitions**: None — content is immutable per build; the resource has no
lifecycle.

## Entity: Served Web Assets

Files embedded into the `braind` binary and served under `/static/`.

| Asset | Served path | Content type |
| --- | --- | --- |
| Page template (`index.html.tmpl`) | — (not served directly; rendered) | — |
| Stylesheet (`style.css`) | `/static/style.css` | `text/css; charset=utf-8` |
| htmx distribution (`htmx.min.js`, pinned `2.0.11`, vendored with version/license note) | `/static/htmx.min.js` | `text/javascript; charset=utf-8` |

**Validation rules**:

- Assets must be present in the binary (`go:embed`); a build ships no external fetch
  (assumption: no CDN, private-deployment safe).
- Served with correct content types so browsers render them normally (FR-007).
- The page references assets with relative URLs (`/static/…`) so no absolute/host
  coupling exists.

## Relationships

```
Landing Page Content ── rendered by ──► index.html.tmpl ──► GET /  (text/html)
style.css / htmx.min.js ── referenced by ──► index.html.tmpl  ──► GET /static/…
```

## Non-goals

- No persistence, indexes, caches, or derived data.
- No note content, paths, credentials, tokens, or operational details anywhere in
  responses (SC-005).
- No request-time reads of vault, configuration secrets, or subsystem state.
