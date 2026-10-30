# Feature Specification: Edit Book Reviews

**Feature Branch**: `011-edit-books`

**Status**: Draft

**Input**: Allow contributors to correct and update book notes already displayed on the shelf.

## User Scenarios & Testing

### User Story 1 - Edit an existing book (Priority: P1)

A signed-in contributor or administrator opens an edit action on a shelf entry, updates its title, author, rating, cover, finished date, or review, and saves. The same note is updated in place and the shelf reflects the changes.

**Independent Test**: Edit a nested book note and verify the same file remains, edited fields appear on the shelf, unrelated frontmatter remains intact, and a viewer or anonymous visitor cannot edit it.

## Requirements

### Functional Requirements

- **FR-001**: The shelf MUST offer an edit action to contributors and administrators when the service is writable.
- **FR-002**: The edit form MUST be prefilled with the selected note's title, author, rating, cover, finished date, and review.
- **FR-003**: A successful edit MUST update the selected Markdown note in place and the updated note MUST appear on the shelf.
- **FR-004**: Editing MUST preserve unrelated frontmatter fields and MUST NOT create a duplicate note or change the note's folder.
- **FR-005**: The service MUST reject invalid fields using the same constraints as review creation and MUST bound the resulting note by the configured per-note size limit.
- **FR-006**: Only contributor and administrator sessions may view or submit edits; CSRF protection MUST apply.
- **FR-007**: Read-only mode MUST hide edit actions and refuse edit requests.
- **FR-008**: The selected note identifier MUST be confined to the configured Books folder; traversal, absolute paths, symlinks, and non-book notes MUST be refused.

## Success Criteria

- **SC-001**: A contributor can update any displayed nested book note without creating another note.
- **SC-002**: Existing unrelated note metadata remains present after an edit.
- **SC-003**: Anonymous users and viewers cannot access or submit the edit flow; read-only mode offers no edit action.

## Assumptions

- Editing supports the same user-editable fields as review creation; status and interest remain sourced from the existing note.
- The vault remains the single source of truth, and edit operations target a specific existing note rather than renaming or moving it.
