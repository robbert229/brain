# HTTP Contract: Book Reviews

**Feature**: `specs/010-books/spec.md` | Model: [data-model.md](./data-model.md)

All routes mount in the existing `newHandler` mux (spec 001) behind the spec 002 auth
surface. Unless noted, unauthenticated browsers `302` to `/login?next=<path>` and API
clients (`Accept: application/json`) get `401 {"error":"unauthenticated"}` exactly as
the landing page behaves today.

## Endpoints

| Method | Path | Auth | Success | Notes |
| --- | --- | --- | --- | --- |
| `GET` | `/books` | any verified session | `200 OK`, `text/html; charset=utf-8` | Server-rendered single shelf list ordered globally by finished date; each series entry shows its folder name and recognized volume position inside the entry, with no series sublists or headings; root entries have no series indicator; "no books yet" empty state; "N notes skipped" footer when nonzero; "Add a review" and per-entry "Edit review" links only when the visitor may write and the service is writable. Headers: `Cache-Control: no-store` (derived from the live vault, session-gated); `Content-Security-Policy: default-src 'self'` (assets from `/static/`, covers from `/books/covers/`). |
| `GET` | `/books/new` | session, role ≥ `user`, writable | `200 OK`, `text/html; charset=utf-8` | Review form page (plain `<form method=post action="/books">` with hidden CSRF field; Pico-styled), including optional `series` and `series_number`. `403` failure page for `viewer` or when `BRAIND_READ_ONLY` is set. |
| `POST` | `/books` | session, role ≥ `user`, writable, valid CSRF | `303 See Other` → `Location: /books` | Form fields include `title` and `review` (required), plus `author`, `rating`, `cover`, `finished`, `series`, `series_number` (optional) and CSRF. On success the note is stored at the generated standalone or series path and appears on the shelf. `422` re-renders the form with per-field errors and preserved values. `403` for viewer role, missing/bad CSRF, or read-only mode. |
| `GET` | `/books/edit?file=<relative-path>` | session, role ≥ `user`, writable | `200 OK`, `text/html; charset=utf-8` | Prefilled form for an existing book note, deriving series and volume from its folder and filename; the relative path is confined to the Books folder. Unsafe paths and non-book notes return `404`; read-only mode refuses editing. |
| `POST` | `/books/edit` | session, role ≥ `user`, writable, valid CSRF | `303 See Other` → `Location: /books` | Hidden `file` relative path plus the same editable fields as create. Success updates the selected note and preserves unrelated frontmatter; changed series assignments move it to a generated collision-safe destination. `422` re-renders validation errors; `403` for viewer role, missing/bad CSRF, or read-only mode; unsafe paths and non-book notes are refused. |
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
disables creation, editing, and deletion (403 + hidden actions); all paths resolve under
the data-lock root.

## Outlines (semantic, not pixel)

- Shelf: one continuous list, globally ordered by finished date newest-first, including series and standalone books.
- Series entry: show a compact indicator with the direct series folder name and the parsed numeric position when available; never create a separate heading or sublist.
- Shelf card: real cover image or bundled accessible fallback cover → `h3` title → byline line
  (author · finished date · rating stars/text) → interest/status chips when present →
  review paragraphs.
- Create/edit forms: labelled fields in data-model.md order, submit button `Save review`
  or `Save changes`; edit forms include the selected relative note path. Server re-render
  on `422` keeps entered values and lists errors above the fields.

## Explicitly out of scope

Per-book detail pages, author/rating facet filters, pagination, Markdown rendering of
review bodies (plain paragraphs are displayed as written).
