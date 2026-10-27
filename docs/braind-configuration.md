# `braind` configuration reference

This page documents the environment settings currently parsed by `braind`.
Some settings configure subsystems that are not implemented yet. Configuration
is validated at startup; secret values are redacted from diagnostics.

| Setting | Default | Purpose |
| --- | --- | --- |
| `BRAIND_LISTEN_ADDR` | `0.0.0.0:8080` | HTTP listen address. |
| `BRAIND_EXTERNAL_URL` | `https://brain.lab.johnrowley.co` | Canonical external origin. |
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
| `OIDC_ENABLED` | `true` | Enable OIDC configuration validation. |
| `OIDC_CONFIGURATION_URL` | empty | HTTPS provider configuration URL. |
| `OIDC_CLIENT_ID` | empty | Registered OIDC client identifier. |
| `OIDC_CLIENT_SECRET` | empty | Confidential-client secret. |
| `OIDC_GROUPS_CLAIM` | `groups` | Claim containing group names. |
| `OIDC_GROUPS_SOURCE` | `id_token` | Group source: `id_token` or `userinfo`. |
| `OIDC_ADMIN_GROUP` | empty | Exact administrator group name. |
| `OIDC_USER_GROUP` | empty | Exact user group name. |
| `OIDC_VIEWER_GROUP` | empty | Exact viewer group name. |
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

The smoke command in the [README](../README.md) disables OIDC and Git backup
because those application subsystems are not yet implemented. Their settings
and defaults are already part of the validated configuration surface.

`OIDC_API_BEARER_ENABLED` and `OIDC_API_AUDIENCE` are parsed for future use but
are outside the initial API contract. The initial API uses the same server-side
sessions as the browser. Deployments MUST leave bearer access disabled until a
future spec defines and verifies it; setting the parsed flag does not make
bearer access a supported feature.
