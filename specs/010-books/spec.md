# Feature Specification: Book Reviews

**Feature Branch**: `010-books`

**Created**: 2026-10-30

**Status**: Draft

**Input**: User description: "Build a 'books' app linked from the landing page. The books page lists books I have read and has the ability to leave new book reviews; the list shows book covers. It reads all markdown files recursively in the vault's `01 - Note Box/Hobby/Books/` folder and populates book entries from them, using their metadata (author, cover, interest, status, finished date, rating) and body review."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Browse the bookshelf (Priority: P1)

A signed-in person follows the "Books" link from the landing page and sees every book they have reviewed: each entry shows the book's cover, title, author, rating, and finished date, with the review text, newest first.

**Why this priority**: The list is the product. Without it nothing else in this feature has value, and it is read-only against the vault, so it can ship alone.

**Independent Test**: Use a disposable data directory mirroring the real layout (nested book folders, a series sub-folder, non-book index notes, a book with missing cover, a book with malformed metadata); verify the page lists exactly the book notes with the fields from their own content, and excludes the non-book notes.

**Acceptance Scenarios**:

1. **Given** a data directory with book notes in nested folders, **When** an authorized person opens the books page, **Then** every book note appears exactly once regardless of folder depth.
2. **Given** a book note with a cover reference whose image exists among the vault's attachments, **When** the list renders, **Then** the cover image is displayed.
3. **Given** a book note whose cover image does not exist, **When** the list renders, **Then** a neutral placeholder is shown instead of a broken image.
4. **Given** folder index notes that carry no book metadata, **When** the list renders, **Then** they are not shown as books.
5. **Given** an anonymous visitor, **When** they request the books page, **Then** they are sent to sign in and no book data is exposed.

---

### User Story 2 - Write a new book review (Priority: P2)

A signed-in person with contributor rights opens the "Add a review" form, fills in title, author, rating, an optional cover filename, and the review text, and submits. The new book appears in the shelf, and the underlying Markdown note opens in Obsidian following the vault's existing book-note conventions.

**Why this priority**: Capture is the write half of the feature; it depends on the shelf existing but is second in value because reading the existing shelf works without it.

**Independent Test**: Submit the form against a disposable data directory; verify a new Markdown file appears in the configured Books folder with frontmatter (author, cover, status, finished date, rating) and a body carrying the title and review text, and that the shelf now includes it.

**Acceptance Scenarios**:

1. **Given** a signed-in contributor, **When** they submit the form with a title and review, **Then** a new book note is stored in the configured Books folder and the shelf shows it.
2. **Given** a submitted note, **When** it is opened as a plain Markdown file, **Then** its frontmatter and body match the existing book-note conventions (title heading, metadata, cover embed) so Obsidian renders it without migration.
3. **Given** a title or cover filename that would escape the Books folder, **When** submitted, **Then** the request is rejected and no file is written anywhere outside the Books folder.
4. **Given** a person without contributor rights (viewer), **When** they attempt to submit the form, **Then** the submission is refused.
5. **Given** the service running in read-only mode, **When** a contributor opens the shelf, **Then** no review form is offered and submissions are refused.

---

### User Story 3 - See who reviewed what, at a glance (Priority: P3)

Each shelf entry also carries the interest level and status recorded on the note, so the shelf reads as a personal reading record rather than a bare list.

**Why this priority**: Polish over the P1 list; the shelf is useful without it.

**Independent Test**: Same fixture vault as US1; verify interest and status values recorded on notes are visible on their entries, and absent cleanly when not recorded.

**Acceptance Scenarios**:

1. **Given** a book note recording interest and status, **When** the list renders, **Then** both appear on its entry.
2. **Given** a book note omitting those fields, **When** the list renders, **Then** the entry shows nothing for them and stays aligned with other entries.

---

### Edge Cases

