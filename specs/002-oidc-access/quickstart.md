# Quickstart: Validate OIDC Sign-In and Role Access

This guide describes the end-to-end validation expected from the feature test
suite. It uses a simulated OIDC provider; no live provider credentials are
needed. See [HTTP contract](contracts/http.md) and [data model](data-model.md).

## Prerequisites

- Go 1.22 or newer.
- Repository dependencies available through the normal Go module workflow.
- A temporary directory for the test vault and daemon data.

## Run focused validation

```sh
go test ./internal/braind/...
```

The integration tests should start a local simulated issuer with discovery,
authorization, token, JWKS, and optional UserInfo/end-session endpoints, then
exercise the daemon over `httptest`.

## Required scenarios and expected results

1. Start login and verify the authorization redirect contains state, nonce,
   `code_challenge_method=S256`, and only configured scopes.
2. Complete a valid callback and verify a fresh opaque cookie is issued. The
   session cookie contains no provider token, subject, email, or group value.
3. Submit unknown, mismatched, expired, or replayed state; invalid nonce,
   issuer, audience, signature, or expiry; and malformed group claims. Each
   case must create no authorized session.
4. Resolve identities matching admin, user, viewer, overlapping groups, and no
   configured groups. Expect exact-match mapping and admin > user > viewer;
   malformed and unmapped identities deny access.
5. For each protected action, compare HTML and JSON authorization outcomes for
   the same session. Missing/expired sessions are 401-equivalent, denied roles
   are 403, and overdue reauthentication during provider outage is 503.
6. Reject unsafe browser and cookie-authenticated JSON requests without a
   valid CSRF token; accept a valid session-bound token. Confirm safe GET does
   not mutate state.
7. Make the provider unavailable after login. Requests work before the
   reauthentication deadline using the last verified role; due requests fail
   closed and do not extend expiry. New login fails without creating a session.
8. Sign out while the provider is unavailable. Local session invalidation
   succeeds; the provider end-session redirect is used only when available and
   configured.
9. Inspect logs, status, cookies, and public errors across these scenarios;
   expect no client secret, tokens, authorization code, raw provider response,
   or raw claims.

## Full repository validation

After the focused scenarios pass, run the repository's standard CI checks as
specified by its contribution guide. The feature must remain independently
testable without a live identity service or manual provider setup.
