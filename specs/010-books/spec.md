# Feature Specification: Book Reviews

**Feature Branch**: `010-books`

**Created**: 2026-10-30

**Status**: Draft

**Input**: User description: "Build a 'books' app linked from the landing page. The books page lists books I have read and has the ability to leave new book reviews; the list shows book covers. It reads all markdown files recursively in the vault's `01 - Note Box/Hobby/Books/` folder and populates book entries from them, using their metadata (author, cover, interest, status, finished date, rating) and body review."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Browse the bookshelf (Priority: P1)

A signed-in person follows the "Books" link from the landing page and sees their reading record in a calm, compact shelf: each responsive row leads with the cover and title, then shows available author, status, interest, rating, finished date, and review text, newest first. A breadcrumb at the top links back to the landing page.

**Why this priority**: The list is the product. Without it nothing else in this feature has value, and it is read-only against the vault, so it can ship alone.

**Independent Test**: Use a disposable data directory mirroring the real layout (nested book folders, a series sub-folder, non-book index notes, a book with missing cover, a book with malformed metadata); verify the page lists exactly the book notes with the fields from their own content, and excludes the non-book notes.

**Acceptance Scenarios**:

1. **Given** a data directory with book notes in nested folders, **When** an authorized person opens the books page, **Then** every book note appears exactly once regardless of folder depth.
2. **Given** a book note with a cover reference whose image exists among the vault's attachments, **When** the list renders, **Then** the cover image is displayed.
3. **Given** a book note whose cover is absent, missing, unreadable, empty, corrupt, or not a supported image, **When** the list renders, **Then** a designed fallback cover is shown instead of an empty tile or broken image.
4. **Given** a book note whose supported cover image is readable, **When** the list renders, **Then** the real cover is shown. The fallback has the same dimensions and an accessible description; it is bundled with the service and works offline.
5. **Given** folder index notes that carry no book metadata, **When** the list renders, **Then** they are not shown as books.
6. **Given** an anonymous visitor, **When** they request the books page, **Then** they are sent to sign in and no book data is exposed.
7. **Given** a signed-in visitor on the shelf, **When** they use the `brain` breadcrumb, **Then** they return to the landing page.
8. **Given** a shelf with books, **When** it is viewed on desktop or a narrow screen, **Then** each book remains a compact row with its cover beside its details, and the row's title, recorded metadata, review, and available edit action remain readable.
9. **Given** a shelf with books, **When** a visitor enters a search term, **Then** only entries matching a title, author, displayed metadata, or review text remain visible, without regard to letter case.
10. **Given** a shelf with books, **When** a visitor selects All books, Finished, or Reading, **Then** the shelf shows entries for that status; the displayed count changes to match, and an explanatory empty state appears when nothing matches.
11. **Given** a note whose body contains its cover embed, **When** the shelf renders the entry, **Then** the cover is represented by the adjacent cover image or fallback and the embed syntax is not repeated as review text.
12. **Given** a visitor moves between the landing page, Books shelf, review forms, and sign-in or notice pages, **When** each view renders, **Then** backgrounds, text, surfaces, links, buttons, and form controls use the same product palette.
13. **Given** the shelf search and status filters are visible, **When** the toolbar renders on desktop or mobile, **Then** the search field and filter buttons have a consistent height and share a common vertical center line.
14. **Given** a contributor opens the create or edit form, **When** the page renders, **Then** it uses the same breadcrumb and page heading style as the shelf, groups book details separately from review text, and keeps its save and cancel actions clear on desktop and mobile.

---

### User Story 2 - Write a new book review (Priority: P2)

A signed-in person with contributor rights opens the "Add a review" form, fills in title, author, rating, an optional cover filename or image upload, and the review text, and submits. The new book appears in the shelf, and the underlying Markdown note opens in Obsidian following the vault's existing book-note conventions.

**Why this priority**: Capture is the write half of the feature; it depends on the shelf existing but is second in value because reading the existing shelf works without it.

**Independent Test**: Submit the form against a disposable data directory; verify a new Markdown file appears in the configured Books folder with frontmatter (author, cover, status, finished date, rating) and a body carrying the title and review text, and that the shelf now includes it.

**Acceptance Scenarios**:

