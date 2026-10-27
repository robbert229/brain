<!--
  SYNC IMPACT REPORT (temporary scratch material — remove before committing)

  Version change: unratified placeholder scaffold → 1.0.0 (initial ratification)
  Modified principles: all five placeholder principle slots replaced by the
    five ratified principles below (I. Independently Testable Features, II.
    Explicit Business Logic, III. Simplicity Over Abstraction, IV. Go Backend
    HTMX Frontend, V. Markdown as the Single Source of Truth).
  Added sections: Core Principles (5 principles), Data & Storage,
    Development Workflow & Quality Gates, Governance, version/dates footer.
  Removed sections: none (unratified placeholder scaffold superseded).
  Follow-up TODOs: none. RATIFICATION_DATE set to 2026-10-25 (today, the date
    of adoption); confirm no earlier informal adoption date should be recorded.
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

All durable application data MUST be stored as Markdown files. Markdown is
the single source of truth; no application database and no independent
canonical copy of note content is permitted. Stored Markdown MUST remain
plain and openable by Obsidian without migration or export.

Rationale: files-as-data keeps the store human-readable, inspectable, and
portable.

## Data & Storage

The filesystem Markdown store is authoritative. Concretely:

- A mutation is successful only after the corresponding file operation is
  durable enough to survive a process restart.
- Derived data (indexes, caches, search results) MUST be recomputable from
  the Markdown source and MUST NEVER be treated as authoritative.
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

**Version**: 1.0.0 | **Ratified**: 2026-10-25 | **Last Amended**: 2026-10-25
