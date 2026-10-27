# Phase 1 Data Model: OIDC Sign-In and Role Access

Entities exist only in memory (research D2/D3). Field-level validation rules are stated in *invariants*; state transitions are the contract the middleware enforces.

## Identity

A provider-verified person as captured at successful callback. Never serialized.

| Field | Type | Invariant |
| --- | --- | --- |
| `Subject` | string | Non-empty `sub` claim; provider-verified. |
| `UserClaim` | string | Configured claim (`email` default) rendered only when display is needed; never logged. |
| `Groups` | []string | Exact values from configured claim/source; stored only for role audit within the session; never exposed. |
| `VerifiedAt` | time.Time | Set at session creation; the `LastVerifiedAt` anchor origin. |

## Role

| Value | Meaning |
| --- | --- |
| `RoleAdministrator` | Membership in `OIDC_ADMIN_GROUP`. |
| `RoleUser` | Membership in `OIDC_USER_GROUP`. |
| `RoleViewer` | Membership in `OIDC_VIEWER_GROUP`. |
| `RoleNone` | No exact match, or claim absent/malformed → deny (FR-005). |

**Resolution** (`ResolveRole`): scan membership; return the highest precedence match. Overlap ⇒ administrator (FR-010). Partial matches (e.g. `access-brain-us`) are not membership.

## Session

| Field | Type | Invariant |
| --- | --- | --- |
| `ID` | string | 32 crypto-random bytes, base64url. Cookie-safe; no derived structure. |
| `Identity` | Identity | Set at creation (FR-002 gate passed). |
| `Role` | Role | Computed once at creation from `ResolveRole`; re-computed on reauth. |
| `CSRFToken` | string | Per-session crypto-random token (research D6). |
| `CreatedAt` | time.Time | Absolute-lifetime anchor. |
| `LastUsedAt` | time.Time | Idle anchor; advanced on successful protected use. |
| `LastVerifiedAt` | time.Time | Reauth anchor; advanced only on full reauthentication. |

**Transitions**
- `created → active` on any allowed protected request (refresh idle).
- `active → expired` when idle or absolute lifetime elapses, or when `reauthInterval` elapses (→ reauth required).
- `any → destroyed` on sign-out, absolute expiry, or sweep. Destroyed sessions are unrecoverable.

**Store contract**: concurrent-safe (mutex/CAS); lookup is O(1) and returns `(Session, outcome)` where outcome ∈ {active, expired, reauthRequired}. Sweeper runs on a fixed interval plus opportunistic creation-time pruning.

## LoginTransaction

Single-use record binding a sign-in request to its callback (research D3).

| Field | Type | Invariant |
| --- | --- | --- |
| `State` | string | 32-byte random; cookie-independent key. |
| `Nonce` | string | 32-byte random; embedded in the authorization request. |
| `Verifier` | string | PKCE S256 verifier; challenge derived from it. |
| `CreatedAt` | time.Time | Transaction TTL = 10 min (`transactionTTL`). |
| `PostLoginRedirect` | string | Safe path the visitor requested; default `/`. |

**Transitions**: `issued → redeemed` (delete wins race ⇒ replay rejected) · `issued → expired` (TTL) · never observable outside the server.

## ProviderHealth (supporting view)

| Value | Meaning |
| --- | --- |
| `HealthHealthy` | Last provider round-trip succeeded. |
| `HealthDegraded` | Last attempt failed (outage) — sessions past reauth fail closed (FR-012). |

An unconfigured provider has no surface at all, so "disabled" is reported by the
status snapshot (`oidc.discovery = disabled`) rather than by a live provider.
