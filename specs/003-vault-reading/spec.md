# Feature Specification: Read-Only Vault Browser and Search

**Feature Branch**: `003-vault-reading`

**Created**: 2026-10-26

**Status**: Draft

**Input**: User description: Capture the first user-facing vault milestone: authorized people can browse, search, and read Markdown notes through a browser and versioned JSON interface while the local filesystem remains authoritative.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Find and read a note (Priority: P1)

An authorized person browses vault folders, opens a note, and can read both a safe rendered view and its Markdown source.

**Why this priority**: Reading the existing vault is the first useful product outcome and does not require mutation behavior.

**Independent Test**: Use a disposable vault containing nested folders, ordinary notes, malformed Markdown, and unsafe path inputs; verify browser results without requiring Sync or backup.

**Acceptance Scenarios**:

1. **Given** an accessible vault and an authorized person, **When** they browse a folder and open a note, **Then** only current filesystem content is shown.
2. **Given** a note with active or unsafe markup, **When** its rendered view is opened, **Then** unsafe content is not executable and the plain source remains available.
3. **Given** a path that is absolute, traverses outside the vault, or reaches an external symlink target, **When** it is requested, **Then** the request is rejected without exposing outside files.

### User Story 2 - Search the current vault (Priority: P2)

An authorized person searches note content and receives bounded, relevant matches that reflect current files.

**Why this priority**: Search makes a personal vault useful as it grows while avoiding stale canonical indexes.

**Independent Test**: Search a fixture vault with expected matches, large files, inaccessible entries, and configured resource limits.

**Acceptance Scenarios**:

1. **Given** a valid query, **When** search completes, **Then** results identify matching notes and useful excerpts from the current vault.
2. **Given** a query exceeds an input or scan limit, **When** search runs, **Then** it stops safely and reports whether results were limited.
3. **Given** a note changes outside the service, **When** it is read or searched afterward, **Then** results reflect the current filesystem state.

### User Story 3 - Use browser and programmatic read access (Priority: P3)

An authorized person uses either ordinary browser navigation or the versioned read interface to retrieve the same vault information.

**Why this priority**: The two surfaces support human use and automation without creating separate business rules.

**Independent Test**: Request the same note, search, and status information through both surfaces and compare content, limits, and authorization decisions.

**Acceptance Scenarios**:

1. **Given** an authorized request, **When** the same read operation is made through HTML and the versioned JSON interface, **Then** both represent the same current data.
2. **Given** JavaScript is unavailable, **When** a person follows links or submits a search, **Then** core read flows still work.

### Edge Cases

- A file disappears, changes, or becomes unreadable between listing and opening.
- The vault contains symlinks, unusual names, non-Markdown files, or very large files.
- Search reaches a file, byte, time, or result ceiling before completion.
- The requested path does not name a note or is outside the configured vault.
- Markdown contains raw HTML, unsafe links, or unsupported Obsidian extensions.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST list directories and read Markdown notes from the configured vault.
- **FR-002**: The service MUST treat the configured vault filesystem as the current source of truth and MUST NOT require a persistent note index.
- **FR-003**: The service MUST reject paths that escape the vault or expose targets outside it.
- **FR-004**: The service MUST enforce configurable search ceilings. Defaults MUST cap each search at 10,000 Markdown files, 256 MiB read, 10 seconds elapsed, and 100 matches; if a ceiling stops search before exhaustion, available results MUST be identified as truncated.
- **FR-005**: The service MUST provide a safe rendered note view and access to the plain Markdown source.
- **FR-006**: The service MUST provide server-rendered, URL-addressable read flows that remain usable without JavaScript.
- **FR-007**: The service MUST provide versioned JSON read operations for status, notes, and search with outcomes consistent with browser behavior.
- **FR-008**: The service MUST authorize every read operation and MUST NOT expose arbitrary filesystem browsing or attachment serving in this milestone.
- **FR-009**: Logs and error responses MUST NOT include complete note contents or credentials.

### Key Entities *(include if feature involves data)*

- **Vault path**: A user-facing logical path resolved only within the configured vault.
- **Note**: A Markdown file with source content and derived display metadata.
- **Search query**: A bounded request for matching current note content.
- **Search result**: A note reference, excerpt, and indication of whether the search was limited.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Authorized users can complete folder-to-note reading in both browser and JSON scenarios without changing vault contents.
- **SC-002**: All tested traversal and external-symlink inputs are rejected, and no file outside the vault is returned.
- **SC-003**: Search returns expected matches from the current fixture within the configured limits (initial defaults: 10,000 files, 256 MiB, 10 seconds, 100 matches) and reports every limit-triggered truncation.
- **SC-004**: In every tested read scenario, browser and JSON surfaces agree on note content, result limits, and access outcome.
- **SC-005**: Rendered test notes containing unsafe markup execute zero active content.

## Assumptions

- This milestone is read/search-only; attachment serving, editing, and plugin-specific rendering are excluded.
- Standard Markdown is rendered safely; the service does not promise pixel-accurate Obsidian desktop rendering.
- Search is on demand against current files and has no authoritative background index.
- The existing configuration defaults are the initial product limits; the representative-vault measurement must confirm they are adequate before the reader milestone is accepted.
- Authentication and role policy are supplied by the OIDC access feature.

## Source Traceability

- [ADR-0001](../../docs/adr/0001-markdown-vault-is-canonical.md), [ADR-0003](../../docs/adr/0003-readiness-is-local.md), [ADR-0004](../../docs/adr/0004-html-and-api-share-behavior.md), roadmap first usable milestone.
