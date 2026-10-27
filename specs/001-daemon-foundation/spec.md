# Feature Specification: Daemon Foundation and Status

**Feature Branch**: `001-daemon-foundation`

**Created**: 2026-10-26

**Status**: Draft; foundation partially implemented

**Input**: User description: Capture the daemon foundation requirements from the existing repository. Specify reliable startup, configuration, single-owner lifecycle, health, status, and privacy-safe diagnostics.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Start and operate the daemon (Priority: P1)

An operator starts the daemon with deployment settings and can tell whether it is running, misconfigured, or shutting down.

**Why this priority**: Other service capabilities depend on a predictable process lifecycle and clear operational state.

**Independent Test**: Start the daemon with valid and invalid settings, query its health surface, and request shutdown without any external provider.

**Acceptance Scenarios**:

1. **Given** valid settings and an available data root, **When** the operator starts the daemon, **Then** it reports its version and begins serving liveness checks.
2. **Given** invalid or incompatible settings, **When** the operator starts the daemon, **Then** it exits unsuccessfully and its diagnostic identifies the setting without exposing secret values.
3. **Given** one daemon owns a data root, **When** a second daemon attempts to own the same root, **Then** the second process fails without serving requests.
4. **Given** the daemon is running, **When** it receives a termination signal, **Then** it stops accepting work, drains requests within the configured shutdown bound, and releases ownership.

### User Story 2 - Diagnose service state safely (Priority: P2)

An operator uses health and status information to distinguish process liveness, local vault availability, and optional upstream subsystem health.

**Why this priority**: Accurate status avoids unnecessary restarts and lets operators diagnose partial outages.

**Independent Test**: Exercise status transitions with simulated storage and subsystem states and inspect logs and public status for sensitive values.

**Acceptance Scenarios**:

1. **Given** the process is alive while an upstream dependency is unavailable, **When** the operator checks liveness, **Then** liveness remains successful and the dependency degradation is reported separately.
2. **Given** the local vault is inaccessible, **When** the operator checks readiness, **Then** readiness is unsuccessful.
3. **Given** a request has a valid correlation identifier, **When** it completes, **Then** the response and completion log use the same identifier; otherwise a generated identifier is used.

### Edge Cases

- The data root is already locked, unavailable, or becomes unavailable during startup.
- Shutdown is requested while requests are in flight or the graceful deadline expires.
- A diagnostic value contains a credential, remote URL, or other configured secret.
- Status has not yet been populated by an optional subsystem.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST report its build version and support a version-only invocation that exits without starting the server.
- **FR-002**: The service MUST validate settings before accepting traffic and MUST provide actionable errors without printing configured secrets.
- **FR-003**: At most one service process MUST own a configured data root at a time.
- **FR-004**: The service MUST distinguish process liveness from readiness to serve local vault requests.
- **FR-005**: Readiness MUST fail when local vault access is unavailable and MUST remain independent of OIDC, Sync, or backup outages.
- **FR-006**: On termination, the service MUST begin shutdown before draining requests and MUST release its data-root ownership when it exits.
- **FR-007**: Operational logs MUST be structured, correlate each HTTP request with a response identifier, and exclude credentials, tokens, and note contents.
- **FR-008**: Published status MUST be safe for concurrent readers and MUST exclude secrets and raw user content.

### Key Entities *(include if feature involves data)*

- **Service lifecycle**: The service's startup, running, degraded, and shutdown state.
- **Health state**: Liveness and local-readiness results.
- **Operational snapshot**: A point-in-time, privacy-safe summary of build and subsystem state.
- **Request identifier**: A valid inbound or generated value used to correlate an HTTP response and its completion log.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In all defined valid-configuration startup scenarios, the operator can obtain a successful liveness result after the listener starts.
- **SC-002**: In all defined invalid-configuration and duplicate-owner scenarios, the service refuses to serve and reports a non-zero failure.
- **SC-003**: In all tested termination scenarios, readiness changes to unavailable before the process exits, and shutdown completes within its declared bound or reports that the bound expired.
- **SC-004**: Security review of representative logs and public status finds zero configured secret values or note contents.

## Assumptions

- Existing `brain` greeting CLI behavior remains supported; this feature adds and governs `braind` without changing that CLI's purpose.
- The current implementation evidence covers FND-001 through FND-005; readiness semantics from FND-006 remain to be completed and verified.
- Shutdown has a positive bounded timeout; the current local default is not treated as the final deployment timeout.

## Source Traceability

- [ADR-0002](../../docs/adr/0002-single-writer-vault-ownership.md), [ADR-0003](../../docs/adr/0003-readiness-is-local.md), [roadmap](../../docs/roadmap.md), [configuration reference](../../docs/braind-configuration.md), `internal/braind/`.
