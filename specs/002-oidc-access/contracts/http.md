# Phase 1 HTTP Contract: OIDC Sign-In and Role Access

All responses carry the standard request-ID behavior from 001. Session cookie name: `braind_session`.

## Public routes (unchanged from 001)

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/healthz`, `/readyz`, `/static/*`, `/{$}` (landing) | Anonymous. |
| GET | `/status` | Anonymous operator view; includes `oidc.active_sessions`. Provider outage never affects `/readyz` (FR-013). |

## Auth routes (new)

| Method | Path | Auth | Success | Failure |
| --- | --- | --- | --- | --- |
| GET | `/login` | anon | `200` sign-in prompt. The prompt issues a login transaction server-side (keyed by `state`) and links to the provider authorization URL carrying `state`, `nonce`, and the S256 PKCE challenge. With `OIDC_AUTO_REDIRECT=true` it `302`s to the provider instead. | `503 {"error":"authentication_unavailable"}` when provider discovery is unreachable. The sign-in routes are always registered: OIDC is mandatory and the daemon refuses to start without a configured provider. |
| GET | `/oauth2/callback` | anon (transaction required) | `302` to `PostLoginRedirect` with `Set-Cookie: braind_session=...; Path=/; HttpOnly; Secure; SameSite=Lax`, rotating the session. Issues CSRF token for the session. | See callback failure matrix. |
| POST | `/logout` | session + CSRF | Destroys session, `302` to `/` with `Set-Cookie: braind_session=""; Max-Age=0` (cleared), and — when discovery advertised `end_session_endpoint` — redirects the browser through provider logout to end the provider session. | `403` CSRF failure; provider-outage logout still destroys the local session and returns `204`. |

### Callback failure matrix (all create no session; FR-002, SC-002, FR-009)

| Condition | Status | Body |
| --- | --- | --- |
| `state` absent/unknown/expired/replayed | `400` | `{"error":"invalid_login_transaction"}` |
| Provider `?error=` response | `303` → `/login` | query `?login_error=access_denied` (sanitized code only) |
| Token endpoint failure | `303` → `/login` | `?login_error=provider_unavailable` |
| ID-token signature/aud/iss/exp/nonce failure | `400` | `{"error":"invalid_id_token"}` (reasons only in logs, redacted) |
| Valid identity, no mapped group | `403` | `{"error":"access_denied"}` — no session cookie. |

## Protected routes (middleware contract)

Everything vault/API from 003+ sits behind `auth.RequireAuth`. The middleware classifies each request:

| Condition | Browser (`Accept: text/html`) | JSON (`Accept: application/json` or `/api/` path) |
| --- | --- | --- |
| No / expired session | `302` `/login?next=<path>` | `401 {"error":"unauthenticated"}` |
| Reauth interval elapsed, provider healthy | `302` `/login?next=<path>` (session destroyed on callback completion) | `401 {"error":"reauthentication_required"}` |
| Reauth due, provider degraded | `503` | `503 {"error":"authentication_unavailable"}` |
| Session active | continue down the handler chain with role in context | same |
| Role below the route's requirement | `403 {"error":"forbidden"}` (HTML: `403` page) | `403 {"error":"forbidden"}` |

**Role requirement** for 002 routes: `/login`/`/oauth2/callback` = none; `/logout` = any authenticated role. Later specs declare per-route minimums (e.g. vault mutations require `user`; 001 status stays public). Both surfaces MUST return the same outcome (FR-006, SC-003) because one middleware makes the decision.

### CSRF

State-changing requests (non-GET/HEAD/OPTIONS) with a session cookie MUST include `X-CSRF-Token` matching the session token, or the CSRF-capable form field for the route. Missing/mismatch ⇒ `403 {"error":"csrf_check_failed"}`.

## Security contract (FR-003/FR-009/SC-005)

- Session cookie value is opaque; cookies never carry claims.
- Logs may contain: outcome codes (`invalid_login_transaction`, `forbidden`), session IDs (rotating), subjects only at debug level. Never tokens, JWKS payloads, `groups` arrays, or raw claims.
- `/status` exposes discovery state, issuer, client ID, role-group names, and `active_sessions` — already the 001 allowlist; extended with no new sensitive fields.
