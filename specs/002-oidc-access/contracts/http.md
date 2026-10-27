# HTTP Contract: OIDC Sign-In and Role Access

All browser and JSON routes use the same authenticated session and role policy.
Names below describe route semantics; route prefixes should follow the existing
versioned API contract when that API is introduced.

## Authentication routes

| Method and path | Behavior |
| --- | --- |
| `GET /auth/login` | Start OIDC Authorization Code flow. Create single-use state, nonce, and S256 PKCE transaction; redirect only to the configured issuer authorization endpoint. |
| `GET /auth/callback` | Validate and consume transaction; exchange code; validate issuer, audience, signature, expiry, nonce, and verified claims. On success rotate/create a server-side session and redirect to a validated local return path. Invalid callbacks create no session. |
| `POST /auth/logout` | Require a valid session and CSRF token; invalidate local session and expire cookie. Redirect through the configured provider end-session endpoint if supported, otherwise to the local signed-out page. |
| `GET /auth/logout/callback` | Accept only the configured post-logout return and render signed-out state; must not restore a local session. |

Login and callback responses are non-cacheable. Authentication failures use a
generic user-safe response and do not expose provider bodies, secrets, tokens,
codes, subject values, or raw claims.

## Session cookie

The cookie contains only a high-entropy opaque session reference. It is set
`Secure` when served over the configured external HTTPS origin, `HttpOnly`,
`SameSite=Lax` or stricter where the callback flow permits, and scoped narrowly
to the service path. It has no provider token or identity data. Responses
carrying or changing authentication state use `Cache-Control: no-store`.

## Authorization outcomes

| Condition | Browser response | JSON response |
| --- | --- | --- |
| No session / expired session | Redirect to login or render sign-in-required response | `401 Unauthorized` |
| Authenticated role lacks permission | `403 Forbidden` page | `403 Forbidden` |
| Role is `none` | Deny protected content; do not establish an authorized session | `403 Forbidden` |
| Reauthentication overdue and provider unavailable | `503 Service Unavailable`, no session extension | `503 Service Unavailable` |
| Authorized | Continue to handler | Continue to handler |

Both surfaces call the same role/action policy. API requests use the same
session cookie; bearer tokens are not part of v1. Error bodies are generic and
contain no sensitive provider detail.

## CSRF

All browser state-changing requests use unsafe HTTP methods and require a
session-bound synchronizer token submitted in a form field or request header.
Missing or invalid tokens are rejected with `403 Forbidden`. GET and HEAD
requests must not change state. JSON endpoints using the browser session cookie
also require CSRF protection for state-changing requests.

## Role policy

The shared policy grants actions according to the configured hierarchy and
route requirements. For the initial policy, admin includes user and viewer
capabilities, user includes viewer capabilities, and viewer receives only the
read actions defined by the corresponding feature. Group claims are exact
matches; role precedence is admin, user, viewer. Unmapped identities are
denied.