1. **Given** a signed-in contributor, **When** they submit the form with a title and review, **Then** a new book note is stored in the configured Books folder and the shelf shows it.
2. **Given** a submitted note, **When** it is opened as a plain Markdown file, **Then** its frontmatter and body match the existing book-note conventions (title heading, metadata, cover embed) so Obsidian renders it without migration.
3. **Given** a title or cover filename that would escape the Books folder, **When** submitted, **Then** the request is rejected and no file is written anywhere outside the Books folder.
4. **Given** a person without contributor rights (viewer), **When** they attempt to submit the form, **Then** the submission is refused.
5. **Given** the service running in read-only mode, **When** a contributor opens the shelf, **Then** no review form is offered and submissions are refused.
6. **Given** a contributor uploads a supported cover image, **When** they submit the form, **Then** the image is validated and stored in the configured attachments folder, and the saved note references its generated local filename.
7. **Given** a contributor opens the create form, **When** it first renders, **Then** the Finished field defaults to today's date.

---

### User Story 3 - See who reviewed what, at a glance (Priority: P3)

Each shelf entry also carries the interest level and status recorded on the note, so the shelf reads as a personal reading record rather than a bare list.

**Why this priority**: Polish over the P1 list; the shelf is useful without it.

**Independent Test**: Same fixture vault as US1; verify interest and status values recorded on notes are visible on their entries, and absent cleanly when not recorded.

**Acceptance Scenarios**:

1. **Given** a book note recording interest and status, **When** the list renders, **Then** both appear on its entry.
2. **Given** a book note omitting those fields, **When** the list renders, **Then** the entry shows nothing for them and stays aligned with other entries.

---

### User Story 4 - Edit an existing book review (Priority: P2)

A signed-in contributor or administrator opens an edit action on a shelf entry, changes its title, author, rating, cover, finished date, review, or series assignment, and saves. The selected note is updated and the shelf reflects the changes; changing the series assignment moves the note to its generated path.

**Why this priority**: Editing lets contributors correct mistakes in existing records while keeping the vault note as the source of truth.

**Independent Test**: Edit a nested book note and verify the same file remains, the changed fields appear on the shelf, unrelated frontmatter remains intact, and viewers cannot access or submit edits.

**Acceptance Scenarios**:

1. **Given** a writable shelf and a contributor, **When** they choose Edit on a book, **Then** a form opens prefilled with that note's title, author, rating, cover, finished date, review, and any series/volume derived from its path.
2. **Given** a valid edit to a nested book note without a series assignment change, **When** the contributor saves it, **Then** the same file is updated in place, unrelated frontmatter is preserved, and the shelf shows the changes.
3. **Given** a viewer or anonymous visitor, **When** they request or submit an edit, **Then** the request is refused.
4. **Given** read-only mode or an unsafe note path, **When** an edit is requested, **Then** editing is unavailable and no file outside the Books folder is changed.
5. **Given** a contributor uploads a replacement cover, **When** the edit is saved, **Then** the existing note is updated in place to reference the new local attachment; leaving the upload blank preserves the current cover.
6. **Given** an existing book has no finished date, **When** its edit form first renders, **Then** the Finished field defaults to today's date; an existing finished date remains prefilled.

---

### User Story 5 - Delete an existing book review (Priority: P2)

A signed-in contributor or administrator chooses to delete a review and sees a confirmation page identifying the book and explaining that its Markdown note will be permanently removed. The note is deleted only after they explicitly confirm; cancelling leaves the review unchanged.

**Why this priority**: Contributors need to remove unwanted or duplicate records while protecting the vault from accidental deletion.

**Independent Test**: Open the delete action for a fixture book, verify the confirmation page names the intended book, cancel and verify the note remains, then confirm deletion and verify it disappears from the shelf and its Markdown note is gone. Viewers, anonymous visitors, read-only mode, and unsafe paths cannot delete notes.

**Acceptance Scenarios**:

1. **Given** a writable shelf and a contributor, **When** they choose Delete review, **Then** a confirmation page identifies the selected book and offers explicit Delete review and Cancel actions; opening the page alone does not delete anything.
2. **Given** a contributor is on the confirmation page, **When** they cancel or navigate away, **Then** the note remains unchanged and visible on the shelf.
3. **Given** a contributor confirms deletion, **When** the request succeeds, **Then** the selected Markdown note is removed and no longer appears on the shelf.
4. **Given** a cover image may be shared with other notes, **When** its review is deleted, **Then** the attachment file is retained.
5. **Given** a viewer, anonymous visitor, or read-only service, **When** they request or submit a deletion, **Then** the request is refused and the note remains unchanged.
6. **Given** an unsafe path, symlink escape, missing note, or non-book note, **When** deletion is requested, **Then** the service refuses the request and does not remove any file.

