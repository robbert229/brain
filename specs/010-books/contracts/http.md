# HTTP Contract: Book Reviews

**Feature**: `specs/010-books/spec.md` | Model: [data-model.md](./data-model.md)

All routes mount in the existing `newHandler` mux (spec 001) behind the spec 002 auth
surface. Unless noted, unauthenticated browsers `302` to `/login?next=<path>` and API
clients (`Accept: application/json`) get `401 {"error":"unauthenticated"}` exactly as
the landing page behaves today.

## Endpoints

| Method | Path | Auth | Success | Notes |
| --- | --- | --- | --- | --- |
| `GET` | `/books` | any verified session | `200 OK`, `text/html; charset=utf-8` | Server-rendered shelf: one card per book (readable cover or bundled accessible fallback cover, title, author, rating, finished, interest, status, review), newest-finished first; "no books yet" empty state; "N notes skipped" footer when nonzero; "Add a review" link only when the visitor may create and the service is writable. Headers: `Cache-Control: no-store` (derived from the live vault, session-gated); `Content-Security-Policy: default-src 'self'` (assets from `/static/`, covers from `/books/covers/`). |
| `GET` | `/books/new` | session, role ≥ `user`, writable | `200 OK`, `text/html; charset=utf-8` | Review form page (plain `<form method=post action="/books">` with hidden CSRF field; Pico-styled). `403` failure page for `viewer` or when `BRAIND_READ_ONLY` is set. |
| `POST` | `/books` | session, role ≥ `user`, writable, valid CSRF | `303 See Other` → `Location: /books` | `application/x-www-form-urlencoded`: `title` (required), `review` (required), `author`, `rating`, `cover`, `finished` (optional) + CSRF. On success exactly one new note exists in the Books folder (data-model.md serialization contract) and the shelf shows it. `422` re-renders the form with per-field errors and preserved values (validation, oversize note). `403` for viewer role, missing/bad CSRF, or read-only mode. |
| `GET` | `/books/covers/{filename}` | any verified session | `200 OK`, image content type | Serves a single image filename from a rooted view of the configured covers folder. `Content-Type` from extension (`.jpg .jpeg .png .webp .gif`); `Last-Modified` and `304` supported; `Cache-Control: private, max-age=3600`. Unsafe names, missing files, and symlinks resolving outside the covers folder return `404` with no detail (FR-012). The shelf only links to supported, readable cover images; other attachments use the bundled fallback. |

## Landing page change

`GET /` gains a "Books" card link to `/books` in the existing template (FR-001). No
other landing behavior changes.

## Configuration

| Variable | Default | Contract |
| --- | --- | --- |
| `BRAIND_BOOKS_FOLDER` | `01 - Note Box/Hobby/Books` | Relative sub-path of `BRAIND_DATA_PATH`; no `..`, not absolute. Missing folder is valid (empty shelf). |
| `BRAIND_COVERS_FOLDER` | `06 - Meta/Attachments` | Same rules; the only directory cover requests may read. |

Existing bounds apply: created notes ≤ `BRAIND_MAX_NOTE_BYTES`; `BRAIND_READ_ONLY`
disables creation (403 + hidden form); all paths resolve under the data-lock root.

## Outlines (semantic, not pixel)

- Shelf card: real cover image or bundled accessible fallback cover → `h3` title → byline line
  (author · finished date · rating stars/text) → interest/status chips when present →
  review paragraphs.
- Form: labelled fields in data-model.md order, submit button `Save review`; server
  re-render on `422` keeps entered values and lists errors above the fields.

## Explicitly out of scope

Editing/deleting notes, per-book detail pages, filters/sort controls, pagination,
Markdown rendering of review bodies (plain paragraphs are displayed as written),
cover upload (cover references an existing attachment filename).
