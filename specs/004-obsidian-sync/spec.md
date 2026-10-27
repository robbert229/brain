# Feature Specification: Obsidian Sync and Bootstrap

**Feature Branch**: `004-obsidian-sync`

**Created**: 2026-10-26

**Status**: Draft; depends on external command-contract evidence

**Input**: User description: Capture requirements for safely initializing and continuously synchronizing one Obsidian vault, including administrator bootstrap, sync health, recovery, and operator visibility.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Bootstrap the remote vault (Priority: P1)

An administrator who has recently reauthenticated connects the service's local vault to the intended Obsidian Sync vault without routine shell access to the deployment.

**Why this priority**: The service cannot keep the vault current until initial authentication and setup are complete.

**Independent Test**: Drive the bootstrap state machine with a fake external CLI and verify successful setup, failures, secret cleanup, and duplicate submissions.

**Acceptance Scenarios**:

1. **Given** Sync reports authentication or setup is required, **When** a recently authenticated administrator completes the approved bootstrap flow, **Then** the chosen remote vault is associated with the configured local vault.
2. **Given** a viewer, an unauthenticated administrator, or a second concurrent attempt, **When** bootstrap is requested, **Then** the operation is denied or reported as already in progress.
3. **Given** bootstrap credentials are submitted, **When** the operation succeeds or fails, **Then** secrets are not echoed, logged, retained in status, or included in captured subprocess output.

### User Story 2 - Keep the vault synchronized (Priority: P1)

The operator sees whether synchronization is running, blocked, or recovering, while people retain access to the local vault during an upstream outage.

**Why this priority**: Synchronization keeps remote and local copies aligned, but must not make local availability depend on the network.

**Independent Test**: Exercise a fake sync process through start, normal operation, unexpected exit, authentication failure, restart backoff, and shutdown.

**Acceptance Scenarios**:

1. **Given** setup is complete, **When** the service starts synchronization, **Then** no more than one sync process is active for the vault.
2. **Given** synchronization exits unexpectedly, **When** recovery is scheduled, **Then** the service reports the failure and retries with bounded backoff without stopping local reads.
3. **Given** the upstream service is unavailable but local storage works, **When** a person reads the vault, **Then** reads remain available and status identifies the Sync problem.
4. **Given** the service is shutting down, **When** termination begins, **Then** readiness is withdrawn and the managed sync process is stopped within the service shutdown bound.

### Edge Cases

- Authentication expires, setup is incomplete, or the external tool changes an undocumented response.
- Bootstrap is retried after partial failure or receives duplicate submissions.
- The sync process exits repeatedly, hangs, or emits sensitive output.
- The selected sync policy drifts from the required policy.
- Sync outages coincide with local storage failure.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST expose whether the configured vault needs authentication, setup, is synchronizing, is recovering, or is blocked.
- **FR-002**: Bootstrap MUST be restricted to administrators and MUST require recent authentication.
- **FR-003**: Bootstrap MUST accept only the documented setup actions and MUST NOT allow a user to submit arbitrary commands or argument names.
- **FR-004**: Bootstrap requests and responses MUST prevent caching, cross-site request forgery, secret reflection, and request-body logging.
- **FR-005**: At most one bootstrap operation and at most one continuous sync process MUST be active at a time.
- **FR-006**: Unexpected sync failure MUST update visible status and trigger bounded recovery; authentication/setup failures MUST avoid tight retry loops.
- **FR-007**: Sync output shown to operators MUST be bounded and sanitized.
- **FR-008**: Upstream Sync failure MUST NOT make a locally accessible vault unready.
- **FR-009**: The service MUST apply and verify the selected bidirectional sync-everything policy before enabling mutations; policy drift MUST be reported.
- **FR-010**: Shutdown MUST stop the managed sync process and MUST NOT start a competing final sync operation.

### Key Entities *(include if feature involves data)*

- **Sync state**: Authentication/setup requirement, running/recovering state, version, and sanitized recent result.
- **Bootstrap attempt**: A single administrator-authorized transition from an unconfigured to configured vault.
- **Sync policy**: The selected direction and file/config categories included in synchronization.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In all tested startup states, operators can distinguish needs-authentication, needs-setup, running, recovering, blocked, and disabled states.
- **SC-002**: Tests never observe more than one active bootstrap attempt or sync process.
- **SC-003**: Every simulated unexpected process failure produces visible status and a retry bounded by the documented policy.
- **SC-004**: With local storage available, all tested upstream outage scenarios preserve local read availability.
- **SC-005**: Secret-leak checks find no submitted credential in responses, logs, status, or retained output.

## Assumptions

- The official Obsidian Headless CLI is the supported synchronization client; its exact tested command contract remains a prerequisite.
- The selected deployment uses bidirectional sync-everything. Other sync modes require an explicit product decision and may remain read-only.
- Bootstrap occurs over HTTPS and is not performed through normal interactive shell access.

## Source Traceability

- [ADR-0002](../../docs/adr/0002-single-writer-vault-ownership.md), [ADR-0003](../../docs/adr/0003-readiness-is-local.md), [ADR-0006](../../docs/adr/0006-use-obsidian-headless-for-sync.md), [roadmap](../../docs/roadmap.md).
