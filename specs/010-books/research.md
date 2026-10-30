# Phase 0 Research: Book Reviews

**Feature**: `specs/010-books/spec.md` | **Date**: 2026-10-30

No NEEDS CLARIFICATION markers remain after this phase; the four product questions
(storage model, cover location, YAML dependency, ceremony) were resolved with the
user during specification.

## R1 — YAML frontmatter parsing library

- **Decision**: Add `gopkg.in/yaml.v3` and parse the frontmatter block with
  `yaml.Unmarshal` into a `map[string]any` (then normalize fields explicitly).
- **Rationale**: The frontmatter is genuine YAML (block sequences for `tags`, the
  quoted-ish `status (Media)` key with parentheses, emoji values, dates that YAML
  would coerce into `time.Time`). Hand-rolling risks silent mis-parses of exactly the
  corpus we must render. `yaml.v3` is the standard Go choice, pure Go, no transitive
  dependencies, and the constitution/ADR practice already accepts established libraries
  for pure mechanics (ADR-0010 precedent for `golang.org/x/oauth2`).
- **Alternatives considered**: hand-rolled line parser (rejected: YAML coercion edge
  cases — `finished: 2025-05-31` decodes to `time.Time`, `rating: 4.5/5` stays a
  string, but `rating: 5/5` unquoted also stays a string only because of the slash —
  too many traps to own); `gopkg.in/yaml.v2` (rejected: v3 is current, better
  `map[string]any` decoding); a full Obsidian/frontmatter library (rejected: none is
  well-established; Principle III).
- **Consequence**: new external dependency in `go.mod` → requires ADR-0012 before merge.

## R2 — Frontmatter split and book detection

- **Decision**: A file is a candidate only if it begins with a `---` line followed by a
  closing `---` line (frontmatter block). Parse the block as YAML; it must decode to a
  mapping. The note is a **book** iff the mapping contains at least one of `cover`,
  `rating`, or `status (Media)`. Everything else (Waypoint/`Books.md` index notes,
  series-folder notes carrying only `tags`, non-mapping YAML, unreadable files) is
  **skipped**, counted, and surfaced on the shelf as a small "N notes skipped" footer.
- **Rationale**: In the 130-file corpus, every real book carries `status (Media)` and
  a `cover`; the only non-books are index notes with nothing but `tags`. Requiring a
  book marker implements FR-003 exactly. Skipping-and-counting rather than
  silently-dropping satisfies FR-011 (one bad note never blanks the shelf) while
  keeping data loss visible.
- **Alternatives considered**: treat any `.md` under the Books folder as a book
  (rejected: renders Waypoint notes as books); show malformed notes with filename as
  title (spec permits it, but a half-parsed entry invites editing the wrong file —
  count-and-skip is unambiguous and still visible); parse `![[embed]]` in the body as
  a book marker (rejected: adds a body-scanning rule for no observed cases).

## R3 — Field normalization (leniency contract)

- **Decision**: `books.Book` holds normalized display values; every field is optional:
  - `title`: first `# ` heading in the body, else the filename base (sans `.md`).
  - `review`: body text after the title heading, trimmed (rendered as plain paragraphs).
  - `author`: string as-is.
  - `finished`: accept YAML `time.Time` (date coercion) or strings `2006-01-02` /
    `2006-1-2`; invalid → zero (sorts undated).
  - `rating`: accept `4.5/5`, `5/5`, bare number scalars (YAML may decode `4.5`);
    parse as `num/den` or float; out of range → 0 with the raw text preserved for display.
  - `cover`: single path segment basename check at parse time (reject `..`, separators,
    absolute); invalid → treated as absent.
  - `interest`, `status`: trimmed strings (`🔥🔥`, `Finished`).
- **Rationale**: FR-004 demands graceful degradation per field; normalizing at parse
  time keeps templates and handlers free of business rules (Principle II) and makes
  every leniency rule table-testable.
- **Alternatives considered**: strict typed frontmatter struct with `yaml` tags
  (rejected: one odd value would fail the whole note); rendering review bodies as
  Markdown (rejected for v1: needs a sanitizer/dep; plain text paragraphs are honest
  and safe — recorded as a known limitation, future feature).

## R4 — Cover resolution and serving

- **Decision**: Resolve covers by filename only inside the configured covers folder
  (`<data>/06 - Meta/Attachments` by default). `GET /books/covers/{filename}` uses
  `os.DirFS(coversDir)` + `http.ServeFileFS` with the path parameter rejected unless it
  is a single clean path segment with an image extension (`.jpg .jpeg .png .webp .gif`);
  content type from the extension; `Last-Modified`/304 come free from `ServeFileFS`;
  `Cache-Control: private, max-age=3600`. Missing/unreadable → 404. Templates render
  the card's `<img>` with an `onerror`-free fallback: when the catalog could not stat a
  cover at scan time, it renders a neutral placeholder tile instead of an `<img>` at all
  (no broken-image affordance, no request).
