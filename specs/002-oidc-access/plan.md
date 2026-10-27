# Implementation Plan: OIDC Sign-In and Role Access

**Branch**: `002-oidc-access` | **Date**: 2026-10-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-oidc-access/spec.md`

## Summary

Add sign-in through one configured OIDC provider, map verified group claims to
administrator, user, viewer, or no access, and enforce the same role policy on
browser and JSON API requests. Use an Authorization Code flow with S256 PKCE,
validate the callback before creating an opaque server-side session, protect
browser mutations with CSRF tokens, and fail closed when an existing session
passes its reauthentication deadline during provider unavailability. The
provider outage must not affect local readiness.

Technical approach: extend the existing Go 1.22 `braind` HTTP daemon and
configuration packages. Keep OIDC protocol validation in a small explicit
authentication boundary, session and login-transaction state in process memory
for the single daemon process, and authorization in one shared policy used by
HTML and JSON handlers. Use a simulated OIDC provider and `httptest` for
independent tests; production provider settings remain environment based. The
live Pocket ID behavior and role precedence are recorded in
[research.md](./research.md).

## Technical Context

**Language/Version**: Go 1.22 (`github.com/johnrowl/brain`)

**Primary Dependencies**: Existing stdlib HTTP server, configuration, logging,
status, and lifecycle packages. OIDC client and JWT validation dependency
selection is documented in [research.md](./research.md); no provider-specific
SDK is required by the contract.

**Storage**: No note-content storage. Session and short-lived login transaction
records are operational in-memory state; vault Markdown remains the only
canonical note store. Sessions do not survive daemon restart.

**Testing**: Go `testing` and `net/http/httptest`, using a local simulated OIDC
issuer and deterministic clocks/randomness seams where required. Tests exercise
handlers and the same policy used in production.

**Target Platform**: Linux server (`braind`) behind HTTPS ingress; standard
browser clients.

**Project Type**: Go web service with server-rendered HTML and JSON endpoints.

**Performance Goals**: Authentication and authorization add no external
provider request to each ordinary request; revalidation is bounded by the
configured interval. No throughput target is specified by the feature.

**Constraints**: One configured OIDC provider; confidential client with
server-side secret and S256 PKCE; strict issuer, audience, expiry, nonce, and
single-use state validation; secure opaque cookies; shared HTML/API sessions
and role policy; no bearer API tokens in v1; exact group matching and
admin > user > viewer precedence; browser CSRF protection; no credentials or
raw claims in logs, cookies, status, or diagnostics. Readiness remains based on
local vault availability.

**Scale/Scope**: One daemon replica and one provider; three roles; login,
callback, logout, session middleware, CSRF, and authorization across protected
browser and JSON routes.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Result | Basis |
| --- | --- | --- |
| I. Independently Testable Features | PASS | A local simulated issuer exercises success, rejection, expiry, replay, outage, and logout with no live identity service. |
| II. Explicit Business Logic | PASS | Group-to-role mapping, precedence, session deadlines, and authorization outcomes are explicit policy functions with direct tests. |
| III. Simplicity Over Abstraction | PASS | One provider and one process-local session store; no account database, multi-provider framework, or token API. Keep protocol and policy seams only where needed for correctness and deterministic tests. |
| IV. Go Backend, HTMX Frontend | PASS | Go handlers render ordinary HTML and forms; HTMX remains an enhancement, with no JavaScript application server. |
| V. Markdown as the Single Source of Truth | PASS | Authentication data is operational state; no canonical note content is stored outside the configured Markdown vault. |
| Data & Storage | PASS | Session state is not note content or a derived vault index. No vault mutation behavior changes. |

No constitution violations identified.

## Project Structure

### Documentation (this feature)

```text
specs/002-oidc-access/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── http.md
└── tasks.md             # Phase 2 output; not created by this plan
```

### Source Code (repository root)

```text
cmd/braind/main.go                    # daemon entry point; configuration wiring
internal/braind/run.go                # construct dependencies and register routes
internal/braind/config/config.go      # OIDC and session configuration validation
internal/braind/auth/                 # OIDC flow, sessions, role policy, CSRF boundary
internal/braind/web/                  # browser and JSON handlers/middleware as needed
internal/braind/*_test.go             # handler and lifecycle integration tests
```

**Structure Decision**: Keep the implementation in the existing Go module and
daemon. Add an `auth` package only for the cohesive security boundary and place
routes alongside existing web handling; avoid new processes, databases, or
frontend tiers. Exact file placement can follow existing package ownership
when tasks are generated.

## Complexity Tracking

No constitution violations; this table is intentionally empty.