---

### User Story 5 - See series membership on the shelf (Priority: P2)

A signed-in reader sees every book in the existing single, newest-finished-first shelf list. Each book that belongs to a series has an indicator inside its entry showing the series name and recognized volume number, when available. Series membership does not create separate lists or change the shelf's global order.

**Why this priority**: Series context should be visible while preserving the compact shelf and its existing browsing order.

**Independent Test**: Create a fixture vault with a series overview note, numbered books using zero-padded and unpadded filenames, a fractional volume, and standalone books with conflicting finished dates. Verify overview notes remain excluded, all books appear once in one list ordered newest-finished-first, and series entries show the series name and parsed volume.

**Acceptance Scenarios**:

1. **Given** book notes beneath a folder inside the Books folder, **When** the shelf renders, **Then** each appears in the same list as standalone books with a series indicator in its entry.
2. **Given** a series filename with a recognized integer or decimal position, optionally labeled `Volume` or `Book`, **When** the shelf renders, **Then** the entry shows the series folder name and parsed volume position.
3. **Given** books with different finished dates, including members of the same series, **When** the shelf renders, **Then** every entry follows the existing global newest-finished-first order.
4. **Given** a series folder with an overview note that has no book metadata, **When** the shelf renders, **Then** the overview is not shown as a book or as a numbered volume.
5. **Given** a series member whose filename has no recognized number, **When** the shelf renders, **Then** its entry still identifies the series and omits the volume position.

---

### User Story 6 - Assign a book to a series (Priority: P2)

A contributor creates or edits a review and can assign it to a series and provide an optional volume number. The service files series books into a folder named after the series and generates a filename containing its numeric position. Editing an existing review can move it into, between, or out of a series while preserving the note's unrelated metadata.

**Why this priority**: Readers can keep the vault's series organization consistent when recording new books without manually moving and renaming notes.

**Independent Test**: Create a series review with volume 8.5 and verify it is stored under the series folder with an orderable filename. Edit a standalone review into that series, change its volume, then remove its series assignment and verify the note moves safely at each step without overwriting another note.

**Acceptance Scenarios**:

1. **Given** a new review with a series and volume number, **When** it is saved, **Then** the service creates or uses a matching first-level folder and stores the note with the series name, a two-digit minimum numeric position, and a title slug in its filename.
2. **Given** a new review with no series, **When** it is saved, **Then** it uses the existing standalone filename and Books folder root.
3. **Given** a volume number with no series, **When** the form is submitted, **Then** validation explains that a series is required and preserves the submitted values.
4. **Given** an existing review, **When** it is edited without changing its series or volume assignment, **Then** its existing path and filename are retained.
5. **Given** an existing review, **When** its series or volume assignment changes, **Then** the note moves to the generated destination while preserving unrelated frontmatter and never overwriting another note.
6. **Given** an existing series review, **When** its series is cleared, **Then** it moves to the Books folder root and uses a generated standalone filename.
7. **Given** a series entry without a recognized volume number, **When** it is edited without changing its assignment, **Then** it remains editable and its existing filename is preserved.

---

### Edge Cases

