# Phase 1 Data Model: Book Reviews

**Feature**: `specs/010-books/spec.md` | Research: [research.md](./research.md)

All entities live in `internal/braind/books`. There is no persistence layer beyond
the vault files themselves (Principle V); every derived value below is recomputed per
request (research R6).

## Entities

### Book (parsed, read side)

One book note on disk, projected into shelf display values. Produced only by
`ParseNote(filename, bytes)`; never constructed by hand outside tests.

| Field | Type | Source | Absent/invalid behavior |
| --- | --- | --- | --- |
| `Title` | string | first `# ` heading of the body, else filename base | always present (fallback) |
| `Review` | string | body after the title heading, trimmed | empty → card shows no prose |
| `Author` | string | frontmatter `author` | empty |
| `Cover` | string | frontmatter `cover` | empty or unusable attachment → bundled accessible fallback cover |
| `Interest` | string | frontmatter `interest` | empty → nothing rendered |
| `Status` | string | frontmatter `status (Media)` | empty → nothing rendered |
| `Finished` | `time.Time` (date) | frontmatter `finished` | zero → sorts undated, date hidden |
| `Rating` | float64 | frontmatter `rating` (`n/5`, halves; also bare number) | 0 → rating hidden; raw text kept in `RatingText` when unparseable |
| `RatingText` | string | raw `rating` value for display | empty |
| `Filename` | string | note filename (diagnostics, skip notices) | always present |
| `Path` | string | slash-separated path relative to the configured Books folder | present for scanned shelf entries; edit target |
| `Series` | string | first folder beneath the configured Books folder | empty for standalone books; otherwise the folder name |
| `SeriesNumber` | float64 | leading number after the series folder name in the filename, optionally after `Volume` or `Book` | zero when no recognized position |
| `HasSeriesNumber` | bool | whether the filename contains a recognized numeric series position | false for standalone or unnumbered books |

Read-side normalization is enforced in `ParseNote`; series name and volume position
are derived during `Scan`; write-side field and path validation is enforced in
`ValidateDraft`.

- `Cover` must be a single clean path segment with an image extension
  (`.jpg .jpeg .png .webp .gif`); otherwise treated as absent (research R3).
- YAML frontmatter must decode to a mapping; scalars/sequences are not books.
- Book detection (FR-003): mapping contains ≥1 of `cover`, `rating`, `status (Media)`.

### Catalog (derived, per request)

Result of scanning `<data>/<books folder>` recursively with `fs.WalkDir`.

- `Entries []Entry` — detected books, ordered per the flat Sort rule below.
- `Skipped int` — `.md` files ignored (index notes, malformed frontmatter); surfaced
  as a shelf footer (research R2).
- `Truncated bool` — visit budget (2000 files) exhausted; shelf shows
  "showing first N" (research R6).
- Missing Books folder → empty Catalog (`Entries` empty, `Skipped` 0), which renders the
  "no books yet" state, never an error.
- A note that cannot be read at all (permission/IO) counts as skipped, others still
  render (FR-011).

**Series membership**: The first path segment beneath `booksDir` names the series.
Books at the root have no series. A directory overview note without book metadata
remains excluded by the existing book-detection rule.

**Series number**: After removing the case-insensitive series folder name from the
filename stem, accept a leading integer or decimal position, optionally preceded by
`Volume` or `Book` and optional whitespace/hyphen separators. Leading zeroes do not
change numeric order. This reads names such as `A Thousand Li 01 - The First Step`,
`The Irregular at Magic High School Volume 01`, `Reborn As a Demonic Tree Book - 1`,
and `Tsukimichi Volume 08.5`.

Series membership and position are derived from the path and filename; the writer
does not duplicate them in frontmatter. Create/edit forms use an optional `Series`
folder name and optional non-negative `SeriesNumber`. When assignment fields change,
the writer generates a destination path; if they do not change during edit, the
original path is retained.

**Shelf order**: All entries, whether standalone or in a series, remain in one list
ordered by `Finished` descending, undated last, then title ascending. Series name and
recognized volume number are display metadata inside each entry and do not affect
list ordering. Numeric positions are parsed numerically (so 8.5 falls between 8 and 9)
and shown when recognized; an unnumbered series entry still shows its series name.

### CoverRef (derived)

`Book.Cover` plus attachment validity, resolved beneath `<data>/<covers folder>`
during the scan. A readable, non-empty supported image produces
`<img src="/books/covers/{url-escaped cover}">`; absent, missing, unreadable, empty,
corrupt, or unsupported attachments produce the bundled accessible fallback-cover
image at the same dimensions (SC-003: zero broken images, no-JS).

### ReviewDraft (write side)

Create or edit form submission, validated by `ValidateDraft` before filesystem writes.

