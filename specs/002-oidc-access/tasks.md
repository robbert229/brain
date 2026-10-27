# Tasks: OIDC Sign-In and Role Access

**Input**: Design documents from `specs/002-oidc-access/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/http.md, quickstart.md

**Tests**: Included — the repository constitution (Principle I) makes tests mandatory,
and spec success criteria SC-001–SC-005 are defined as testable outcomes. Every auth
test runs against `internal/braind/auth/authtest`, the simulated provider (research D4);
nothing in CI touches a live identity service.

**Organization**: Tasks are grouped by user story. US1 (sign-in) and US2 (roles) are
P1; US3 (provider outage) is P2 and extends US2's middleware.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 = sign in securely (P1); US2 = least-privilege access (P1); US3 = continue sessions during outage (P2)

## Path Conventions

Single Go module: `internal/braind/auth/` (new), `internal/braind/auth/authtest/` (new),
`internal/braind/run.go`, `internal/braind/status/`, `deploy/`, `docs/`.
Tests live next to their package (`_test.go` files in the same package).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Prepare the tree; the project and 001 foundation already exist.

- [X] T001 Create feature branch `002-oidc-access` from `001-daemon-foundation` HEAD (that branch carries the readiness/status surface 002 wires into; rebase onto `main` if 001 merges first)
- [X] T002 Run baseline `go test ./...` and confirm all 001 foundation and landing-page tests pass before any change (record result)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The auth primitives every user story consumes: OIDC validation,
session store, login transactions, role resolution, and the simulated provider.

**⚠️ CRITICAL**: No user story work begins until this phase is complete.

- [X] T003 Create `internal/braind/auth/` (doc comment: package implements OIDC sign-in, sessions, and role policy per specs/002-oidc-access) with no external dependencies (stdlib only, research D1)
- [X] T004 [P] Implement pure role resolution in `internal/braind/auth/role.go`: `Role` constants (`administrator`/`user`/`viewer`/none), `RoleConfig{AdminGroup,UserGroup,ViewerGroup string}`, and `ResolveRole(groups []string, cfg RoleConfig) Role` using exact, case-sensitive membership with admin>user>viewer precedence (FR-010); absent/empty/wrong-type claims resolve to no access (FR-005). No I/O, no config import (Principle II)
- [X] T005 [P] Implement the in-memory session store in `internal/braind/auth/session.go`: `Session` struct per data-model.md; `Create(identity, role) (Session, error)` issuing 32-byte base64url IDs; `Lookup(id)` returning outcome `active|expired|reauthRequired` enforcing idle timeout, absolute lifetime, and the reauth-interval anchor `LastVerifiedAt` (FR-007, FR-012); `Destroy(id)`; `Rotate(id)` replacing the ID atomically; `Count()` for status; mutex-protected with a fixed-interval sweeper plus create-time pruning (research D2)
- [X] T006 [P] Implement single-use login transactions in `internal/braind/auth/transaction.go`: `Issue(redirect string) (Transaction, error)` creating `state`/`nonce`/S256-verifier triples with 10-minute TTL; `Redeem(state)` deleting atomically so the second read fails (replay rejection, SC-002); `Prune()` for expired entries (research D3)
- [X] T007 Implement OIDC provider access in `internal/braind/auth/oidc.go`: discovery fetch + validation (exact issuer equality, endpoint origin checks), JWKS fetch/kid lookup, ID-token verification (RS256 signature, iss/aud/exp/nonce/iat window per research D1), authorization-URL construction with S256 PKCE challenge, code→token exchange using `client_secret_basic`, `nonce` propagation, and a `Provider` type exposing `DiscoveryHealth()`. Model on `tools/oidcverify/main.go` (already proven); MUST NOT log tokens or claims (FR-003)
- [X] T008 Implement the simulated provider in `internal/braind/auth/authtest/provider.go`: `httptest`-backed discovery, JWKS (rotatable RSA keys), authorize (validates `code_challenge`/S256 and calls a callback hook), token (issues RS256 JWTs with caller-controlled claims), optional logout endpoint, and controls to fail endpoints, revoke keys, and mark provider unavailable for outage tests (research D4). Document that production code MUST NOT import it
- [X] T009 [P] Unit tests for the primitives: `role_test.go` (valid/missing/duplicated/partial/nested-wrong-type groups + precedence matrix, SC-001); `session_test.go` (idle/absolute expiry, rotate destroys old ID, concurrent lookup with `-race`, reauth-outcome); `transaction_test.go` (TTL expiry, redeem-once, concurrent replay loses)

**Checkpoint**: Primitives pass `go test ./internal/braind/auth/... -race`; handlers can be built on them.

---

## Phase 3: User Story 1 - Sign in securely (Priority: P1) 🎯 MVP

**Goal**: A visitor completes a valid sign-in against a provider and returns as an
authenticated user with a server-side opaque session; any invalid callback creates
no session; sign-out ends local (and, when supported, provider) sessions.

**Independent Test**: With `authtest` as the only provider, complete sign-in, reject
callbacks, expire transactions, and sign out (quickstart automated rows 1–3).

### Tests for User Story 1 (written before implementation)

- [X] T010 [P] [US1] Write `internal/braind/auth/handler_test.go` contract tests: valid callback sets `braind_session` with `Path=/; HttpOnly; Secure; SameSite=Lax` and 302s to the post-login redirect (US1-AC1); unknown/expired/replayed `state` → 400 `invalid_login_transaction` (US1-AC2, SC-002); provider `?error=` → 303 back to `/login` without session; bad signature/audience/nonce/expiry → 400 `invalid_id_token` and zero cookie (US1-AC2 matrix)

### Implementation for User Story 1

- [X] T011 [US1] Implement `internal/braind/auth/handler.go` `GET /login`: issue a transaction, render a minimal server-rendered sign-in page linking the provider authorization URL (state, nonce, S256 challenge) or 302 directly when `AutoRedirect`; 503 `authentication_unavailable` when discovery is unreachable (contracts/http.md)
- [X] T012 [US1] Implement the callback flow in `internal/braind/auth/handler.go` `GET /oauth2/callback`: redeem transaction (single use), exchange code with the PKCE verifier, verify the ID token through `oidc.go` (issuer/aud/exp/nonce), extract groups from the configured source (`id_token` default; `userinfo` alternative, FR-004), resolve role, and create a session on success only. Unmapped identity → 403 `access_denied`, no cookie (FR-005)
- [X] T013 [US1] Implement `POST /logout` in `internal/braind/auth/handler.go`: destroy the session, clear the cookie (`Max-Age=0`), and when discovery advertised `end_session_endpoint` redirect through provider logout; when the provider is unreachable, destroy locally and answer 204 (US1-AC3, edge case)
- [X] T014 [US1] Wire handlers into the daemon HTTP surface in `internal/braind/run.go`: register `/login`, `/oauth2/callback`, `/logout` only when `OIDC_ENABLED` is true, reusing existing config accessors (no new keys) and passing `ExternalURL` + `/oauth2/callback` as the redirect URI. Keep `/healthz`, `/readyz`, `/status`, `/static/`, landing page registered as-is
- [X] T015 [US1] Test callback privacy in `internal/braind/auth/privacy_test.go`: assert every failure path writes only outcome codes to logs and contains no token material, no raw claims, and no configured client secret (FR-009, SC-005), and that the session cookie value is opaque random data unrelated to claims (FR-003)
- [X] T016 [US1] Run the Phase 3 checkpoint: `go test ./internal/braind/auth/... -race -run 'Callback|Login|Logout|Privacy'` green; manual smoke against `authtest` following quickstart scenario 1

**Checkpoint**: Sign-in, callback rejection, and sign-out work independently of later vault specs.

---

## Phase 4: User Story 2 - Receive least-privilege access (Priority: P1)

**Goal**: Every protected request is classified by one middleware policy that maps
the session role to allow/deny identically for browser and JSON surfaces.

**Independent Test**: Present valid, malformed, missing, overlapping, and unmapped
group claims to the same policy used by browser and API scenarios (SC-003).

### Tests for User Story 2 (written before implementation)

- [X] T017 [P] [US2] Write `internal/braind/auth/middleware_test.go`: anonymous protected request → 302 with `next` for HTML accept, 401 `unauthenticated` for JSON accept; expired session → same; active session reaches the handler; role-below-requirement → 403 `forbidden` on both surfaces; multi-group session resolves by precedence (US2-AC2); identical outcomes across `Accept: text/html` vs `application/json` for the same role/action pair (US2-AC4)
- [X] T018 [P] [US2] Write `internal/braind/auth/csrf_test.go`: state-changing request with a session but no `X-CSRF-Token` → 403 `csrf_check_failed`; mismatched token → 403; valid token passes; GET requests exempt (FR-008)

### Implementation for User Story 2

- [X] T019 [US2] Implement `internal/braind/auth/middleware.go` `RequireAuth(policy)` — a decorator that looks up the session, applies `active|expired|reauthRequired` outcomes, compares `Session.Role` against the route's minimum role, and stores the identity in request context via `ContextWithIdentity(r)`/`FromContext(ctx)` for later vault specs (FR-006)
- [X] T020 [US2] Implement CSRF validation inside `middleware.go`: for non-GET/HEAD/OPTIONS requests carrying a session cookie, require the session `CSRFToken` in the `X-CSRF-Token` header (constant-time compare), 403 otherwise; skip when no session exists (login/logout transitions) — no new config keys (research D6)
- [X] T021 [US2] Apply the middleware in `internal/braind/run.go` so protected vault/API routes added by 003+ inherit it: mount `RequireAuth` around a `protectedMux` that currently registers only `/logout` (POST, any authenticated role) while probes/status/landing stay outside it. Document the per-route minimum-role convention in `contracts/http.md` terms inside `middleware.go`
- [X] T022 [US2] Test policy parity across surfaces in `internal/braind/auth/policy_test.go`: drive the same handler through HTML-style and JSON-style requests for each of {none, viewer, user, admin} × {allow-user, allow-admin, public} and assert byte-identical decisions (SC-003)

**Checkpoint**: One policy decision serves both surfaces; role precedence and CSRF verified.

---

## Phase 5: User Story 3 - Continue an existing session during provider outage (Priority: P2)

**Goal**: During an identity-provider outage, live sessions keep their last verified
role until the reauthentication interval, then fail closed without extension.

**Independent Test**: Establish a session with `authtest`, make it unavailable, and
exercise requests before/after the reauthentication interval and session expiry.

### Tests for User Story 3 (written before implementation)

- [X] T023 [P] [US3] Write `internal/braind/auth/outage_test.go`: within-window requests succeed with the cached role while the provider is down (US3-AC1); at/after `OIDC_REAUTH_INTERVAL` a protected request returns 503 `authentication_unavailable` and the session is neither created nor extended (US3-AC2); new sign-in attempts while verification is unavailable create no session and `/readyz` stays 200 (US3-AC3, FR-013)

### Implementation for User Story 3

- [X] T024 [US3] Implement provider health tracking in `internal/braind/auth/oidc.go`: a `Health()` view (`healthy|degraded|disabled`) updated by the last successful discovery/token/JWKS operation with retry that never blocks request paths, plus `DiscoveryState()` for the status snapshot
- [X] T025 [US3] Extend `middleware.go` reauth handling: when `Lookup` returns `reauthRequired`, consult health — if healthy, 302/401 to sign in (session destroyed at callback completion per data-model transitions); if degraded, 503 `authentication_unavailable` without touching session expiry (FR-012)
- [X] T026 [US3] Implement the callback-side reauth refresh in `internal/braind/auth/handler.go`: completing sign-in for an already-signed-in identity rotates the session (new ID, refreshed `LastVerifiedAt`) rather than stacking sessions, so reauth intervals cannot accumulate

**Checkpoint**: Outage semantics pass; readiness unaffected.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T027 [P] Feed the status snapshot from `internal/braind/status/status.go`: keep `oidc.active_sessions` accurate via `SessionStore.Count()` (called from the status refresh path, not from request paths) and publish `oidc.discovery` as `disabled|pending|ready|degraded` from `Provider.Health()` (001 contract extension, no new sensitive fields)
- [ ] T028 [P] Update `deploy/install.yaml` with a commented OIDC block (`OIDC_ENABLED` and friends already listed) noting that `BRAIND_EXTERNAL_URL` must match the public origin for the callback path, and document session/CSRF cookie attributes in `docs/braind-configuration.md`
- [ ] T029 [P] Write `docs/adr`-consistent operator notes: add a short section to `docs/braind-configuration.md` (or a new `docs/auth.md`) describing restart-means-sign-out (research D2), the reauth-interval outage rule (FR-012), and that readiness never depends on the provider (FR-013/ADR-0003)
- [ ] T030 Run the full quickstart validation: `go test ./... -race` plus the automated scenario table (US1–US3, SC-001–SC-005), then the live-provider manual steps if available; record results in this file and the PR description

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: depends on Setup; BLOCKS all stories. T004/T005/T006 are independent files (parallel); T007 depends on nothing new; T008 depends on T007's shapes; T009 tests T004–T006.
- **US1 (Phase 3)**: depends on Foundational. Tests T010 before T011–T013; T014 wires after handlers exist.
- **US2 (Phase 4)**: depends on Foundational + US1 (sessions must exist to enforce). Tests T017/T018 before T019/T020.
- **US3 (Phase 5)**: depends on US2 middleware (extends its outcome handling).
- **Polish (Phase 6)**: depends on all stories.

### Parallel Opportunities

- T004, T005, T006, T007 in one PR-batch (different files)
- T009 test trio in parallel
- T017 and T018 in parallel
- T027, T028, T029 in parallel

---

## Implementation Strategy

MVP is Phase 1 + 2 + 3: signed-in sessions with denied non-members. Phase 4 makes
policy consistent across surfaces (required before 003 vault reading). Phase 5 is
resilience polish deployable alongside.

### Commit strategy

Conventional Commits, ≤72-char subjects (CI lint): one commit per phase checkpoint,
e.g. `feat(auth): add OIDC verification, sessions, and role resolution (spec 002)` for
Phase 2, `feat(auth): sign in through OIDC with single-use transactions (spec 002)` for
Phase 3, `feat(auth): enforce one role policy across browser and API (spec 002)` for
Phase 4, `feat(auth): fail closed when provider reauth is overdue (spec 002)` for Phase 5.
