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

Validation rules (all enforced in `ParseNote`, table-tested):

- `Cover` must be a single clean path segment with an image extension
  (`.jpg .jpeg .png .webp .gif`); otherwise treated as absent (research R3).
- YAML frontmatter must decode to a mapping; scalars/sequences are not books.
- Book detection (FR-003): mapping contains ≥1 of `cover`, `rating`, `status (Media)`.

### Catalog (derived, per request)

Result of scanning `<data>/<books folder>` recursively with `fs.WalkDir`.

- `Books []Book` — detected books, ordered per the Sort rule below.
- `Skipped int` — `.md` files ignored (index notes, malformed frontmatter); surfaced
  as a shelf footer (research R2).
- `Truncated bool` — visit budget (2000 files) exhausted; shelf shows
  "showing first N" (research R6).
- Missing Books folder → empty Catalog (`Books` empty, `Skipped` 0), which renders the
  "no books yet" state, never an error.
- A note that cannot be read at all (permission/IO) counts as skipped, others still
  render (FR-011).

**Sort**: `Finished` descending (newest first) → zero-`Finished` after all dated →
`Title` ascending (case-insensitive) as final tiebreak.

### CoverRef (derived)

`Book.Cover` plus attachment validity, resolved beneath `<data>/<covers folder>`
during the scan. A readable, non-empty supported image produces
`<img src="/books/covers/{url-escaped cover}">`; absent, missing, unreadable, empty,
corrupt, or unsupported attachments produce the bundled accessible fallback-cover
image at the same dimensions (SC-003: zero broken images, no-JS).

### ReviewDraft (write side)

Form submission, validated by `ValidateDraft` before any filesystem touch.

| Input | Required | Rule (FR-007, FR-010) |
| --- | --- | --- |
| `Title` | yes | ≤200 runes, no control characters, not blank after trim |
| `Review` | yes | non-empty after trim; rendered note ≤ `MaxNoteBytes` |
| `Author` | no | ≤200 runes, no control characters |
| `Rating` | no | empty, `n/5` or `n/5.0`/half steps `0–5`, or bare number `0–5` |
| `Cover` | no | empty or single clean segment with image extension |
| `Finished` | no | empty or `YYYY-MM-DD`, not in the future |
| `CSRF` | yes | matches session token (existing auth surface check) |

Any violation → the form re-renders (`422`) with per-field messages and the submitted
values preserved; nothing is written.

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

**Filename algorithm**: slugify title (lowercase, runs of non-alphanumerics → `-`,
trim `-`; empty → `book`) + `.md`; on collision with any existing file in the Books
folder root, append `-2`, `-3`, … . Persisted atomically via `O_CREATE|O_EXCL` temp
file in the same folder → write+fsync → rename → folder fsync (research R5). `O_EXCL`
makes "never overwrites" a kernel guarantee, not a check.

## State transitions

Review submission lifecycle (single writer, spec 001 lock held):

`draft (form)` → `valid (ValidateDraft ok)` → `persisted (rename complete)` →
`visible (next shelf scan)` — every step forward is required; any validation or IO
failure stops at the current step with a user-visible form error and **no** partial
file (the temp file is removed on failure).

## Roles (from spec 002, no new identities)

- Any verified session → may read the shelf and covers.
- `RoleUser`/`RoleAdministrator` → may open the form and submit (with CSRF).
- `RoleViewer` → read-only: no form control, submissions refused (`403`).
- `BRAIND_READ_ONLY=true` → same as viewer for the create flow, enforced server-side
  regardless of role, plus the shelf hides the form link.
