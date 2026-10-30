# `braind` configuration reference

This page documents the environment settings currently parsed by `braind`.
Some settings configure subsystems that are not implemented yet. Configuration
is validated at startup; secret values are redacted from diagnostics.

| Setting | Default | Purpose |
| --- | --- | --- |
| `BRAIND_LISTEN_ADDR` | `0.0.0.0:8080` | HTTP listen address. |
| `BRAIND_EXTERNAL_URL` | `https://brain.lab.johnrowley.co` | Canonical external origin. |
| `BRAIND_SHUTDOWN_TIMEOUT` | `5s` | Maximum time to drain in-flight requests during graceful shutdown. Positive Go duration; the Kubernetes `terminationGracePeriodSeconds` should exceed it. |
| `BRAIND_DATA_PATH` | `/data` | Persistent application root and process-lock location. |
| `BRAIND_VAULT_PATH` | `/data/vault` | Markdown vault root. |
| `BRAIND_READ_ONLY` | `false` | Disable filesystem mutations. |
| `BRAIND_MAX_NOTE_BYTES` | `2MiB` | Maximum permitted Markdown note size. |
| `BRAIND_SYNC_ENABLED` | `true` | Enable the planned Obsidian Sync supervisor. |
| `BRAIND_OB_PATH` | `ob` | Obsidian Headless executable path. |
| `BRAIND_SEARCH_MAX_FILES` | `10000` | Maximum files examined by a search. |
| `BRAIND_SEARCH_MAX_BYTES` | `256MiB` | Maximum aggregate bytes read by a search. |
| `BRAIND_SEARCH_TIMEOUT` | `10s` | Maximum duration of a search. |
| `BRAIND_SEARCH_MAX_RESULTS` | `100` | Maximum search results returned. |
| `BRAIND_GIT_BACKUP_ENABLED` | `true` | Enable the planned Git backup coordinator. |
| `BRAIND_GIT_DIR` | `/data/git/vault.git` | Git metadata directory, outside the vault tree. |
| `BRAIND_GIT_REMOTE_URL` | empty | Private backup repository URL; credentials must not be embedded. |
| `BRAIND_GIT_BRANCH` | `main` | Backup branch. |
| `BRAIND_GIT_BACKUP_INTERVAL` | `6h` | Interval between automatic backup attempts. |
| `BRAIND_GIT_QUIET_PERIOD` | `30s` | Desired quiet period before a backup snapshot. |
| `BRAIND_GIT_SSH_KEY_PATH` | `/run/secrets/git/ssh-privatekey` | SSH private key file used for backup. |
| `BRAIND_GIT_KNOWN_HOSTS_PATH` | `/run/secrets/git/known_hosts` | Pinned SSH host keys for backup. |
| `BRAIND_LOG_LEVEL` | `info` | Structured log verbosity. |
| `OIDC_CONFIGURATION_URL` | empty | Provider configuration URL. HTTPS, except an HTTP loopback provider in development. Required: OIDC is always enabled. |
| `OIDC_CLIENT_ID` | empty | Registered OIDC client identifier. Required. |
| `OIDC_CLIENT_SECRET` | empty | Confidential-client secret. Required. |
| `OIDC_GROUPS_CLAIM` | `groups` | Claim containing group names. |
| `OIDC_GROUPS_SOURCE` | `id_token` | Group source: `id_token` or `userinfo`. |
| `OIDC_ADMIN_GROUP` | empty | Exact administrator group name. Required. |
| `OIDC_USER_GROUP` | empty | Exact user group name. Required. |
| `OIDC_VIEWER_GROUP` | empty | Exact viewer group name. Required. |
| `OIDC_USER_CLAIM` | `email` | Claim used as the displayed username. |
| `OIDC_PROVIDER_NAME` | derived from issuer host | Provider label shown to users. |
| `OIDC_AUTO_REDIRECT` | `false` | Automatically redirect unauthenticated page visits to sign-in. |
| `OIDC_EXTRA_SCOPES` | `groups` | Additional scopes requested from the provider. |
| `OIDC_API_BEARER_ENABLED` | `false` | Reserved future setting; bearer API access is outside the initial contract. |
| `OIDC_API_AUDIENCE` | empty | Reserved audience setting for future bearer API access. |
| `BRAIND_SESSION_IDLE_TIMEOUT` | `1h` | Maximum session inactivity. |
| `BRAIND_SESSION_ABSOLUTE_LIFETIME` | `8h` | Maximum session lifetime. |
| `OIDC_REAUTH_INTERVAL` | `1h` | Maximum interval between role re-evaluations. |
| `BRAIND_TRUSTED_PROXY_CIDRS` | empty | Trusted proxy ranges for forwarded request metadata. |

