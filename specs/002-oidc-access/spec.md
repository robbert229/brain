# Feature Specification: OIDC Sign-In and Role Access

**Feature Branch**: `002-oidc-access`

**Created**: 2026-10-26

**Status**: Draft

**Input**: User description: Capture repository requirements for authenticating people through one configured OpenID Connect provider, assigning admin/user/viewer access from configured groups, and applying consistent authorization to browser and API actions.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Sign in securely (Priority: P1)

A person signs in through the configured identity provider and returns to the service as an authenticated user.

**Why this priority**: The vault is private and must not be accessible to anonymous visitors.

**Independent Test**: Use a simulated provider to exercise successful sign-in, rejected callbacks, expired transactions, and sign-out without a live identity service.

**Acceptance Scenarios**:

1. **Given** a configured provider and an unauthenticated visitor, **When** the visitor completes a valid sign-in, **Then** the service creates an opaque server-side session and grants only the role supported by verified claims.
2. **Given** a callback with invalid state, nonce, issuer, audience, signature, or expiry, **When** the callback is processed, **Then** no authenticated session is created.
3. **Given** an authenticated person signs out, **When** sign-out completes, **Then** the local session is invalidated and the provider logout flow is used when supported.

### User Story 2 - Receive least-privilege access (Priority: P1)

An administrator configures role groups, and each authenticated person can perform only actions allowed by their resolved role.

**Why this priority**: Shared-vault access must be explicit and consistent across all entry points.

**Independent Test**: Present valid, malformed, missing, overlapping, and unmapped group claims to the same authorization policy used by browser and API scenarios.

**Acceptance Scenarios**:

1. **Given** a person belongs to a configured role group, **When** they request an allowed action, **Then** the action is granted according to the configured role hierarchy.
2. **Given** a person belongs to more than one configured role group, **When** the effective role is resolved, **Then** administrator takes precedence over user, and user takes precedence over viewer.
3. **Given** a person has no recognized role or malformed group claims, **When** they request protected content, **Then** access is denied.
4. **Given** the same person and action use the browser and JSON API, **When** policy is evaluated, **Then** both surfaces return the same authorization outcome.

### User Story 3 - Continue an existing session during provider outage (Priority: P2)

A person with an existing session can keep using the service briefly during an identity-provider outage, while the service refuses new or overdue authentication checks.

**Why this priority**: An upstream outage should not immediately end useful local work, but it must not extend stale authorization indefinitely.

**Independent Test**: Establish a session with a test provider, make the provider unavailable, and exercise requests before and after the configured reauthentication interval and session expiry.

**Acceptance Scenarios**:

1. **Given** an unexpired session whose role was recently verified, **When** the identity provider becomes unavailable before the reauthentication interval elapses (one hour by default), **Then** the session continues to work using its last verified role.
2. **Given** a session reaches its reauthentication interval while the provider is unavailable, **When** the person requests protected content, **Then** the service fails closed with an authentication-unavailable outcome and does not extend the session.
3. **Given** a new person attempts to sign in while provider verification is unavailable, **When** sign-in is attempted, **Then** no session is created.

### Edge Cases

- Provider discovery is unavailable during startup or becomes unavailable during use.
- A login transaction expires or is replayed.
- Group claims are absent, malformed, duplicated, or contain a value that only partially matches a configured group.
- A session expires while a browser action is in progress.
- Provider logout is unsupported or temporarily unavailable.
- Provider discovery or verification is unavailable while a session is active or reauthentication is due.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST authenticate people through one configured OIDC provider using a protected authorization flow.
- **FR-002**: The service MUST validate the identity, audience, expiry, nonce, and one-time login transaction before creating a session.
- **FR-003**: The service MUST keep provider tokens and raw claims out of browser session identifiers and user-visible diagnostics.
- **FR-004**: The service MUST support configurable group-claim name and claim source, with exact matching against distinct administrator, user, and viewer groups.
- **FR-005**: The service MUST deny access to authenticated identities that do not map to a configured role.
- **FR-006**: The service MUST apply a single role policy to browser and JSON API requests.
- **FR-007**: The service MUST support secure session expiry, rotation, and sign-out.
- **FR-008**: Browser state-changing requests MUST be protected against cross-site request forgery.
- **FR-009**: Authentication failures MUST not reveal secrets, tokens, or sensitive provider response contents.
- **FR-010**: When multiple configured groups match, the effective role MUST use this precedence: administrator, then user, then viewer.
- **FR-011**: The initial JSON API MUST authenticate people through the same server-side browser sessions; application-issued API tokens and bearer-token API access are outside the initial contract.
- **FR-012**: If provider verification is unavailable, the service MUST allow an existing session to use its last verified role only until the configured reauthentication interval (one hour by default); afterward protected requests MUST fail closed without extending the session.
- **FR-013**: Provider unavailability MUST NOT make the service unready when the local vault is accessible.

### Key Entities *(include if feature involves data)*

- **Identity**: A provider-verified person and the claims needed for display and access decisions.
- **Role**: One of administrator, user, viewer, or no access.
- **Session**: An opaque, time-limited reference to an authenticated identity.
- **Login transaction**: A short-lived, single-use record binding a sign-in request to its callback.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every valid test identity resolves to exactly one effective role or no access.
- **SC-002**: All tested invalid, expired, or replayed login callbacks fail to create a session.
- **SC-003**: For every tested role/action pair, browser and API authorization outcomes match, including multi-group precedence.
- **SC-004**: During provider outage tests, existing sessions work only until reauthentication is due; overdue or new sessions fail closed.
- **SC-005**: Review of authentication logs, cookies, and public status finds zero provider tokens or raw claims.

## Assumptions

- The initial product supports one configured provider and the three hierarchical roles recorded in [ADR-0005](../../docs/adr/0005-oidc-group-based-roles.md).
- Pocket ID is the current target provider. Portable requirements are based on standard OIDC behavior; the completed spike supplies provider-specific evidence.
- Browser sessions are the only supported v1 API authentication. Provider-issued and application-issued bearer tokens are deferred until separately specified and verified.
- Provider outage preserves an existing session only until its configured reauthentication interval; idle and absolute expiry remain in force.

## Source Traceability

- [ADR-0004](../../docs/adr/0004-html-and-api-share-behavior.md), [ADR-0005](../../docs/adr/0005-oidc-group-based-roles.md), [Pocket ID verification spike](../../docs/spikes/spk-002-pocket-id.md).