- Malformed or unreadable metadata on one note must not blank the shelf: the entry is skipped or shown with its filename as title, and other books still render.
- Two notes with the same title (or a submitted title colliding with an existing filename): both appear in the shelf; a new review never overwrites an existing note (a distinct filename is chosen).
- Very large review bodies or absurdly deep folder trees: bounded, with a clear refusal rather than unbounded memory use.
- Cover filenames that reference other folders, absolute paths, or `..` traversal: refused or ignored, never served.
- Attachments with an image extension but invalid image data, no read permission, or zero bytes are treated as missing covers; a fallback cover is rendered.
- The Books folder is missing or empty: the shelf renders an "no books yet" state, not an error.
- Notes whose frontmatter is valid YAML but not a mapping (list or scalar): skipped as non-books.
- Image attachments that are not images (wrong extension) or unreadable: treated as missing covers.
- Editing a nested note must retain its relative path unless its series or volume assignment changes; traversal, absolute paths, symlinks, and non-book notes are refused.
- Editing must preserve unrelated frontmatter and must not create a duplicate note.
- Series membership comes from the first folder beneath the Books folder; its name is the displayed series name. The displayed volume position comes from the filename, not the Markdown heading or finished date.
- Series names supplied through the form are single folder names; slash, backslash, control characters, dot, and dot-dot values are rejected.
- Series and volume assignment are derived from the folder and filename. They are not duplicated in YAML frontmatter.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The landing page MUST offer a link to the books shelf.
- **FR-002**: The shelf MUST list every book note found by recursively scanning the configured Books folder beneath the service's data directory.
- **FR-003**: A note counts as a book when it carries book metadata (a status, rating, or cover field in its frontmatter); notes without any such metadata (for example folder index notes) MUST NOT be listed as books.
- **FR-004**: Each shelf entry MUST show title, review text, and any recorded author, rating, finished date, interest, and status; missing values MUST degrade gracefully.
- **FR-005**: Each shelf entry MUST show its cover image resolved by filename from the configured attachments folder when the attachment is a readable, non-empty supported image. When a cover is absent, missing, unreadable, empty, corrupt, or not a supported image, the shelf MUST show a designed fallback cover instead of an empty tile or broken image. The fallback MUST have the same cover dimensions, an accessible text alternative, and be available offline as part of the service.
- **FR-006**: All books MUST appear in one shelf list ordered newest-first by finished date, with undated books after dated ones, then by title. Series membership and volume position MUST be shown inside each applicable entry and MUST NOT split or reorder the list.
- **FR-007**: Signed-in users with contributor or administrator rights MUST be able to submit a review form with a required title and review, plus optional author, rating (`n/5`, halves allowed), cover filename, series name, and volume number.
- **FR-008**: A successful submission MUST durably create one new Markdown note at its generated standalone or series path whose frontmatter and body match the vault's existing book-note conventions (metadata block, title heading, review text, cover embed), MUST NOT overwrite any existing note, and MUST then appear on the shelf.
- **FR-009**: The shelf MUST require a signed-in session; the review form MUST require contributor or administrator rights; read-only mode MUST disable creation (no form, submissions refused).
- **FR-010**: The service MUST reject unsafe inputs: cover names resolving outside the attachments folder, titles or series names that would escape the Books folder, invalid volume numbers, review text beyond the configured per-note size limit, and control characters in titles or series names.
- **FR-011**: A single malformed note MUST NOT prevent the remaining books from being listed.
- **FR-012**: Serving cover images MUST NOT expose any file outside the configured attachments folder.
- **FR-013**: In writable mode, the shelf MUST offer an edit action to contributor and administrator sessions.
- **FR-014**: The edit form MUST be prefilled with the selected note's title, author, rating, cover, finished date, review, and any series and volume derived from its path and filename.
- **FR-015**: A successful edit MUST update the selected Markdown note, preserve unrelated frontmatter, and MUST NOT create a duplicate note. The existing path MUST be retained when the series assignment is unchanged; series assignment changes follow FR-028.
- **FR-016**: Edits MUST apply the same field validation and per-note size limit as review creation.
- **FR-017**: The edit operation MUST refuse traversal, absolute paths, symlink escapes, and notes that are not book notes.
- **FR-018**: The edit form and submit route MUST require contributor or administrator rights; edit submissions MUST pass the existing CSRF check. Read-only mode MUST hide edit actions and refuse edit requests.
- **FR-019**: A book note inside a first-level folder beneath the configured Books folder MUST show that folder's name as a series indicator inside its shelf entry. Book notes in the Books folder root MUST have no series indicator. Series membership MUST NOT create a separate shelf list or heading.
- **FR-020**: The service MUST determine a series book's reading position from its filename when the filename starts with the series folder name followed by an integer or decimal position, optionally prefixed by `Volume` or `Book` and separated by whitespace or a hyphen. Parsing MUST be case-insensitive and accept leading zeroes.
- **FR-021**: A series entry MUST show its recognized numeric filename position alongside the series name, preserving fractional values (for example, `Series: Primal Hunter · Book 8.5`). If no position is recognized, the series name MUST still be shown without a volume label.
- **FR-022**: The shelf MUST render all books in a single list ordered by finished date newest-first, then undated entries, with a case-insensitive title tiebreak. Series membership and volume position MUST NOT affect this order. The shelf MUST NOT render series headings or sublists.
- **FR-023**: Folder overview notes that do not meet the existing book metadata rule MUST remain excluded from series membership and the shelf.
- **FR-024**: Create and edit forms MUST provide an optional series name and optional non-negative integer or decimal volume number. A volume number without a series MUST be rejected with a field-level validation message.
- **FR-025**: A series name MUST be a single safe folder name, limited to 100 characters; path separators, control characters, `.` and `..` MUST be rejected.
- **FR-026**: Creating a review with a series MUST store it in a first-level folder named for that series. When a volume number is supplied, the generated filename MUST begin with the series name and a two-digit minimum numeric position, followed by a title slug; decimal positions MUST be retained (for example, `Primal Hunter 08.5 - title.md`). Without a number, the filename MUST identify the series and title while remaining unnumbered.
- **FR-027**: Review forms MUST derive the series from the existing folder and the volume number from the filename. Saving an edit without changing that assignment MUST preserve the existing path and filename.
- **FR-028**: Changing an edit's series or volume assignment MUST move the note to its generated destination, preserving unrelated frontmatter. Clearing the series MUST move the note to a generated standalone filename at the Books folder root.
- **FR-029**: Create and edit operations MUST never overwrite another note when a generated filename collides; a deterministic available suffix MUST be used.