- **Rationale**: The user confirmed attachments live in `06 - Meta/Attachments/`, so a
  vault-wide filename index is unnecessary (simpler — Principle III). `DirFS`+`ServeFileFS`
  gives traversal containment in one stdlib pair; the stat-at-scan check implements
  SC-003's "no broken images". FR-012 is satisfied structurally: the handler cannot
  name a file outside the covers folder.
- **Alternatives considered**: recursive vault-wide cover index built at startup
  (rejected: extra moving part for a layout we were told about); serving covers
  embedded-in-note `![[...]]` targets from arbitrary vault paths (rejected: FR-012);
  `onerror` JS placeholder (rejected: Principle IV prefers no-JS correctness).

## R5 — Safe note creation (no overwrite, atomic, in-folder)

- **Decision**: `writer.CreateReview(dataDir, booksDir, draft, maxBytes)`:
  1. Validate draft (title and review required; title ≤200 runes, no control chars;
     rating matches `0–5` in halves or empty; optional `finished` date `YYYY-MM-DD`;
     cover is single-segment image filename or empty).
  2. Slugify the title (N/A → lowercase ASCII words joined by `-`; empty slug falls
     back to `book`) and pick `slug.md`, `slug-2.md`, … — the first name that does not
     exist.
  3. Render the note (frontmatter `tags: [Hobby/Books]`, `type: Zettel`, then `author`,
     `cover`, `interest`-free, `status (Media): Finished`, `finished`, `rating`; then
     `# title`, review paragraphs, `![[cover]]` embed) and enforce `maxBytes`.
  4. Create atomically: `O_CREATE|O_EXCL|O_WRONLY` temp file in the Books folder,
     write + `File.Sync`, `os.Rename` onto the target, `Sync` on the Books folder.
     `O_EXCL` retry on the (vanishingly rare) race between step 2 and 4 by bumping the
     suffix.
- **Rationale**: FR-008's "never overwrite" is guaranteed by `O_EXCL` at the kernel,
  not by a check-then-write. The temp+rename+fsync pattern is the same durability
  posture spec 001 requires for the data directory. Reusing the Obsidian conventions
  observed in all 130 notes keeps Principle V ("openable without migration") literally
  true.
- **Alternatives considered**: write `status (Media):` from a form choice (rejected:
  every sample is `Finished`; hard-coding it matches how the user reviews *after*
  reading, and future edit features can change it); writing to a user-chosen
  subfolder (rejected: keeps filename-safety surface small — reviews always land in
  the Books folder root); appending to existing notes (rejected pre-spec by the user).

## R6 — Scan strategy: on-demand vs cached

- **Decision**: Scan the Books folder on every `GET /books`. No cache, no watcher, no
  TTL. `fs.WalkDir` with a bounded visit budget (files ≤2000, total bytes ≤`Search.MaxBytes`
  class bound via 2 MiB × budget cap); on budget exhaustion the shelf renders what it
  has plus a "showing first N" notice.
- **Rationale**: 130 notes ≈ a few ms of IO; a cache would violate Principle III until
  a demonstrated need and would fight Principle V (derived data must be recomputable —
  easiest when it *is* recomputed). Obsidian edits on the PVC become visible on next
  page load for free.
- **Alternatives considered**: mtime-keyed in-memory index (rejected: premature);
  scan at startup only (rejected: stale after Obsidian edits).

## R7 — Configuration surface

- **Decision**: Two new `config` keys, both relative sub-paths of `BRAIND_DATA_PATH`:
  `BRAIND_BOOKS_FOLDER` (default `01 - Note Box/Hobby/Books`) and
  `BRAIND_COVERS_FOLDER` (default `06 - Meta/Attachments`). Validation mirrors the
  existing `isPathWithin` style: must be relative, must contain no `..` segments, may
  not be absolute. The missing folder is not a config error (shelf renders the empty
  state; FR: "no books yet").
- **Rationale**: Spec Assumption says both paths are configuration, not hard-coded
  behavior; anchoring at the data dir (not `BRAIND_VAULT_PATH`) matches the live PVC
  layout where `01 - Note Box` and `06 - Meta` sit directly under `/data`.
- **Alternatives considered**: reuse `BRAIND_VAULT_PATH` as the anchor (rejected: on
  the live install `/data/vault` does not contain the Obsidian folders); absolute-path
  envs (rejected: would escape the data-lock's protected root, FR-010/FR-012).

## R8 — Authz and read-only posture

- **Decision**: Reuse the spec 002 surface exactly as `landing.go` does. Shelf
  (`GET /books`, `GET /books/covers/…`) requires any verified session (`RequireAuth`,
  all roles — the shelf is read like the landing page). The form page and `POST /books`
  additionally require `Role.Meets(RoleUser)` and a valid CSRF token; when
  `BRAIND_READ_ONLY` is set, the shelf hides the "Add a review" control and `POST
  /books` returns `403` with the standard failure page.
- **Rationale**: FR-009 maps 1:1 onto existing middleware; viewer denial is the
  existing role check, not a new rule.
- **Alternatives considered**: administrator-only reviews (rejected: `user` is the
  contributor role throughout the app); returning `405` in read-only mode (rejected:
  the route exists and is understood — `403` communicates "policy, not absence").
