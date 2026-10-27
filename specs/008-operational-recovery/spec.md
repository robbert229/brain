# Feature Specification: Operational Recovery and Hardening

**Feature Branch**: `008-operational-recovery`

**Created**: 2026-10-26

**Status**: Draft

**Input**: User description: Capture operational requirements for privacy-safe monitoring, threat-focused validation, recovery runbooks, and release readiness for the private vault service.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Detect degraded service safely (Priority: P1)

An operator can tell whether the service, vault, identity, synchronization, or backup is degraded using status and aggregate measurements that do not expose private content.

**Why this priority**: Operators need to detect actionable failures without leaking identities, note paths, or content.

**Independent Test**: Drive representative healthy, stale, degraded, and failed states and inspect status, metrics, and logs for sensitive labels.

**Acceptance Scenarios**:

1. **Given** a Sync or backup subsystem becomes stale or degraded, **When** an operator views status, **Then** the affected subsystem and last known outcome are distinguishable from local service health.
2. **Given** aggregate metrics are collected, **When** labels are inspected, **Then** they contain no identity, vault path, query, remote, or raw error text.
3. **Given** a failure is recorded, **When** the operator reviews diagnostics, **Then** actionable sanitized context is available without credentials or note content.

### User Story 2 - Recover from failure (Priority: P2)

An operator follows a tested runbook to recover from common service, storage, authentication, synchronization, backup, or deployment failures.

**Why this priority**: Recovery procedures reduce the risk of data loss and prolonged outages.

**Independent Test**: Use a disposable environment to inject each documented failure and follow the runbook through validated recovery.

**Acceptance Scenarios**:

1. **Given** an expired provider session or unavailable upstream, **When** the operator follows the relevant runbook, **Then** service recovery restores the intended subsystem without exposing secrets.
2. **Given** damaged or lost storage, **When** an operator restores a selected backup to a new destination, **Then** content is validated before any live-vault replacement decision.
3. **Given** a failed rollout, **When** rollback steps are followed, **Then** the service returns to a healthy version while preserving persistent data.

### Edge Cases

- Metrics cardinality grows because of path, query, identity, or error labels.
- A full volume prevents logs, metadata, or backups from being written.
- A recovery procedure would overwrite the live vault or destroy evidence needed to diagnose a failure.
- External validation is required for cluster behavior unavailable in local tests.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST expose privacy-safe aggregate measurements for HTTP, authentication, authorization, search, Sync, and backup health.
- **FR-002**: Measurements MUST NOT use person identifiers, note paths, search queries, remote locations, or unbounded error text as labels.
- **FR-003**: The project MUST maintain tested procedures for expired authentication, unavailable identity provider, Sync failure, backup failure/divergence, full storage, failed rollout, lost storage, and backup restore.
- **FR-004**: Recovery procedures MUST validate restored content in an isolated destination before changes to the live vault.
- **FR-005**: Each trust boundary MUST have an automated negative test or a documented reason external validation is required.
- **FR-006**: Release readiness MUST record dependency/image review, browser/accessibility review, resource/search measurements, known limitations, and milestone acceptance evidence.
- **FR-007**: Operational failures MUST be diagnosable without routine shell access to the service container.

### Key Entities *(include if feature involves data)*

- **Operational measurement**: An aggregate value with bounded, privacy-safe dimensions.
- **Failure scenario**: A reproducible fault condition and expected service behavior.
- **Recovery procedure**: Ordered operator actions and verification evidence for returning to service.
- **Release evidence**: Results showing that defined milestone gates and security expectations have been met.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every defined degraded subsystem state can be distinguished from local liveness and vault readiness.
- **SC-002**: Metric descriptor review finds zero prohibited high-cardinality or sensitive labels.
- **SC-003**: Every listed recovery scenario has a tested procedure with a recorded success/failure outcome.
- **SC-004**: Every documented trust boundary has a negative automated test or a named external validation step.
- **SC-005**: Release review has evidence for every required milestone gate and explicitly records unresolved limitations.

## Assumptions

- This work follows the core service, authentication, synchronization, backup, and deployment milestones.
- Metrics are intended for operator diagnosis and are not a user analytics product.
- Recovery validation never overwrites the live vault automatically.

## Source Traceability

- [ADR-0003](../../docs/adr/0003-readiness-is-local.md), [roadmap](../../docs/roadmap.md).
