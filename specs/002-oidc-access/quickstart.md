# Quickstart: OIDC Sign-In and Role Access

## Automated verification (CI, no live provider)

The simulated provider in `internal/braind/auth/authtest` stands in for Pocket ID with the same claim shapes recorded in [SPK-002](../../docs/spikes/spk-002-pocket-id.md).

```sh
go test ./internal/braind/auth/... ./internal/braind/ -race -timeout 180s
```

Expected: all packages `ok`. Coverage of the spec's acceptance scenarios:

| Scenario | Test surface |
| --- | --- |
| US1-AC1 valid sign-in creates an opaque session with a resolved role | `TestCallbackCreatesSession` |
| US1-AC2 invalid state / nonce / issuer / audience / signature / expiry create no session | `TestCallbackRejects*` |
| US1-AC3 sign-out destroys the local session and uses provider logout when advertised | `TestLogout*` |
| US2-AC1–3 role hierarchy, multi-group precedence, unmapped/claim-less denial | `TestResolveRole`, `TestMiddlewareRoleEnforcement` |
| US2-AC4 browser and JSON outcomes match | `TestSamePolicyAcrossSurfaces` |
| US3-AC1 session survives provider outage inside the reauth window | `TestOutageWithinReauthWindow` |
| US3-AC2 overdue session fails closed while provider is down | `TestOutageReauthDueFailsClosed` |
| US3-AC3 no session while verification is unavailable | `TestSignInFailsWhileProviderDown` |
| SC-005 zero token/claim disclosure | `TestPrivacyAcrossAuthLogs` |

## Manual verification with the live Pocket ID provider

Preconditions: `OIDC_*` configured per the spike; `BRAIND_EXTERNAL_URL=https://brain.lab.johnrowley.co`; account membership in a test role group.

```sh
go build -o /tmp/opencode/braind ./cmd/braind
OIDC_ENABLED=true \
  OIDC_CONFIGURATION_URL=https://oidc.lab.johnrowley.co/.well-known/openid-configuration \
  OIDC_CLIENT_ID=braind OIDC_CLIENT_SECRET=<secret> \
  OIDC_ADMIN_GROUP=access-brain-admin OIDC_USER_GROUP=access-brain-user \
  OIDC_VIEWER_GROUP=access-brain-viewer \
  BRAIND_EXTERNAL_URL=https://brain.lab.johnrowley.co \
  BRAIND_DATA_PATH=/tmp/brain-qstart/data \
  BRAIND_VAULT_PATH=/tmp/brain-qstart/data/vault \
  BRAIND_GIT_DIR=/tmp/brain-qstart/data/git/vault.git \
  BRAIND_LISTEN_ADDR=127.0.0.1:18082 /tmp/opencode/braind
mkdir -p /tmp/brain-qstart/data/vault
```

Scenarios to walk:

1. **Sign-in (US1)**: `curl -sI http://127.0.0.1:18082/login` shows the prompt (or follows the provider with `OIDC_AUTO_REDIRECT=true`). Complete sign-in in a browser; the daemon must return `Set-Cookie: braind_session=<opaque>; Path=/; HttpOnly; Secure; SameSite=Lax` and land on the requested page.
2. **Role denial (US2)**: an account in no configured group returns `403` at the protected surface and the daemon log contains only an outcome code, no claims.
3. **Sign-out (US1-AC3)**: `POST /logout` with the session cookie and `X-CSRF-Token` (available in the page) returns `302` and clears the cookie; a subsequent request is unauthenticated (`401`/`302`).
4. **Outage (US3)**: block the provider (DNS sinkhole or `/etc/hosts`) and confirm active sessions keep working until `OIDC_REAUTH_INTERVAL` elapses, then return `503 authentication_unavailable`, while `/readyz` still reports `200 ready` (FR-013).
5. **Replay (SC-002)**: re-hit the saved callback URL: `400 invalid_login_transaction`, no cookie.
6. **Restart**: restart the process and confirm sessions require new sign-in (in-memory store, research D2).

> Note: `BRAIND_GIT_DIR` must be beneath `BRAIND_DATA_PATH` even with backup disabled (path validation is unconditional), like 001's quickstart.
