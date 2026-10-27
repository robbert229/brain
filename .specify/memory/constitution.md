<!--
  SYNC IMPACT REPORT (temporary scratch material — remove before committing)

  Version change: 1.0.0 → 2.0.0 (major scope clarification of Principle V:
    canonical Markdown storage applies to vault note content, not every form of
    durable operational data).
  Modified principles: V. Markdown as the Single Source of Truth (scope
    narrowed from all durable application data to canonical vault note content).
  Added sections: none.
  Removed sections: none.
  Migration: audit durable note-content stores for independent canonical
    copies; no migration is required for sessions, logs, configuration, or
    backup metadata.
  Follow-up TODOs: none.
-->

# Brain Constitution

## Core Principles

### I. Independently Testable Features

Every feature MUST be independently testable: it MUST be exercisable through
its own interface and automated tests, without requiring another feature, a
live external service, or manual setup. Feature tests MUST run in CI and MUST
NOT depend on shared mutable global state.

Rationale: independent testability keeps each feature understandable,
regression-safe, and removable on its own.

### II. Explicit Business Logic

Business logic MUST be represented explicitly and unambiguously in code —
named, typed, and reviewable — and MUST NOT be buried in UI wiring, HTML
templates, client scripts, or incidental data plumbing. Every business rule
MUST be covered by automated tests that verify the rule directly.

Rationale: implicit logic cannot be reviewed, tested, or reasoned about.

### III. Simplicity Over Abstraction

The simplest solution that satisfies the requirement MUST be chosen.
Abstractions MUST NOT be introduced without a demonstrated, repeated need;
unnecessary abstraction is a defect. New abstractions or internal frameworks
MUST be justified during review.

Rationale: simple, maintainable code outlives clever abstractions.

### IV. Go Backend, HTMX Frontend

The backend MUST be implemented in Go. The frontend MUST be server-rendered
HTML enhanced with HTMX, and core flows MUST remain functional as ordinary
HTTP requests and form submissions when JavaScript is unavailable. No
Node.js application server is permitted.

Rationale: one language for the backend, one server-rendered frontend, no
separate JavaScript application tier.

### V. Markdown as the Single Source of Truth

Durable vault note content MUST be stored as ordinary Markdown files in the
configured Obsidian vault. The vault is the only canonical store of note
content; the service MUST NOT maintain an independent canonical copy of notes
in an application database or another store. Stored Markdown MUST remain
openable by Obsidian without migration or export. Operational data such as
sessions, logs, configuration, and backup metadata is outside this principle.

Rationale: files-as-data keeps note content human-readable, inspectable, and
portable without constraining how the service manages operational state.

## Data & Storage

The configured vault filesystem is authoritative for note content. Concretely:

- A mutation is successful only after the corresponding file operation is
  durable enough to survive a process restart.
- Derived data (indexes, caches, search results) MUST be recomputable from
  the Markdown source and MUST NEVER be treated as authoritative for note
  content.
- Ordinary Markdown files and Obsidian conventions MUST be preserved so the
  vault remains openable in Obsidian.

## Development Workflow & Quality Gates

- Every feature MUST land together with tests that demonstrate its
  independent testability (Principle I).
- Changes to business logic MUST include direct tests of the changed rules
  (Principle II).
- Code review MUST verify compliance with this constitution; changes that
  violate a principle MUST be rejected or revised.
- Unnecessary abstraction MUST be called out and simplified in review
  (Principle III); justified complexity is acceptable.
- Runtime development guidance lives in the Spec Kit guidance files under
  `.specify/`.

## Governance

This constitution supersedes all other development practices and ad-hoc
conventions. Conflicts resolve in favor of this document.

Amendment procedure:

- Propose the change, document its rationale, and update this document.
- Bump CONSTITUTION_VERSION per the versioning policy.
- Update LAST_AMENDED_DATE to the approval date.
- MAJOR amendments MUST include a migration plan and review of affected
  features.

Versioning policy:

- MAJOR: backward-incompatible removal or redefinition of a principle.
- MINOR: new principle or section, or materially expanded guidance.
- PATCH: clarifications, wording, and non-semantic refinements.

Compliance review: every feature plan, pull request, and review verifies
compliance; unexplained complexity or new abstractions MUST be justified
before merge.

**Version**: 2.0.0 | **Ratified**: 2026-10-25 | **Last Amended**: 2026-10-26
