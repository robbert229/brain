# Feature Specification: Private Service Deployment

**Feature Branch**: `007-private-deployment`

**Created**: 2026-10-26

**Status**: Draft; environment decisions remain open

**Input**: User description: Capture requirements for packaging and deploying the service privately as a single-instance, persistent, hardened Kubernetes service with secure ingress, managed configuration, and recoverable upgrades.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Deploy the private service (Priority: P1)

An operator deploys the service into the intended cluster and reaches it through authenticated HTTPS while its vault data survives pod replacement.

**Why this priority**: A reproducible private deployment is required for the intended always-available service.

**Independent Test**: Validate rendered deployment resources and perform a disposable-cluster deployment with storage, TLS, probes, and secret references.

**Acceptance Scenarios**:

1. **Given** an authorized cluster operator and required secrets, **When** the deployment is installed, **Then** one service instance starts with persistent vault storage and passes local health checks.
2. **Given** a client connects through the intended external address, **When** it accesses the service, **Then** traffic uses valid TLS and internal-only services are not externally exposed.
3. **Given** the pod is replaced or restarted, **When** it starts again, **Then** the same persistent vault and required service state remain available.

### User Story 2 - Operate a hardened single-instance service (Priority: P2)

An operator can verify that only one replica is allowed, credentials remain externally managed, and the workload has only the privileges and network access it needs.

**Why this priority**: A single writable vault and secrets require explicit deployment boundaries.

**Independent Test**: Inspect deployment policy and attempt forbidden scaling, secret exposure, privilege escalation, and network access in a disposable cluster.

**Acceptance Scenarios**:

1. **Given** the service is running, **When** an operator attempts to scale it above one replica, **Then** the deployment policy prevents a second active writer.
2. **Given** image or deployment configuration is inspected, **When** credentials are searched, **Then** secret values are found only in operator-managed secret storage.
3. **Given** an upgrade or rollback is initiated, **When** it completes, **Then** persistent data remains intact and the operator can verify service health.

### Edge Cases

- The storage driver does not support the required single-pod access mode.
- Image pull, certificate issuance, or secret configuration fails.
- A rollout starts while the old instance still owns the volume.
- An operator attempts to expose internal services or increase replicas.
- An upgrade fails after the new image starts.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The deployment MUST run no more than one active writer for a vault at any time.
- **FR-002**: Vault and service state MUST persist across pod replacement and routine upgrade.
- **FR-003**: External browser access MUST use HTTPS; internal services MUST remain private.
- **FR-004**: The workload MUST run with non-root identity, least privilege, no unnecessary cluster API access, and read-only filesystem protections where compatible with required writable paths.
- **FR-005**: Credentials MUST be supplied through operator-managed secret references and MUST NOT be embedded in images, public manifests, or ordinary configuration.
- **FR-006**: The deployment MUST define startup, liveness, and readiness behavior consistent with the daemon's local availability contract.
- **FR-007**: Network access MUST be restricted to documented application ingress, DNS, identity-provider, synchronization, and backup destinations.
- **FR-008**: Operators MUST be able to identify the deployed application/runtime version and verify image integrity.
- **FR-009**: Upgrade and rollback procedures MUST preserve the persistent vault and expose a health outcome.
- **FR-010**: Deployment documentation MUST cover bootstrap, backup, restore, and recovery without requiring routine shell access to the pod.

### Key Entities *(include if feature involves data)*

- **Deployment**: The service workload, network exposure, health policy, and runtime configuration.
- **Persistent vault**: The durable filesystem used by the service and synchronization client.
- **Operator-managed secret**: A credential supplied at runtime without being embedded in source or image artifacts.
- **Release artifact**: A versioned, integrity-verifiable deployable image.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A clean deployment into the supported disposable cluster reaches healthy status with exactly one active writer.
- **SC-002**: Replacing or restarting the workload preserves all representative vault files and service state.
- **SC-003**: Attempts to scale above one or access the service through an unapproved external path are rejected.
- **SC-004**: Security inspection finds no credential values in image layers, rendered non-secret configuration, or application logs.
- **SC-005**: A tested upgrade and rollback preserve the vault and restore a healthy service within the documented recovery procedure.

## Assumptions

- Kubernetes/k3s is the target runtime; the exact cluster, storage class, DNS name, registry, and TLS issuer are deployment-specific.
- The service and Obsidian Headless runtime are distributed only through private artifacts under the operator's reviewed packaging assumption.
- Multi-architecture publication is intended, but supported architectures depend on successful runtime feasibility work.
- This spec covers deployability; user-facing application behavior remains in the other feature specs.

## Source Traceability

- [ADR-0002](../../docs/adr/0002-single-writer-vault-ownership.md), [ADR-0008](../../docs/adr/0008-private-single-replica-kubernetes.md), [ADR-0009](../../docs/adr/0009-private-obsidian-headless-artifacts.md), [roadmap](../../docs/roadmap.md).