### Key Entities

- **Book Review Note**: A Markdown file in the Books folder holding one reviewed book: frontmatter metadata (author, cover filename, interest, status, finished date, rating) plus a body with the title heading, review prose, and a cover embed. The vault file is the single source of truth; the shelf is derived, never authoritative.
- **Shelf Entry**: One book's presentation on the shelf: resolved fields from its note plus the cover image bytes from the attachments folder.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: From the landing page, a signed-in person reaches the complete shelf in one click; a fixture shelf of 130 books in nested folders lists all 130, each exactly once.
- **SC-002**: A contributor can go from opening the form to seeing their new book on the shelf in under one minute, and the stored note opens in Obsidian with its metadata intact and no migration or repair needed.
- **SC-003**: Every book with a readable supported cover displays that cover; every book without one shows the bundled fallback cover, with zero broken-image indicators.
- **SC-004**: Anonymous visitors and viewer-role members cannot read the shelf or file a review; no book title or cover is disclosed to them.
- **SC-005**: A contributor can edit any nested shelf entry without creating a duplicate; its path remains stable when series assignment fields are unchanged.
- **SC-006**: Existing unrelated frontmatter remains intact after an edit; unauthorized and read-only edit requests never change a file.
- **SC-007**: The shelf renders all books in one list ordered newest-finished-first, and each series entry shows its series name and recognized volume position without a separate series list or heading.
- **SC-008**: Standalone books remain in the same shelf list without a series indicator; series overview notes remain absent from the shelf.
- **SC-009**: A review created with a series and fractional volume appears in the single shelf list with its series name and volume indicator, and its note is stored beneath the matching folder.
- **SC-010**: Editing a review's series assignment moves exactly one note to the generated destination, preserves unrelated frontmatter, and does not overwrite a collision.

## Assumptions

- The vault layout matches the live installation: book notes under `01 - Note Box/Hobby/Books/` and image attachments under `06 - Meta/Attachments/`, both beneath the service data directory; both folder paths are configuration, not hard-coded behavior.
- Existing note conventions are the contract for generated notes (frontmatter keys `author`, `cover`, `interest`, `status (Media)`, `finished`, `rating`; title as `#` heading; `![[cover.jpg]]` embed), validated against the provided examples.
- One submitted review creates one new book note; editing retains its path unless the series/volume assignment changes. Deleting notes through the web interface is out of scope.
- Filtering (by status, author, rating) and detail pages are out of scope for this feature; the single shelf list is newest-finished-first, and each series entry shows its series name and recognized volume position.
- The existing authentication and session system (spec 002) provides identity, roles, and CSRF protection; this feature adds no new authentication of its own.
- Newest-first means the recorded finished date; entries without a finished date sort after dated entries, alphabetically, regardless of series membership.
- The service's single-writer ownership of the data directory (spec 001) holds, so no concurrent-mutation handling beyond atomic writes is required.
- Editing changes title, author, rating, cover, finished date, review, and optional series assignment; status and interest remain sourced from existing note metadata.