The smoke command in the [README](../README.md) disables Git backup (not yet
implemented) and points the required OIDC settings at a placeholder issuer: the
daemon always demands one provider, and while that issuer is unreachable the
probes and status still work; the landing page redirects to sign-in, which
stays degraded until a reachable issuer is configured.

## Sign-in, sessions, and CSRF

OIDC is always enabled. Configuration validation requires one provider's URL,
client identifier, secret, and the three role groups before the daemon starts,
and `braind` refuses to start without a usable sign-in surface. What the
running daemon does with them is what an operator needs to know, because it
sets the failure modes you will see.

**One provider.** Exactly one provider is supported. The daemon discovers its
endpoints and signing keys from `OIDC_CONFIGURATION_URL`; there is no fallback
issuer and no per-user provider choice.

**Sessions live in memory.** There is no session store to back up, replicate, or
keep warm across replicas. A restart, reschedule, or rollout signs every browser
out. Keep `BRAIND_SESSION_ABSOLUTE_LIFETIME` modest and expect sign-ins around
deploys.

**The session cookie is the whole client.** Its value is 32 random bytes; tokens
and claims stay server-side.

| Attribute | Value | Reason |
| --- | --- | --- |
| Name | `braind_session` | the only browser-visible part of a session |
| `Path` | `/` | every route shares one session |
| `HttpOnly` | set | scripts cannot read or copy it |
| `Secure` | set | sign-in must never travel over plaintext |
| `SameSite` | `Lax` | cross-site POSTs carry no session |
| `Max-Age` | absent | closing the browser ends the session |

`BRAIND_EXTERNAL_URL` must match how people actually reach the daemon: it builds
the provider `redirect_uri`, so a mismatch sends the callback to a host the
browser never asked to be signed into.

**State-changing requests need a CSRF token.** `GET`, `HEAD`, and `OPTIONS` carry
no risk and are exempt. Everything else from an authenticated browser must prove
the sender issued the page, by sending the per-session token in the `X-CSRF-Token`
header, or in a `csrf_token` form field for plain form posts. A missing or wrong
token is `403 csrf_check_failed`, the same answer on pages and JSON.

**A provider outage affects sign-in, never the vault or readiness.** A session
that is inside `OIDC_REAUTH_INTERVAL` keeps working while the provider is
unreachable, because its role was verified at sign-in. Once the interval has
passed, the daemon must re-verify: browsers are sent to `/login?next=…` and API
clients receive `reauthentication_required`. If the provider cannot be consulted
at all, the answer is `503 authentication_unavailable` and the session is **not**
extended, so repeated retries cannot ride out an outage. `/readyz` ignores
provider state entirely, so a down issuer never drains a Kubernetes endpoint.

**Sign-in outcomes carry no secrets.** Sign-in, denial, and failure responses
disclose no tokens, claims, or credentials, and daemon logs record only the
outcome name (`granted`, `forbidden`, `invalid_login_transaction`, …).

## `.env` files

At startup `braind` loads a dot-env file named `.env` from the current working
directory, so a local run can keep its settings in one file instead of inline
environment exports:

```bash
go run ./cmd/braind          # reads ./.env in the working directory
```

Behavior:

- **Precedence**: a variable already set in the process environment wins over a
  value in `.env`; the file only fills in variables that are otherwise unset.
- **Missing file**: `.env` is optional; its absence is not an error. A file
  that cannot be parsed (invalid `KEY=VALUE` line or variable name) aborts
  startup with an error identifying the file and line, like other
  configuration validation.
- **Format**: one `KEY=VALUE` per line; blank lines and lines starting with `#`
  are ignored; an optional `export ` prefix is accepted; values wrapped in a
  matching pair of single or double quotes have the quotes removed. There is no
  variable interpolation or line continuation.
- **Secrets**: values read from `.env` (for example `OIDC_CLIENT_SECRET`) are
  redacted from diagnostics just like process-environment secrets.

`OIDC_API_BEARER_ENABLED` and `OIDC_API_AUDIENCE` are parsed for future use but
are outside the initial API contract. The initial API uses the same server-side
sessions as the browser. Deployments MUST leave bearer access disabled until a
future spec defines and verifies it; setting the parsed flag does not make
bearer access a supported feature.
