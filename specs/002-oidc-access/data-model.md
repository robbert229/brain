# Data Model: OIDC Sign-In and Role Access

Authentication records are operational, in-memory state. They contain no
canonical note content and are cleared when the daemon restarts.

## Identity

Represents the minimum information accepted from a successfully verified OIDC
identity.

| Field | Type | Rules |
| --- | --- | --- |
| Subject | string | Required, non-empty, provider-scoped stable identifier. Never placed in the browser cookie. |
| Display value | string | Optional configured user claim (email by default); sanitize for display and do not use as the authorization key. |
| Role | Role | Exactly `admin`, `user`, `viewer`, or `none`; derived only from verified groups. |
| Verified at | time | Set only after full token and transaction validation; refreshed only after successful provider verification. |

### Role resolution

The configured group claim must be an array/list of strings. Compare exact
values with the distinct configured admin, user, and viewer groups. If more
than one matches, choose `admin`, then `user`, then `viewer`. Missing,
malformed, and unrecognized claims resolve to `none`; such identities receive
no session for protected access.

## Session

| Field | Type | Rules |
| --- | --- | --- |
| ID | random opaque bytes | High entropy; only its encoded value is sent in the cookie. Rotate when authentication is established or privilege is refreshed. |
| Identity | Identity | Contains verified minimal identity fields and effective role, never raw tokens or unfiltered claims. |
| CreatedAt | time | Set on successful callback. |
| LastActivityAt | time | Updated only on accepted authenticated activity. |
| LastVerifiedAt | time | Set on successful provider verification; controls outage grace. |
| CSRFToken | random opaque bytes | Session-bound synchronizer token; not the session ID and not placed in URLs. |

Session is valid only while both idle timeout and absolute lifetime are within
limits. Reauthentication becomes due at `LastVerifiedAt + OIDC_REAUTH_INTERVAL`.
If the provider is unavailable before that deadline, the last verified role
may be used until the deadline. Once due, protected requests fail closed and
do not extend the session. Logout deletes the record and expires the cookie.

## Login transaction

| Field | Type | Rules |
| --- | --- | --- |
| State | random opaque bytes | High entropy, unique, compared in constant time where supported, and consumed once. |
| Nonce | random opaque bytes | Bound to the OIDC request and matched to the verified ID token. |
| PKCE verifier | random secret | Kept server-side until callback; challenge uses S256. |
| CreatedAt / ExpiresAt | time | Short bounded lifetime; expired transactions cannot be used. |
| Return path | local path | Optional validated same-origin path; reject absolute URLs and protocol-relative paths. |

A transaction transitions from pending to consumed or expired. A consumed,
expired, unknown, or mismatched transaction cannot create a session.

## Configuration

Immutable validated process configuration includes provider discovery URL and
client credentials, expected issuer/client ID, claim name and source, three
distinct group names, display claim, extra scopes, idle and absolute session
lifetimes, reauthentication interval, external/callback URL context, and
trusted proxy CIDRs. Secret values are never exposed by status or diagnostics.
