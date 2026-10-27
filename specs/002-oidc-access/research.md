# Research: OIDC Sign-In and Role Access

## 1. OIDC provider behavior and flow

**Decision**: Use one configured OIDC issuer with Authorization Code flow,
S256 PKCE, confidential-client authentication using `client_secret_basic`, and
the scopes `openid profile email groups` plus explicitly configured extras.
Use discovery only after checking its issuer exactly matches the configured
issuer. Validate the ID-token signature against discovered JWKS and validate
issuer, audience, expiry, nonce, subject, and the one-time login transaction
before creating a local session.

**Rationale**: The Pocket ID verification spike observed these capabilities in
the deployed provider, including RS256, S256 PKCE, ID-token groups, UserInfo,
and RP-initiated logout. Production is a confidential client even though the
disposable spike used a public client. Keeping issuer and callback validation
strict avoids trusting discovery or redirect data from a request.

**Alternatives considered**: Implicit flow (does not meet the protected
server-side code flow requirement); public client (not the production
deployment model); UserInfo-only groups (an extra request without benefit for
the verified provider, retained as a configurable claim source for portability).

## 2. Provider integration and dependency boundary

**Decision**: Use an OIDC client implementation that performs standards-based
discovery, code exchange, JWKS signature verification, and claim validation;
keep application-specific checks (configured issuer/audience, nonce, state,
one-time transaction, and role mapping) explicit in the `auth` boundary. Select
and pin the smallest maintained Go dependency when implementation tasks are
prepared. Do not reuse the standalone `tools/oidcverify` as a runtime library.

**Rationale**: The verifier is a standard-library-only spike tool and its
purpose is evidence collection, while the production path needs maintained
protocol handling and testable application policy. Explicit application
checks make the security contract reviewable and provider-independent.

**Alternatives considered**: Hand-implement all OIDC/JWT/JWKS behavior in the
daemon (too much security-sensitive protocol code); couple the daemon to a
provider-specific SDK (reduces portability).

## 3. Group claims and role resolution

**Decision**: Read the configured claim from the selected source (`id_token` by
default; `userinfo` is supported), require a list of strings, and compare each
value exactly against three distinct configured group names. Resolve overlap
using administrator > user > viewer. Missing, malformed, or unmatched claims
produce no access. Do not expose raw group values outside the role decision.

**Rationale**: ADR-0005 specifies exact matching and precedence. The Pocket ID
spike observed `groups` as an array of strings in both ID token and UserInfo,
with identical sets; the ID token avoids another network request.

**Alternatives considered**: Substring matching (permits accidental or
malicious partial matches); implicit default-user role (violates least
privilege); UserInfo as the default source (unnecessary request for the
verified provider).

## 4. Session and provider outage behavior

**Decision**: Use a cryptographically random opaque cookie value referencing a
server-side session record. Store only the minimum verified identity/display
fields and role needed by the application, plus creation, last-activity,
absolute-expiry, and last-verification times. Rotate the session identifier at
authentication boundaries; invalidate it on logout. Apply idle and absolute
expiry independently. A provider outage may use the last verified role only
until `OIDC_REAUTH_INTERVAL` (one hour by default); afterward protected
requests fail closed and the session is not extended. Restart clears sessions.

**Rationale**: This meets FR-003, FR-007, FR-012, and the private single-replica
deployment model without adding durable identity storage. The feature needs
local work to continue briefly during outages, but must not extend stale
authorization.

**Alternatives considered**: Provider tokens in browser cookies (exposes
credentials and claims); durable shared session database (unneeded for the
single replica and creates another operational dependency); unlimited stale
session use (violates fail-closed reauthentication).

## 5. Browser and JSON authorization, CSRF, and logout

**Decision**: Resolve the session and role once through shared middleware and
call the same explicit authorization policy for browser and JSON routes. Keep
v1 JSON authentication on the browser session cookie; do not add application
bearer tokens. Protect browser state changes using a synchronizer CSRF token
bound to the server-side session, checking it on unsafe methods. Use
SameSite cookies and restrictive cookie attributes. Logout invalidates local
state first and uses the discovered end-session endpoint when available, with
only a preconfigured/validated post-logout redirect URI.

**Rationale**: ADR-0004 requires shared behavior and FR-008 requires CSRF
protection. Local invalidation must not depend on the provider being online.
Ordinary HTTP forms remain functional without JavaScript.

**Alternatives considered**: Separate browser and API policy (drift risk);
bearer tokens (explicitly deferred by FR-011); relying on SameSite alone for
CSRF (does not satisfy the stated explicit CSRF requirement); provider logout
before local invalidation (provider outage would retain a valid local session).

## 6. Configuration, readiness, and diagnostics

**Decision**: Keep provider, client, claim, group, cookie/session, and trusted
proxy settings in validated process configuration. Never include the client
secret, tokens, authorization codes, raw claims, or provider response bodies
in logs/status/errors. Provider discovery or verification failures do not
change readiness while local vault access is healthy.

**Rationale**: The existing configuration package already has OIDC/session
settings, secret-aware logging, trusted proxy configuration, and readiness
separated from remote integrations. The new behavior should complete these
boundaries rather than invent another config system.

**Alternatives considered**: Provider secrets in request-visible settings or
status (leaks credentials); tying readiness to live identity-provider
availability (blocks local use during upstream outage).

## Repository evidence

- `docs/adr/0005-oidc-group-based-roles.md`: one provider, exact group mapping,
  role precedence, and no local accounts.
- `docs/adr/0004-html-and-api-share-behavior.md`: shared browser/API domain
  behavior and shared server-side sessions.
- `docs/spikes/spk-002-pocket-id.md`: Pocket ID discovery and live flow,
  including RS256, S256 PKCE, ID-token groups, UserInfo, and logout support.
- `internal/braind/config/config.go`: existing OIDC, role-group, cookie
  lifetime, reauthentication, and trusted-proxy configuration surface.
- `.specify/memory/constitution.md`: Go backend, server-rendered HTMX frontend,
  independent tests, explicit policy, and Markdown vault as canonical note
  content.

No unresolved requirement clarification remains. The concrete Go OIDC module
and cookie name/attributes are implementation choices to be pinned during
task execution and covered by the contracts here.
