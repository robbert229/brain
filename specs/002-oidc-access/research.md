# Phase 0 Research: OIDC Sign-In and Role Access

## D1 — Implement OIDC with the standard library, not `go-oidc`

**Decision**: Implement discovery, S256 PKCE, RS256 ID-token validation, and token exchange directly on the standard library, modeled on the live-verified pattern in `tools/oidcverify/main.go`.

**Rationale**: The spike tool already proves the full flow against Pocket ID with zero dependencies, and the flow we need is narrow (one provider, one client, code flow only). `coreos/go-oidc` would add a dependency tree and its own abstractions for generality we do not use (Principle III; the repository has no third-party dependencies to date). Validation rules mirror `oidcverify`: exact-issuer equality, endpoint-origin validation against the issuer, `aud` contains client ID, `exp`/`iat` window, `nonce` equality, and signature verification against the keyed JWKS kid.

**Consequence**: We own JWT/JWKS edge cases. Mitigate with a dedicated test matrix (wrong signature, unknown kid, expired token, issuer mismatch, audience mismatch, missing `exp`, non-string `groups`).

## D2 — Sessions: in-memory, opaque, three independent timers

**Decision**: Server-side session store in memory. IDs are 32 random bytes, base64url-encoded (cookie-safe, no structure that leaks anything). Each session records `AuthenticatedAt` (absolute expiry anchor), `LastUsedAt` (idle expiry), `LastVerifiedAt` (reauth anchor), identity display fields (subject, user claim value), and resolved role. Lookup enforces idle timeout, absolute lifetime, and — in combination with provider health — the reauth interval (FR-012); a background sweeper removes expired entries so `ActiveSessions` for `/status` stays accurate.

**Rationale**: Sessions are operational data, so Principle V does not require persistence. Persistence would require at-rest key management for replay resistance and buys nothing for a single-replica service whose restart is already disruptive (vault lock is exclusive). Rotation means replacing the cookie value on every successful reauth, not per request; the old ID is destroyed immediately.

**Consequence**: Process restart signs everyone out (documented in quickstart). Idle (1h), absolute (8h), and reauth (1h) defaults come from existing config keys; validation already ensures idle ≤ absolute and reauth ≤ absolute.

## D3 — Login transactions: short TTL, single-use, in-memory

**Decision**: `state`, `nonce`, `PKCE verifier`, redirect target, and creation time are stored in one transaction record keyed by `state`, TTL 10 minutes, deleted atomically when the callback reads it (sync.Once/CAS-style map delete).

**Rationale**: Exactly the one-time-transaction semantics FR-002/SC-002 demand, plus replay rejection by construction. A cache-style store would permit double redemption under races, which is the exact edge case the spec calls out.

**Consequence**: Concurrent duplicate callbacks: the first delete wins, the second finds no transaction and is rejected. Sweeper reaps abandoned transactions.

## D4 — Test provider: `internal/braind/auth/authtest`, not per-test fakes

**Decision**: A shared, exported simulated provider package (`auth/authtest`) that serves discovery, JWKS (rotatable RSA keys), authorize (with S256 verifier echo and redirect to caller-registered callback paths), token (issues RS256 JWTs with caller-controlled claims), and optionally logout. It exposes hooks to fail endpoints, rotate/destroy keys, expire tokens, and emit arbitrary claim shapes.

**Rationale**: Principle I demands CI-testability without live services; one provider implementation keeps 003–005's future auth tests cheap and prevents fake-provider drift from real Pocket ID semantics recorded in spike SPK-002 (claim sources, `groups` claim, logout endpoint discovery).

**Consequence**: The fake is test infrastructure but lives in-repo under a non-`_test` path so `internal/braind` daemon tests can reuse it; it must never be imported by production code (enforced by keeping it dependency-light and documented).

## D5 — Role resolution: one pure function, exact match, deny by default

**Decision**: `auth.ResolveRole(groups []string, cfg RoleConfig) Role` — a named pure function returning administrator > user > viewer > no-access by exact, case-sensitive string equality against the configured distinct groups (config validation already requires distinctness). A malformed or absent groups claim yields no access, not a zero-value role (FR-005, SC-001).

**Rationale**: Centralizes ADR-0005 precedence so browser and JSON paths cannot diverge (FR-006). Pure function = trivially exhaustive tests (valid, missing, duplicated, partially matching like `"access-brain-use"`, nested/wrong-type claims).

## D6 — CSRF: SameSite=Lax cookie + per-session token on state-changing requests

**Decision**: Session cookie is `HttpOnly`, `Secure`, `SameSite=Lax`. State-changing requests (methods other than GET/HEAD/OPTIONS, and any request whose cookie-authenticated session could mutate state) must carry a CSRF token issued with the session and validated by the auth middleware; server-rendered forms embed it. v1 state-changing surfaces are `POST /logout` (and future vault mutations inherit the same middleware).

**Rationale**: SameSite=Lax blocks cross-site POSTs at the browser but is not a server-side guarantee (non-browser clients, older browsers). A token bound to the session is the minimal explicit defense FR-008 names; double-submit without a session-bound token is weaker and adds no simplicity.

## D7 — Provider-outage policy: reauth-interval gate with an explicit health view

**Decision**: Sessions do not carry provider tokens; verification is complete at login. Per FR-012, when the reauth interval elapses the middleware treats the session as `reauthRequired`: protected requests redirect to `/login` (browser) or return `401 {"error":"reauthentication_required"}` (JSON). While the provider is healthy this yields a new session with re-resolved claims; while it is unreachable, no session is created and the protected request returns `503 {"error":"authentication_unavailable"}` — the session's expiry is never extended past the gate (SC-004). Provider health is a coarse in-memory view (`healthy | degraded | unknown`) updated by the last successful discovery/token/JWKS operation; discovery failure at startup is retried but MUST NOT affect `/readyz` (FR-013, ADR-0003).

**Rationale**: Re-verifying every request would make request latency depend on the provider and violate FR-013's intent. The interval is the one place the spec permits cached authorization, and it is bounded by config. The 503 outcome distinguishes "you are not authenticated" (401) from "we cannot safely re-check right now" so clients do not interpret outage as failure of their own identity.

## D8 — Route surface: mount auth handlers under explicit paths, keep probes public

**Decision**: New daemon routes: `GET /login` (sign-in prompt), `GET /oauth2/callback` (redirect target), `POST /logout`. Protected routes (all vault/API routes from later specs) sit behind the `auth.RequireAuth` middleware; probes (`/healthz`, `/readyz`), static assets, `/status`, and the landing page stay public.

**Rationale**: Probes must stay anonymous for Kubernetes (ADR-0003) and `/status` must remain operator-visible (001 contract). Keeping the landing page public preserves the 009 contract; authentication enforcement applies to the vault surface where 003 begins.
