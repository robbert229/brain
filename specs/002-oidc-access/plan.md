# Implementation Plan: OIDC Sign-In and Role Access

**Branch**: `002-oidc-access` | **Date**: 2026-10-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-oidc-access/spec.md`

## Summary

Add OpenID Connect sign-in to `braind` using one configured provider (Authorization Code flow with S256 PKCE, RS256 ID-token validation against discovered JWKS), an opaque server-side session store, and a single role policy that maps exact-match groups claims to administrator/user/viewer. Authentication is a new `internal/braind/auth` package mounted between the existing `logging.HTTPMiddleware` and the daemon mux, with a middleware that resolves the session role once per request so browser and JSON surfaces share one decision (FR-006, ADR-0004/ADR-0005). Provider interaction uses `go-oidc` and `oauth2` behind a small facade (ADR-0010); tests use an in-repo simulated provider so the feature is CI-testable without a live identity service (Principle I).

## Technical Context

**Language/Version**: Go 1.26 (tracks the minimum of `go-oidc`/`oauth2`, added in [ADR-0010](../../docs/adr/0010-adopt-standard-oidc-libraries.md))

**Primary Dependencies**: `github.com/coreos/go-oidc/v3` (discovery, JWKS, ID-token verification) and `golang.org/x/oauth2` (authorization-code flow, PKCE, token exchange) on top of the standard library. `tools/oidcverify/main.go` remains the dependency-free independent verifier for real providers.

**Storage**: None. Sessions and login transactions are in-memory (operational data — Principle V explicitly excludes them from the vault-as-truth rule). A restart signs everyone out; that is acceptable for v1 (documented).

**Testing**: `go test` with `httptest.Server` simulating the provider (discovery, JWKS, authorize, token, logout endpoints). No live provider, no secrets committed. `-race` required for session/transaction store tests.

**Target Platform**: Linux container (Kubernetes, single replica — ADR-0008).

**Project Type**: Single-binary web service (`braind`), same module as 001.

**Performance Goals**: Authentication adds one provider round-trip per login only; request-time auth cost is an O(1) map lookup (session) per protected request. Discovery fetch is background/startup with cache; per-request path never calls the provider.

**Constraints**: Session cookie `HttpOnly`, `Secure`, `SameSite=Lax`; state-changing requests carry a CSRF token (FR-008); zero token/raw-claim disclosure in logs, cookies, and `/status` (FR-003, SC-005); readiness never depends on provider health (FR-013, ADR-0003).

**Scale/Scope**: One provider, three roles, single admin audience. Roughly 5 new source files plus wiring; ~25 tests.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Result |
| --- | --- |
| I. Independently testable | PASS — simulated provider package exercises sign-in, callback rejection, expiry, replay, outage, and role policy in CI. |
| II. Explicit business logic | PASS — role resolution is a named pure function (`ResolveRole`); session/CSRF/outage rules live in named middleware steps, each with direct tests. |
| III. Simplicity over abstraction | PASS — OIDC protocol mechanics delegated to `go-oidc`/`oauth2` (ADR-0010, superseding the original stdlib-only decision); one auth package; no framework. |
| IV. Go backend, HTMX frontend | PASS — sign-in/out are ordinary form POST/redirect flows; login landing stays server-rendered HTML. |
| V. Markdown single source of truth | PASS (N/A) — sessions/transactions are operational data, explicitly out of scope of the principle. |

No violations; no complexity tracking needed.

## Project Structure

### Documentation (this feature)

```text
specs/002-oidc-access/
├── plan.md              # this file
├── research.md          # Phase 0: stdlib OIDC, session design, test provider, CSRF, outage policy
├── data-model.md        # Phase 1: Identity, Role, Session, LoginTransaction
├── quickstart.md        # Phase 1: end-to-end verification with the simulated provider
├── contracts/http.md    # Phase 1: login/callback/logout routes, auth outcomes, middleware contract
└── tasks.md             # Phase 2 (/speckit.tasks)
```

### Source Code (repository root)

```text
internal/braind/
├── auth/                    # new package: everything authentication
│   ├── oidc.go              #   discovery, JWKS, ID-token validation, S256 PKCE, token exchange
│   ├── role.go              #   ResolveRole(groups, cfg) Role — precedence admin>user>viewer
│   ├── session.go           #   opaque session store: create/lookup/rotate/destroy, idle+absolute+reauth timers
│   ├── transaction.go       #   single-use login transactions (state, nonce, verifier, expiry)
│   ├── handler.go           #   /login, /oauth2/callback, /logout handlers + login prompt page
│   └── middleware.go        #   RequireAuth policy middleware + CSRF check for state-changing methods
├── auth/authtest/           # exported test helper: simulated OIDC provider (used by internal/braind tests too)
│   └── provider.go
├── run.go                   # mount /login /oauth2/callback /logout + wrap protected routes; feed status store
├── status/status.go         # discovery state + ActiveSessions wiring (fields already exist)
├── config/config.go         # existing OIDC/session keys — no new keys expected
└── logging/                 # unchanged; redaction list already covers configured secrets

tools/oidcverify/            # unchanged spike reference
```

**Structure Decision**: One new package (`internal/braind/auth`) keeps tokens, sessions, and policy decisions out of `run.go`'s HTTP wiring while avoiding a speculative framework (Principle III). The simulated provider lives under `auth/authtest` so both `auth` tests and full-daemon `braind` tests share one provider without duplicating a fake.
