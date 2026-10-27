# Feature Specification: Landing Page for the Brain Web Interface

**Feature Branch**: `009-landing-page`

**Created**: 2026-10-26

**Status**: Draft

**Input**: User description: Provide a landing page for the main brain web interface. This landing page should render a pleasant well designed landing page for brain. It should give a brief description of what brain is, "a second brain, powered by markdown", and state that the web interface is under construction. It should be powered by HTMX for the frontend, and that HTMX should be served by the main braind service.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A first-time visitor learns what brain is (Priority: P1)

A first-time visitor reaches the main web address of brain and immediately understands what the product is — "a second brain, powered by markdown" — and that the web interface is being built and is not yet available.

**Why this priority**: Communicating the product's purpose and current status is the entire job of a landing page; without it the main web address is a dead end.

**Independent Test**: Request the service's root web address and verify the product name, the tagline, and the under-construction notice are all present and rendered in a browser; no other feature is required.

**Acceptance Scenarios**:

1. **Given** the service is running, **When** a first-time visitor loads the root web address, **Then** the page displays the product name and the tagline "a second brain, powered by markdown".
2. **Given** the page has loaded, **When** the visitor reads it, **Then** it clearly states that the web interface is under construction.
3. **Given** the page is displayed, **When** the layout is inspected, **Then** the design is pleasant and no text is clipped, overlapping, or missing at the visitor's viewport.

### User Story 2 - The page works for every visitor (Priority: P2)

The visitor can read the page on any common device or browser, including when client-side scripts do not run, because all meaningful content is delivered with the initial page.

**Why this priority**: The project requires core flows to function without JavaScript, and a pleasant landing page must not exclude mobile, assistive-technology, or no-script visitors.

**Independent Test**: Render the page with scripting disabled and at representative mobile, tablet, and desktop widths; confirm all content remains visible and legible.

**Acceptance Scenarios**:

1. **Given** client-side scripting is disabled, **When** the visitor loads the root web address, **Then** the full description, tagline, and construction notice still render and are readable.
2. **Given** viewport widths from a small phone to a wide desktop monitor, **When** the page is rendered at each width, **Then** there is no horizontal scrolling and no overlapping content.
3. **Given** the page is read without a pointer, **When** a visitor tabs or scrolls through it, **Then** the product name, tagline, and construction notice are each reachable and legible.

### User Story 3 - The page stays private and reachable (Priority: P3)

The visitor never sees private content on the landing page, and the page remains available even when optional subsystems such as authentication, Sync, backup, or the vault itself are unavailable.

**Why this priority**: The landing page is public-facing marketing copy that must not leak operational or note data, and it must not become a source of outages for the daemon.

**Independent Test**: Load the root web address while subsystems are unavailable and scan the rendered content for sensitive values and broken links.

**Acceptance Scenarios**:

1. **Given** the vault or an optional subsystem is unavailable, **When** the visitor loads the root web address, **Then** the landing page still renders normally.
2. **Given** the rendered page is scanned for sensitive data, **Then** it contains no note text, note paths, credentials, tokens, or operational details.
3. **Given** the page includes any links, **When** each is followed, **Then** it does not lead to a broken or unfinished surface.

### Edge Cases

- The root address is requested while the service is still starting or is shutting down — the visitor receives the service's normal refusal, and the page is not expected to be served in that window.
- Client-side scripting is unavailable or blocked — the page must not depend on it for content.
- Very small (under 320 px), very large, zoomed, or text-enlarged viewports — layout must not break.
- The tagline must appear exactly as specified, including the spelling and capitalization of "markdown".
- A configured secret or note path accidentally referenced in page content must never be rendered.
- Concurrent requests to the root and to existing service endpoints must not interfere with each other.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST serve the landing page at the root of its primary web address so that visiting the main web address shows the page.
- **FR-002**: The page MUST display the product name, the tagline exactly as "a second brain, powered by markdown", and a clear notice stating that the web interface is under construction.
- **FR-003**: All meaningful content of the page MUST be delivered with the initial page request so the page is fully readable when client-side scripting is disabled.
- **FR-004**: The page MUST render without horizontal scrolling or overlapping content at representative mobile, tablet, and desktop viewport widths.
- **FR-005**: The page MUST contain only static marketing content and MUST NOT render vault note content, credentials, tokens, or operational details.
- **FR-006**: The page MUST render successfully even when the vault or optional subsystems (authentication, Sync, backup) are unavailable.
- **FR-007**: Serving the landing page MUST NOT degrade the daemon's existing endpoints, and the page MUST load as a normal web page in a browser without errors.

### Key Entities *(include if feature involves data)*

- **Landing page content**: The static marketing copy — product name, tagline, and construction notice — with no dependency on runtime state.
- **Served web assets**: Styles and client-enhancement scripts that the service itself delivers so the page needs no external network access.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In 100% of tested loads of the root web address, a first-time visitor sees the product name, the tagline "a second brain, powered by markdown", and the under-construction notice.
- **SC-002**: The page renders without horizontal scrolling or overlapping content at 320 px, 768 px, 1024 px, and 1920 px viewport widths.
- **SC-003**: With client-side scripting disabled, the full marketing content still renders and is readable.
- **SC-004**: The page's main content is visible within 2 seconds for 95% of loads on a typical local connection.
- **SC-005**: Automated scans of the served page find zero note contents, note paths, credentials, tokens, or operational details.
- **SC-006**: The daemon's existing endpoints meet their prior behavior after the landing page is added (no regressions).

## Assumptions

- The landing page is reachable without authentication because it contains only public marketing copy and no private content.
- Per the user's explicit direction, the page is enhanced with HTMX and the HTMX assets are served by the main braind service itself (no external CDN), consistent with the project constitution (Go backend, server-rendered HTML enhanced with HTMX, no separate JavaScript application tier) and with private-deployment goals.
- The tagline must appear exactly as provided by the user: "a second brain, powered by markdown".
- The page content is static and does not read from the vault or any subsystem.
- An accessible, responsive design is required; the page is not a decorative-only placeholder.
- This feature delivers only the landing page; no functional web interface functionality is in scope.

## Source Traceability

- [Constitution Principle IV (Go Backend, HTMX Frontend)](../../.specify/memory/constitution.md), [roadmap M1 local secure reader](../../docs/roadmap.md), `internal/braind/run.go` (existing HTTP handler surface).
