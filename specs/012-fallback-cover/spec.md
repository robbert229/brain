# Feature Specification: Fallback Book Covers

**Feature Branch**: `012-fallback-covers`

**Status**: Draft

**Input**: Improve the books shelf so books without an available cover still have a designed fallback cover.

## User Scenarios & Testing

### User Story 1 - See a complete bookshelf (Priority: P1)

A signed-in person browsing the shelf sees a consistent cover tile for every book. Existing readable covers appear as before; books with no cover, a missing cover, an unreadable cover, or a non-image attachment show a clearly designed fallback cover instead of an empty block or broken image.

**Independent Test**: Browse a fixture shelf containing a readable image, no cover metadata, a missing image, and an unreadable or invalid image; verify real covers display and all other entries display the fallback without broken-image indicators.

## Requirements

### Functional Requirements

- **FR-001**: The shelf MUST display the configured image for a book when the attachment is a readable supported image.
- **FR-002**: When a cover is absent, missing, unreadable, empty, or not a supported image, the shelf MUST display a designed fallback cover.
- **FR-003**: The fallback MUST retain the same dimensions and layout as a real cover, identify itself accessibly as a missing cover, and MUST NOT expose vault paths or attachment names.
- **FR-004**: The fallback MUST be available offline as part of the application and MUST NOT depend on an external service.
- **FR-005**: Cover lookup and serving MUST remain confined to the configured attachments folder.

## Success Criteria

- **SC-001**: Every shelf entry has a cover visual, with zero broken-image indicators for absent or unreadable attachments.
- **SC-002**: The shelf remains usable without network access to third-party services.
- **SC-003**: The fallback renders at the same cover dimensions as configured book covers and has an accessible text alternative.

## Assumptions

- A single branded fallback cover is sufficient; per-book generated artwork is out of scope.
- Existing cover images and book-note data remain unchanged.
