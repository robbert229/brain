# Tasks: OIDC Sign-In and Role Access

**Input**: Design documents from `/specs/002-oidc-access/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md),
[research.md](./research.md), [data-model.md](./data-model.md),
[contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: Included because the Constitution requires independent feature tests
and direct tests for business rules. Tests use a local simulated OIDC issuer.

**Organization**: Tasks are grouped by user story. Each story phase has a
standalone acceptance path; story dependencies are listed below.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish reusable local OIDC test infrastructure and document
the chosen protocol dependency before feature work.

- [ ] T001 Select and pin a maintained Go OIDC client dependency in `go.mod` and `go.sum`; record the choice and version in `specs/002-oidc-access/research.md`
- [ ] T002 [P] Add a reusable simulated OIDC issuer fixture with discovery, JWKS, authorization, token, UserInfo, and end-session endpoints in `internal/braind/auth/oidc_test.go`
- [ ] T003 [P] Document the v1 authentication configuration variables and safe sample values in `docs/braind-configuration.md`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Implement shared configuration and dependency seams required by
all user stories.

**⚠️ CRITICAL**: Complete this phase before story implementation.

- [ ] T004 Extend OIDC and session configuration validation in `internal/braind/config/config.go` for callback/external URL constraints, distinct role groups, and v1 session settings; reject invalid configuration without including secrets
- [ ] T005 [P] Define typed role, identity, session, and login-transaction records with the constraints from `specs/002-oidc-access/data-model.md` in `internal/braind/auth/model.go`
- [ ] T006 [P] Define injectable clock, cryptographic random source, and provider interfaces for deterministic auth flows in `internal/braind/auth/provider.go`
- [ ] T007 Integrate authentication dependency construction and route registration into `internal/braind/run.go` while preserving `/healthz` and local readiness behavior
- [ ] T008 Add sanitized auth error mapping and ensure secrets are registered with the existing redacting logger in `internal/braind/run.go`

**Checkpoint**: Configuration and test seams are ready; no protected behavior
is exposed yet.

---

## Phase 3: User Story 1 - Sign in securely (Priority: P1) 🎯 MVP

**Goal**: Authenticate through the configured OIDC provider, establish an
opaque local session only after full callback validation, and invalidate it on
sign-out.

**Independent Test**: Using the simulated issuer, verify successful sign-in
and sign-out, and verify invalid, expired, malformed, and replayed callbacks
never create an authenticated session.

### Tests for User Story 1

- [ ] T009 [P] [US1] Test authorization redirect fields, S256 PKCE, configured scopes, and transaction expiry in `internal/braind/auth/oidc_test.go`
- [ ] T010 [P] [US1] Test callback rejection for state, nonce, issuer, audience, signature, expiry, missing subject, and replay in `internal/braind/auth/oidc_test.go`
- [ ] T011 [P] [US1] Test opaque cookie properties, session rotation, logout invalidation, and generic failure responses in `internal/braind/auth/session_test.go`

### Implementation for User Story 1

- [ ] T012 [US1] Implement single-use login transaction storage with high-entropy state, nonce, S256 PKCE verifier, bounded expiry, and safe local return path in `internal/braind/auth/transaction.go`
- [ ] T013 [US1] Implement OIDC discovery and authorization-code exchange using the pinned client; require exact configured issuer, configured audience, valid signature, expiry, nonce, and subject before returning verified identity in `internal/braind/auth/oidc.go`
- [ ] T014 [US1] Implement the session store with cryptographically random opaque IDs, server-side minimal identity, idle timeout, absolute lifetime, session rotation, and invalidation in `internal/braind/auth/session.go`
- [ ] T015 [US1] Implement login, callback, logout, and logout-callback handlers with no-store responses and generic errors in `internal/braind/auth/handlers.go`
- [ ] T016 [US1] Set and clear the session cookie with Secure, HttpOnly, SameSite, narrow path, and expiry attributes in `internal/braind/auth/cookie.go`
- [ ] T017 [US1] Wire auth handlers and session cookie configuration into `internal/braind/run.go`

**Checkpoint**: A valid simulated login establishes an opaque local session;
invalid callbacks do not; local logout always invalidates the session.

---

## Phase 4: User Story 2 - Receive least-privilege access (Priority: P1)

**Goal**: Resolve configured groups to one least-privilege role and enforce a
single authorization policy across browser and JSON requests.

**Independent Test**: Supply valid, missing, malformed, overlapping, and
unmapped verified group claims and compare browser and JSON outcomes for the
same role/action pairs.

### Tests for User Story 2

- [ ] T018 [P] [US2] Test exact group matching, malformed/missing claims, no-access mapping, and admin > user > viewer precedence in `internal/braind/auth/roles_test.go`
- [ ] T019 [P] [US2] Test one shared authorization policy returns matching allow/deny decisions for browser and JSON requests in `internal/braind/auth/policy_test.go`
- [ ] T020 [P] [US2] Test session-required, forbidden-role, and CSRF outcomes on browser and JSON routes in `internal/braind/auth/middleware_test.go`

### Implementation for User Story 2

- [ ] T021 [US2] Implement strict configured-source group claim parsing and exact role mapping, requiring a list of strings and applying admin > user > viewer precedence in `internal/braind/auth/roles.go`
- [ ] T022 [US2] Implement a shared role/action authorization policy with explicit viewer read, user mutation, and admin management permissions in `internal/braind/auth/policy.go`
- [ ] T023 [US2] Implement session authentication and shared role middleware that maps absent sessions to 401, denied roles to 403, and unmapped identities to denial in `internal/braind/auth/middleware.go`
- [ ] T024 [US2] Implement session-bound synchronizer CSRF token validation for unsafe browser and cookie-authenticated JSON requests in `internal/braind/auth/csrf.go`
- [ ] T025 [US2] Apply the shared session, authorization, and CSRF boundaries to protected browser and JSON handlers in `internal/braind/run.go`
- [ ] T026 [US2] Return generic HTML and JSON authentication/authorization errors according to `specs/002-oidc-access/contracts/http.md` in `internal/braind/auth/handlers.go`

**Checkpoint**: Role/action decisions match across both surfaces; unsafe
cookie-authenticated requests require a valid CSRF token.

---

## Phase 5: User Story 3 - Continue a session during provider outage (Priority: P2)

**Goal**: Preserve an existing verified session only until its reauthentication
deadline, and fail closed after that deadline or for new sign-ins.

**Independent Test**: Log in with the simulated issuer, make it unavailable,
and verify access before and after the configured reauthentication deadline;
also verify new sign-in fails and does not create a session.

### Tests for User Story 3

- [ ] T027 [P] [US3] Test provider outage before reauthentication deadline uses the last verified role without a provider call on each request in `internal/braind/auth/outage_test.go`
- [ ] T028 [P] [US3] Test overdue sessions fail closed with 503 and do not extend idle, absolute, or reauthentication deadlines in `internal/braind/auth/outage_test.go`
- [ ] T029 [P] [US3] Test new sign-in fails without creating a session when discovery, token exchange, or verification is unavailable in `internal/braind/auth/oidc_test.go`

### Implementation for User Story 3

- [ ] T030 [US3] Implement reauthentication due checks and refresh verified identity/role only after successful provider verification in `internal/braind/auth/session.go`
- [ ] T031 [US3] Map provider outage before the deadline to continued local authorization and overdue reauthentication to a non-extending 503 outcome in `internal/braind/auth/middleware.go`
- [ ] T032 [US3] Ensure discovery and sign-in failures never make local readiness fail while the vault remains accessible in `internal/braind/run.go`

**Checkpoint**: Existing sessions work only within the configured grace
interval; new and overdue authentication checks fail closed during outage.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Verify security boundaries and documentation across all stories.

- [ ] T033 [P] Audit logs, status output, cookies, and public error bodies for client secrets, tokens, codes, raw provider bodies, subjects, and raw claims; add regression coverage in `internal/braind/auth/security_test.go`
- [ ] T034 [P] Update public configuration and deployment guidance for OIDC client registration, HTTPS callback, group mapping, proxy trust, and session defaults in `docs/braind-configuration.md`
- [ ] T035 Run the scenarios in `specs/002-oidc-access/quickstart.md` and record any contract or design corrections in the relevant feature artifacts

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup; blocks all user stories.
- **User Story 1 (Phase 3)**: Depends on Foundation; creates the authenticated session primitive.
- **User Story 2 (Phase 4)**: Depends on Foundation and the session interface from US1. Its role mapping/policy tests can be authored independently, but protected route integration requires US1 sessions.
- **User Story 3 (Phase 5)**: Depends on US1 session and provider integration, and US2 shared authorization middleware.
- **Polish (Phase 6)**: Depends on all desired stories.

### User Story Dependencies

- **US1 (P1)**: Starts after Foundation; no story dependency. MVP scope.
- **US2 (P1)**: Policy is independently testable, but full route integration depends on US1 authentication/session support.
- **US3 (P2)**: Builds on US1 sessions and US2 middleware so outage behavior is consistent across both surfaces.

### Parallel Opportunities

- T002 and T003 can proceed in parallel with the dependency selection task T001.
- T005 and T006 can proceed in parallel after setup.
- US1 test tasks T009–T011 can proceed in parallel once the fixture and model seams exist; T012 and T013 can be developed separately, then integrated by T015.
- US2 role tests (T018), policy tests (T019), and middleware tests (T020) can proceed in parallel; implementation of roles (T021) and policy (T022) can proceed in parallel after model interfaces are fixed.
- US3 outage test cases T027–T029 can be prepared in parallel after the simulated issuer and session interface are available.
- T033 and T034 can proceed in parallel after behavior stabilizes.

## Parallel Example: User Story 1

```text
Task: T009 Add login redirect and transaction tests in internal/braind/auth/oidc_test.go
Task: T010 Add invalid callback tests in internal/braind/auth/oidc_test.go
Task: T011 Add cookie and logout tests in internal/braind/auth/session_test.go
```

Tasks sharing a file should be combined or sequenced by the implementer; the
parallel examples identify independent behavior areas, not simultaneous edits
to the same file.

## Implementation Strategy

### MVP First (User Story 1)

1. Complete Setup and Foundational phases.
2. Complete US1 with login, strict callback validation, opaque sessions, and
   local logout.
3. Validate US1 using the simulated issuer. This delivers secure sign-in, while
   protected feature routes remain unavailable until US2 is complete.

### Incremental Delivery

1. Add US2 role resolution, shared authorization, and CSRF, then validate
   parity across browser and JSON routes.
2. Add US3 bounded outage grace and fail-closed behavior.
3. Complete cross-cutting security review and quickstart validation.

## Notes

- Every task uses checkbox, sequential ID, required story label where
  applicable, and an explicit file path.
- `[P]` marks work that can be performed independently without editing the
  same file as another parallel task.
- Test tasks are included to satisfy the constitution's independent test and
  explicit business-rule coverage requirements.
