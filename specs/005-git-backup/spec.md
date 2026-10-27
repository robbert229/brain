# Feature Specification: Git Backup and Isolated Restore

**Feature Branch**: `005-git-backup`

**Created**: 2026-10-26

**Status**: Draft; operator decisions remain open

**Input**: User description: Capture requirements for creating coherent Git backups of the vault, securely pushing them to a private remote, surfacing backup state, and restoring a chosen backup without overwriting the live vault.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Create and confirm a vault backup (Priority: P1)

An operator or administrator creates a backup that represents a coherent vault state and can confirm that it reached the configured private destination.

**Why this priority**: Obsidian Sync keeps devices synchronized but is not the vault's independent history and recovery mechanism.

**Independent Test**: Use temporary repositories and a controllable sync process to exercise no-change runs, file changes, failed pushes, and retries.

**Acceptance Scenarios**:

1. **Given** backup is due or an authorized administrator requests it, **When** the backup runs, **Then** the resulting snapshot includes a coherent set of vault additions, changes, and deletions.
2. **Given** the remote is reachable and the backup is valid, **When** push completes, **Then** status records the confirmed backup revision and time.
3. **Given** the remote is unavailable or has diverged, **When** push fails, **Then** local backup work is retained and the system reports a blocked or retrying state without rewriting remote history.
4. **Given** no new files changed but a local backup remains unpushed, **When** another backup attempt runs, **Then** it retries delivery of pending work.

### User Story 2 - Restore without risking the live vault (Priority: P2)

An operator validates a selected backup in a separate empty destination before deciding whether to use it for recovery.

**Why this priority**: A backup is only useful if recovery can be performed without overwriting the currently live data.

**Independent Test**: Restore a selected test revision into an isolated destination and compare representative notes, file counts, and binary checksums.

**Acceptance Scenarios**:

1. **Given** a valid selected backup and a new empty destination, **When** restore is performed, **Then** files are recovered there while the live vault remains unchanged.
2. **Given** the destination is the live vault or is not empty, **When** restore is attempted, **Then** the operation refuses to proceed.
3. **Given** restore completes, **When** validation is run, **Then** repository integrity and representative content checks are reported.

### Edge Cases

- A file changes during backup or synchronization cannot be quiesced.
- The remote is unavailable, credentials expire, host identity fails validation, or history diverges.
- A push is interrupted after local commit creation.
- Backup metadata is corrupt or configured remote/branch differs from the expected destination.
- Restore is requested for the live or a non-empty destination.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST create backups from the complete vault state, including additions, modifications, and deletions.
- **FR-002**: A successful backup MUST represent one coherent point in time and MUST coordinate with supported writers and synchronization activity.
- **FR-003**: The service MUST support an automatic backup cadence and an administrator-triggered backup; both MUST use the same single-flight process.
- **FR-004**: Backup delivery MUST use a configured private destination and MUST report local, pending, confirmed, retrying, and blocked outcomes.
- **FR-005**: Backup failure MUST retain recoverable local work for a later retry.
- **FR-006**: The service MUST NOT pull, merge, rebase, reset, or force-push remote history as part of backup.
- **FR-007**: Credentials and remote secret material MUST be kept out of images, user-facing status, logs, and vault content.
- **FR-008**: Restore MUST target a new isolated empty destination and MUST never overwrite the live vault.
- **FR-009**: Restore validation MUST report repository integrity and checks of representative text and binary content.

### Key Entities *(include if feature involves data)*

- **Backup revision**: A coherent immutable point-in-time snapshot of the vault.
- **Backup destination**: The configured private repository, branch, and trust settings.
- **Backup attempt**: A scheduled or requested operation and its outcome.
- **Restore destination**: A separate empty location used to validate recovered data.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every successful test backup contains all expected changed, added, and deleted files from its coherent snapshot.
- **SC-002**: Unavailable remote, credential, host-validation, and divergence tests preserve local backup work and report a recoverable state.
- **SC-003**: At most one scheduled or requested backup runs at a time.
- **SC-004**: All tested restore attempts leave the live vault unchanged; valid isolated restores pass integrity and representative file checks.
- **SC-005**: Security review finds zero credentials in images, logs, status, and backup content other than intended vault data.

## Assumptions

- Backup is a separate recovery mechanism; Obsidian Sync is not considered a backup.
- The intended remote is private and append-only from the service's perspective. Exact provider, remote URL, branch, and credentials remain open.
- The proposed six-hour automatic cadence plus administrator-triggered backups is provisional.
- Whether repository-side encryption is sufficient or client-side encryption is required remains unresolved.
- Vault size and backup duration limits must be confirmed from representative measurements before final planning.

## Source Traceability

- [ADR-0001](../../docs/adr/0001-markdown-vault-is-canonical.md), [ADR-0007](../../docs/adr/0007-git-backup-is-one-way.md), [open decisions](../../docs/roadmap.md#decisions-to-resolve).