- Malformed or unreadable metadata on one note must not blank the shelf: the entry is skipped or shown with its filename as title, and other books still render.
- Two notes with the same title (or a submitted title colliding with an existing filename): both appear in the shelf; a new review never overwrites an existing note (a distinct filename is chosen).
- Very large review bodies or absurdly deep folder trees: bounded, with a clear refusal rather than unbounded memory use.
- Cover filenames that reference other folders, absolute paths, or `..` traversal: refused or ignored, never served.
- The Books folder is missing or empty: the shelf renders an "no books yet" state, not an error.
- Notes whose frontmatter is valid YAML but not a mapping (list or scalar): skipped as non-books.
- Image attachments that are not images (wrong extension) or unreadable: treated as missing covers.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The landing page MUST offer a link to the books shelf.
- **FR-002**: The shelf MUST list every book note found by recursively scanning the configured Books folder beneath the service's data directory.
- **FR-003**: A note counts as a book when it carries book metadata (a status, rating, or cover field in its frontmatter); notes without any such metadata (for example folder index notes) MUST NOT be listed as books.
- **FR-004**: Each shelf entry MUST show title, review text, and any recorded author, rating, finished date, interest, and status; missing values MUST degrade gracefully.
- **FR-005**: Each shelf entry MUST show the book's cover image resolved by filename from the configured attachments folder; when the image does not exist or is unreadable, a placeholder MUST be shown instead of a broken reference.
- **FR-006**: The shelf MUST be ordered newest-first by finished date, with undated books after dated ones, then by title.
- **FR-007**: Signed-in users with contributor or administrator rights MUST be able to submit a review form with a required title, an optional author, an optional rating (`n/5`, halves allowed), an optional cover filename, and the review text.
- **FR-008**: A successful submission MUST durably create one new Markdown note in the configured Books folder whose frontmatter and body match the vault's existing book-note conventions (metadata block, title heading, review text, cover embed), MUST NOT overwrite any existing note, and MUST then appear on the shelf.
- **FR-009**: The shelf MUST require a signed-in session; the review form MUST require contributor or administrator rights; read-only mode MUST disable creation (no form, submissions refused).
- **FR-010**: The service MUST reject unsafe inputs: cover names resolving outside the attachments folder, titles that would escape the Books folder, review text beyond the configured per-note size limit, and control characters in titles.
- **FR-011**: A single malformed note MUST NOT prevent the remaining books from being listed.
- **FR-012**: Serving cover images MUST NOT expose any file outside the configured attachments folder.

### Key Entities

- **Book Review Note**: A Markdown file in the Books folder holding one reviewed book: frontmatter metadata (author, cover filename, interest, status, finished date, rating) plus a body with the title heading, review prose, and a cover embed. The vault file is the single source of truth; the shelf is derived, never authoritative.
- **Shelf Entry**: One book's presentation on the shelf: resolved fields from its note plus the cover image bytes from the attachments folder.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: From the landing page, a signed-in person reaches the complete shelf in one click; a fixture shelf of 130 books in nested folders lists all 130, each exactly once.
- **SC-002**: A contributor can go from opening the form to seeing their new book on the shelf in under one minute, and the stored note opens in Obsidian with its metadata intact and no migration or repair needed.
- **SC-003**: 100% of books whose cover image exists in the attachments folder display their cover; every book without a resolvable cover shows the placeholder (no broken-image affordances).
- **SC-004**: Anonymous visitors and viewer-role members cannot read the shelf or file a review; no book title or cover is disclosed to them.

## Assumptions

- The vault layout matches the live installation: book notes under `01 - Note Box/Hobby/Books/` and image attachments under `06 - Meta/Attachments/`, both beneath the service data directory; both folder paths are configuration, not hard-coded behavior.
- Existing note conventions are the contract for generated notes (frontmatter keys `author`, `cover`, `interest`, `status (Media)`, `finished`, `rating`; title as `#` heading; `![[cover.jpg]]` embed), validated against the provided examples.
- One submitted review creates one new book note; editing or deleting notes through the web interface is out of scope (future features).
- Filtering (by status, author, rating) and detail pages are out of scope for this feature; the shelf shows everything newest-first.
- The existing authentication and session system (spec 002) provides identity, roles, and CSRF protection; this feature adds no new authentication of its own.
- "Newest first" means the recorded finished date; books with no finished date sort after dated books, alphabetically.
- The service's single-writer ownership of the data directory (spec 001) holds, so no concurrent-modulation handling beyond atomic writes is required.
