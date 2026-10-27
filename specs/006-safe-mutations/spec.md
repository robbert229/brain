# Feature Specification: Safe Note Mutations

**Feature Branch**: `006-safe-mutations`

**Created**: 2026-10-26

**Status**: Draft; later milestone

**Input**: User description: Capture the later milestone for creating, editing, renaming, and trashing notes without silently overwriting external or synchronized changes.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Create or edit a note safely (Priority: P1)

An authorized user creates a note or edits an existing note and receives a clear conflict if the file changed since it was read.

**Why this priority**: Safe writing is the core value of the mutation milestone; silent data loss is unacceptable.

**Independent Test**: Submit create and update operations against a fixture vault, including concurrent external changes and interrupted writes.

**Acceptance Scenarios**:

1. **Given** an authorized user creates a new valid note at an unused path, **When** the save completes, **Then** the note is available in the vault and survives process restart.
2. **Given** an authorized user edits a note with its current revision, **When** the save completes, **Then** the replacement is complete and the updated note can be read.
3. **Given** the note changed after the user read it, **When** the user submits a stale revision, **Then** the service preserves both versions and presents an explicit conflict without overwriting the newer content.
4. **Given** an unauthenticated person or viewer attempts a mutation, **When** the request is evaluated, **Then** it is denied.

### User Story 2 - Rename or move a note to trash (Priority: P2)

An authorized user renames or moves a note while preserving its content and has a recoverable trash outcome for deletion.

**Why this priority**: Organization and removal are common note-management needs but depend on safe write primitives.

**Independent Test**: Exercise rename collisions, path boundaries, trash behavior, and failure recovery using a temporary vault.

**Acceptance Scenarios**:

1. **Given** a valid source and unused destination within the vault, **When** the user renames a note, **Then** the destination contains the original content and the old path no longer does.
2. **Given** a rename would escape the vault or overwrite an existing note, **When** it is requested, **Then** it is rejected without changing either file.
3. **Given** a user trashes a note, **When** the operation succeeds, **Then** the note is moved to recoverable system trash with enough information to identify its original location.
4. **Given** trash storage is unavailable, **When** deletion is requested, **Then** the service reports failure and does not permanently unlink the note.

### Edge Cases

- External Sync changes a note between revision check and write.
- The process stops during a write or move.
- Two users create or rename to the same path concurrently.
- A path is a symlink, collides by case on the target filesystem, or crosses directories.
- The user lacks authorization or the vault enters explicit read-only mode.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Create and update operations MUST validate logical paths, content size, and authorization.
- **FR-002**: Updating an existing note MUST require a current revision precondition; stale updates MUST produce an explicit conflict and preserve current content.
- **FR-003**: A successful create, update, or rename MUST be durable enough to survive process restart.
- **FR-004**: Failed writes MUST leave the prior file intact.
- **FR-005**: Browser forms MUST remain usable without JavaScript and MUST provide clear validation and conflict outcomes.
- **FR-006**: JSON mutation operations MUST use the same validation, authorization, and concurrency rules as browser actions.
- **FR-007**: Rename MUST stay within the vault and MUST reject collisions rather than replace unrelated content.
- **FR-008**: Delete MUST move notes to the configured recoverable system trash; permanent unlink is not an allowed fallback.
- **FR-009**: Mutations MUST be denied when the vault is read-only, unavailable, or its required sync policy has drifted.
- **FR-010**: Mutation responses and logs MUST NOT disclose unrelated note contents or credentials.

### Key Entities *(include if feature involves data)*

- **Note revision**: A token representing the content version a user viewed.
- **Mutation request**: A validated create, update, rename, or trash operation and its authorization context.
- **Conflict**: A stale or colliding operation that requires the user to review current content.
- **Trash record**: A recoverable removed note and its original location metadata.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All successful create/update/rename test cases preserve complete expected content across restart.
- **SC-002**: All stale-revision and collision scenarios return an explicit conflict and preserve the newer or pre-existing content.
- **SC-003**: All tested unauthorized and read-only mutation attempts leave the vault unchanged.
- **SC-004**: Every successful delete test is recoverable from system trash; no failure path permanently unlinks a note.
- **SC-005**: The same tested mutation inputs produce equivalent business outcomes through browser and JSON surfaces.

## Assumptions

- Mutation is a separate later milestone after read-only browsing, authentication, and the required synchronization safeguards.
- Only standard Markdown notes are mutable in the initial scope; attachments and collaborative real-time editing are excluded.
- Conflict handling preserves user content and asks for an explicit follow-up action; automatic merge is not assumed.

## Source Traceability

- [ADR-0001](../../docs/adr/0001-markdown-vault-is-canonical.md), [ADR-0004](../../docs/adr/0004-html-and-api-share-behavior.md), [roadmap](../../docs/roadmap.md).