| Input | Required | Rule (FR-007, FR-010) |
| --- | --- | --- |
| `Title` | yes | ≤200 runes, no control characters, not blank after trim |
| `Review` | yes | non-empty after trim; rendered note ≤ `MaxNoteBytes` |
| `Series` | no | safe single folder name, ≤100 runes; empty means standalone |
| `SeriesNumber` | no | non-negative integer or decimal; requires `Series` |
| `SeriesSet` | transport marker | distinguishes an explicit empty assignment from older callers that omit series fields |
| `Author` | no | ≤200 runes, no control characters |
| `Rating` | no | empty, `n/5` or `n/5.0`/half steps `0–5`, or bare number `0–5` |
| `Cover` | no | empty or single clean segment with image extension |
| `CoverUpload` | no | JPG, PNG, GIF, or WebP; actual image data; ≤8 MiB; valid upload replaces `Cover` with generated filename |
| `Finished` | no | empty or `YYYY-MM-DD`, not in the future; form defaults to today for new or undated records |
| `CSRF` | yes | matches session token (existing auth surface check) |

Any violation → the form re-renders (`422`) with per-field messages and the submitted
text values preserved; the review note is not created or updated. A valid cover may
already have been stored as an attachment if another review field fails validation.

### Uploaded cover

The create/edit forms submit an optional `cover_upload` file as `multipart/form-data`.
The server accepts JPG, PNG, GIF, and WebP images up to 8 MiB, checks the file
signature and image structure, then stores bytes atomically under the configured
attachments folder using a SHA-256-derived filename. The generated filename becomes
the draft's `Cover` value and is serialized into frontmatter and the Obsidian embed.
When no new file is uploaded, the existing cover filename field continues to work;
on edit, leaving the upload blank preserves the current cover. If a valid upload is
stored but later review validation fails, the unattached image may remain in the
attachments folder and can be reused or removed manually.

### Generated note (serialization contract)

The exact bytes `CreateReview` writes, matching the corpus conventions so Obsidian
opens it with metadata intact (FR-008, Principle V):

```markdown
---
tags:
  - Hobby/Books
type: Zettel
author: <Author, omitted when empty>
cover: <Cover, omitted when empty>
status (Media): Finished
finished: <Finished as YYYY-MM-DD, omitted when empty>
rating: <Rating as n/5 with halves, omitted when empty>
---

# <Title>

<Review as paragraphs>

![[<Cover>]]        ← only when Cover set
```

**Filename algorithm**: standalone notes retain the existing title slug + `.md` in
the Books folder root. Series notes use `<Series>/<Series> <position> - <title-slug>.md`,
with a two-digit minimum integer position and any decimal fraction retained; a series
without a number uses an unnumbered filename. Collisions append `-2`, `-3`, … and
never overwrite. Notes are published atomically through a temporary file and a
no-overwrite link in the destination folder, followed by a folder sync.

### Editing an existing note

The selected note is addressed by a relative path beneath the configured Books
folder. Traversal, absolute paths, symlink escapes, non-files, and notes that no
longer parse as books are refused. A valid edit updates title, author, rating, cover,
finished date, and review; unrelated YAML fields, status, and interest are preserved.
If series/number fields are unchanged, the update replaces the selected file and
retains its path. If they change, the note is safely published at the generated
destination without overwriting another note, then the old path is removed. The
resulting bytes remain bounded by `MaxNoteBytes`.

### Deleting an existing note

Deletion is addressed by the same slash-separated relative path used for editing.
The contributor first opens a side-effect-free confirmation page that identifies the
book and explains that its Markdown note will be permanently removed. Only the
confirmation form's authorized, CSRF-validated POST removes the note. The target must
still be a regular file whose contents parse as a book note and must remain confined
to the configured Books folder; traversal, absolute paths, symlink escapes, missing
files, and non-book notes are refused. Removal affects only the Markdown note and
syncs its containing directory. Cover attachments are retained because other notes
may reference them.

## State transitions

Review creation lifecycle (single writer, spec 001 lock held):

`draft (form)` → `valid (ValidateDraft ok)` → `persisted (rename complete)` →
`visible (next shelf scan)` — every step forward is required; any validation or IO
failure stops at the current step with a user-visible form error and **no** partial
file (the temp file is removed on failure).

Existing-note edit lifecycle: `existing book note` → `safe in-root path and book
validation` → `validated replacement` → `atomic replacement` → `visible on next shelf
scan`. Validation or I/O failure leaves the original note intact.

Existing-note deletion lifecycle: `shelf delete action` → `confirmation page (no
write)` → `explicit CSRF-validated confirmation` → `safe in-root book-note removal`
→ `absent on next shelf scan`. Canceling or any refusal leaves the note and cover
attachments intact.

## Roles (from spec 002, no new identities)

- Any verified session → may read the shelf and covers.
- `RoleUser`/`RoleAdministrator` → may open create/edit/delete flows and submit (with CSRF).
- `RoleViewer` → read-only: no create/edit/delete controls, submissions refused (`403`).
- `BRAIND_READ_ONLY=true` → same as viewer for create/edit/delete flows, enforced server-side
  regardless of role, plus the shelf hides all write actions.
