# HTTP Contract: Landing Page (served by `braind`)

**Feature**: [spec.md](../spec.md) | **Date**: 2026-10-26

Baseline: `braind` already serves `GET /healthz` (Application HTTP surface from
FND-001). This contract adds the root route and a static-asset prefix without changing
the existing surface.

## Routes

| Method | Path | Status | Content-Type | Body / Notes |
| --- | --- | --- | --- | --- |
| `GET` | `/` | `200 OK` | `text/html; charset=utf-8` | Landing page, behind the shared authentication decision (spec 002): any verified session is served; anonymous browsers `302` to `/login?next=%2F` and API clients (`Accept: application/json`) get `401 {"error":"unauthenticated"}`. MUST contain the product name `brain`, the exact tagline `a second brain, powered by markdown`, and the under-construction notice. Headers: `Cache-Control: private, max-age=3600` (session-gated: never a shared cache); `Content-Security-Policy: default-src 'self'`. |
| `GET` | `/static/style.css` | `200 OK` | `text/css; charset=utf-8` | Embedded stylesheet. |
| `GET` | `/static/htmx.min.js` | `200 OK` | `text/javascript; charset=utf-8` | Embedded, vendored htmx `2.0.11`. |
| `GET` | `/healthz` | `200 OK` | `text/plain; charset=utf-8` | Unchanged: body `ok\n`, `Cache-Control: no-store`. |
| any | `/` (other methods) | `405 Method Not Allowed` | — | Only `GET` is registered on `/`. |
| `GET` | any other path | `404 Not Found` | — | Existing behavior preserved. `/readyz` is no longer an example: it is now served by [001 Daemon foundation](../../specs/001-daemon-foundation/contracts/http.md) (`200 ready` / `503 not ready`). |

## Response guarantees

1. **Server-rendered**: every word of the page content is present in the initial HTML
   response; the page is fully readable with client-side scripting disabled (FR-003,
   SC-003).
2. **Privacy-safe**: no response contains note content, note paths, credentials,
   tokens, or operational details (FR-005, SC-005).
3. **Availability**: the page is static; it renders without reading the vault or any
   optional subsystem and remains available during their outages (FR-006).
4. **No regressions**: `/healthz` behavior and 404/405 semantics are unchanged
   (FR-007, SC-006).
5. **No external network**: the page and its assets load entirely from `braind`
   (CSP `default-src 'self'`).

## Caching model

- Landing page: `Cache-Control: private, max-age=3600` (static copy, but the
  route now requires a session, so only private caches may retain it).
- Static assets: `Cache-Control: public, max-age=3600` with correct content types.
- `/healthz`: `Cache-Control: no-store` (operational — unchanged).

## Verification mapping

| Contract point | Success criteria |
| --- | --- |
| `/` serves copy | SC-001 |
| Responsive layout, no clipping | SC-002 |
| Server-rendered content | SC-003 |
| Fast static render | SC-004 |
| No sensitive content | SC-005 |
| Existing endpoints unchanged | SC-006 |
