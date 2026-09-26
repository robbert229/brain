# `braind` design

Status: proposed design; no implementation exists yet

Last updated: 2026-09-25

## 1. Summary

`braind` is a single-vault, single-instance daemon for an Obsidian vault. It runs as an HTTP server, serves a server-rendered HTMX web application, authenticates multiple people through one OIDC provider, and supervises Obsidian's `ob sync --continuous` process so that the vault on local persistent storage stays synchronized with an Obsidian Sync remote vault.

The product roadmap should let its operator:

- authenticate people with an OpenID Connect provider and authorize them as an administrator, user, or viewer;
- see whether the daemon, local vault, and Obsidian Sync process are healthy;
- browse, search, and read Markdown notes from a browser in the first milestone, then add safe create/edit operations in the mutation milestone;
- perform the same milestone-appropriate note and search operations through a versioned JSON API;
- observe sync state and recent sync output without giving the browser shell access;
- deploy the service to Kubernetes as one StatefulSet with one persistent volume and, by design and policy, never more than one running replica.

The filesystem is the source of truth. There is no application database in v1. Obsidian Headless and `braind` both operate on the same vault directory in the same container. This is deliberate: it avoids cross-container lifecycle races, makes a single supervisor responsible for sync, and lets the pod's `ReadWriteOncePod` volume be the physical single-writer boundary.

This document is intentionally more specific than the current program. It is the contract the implementation and Kubernetes manifests should converge on.

## 2. Goals

### 2.1 Product goals

1. Provide a useful browser interface to a private Markdown vault without requiring the Obsidian desktop application on the server.
2. Keep the server's vault synchronized through the official Obsidian Headless CLI.
3. Preserve ordinary Markdown files and Obsidian conventions. A user must remain able to open the same vault in Obsidian without a migration or export.
4. Remain understandable and operable as a small personal service. A failed sync must be diagnosable from the web UI, process logs, and Kubernetes status.
5. Be safe under restart, rollout, network loss, and externally synchronized file changes.

### 2.2 Engineering goals

- One Go binary for the HTTP application, on-demand vault operations, and child-process supervision.
- Server-rendered HTML with HTMX enhancements. Core reading in the first milestone, and editing once introduced, must still work as normal HTTP requests/form submissions when JavaScript is unavailable.
- A versioned JSON API from the first release, sharing the same domain services, authorization policy, validation, and concurrency controls as the HTML application.
- Native OpenID Connect authentication with role-based authorization derived from configurable group-claim mappings.
- No Node.js application server. Node.js exists in the image only because Obsidian Headless requires Node.js 22 or later.
- A single OCI image built with `ko` where feasible, containing `braind`, Node.js, and a pinned `obsidian-headless` package.
- A Kubernetes StatefulSet with exactly one desired replica, stable storage, explicit health probes, and graceful termination.
- Secure defaults: non-root execution, no Kubernetes API permissions, no public Service, no credentials in the image or ConfigMap, and no endpoint that accepts arbitrary filesystem paths or commands.

## 3. Non-goals for v1

- Reimplementing Obsidian Sync or its wire protocol.
- Running more than one `braind` replica, active/passive failover, or horizontal scaling.
- Collaborative real-time editing.
- Rendering every Obsidian plugin or Obsidian's entire Markdown dialect exactly as the desktop app does.
- Installing or executing community plugins on the server.
- Exposing a general shell, arbitrary command runner, or filesystem browser.
- GraphQL, unversioned JSON endpoints, or a second set of API-only business rules.
- Serving as a general-purpose Git repository manager. V1 includes one narrowly defined, one-way Git backup workflow for the vault; Sync is not backup.
- Supporting multiple vaults, multiple OIDC providers, local password accounts, per-note ACLs, or permissions more granular than the three application roles.
- Automatically creating an Obsidian remote vault.
- Storing an independent canonical copy of note content in a database.

## 4. Design principles

### 4.1 The vault is the database

The durable data model is the directory tree under `BRAIND_VAULT_PATH`. V1 maintains no full-text, metadata, or persistent index. Directory listings, note metadata, and search results are computed from the current filesystem on demand. A note mutation is successful only after the corresponding filesystem operation is durable enough to survive a process restart.

### 4.2 One owner process, one sync process, one volume

One `braind` process owns the HTTP listener and supervises at most one `ob sync --continuous` child. There is no sync sidecar. A sidecar would split readiness, termination, logging, and restart policy between two supervisors while still sharing one mutable directory.

The pod uses a `ReadWriteOncePod` PVC where the cluster's CSI driver supports it. Unlike `ReadWriteOnce`, `ReadWriteOncePod` prevents a second pod anywhere in the cluster from mounting the same claim. This complements, but does not replace, the one-replica StatefulSet policy.

### 4.3 Local availability does not depend on the internet

If Obsidian is unreachable, the already-synchronized vault remains readable and editable. Sync failure is prominently visible, but it does not make the HTTP readiness probe fail. Otherwise a temporary upstream outage would remove a still-useful local service from its Service endpoints and could provoke unhelpful restarts.

### 4.4 Progressive enhancement

The server owns routing, validation, authorization, and HTML rendering. HTMX swaps fragments to make navigation and editing responsive. URL-addressable pages and ordinary form responses remain the baseline.

### 4.5 Safe path handling is a boundary, not a convention

Every path supplied by a client is an untrusted logical vault path. The application resolves it beneath the configured vault root, rejects absolute paths and traversal, does not follow symlinks outside the vault, and never passes it through a shell.

## 5. High-level architecture

```text
                              HTTPS ingress
                                      |
                              ClusterIP Service
                                      |
              +-----------------------------------------------+
              | StatefulSet pod: braind-0                     |
              |                                               |
              |  +-----------------------------------------+  |
              |  | braind process                          |  |
              |  |                                         |  |
              |  | HTTP + HTMX     on-demand vault search  |  |
              |  | note service     sync supervisor         |  |
              |  +-----------+---------------+-------------+  |
              |              |               |                |
              |              |        child: ob sync          |
              |              |          --continuous          |
              |              |               |                |
              |  /data/vault +---------------+                |
              |  /data/home/.obsidian-headless                |
              +----------------------+------------------------+
                                     |
                            ReadWriteOncePod PVC
                                     |
                              Obsidian Sync service
```

### 5.1 Internal components

The Go process should be divided by responsibility even if it initially lives in one binary:

| Component | Responsibility |
| --- | --- |
| Configuration | Parse environment/flags once, apply defaults, validate incompatible settings, redact secrets in diagnostics. |
| HTTP server | Routing, request limits, security headers, timeouts, HTML/full-page versus fragment negotiation, graceful shutdown. |
| Identity service | OIDC discovery and login flow, callback validation, server-side sessions, logout, group extraction, and role resolution. |
| Authorization policy | Central role checks shared by HTML handlers and the v1 JSON API; deny by default. |
| Note service | Path validation, reads, optimistic concurrency checks, atomic writes, create/rename/delete operations. |
| Vault query service | Bounded directory listing and on-demand Markdown search directly against the current filesystem; no background index or watcher. |
| Sync supervisor | Discover setup state, start exactly one `ob sync --continuous`, capture sanitized output, restart with backoff, and stop the child on shutdown. |
| Backup coordinator | Serialize a quiesced vault snapshot into a local Git commit and push it to the configured private remote without ever pulling, merging, rebasing, or force-pushing. |
| Status store | Publish an immutable snapshot of vault/sync/authentication state to handlers and health endpoints. |
| Renderer | Render Markdown to sanitized HTML and build server-side templates/fragments. |

Package boundaries should follow these responsibilities rather than HTTP route names. Filesystem and process execution should sit behind narrow interfaces so they can be tested without running Obsidian Headless.

## 6. Process and lifecycle model

### 6.1 Startup sequence

`braind` starts in the following order:

1. Parse and validate configuration. Invalid static configuration exits with a non-zero status.
2. Load and validate the OIDC claim/group configuration and client credentials. Missing, duplicate, or invalid role-group configuration is fatal when OIDC is enabled.
3. Create or verify the data, home, and vault directories with restrictive permissions.
4. Acquire an exclusive advisory lock at `/data/braind.lock`. Failure to acquire it is fatal. The lock prevents two `braind` processes in one mounted volume from proceeding even if an operator bypasses the StatefulSet design.
5. Start the HTTP listener. `/healthz` may succeed, while `/readyz` returns `503` until the vault access check completes.
6. Discover OIDC provider metadata and signing keys. Failure is a visible degraded authentication state with bounded retry; it does not destroy existing sessions or restart the pod.
7. Verify that the vault root is accessible in the configured read/write mode without recursively scanning it.
8. When Git backup is enabled, verify or initialize the separate repository metadata, validate the fixed remote/branch configuration, and run non-destructive repository integrity checks. Backup configuration failure is visible and blocks backup, not vault reads.
9. Inspect Obsidian Headless authentication and vault setup using supported `ob` commands. Do not parse undocumented credential contents.
10. If authentication and setup are complete, reconcile the selected sync-everything policy and start `ob sync --path <vault> --continuous`. Otherwise enter a visible `needs-authentication` or `needs-setup` state and expose the admin bootstrap flow.
11. Mark `/readyz` ready when the vault access check succeeds. Sync, backup, and OIDC discovery may still be degraded and are reported separately.

Starting HTTP before sync is intentional. It provides a status surface during bootstrap and upstream outages.

### 6.2 Runtime invariants

- There is zero or one live `ob sync` child.
- Only the sync supervisor may spawn `ob`.
- Only the backup coordinator may spawn Git/SSH children, and only one backup operation runs at a time.
- Child tools are invoked with fixed argument vectors and controlled environments; client input is never interpreted as a shell command.
- Every route is public or protected by one central authorization policy; handlers do not make ad hoc role decisions.
- Passwords, tokens, and full note contents never appear in structured logs.
- All application reads and writes consult the current filesystem rather than cached note content.
- A sync child exit does not exit the HTTP server. It changes status and schedules a bounded restart.

### 6.3 Sync restart policy

An unexpected `ob` exit is recorded with its timestamp, exit code, and a sanitized tail of standard output/error. The supervisor restarts with exponential backoff and jitter, for example 1 s, 2 s, 4 s, 8 s, 15 s, 30 s, then 60 s maximum. Ten minutes of healthy execution resets the backoff.

Authentication/setup errors are not tight-looped. They transition to a blocked state and are rechecked on a slow interval (for example 60 seconds) or by an operator-triggered “recheck” action. The action asks the supervisor to rerun its fixed status checks; it never accepts a command from the browser.

### 6.4 Shutdown sequence

On `SIGTERM` or `SIGINT`:

1. Readiness immediately returns `503`.
2. New mutation requests return `503`; in-flight HTTP requests receive a short grace period.
3. Cancel any Git push or backup transaction without starting a final backup; preserve already-created local commits for the next start.
4. Send `SIGTERM` to the `ob` child process group and wait for it to exit.
5. After an internal deadline, send `SIGKILL` to remaining managed child process groups so no orphan holds files or delays volume detach.
6. Flush application logs, release the volume lock, and exit.

The Kubernetes pod should have a termination grace period long enough for this sequence (initially 60 seconds). `braind` should not launch a separate final one-time sync during termination: doing so creates a race with the continuous process and makes shutdown duration depend on the network.

## 7. Obsidian Headless integration

### 7.1 Supported external contract

The image installs the official `obsidian-headless` npm package, pinned to an exact tested version. Its `ob` executable must be on `PATH`. The version is printed at startup and exposed on the diagnostics page.

The currently documented prerequisites and commands are:

- Node.js 22 or later;
- an active Obsidian Sync subscription;
- `ob login` for authentication;
- `ob sync-setup --vault <id-or-name> --path <path>` for local association;
- `ob sync --path <path> --continuous` for the long-running sync;
- `ob sync-status --path <path>` for operator-visible status;
- `ob sync-config --path <path>` for explicit sync-mode configuration.

Obsidian Headless is currently an open beta. Its package version, command behavior, output parsing, and upgrade procedure must therefore be treated as compatibility-sensitive.

### 7.2 Persistent layout

The pod sets `HOME=/data/home` and mounts one PVC at `/data`:

```text
/data/
  braind.lock
  home/
    .obsidian-headless/       # CLI-managed authentication/configuration
  vault/                      # configured Obsidian vault
    .obsidian/                # vault configuration, subject to sync policy
    ... Markdown and attachments
```

Persisting the entire dedicated home beneath the PVC avoids assumptions about which CLI-managed files must survive a restart. The container has no other user's home data. File permissions should be owned by the pod's non-root UID/GID and default to owner-only where possible.

### 7.3 First-time bootstrap

V1 should not require `kubectl exec`. First-time setup is an admin-only web workflow at `/admin/obsidian/bootstrap`, available only while the CLI reports `needs-authentication` or `needs-setup`.

The workflow is:

1. Deploy `braind` and sign in through Pocket ID as a mapped administrator.
2. Require a fresh OIDC authentication before displaying the bootstrap form.
3. Collect the Obsidian email, password, current MFA code when applicable, remote vault ID/name, and E2EE password when applicable over HTTPS.
4. Invoke only the fixed documented `ob login` and `ob sync-setup` operations, without a shell or user-controlled argument names. The dedicated `HOME` causes CLI-managed authentication/setup state to persist on the PVC.
5. Apply and verify the selected bidirectional, sync-everything configuration.
6. Start `ob sync --continuous`, discard the submitted secrets, and close the bootstrap form for the configured vault unless an administrator explicitly enters a destructive re-bootstrap flow.

Bootstrap endpoints use `Cache-Control: no-store`, CSRF protection, strict body limits, no request-body logging, no response reflection, and no browser autocomplete. Only one bootstrap attempt may run at a time. Errors are sanitized so credentials and E2EE material cannot enter logs or the sync-output ring buffer.

The implementation spike should first try driving the CLI's documented interactive prompts over a private subprocess stdin/PTY so secrets do not appear in its argument vector. Because prompt text is not a documented machine interface, the contained fallback is the documented `--email`, `--password`, `--mfa`, and setup `--password` flags. That fallback briefly exposes values in the child command line to processes with sufficient access inside the same pod; the pod has one non-root application container, no shell endpoint, no service-account token, and no debugging sidecar. This residual risk must be recorded and retested on Headless upgrades.

The application never stores the submitted account password, MFA code, or E2EE password after the command finishes. It persists only the opaque CLI-managed state that `ob` writes under the dedicated PVC-backed home. It does not depend on undocumented token file formats. An emergency `kubectl exec` runbook may remain for recovery, but it is not the normal bootstrap path.

### 7.4 Sync mode

The initial deployment uses `bidirectional` mode with `BRAIND_READ_ONLY=false`. The implementation understands the other documented modes so a future read-only deployment cannot accidentally accept writes:

| Obsidian mode | `braind` mutation mode | Intended use |
| --- | --- | --- |
| `bidirectional` | read/write | Default personal knowledge service. Browser edits sync back to other devices. |
| `pull-only` | read-only | Search/read mirror. All note-mutation routes are disabled. |
| `mirror-remote` | read-only | Disposable local mirror semantics; local edits would be reverted and therefore are prohibited. |

The daemon should verify the effective mode from `ob sync-config` when feasible and warn if it conflicts with `BRAIND_READ_ONLY`. It must not silently change an existing vault's sync configuration on every boot.

### 7.5 Sync-everything policy

The selected policy has no excluded folders and enables every attachment/configuration category currently documented by Obsidian Headless:

- file types: `image,audio,video,pdf,unsupported`;
- configuration categories: `app,appearance,appearance-data,hotkey,core-plugin,core-plugin-data,community-plugin,community-plugin-data`;
- excluded folders: empty;
- sync mode: `bidirectional`.

Markdown and other ordinary vault files already participate in Sync; `unsupported` enables additional attachment types. Community plugin code/data may be synchronized as files but is never executed by `braind`.

Bootstrap applies this policy with `ob sync-config`. Normal startup reads and verifies it before launching continuous sync. If it drifts, `braind` reports the exact non-secret difference and refuses mutations until an administrator confirms reconciliation; it does not silently override a deliberate operator change. When a pinned Headless upgrade introduces new selectable categories, the release process must review and deliberately add them so “everything” remains true.

### 7.6 Sync output and status

The supervisor captures line-oriented stdout/stderr in a fixed-size in-memory ring buffer (for example, the last 200 sanitized lines). It emits structured application logs and presents a smaller tail in the UI.

Do not build correctness around human-readable `ob` output. Until the CLI exposes a documented machine-readable format, parsing should be conservative:

- process running/exited and exit code are authoritative;
- timestamps of child start/exit are authoritative;
- recognized messages may improve display but must be labeled best-effort;
- unknown output is retained in sanitized diagnostics rather than interpreted;
- readiness does not depend on a fragile “fully synced” text match.

### 7.7 Version upgrades

The package version is pinned in the image build, not installed at pod startup. An upgrade requires:

1. read release notes;
2. build a new immutable image;
3. test login discovery, `sync-status`, setup detection, continuous sync, shutdown, and a bidirectional edit against a disposable test vault;
4. back up the production PVC;
5. roll out the new image to the one-pod StatefulSet;
6. verify the status page and logs before accepting the upgrade.

Never use `npm install -g ...@latest` in the pod entrypoint.

## 8. Vault and note model

### 8.1 Logical paths

External routes use slash-separated, UTF-8 logical paths relative to the vault, for example `Projects/braind.md`. Route parameters are URL-encoded. The canonical note identifier is the normalized logical path, not an inode, title, or generated database ID.

Validation rules:

- reject empty paths where a file is required;
- reject NUL bytes, absolute paths, `.`/`..` segments, and platform separators other than `/`;
- enforce configurable maximum path and request sizes;
- accept `.md` for editable notes in v1;
- hide `.obsidian`, `.trash`, application-private directories, and dotfiles from ordinary browsing;
- resolve and verify the final path stays under the real vault root;
- do not follow symlinks for mutations; symlinks should be hidden by default for reads as well.

Case-only renames are filesystem-dependent and need a two-step same-directory rename when supported. Name collisions return `409 Conflict`.

### 8.2 Note metadata

Metadata is computed from the requested file on demand:

- logical path and basename;
- title (frontmatter title, first H1, then basename fallback);
- modification time and size;
- a strong content fingerprint used for optimistic concurrency.

Unknown frontmatter is preserved byte-for-byte when a user edits the full source. Structured frontmatter editing is deferred until round-trip preservation behavior is specified.

### 8.3 Reads and Markdown rendering

V1 does not attempt explicit Obsidian feature compatibility. It renders standard CommonMark/GFM through a sanitized server-side renderer and offers a plain-source view. Obsidian-specific constructs such as wiki links, embeds, callouts, block references, properties, Mermaid, MathJax, and plugin syntax may render as ordinary text or unsupported Markdown. This is acceptable for the initial read/search milestone and keeps the filesystem format untouched.

Rendered HTML is sanitized even though the vault is private. Raw HTML, dangerous URL schemes, inline event handlers, and scripts are removed. Content Security Policy is defense in depth, not the sanitizer.

Local attachment serving, inline vault images, attachment upload, and attachment mutation are out of scope for v1. Ordinary external HTTPS links may render with safe link attributes. Explicit Obsidian syntax support should be added only when a concrete workflow needs it.

### 8.4 Writes and optimistic concurrency

An edit form includes a revision token derived from the exact bytes read (prefer a strong content hash; mtime and size alone are insufficient). On save:

1. validate the path and request size;
2. re-read the current file and compare its revision;
3. if changed, return `409` with the submitted text, current text, and a conflict UI; never overwrite silently;
4. write to a restrictive temporary file in the same directory;
5. flush and close it;
6. atomically rename it over the destination;
7. best-effort sync the parent directory on filesystems where this is supported;
8. return the new revision and rendered note.

This guards against edits arriving from `ob sync` between browser load and save. It does not attempt an automatic three-way merge in v1.

New notes use create-if-absent semantics. Parent directories may be created after validation. Rename is same-volume atomic where possible but does not promise to rewrite incoming wiki links in v1.

Obsidian's documented default deletion behavior is system trash. On Linux, `braind` follows that semantic by moving deleted files into the persisted user's XDG trash beneath `/data/home`, retaining original-location metadata where supported. If the move cannot be completed, deletion fails; it never falls back to permanent unlink. Because system trash is outside the vault, trashed files do not sync as live vault content. Delete and restore UI/API work remains after the read-only milestone.

### 8.5 On-demand filesystem queries

V1 has no background filesystem watcher, index, or reconciliation loop. Explorer requests read a single requested directory and return bounded entries. Search requests walk eligible Markdown files at request time with cancellation, bounded concurrency, and limits for elapsed time, files visited, bytes read, and results returned. Hidden/configuration/trash directories and symlinks are skipped.

This makes every result current with respect to completed filesystem writes and `ob` downloads, at the cost of search latency proportional to vault size. A truncated search response says why it stopped and returns `truncated: true` in the API. Search cancellation must promptly stop filesystem work when the client disconnects. Indexing remains a future optimization only after measured vault size or latency justifies it.

## 9. Web application design

### 9.1 Information architecture

The v1 interface has four main surfaces:

1. **Dashboard** — role-aware status: every role sees basic daemon/vault/sync availability, while only administrators see filesystem paths, versions, child details, errors, mapping diagnostics, and bootstrap guidance.
2. **Explorer** — directory-at-a-time navigation, with create-note action when writable.
3. **Search** — bounded on-demand text search with snippets; keyboard-friendly and URL-addressable.
4. **Note** — rendered view, source editor, rename, and trash actions.

The layout uses semantic HTML and has visible focus states, labels, useful empty states, and accessible status announcements. The first design should be functional on narrow screens before adding dense desktop navigation.

### 9.2 HTML and HTMX response model

Each navigable GET has one handler and one view model. When `HX-Request: true` is absent, it returns the full document shell. When present, it returns the main content fragment and uses `HX-Push-Url` so back/forward and copied URLs work.

Mutating forms use `POST` with a CSRF token. Successful HTMX responses swap the affected region and may trigger named client events such as `braind:note-saved`. Non-HTMX submissions use Post/Redirect/Get. Validation failures use `422`; edit conflicts use `409`; missing notes use `404`.

No business rule should exist only in browser JavaScript. Small JavaScript modules are acceptable for editor ergonomics, confirmation dialogs, focus management, and reconnect behavior.

### 9.3 Proposed routes

| Method | Route | Minimum role | Purpose |
| --- | --- | --- | --- |
| `GET` | `/` | Viewer | Role-aware dashboard or redirect to `/notes`. |
| `GET` | `/notes` | Viewer | Explorer/recent notes. |
| `GET` | `/notes/view?path=...` | Viewer | Render a note. |
| `GET` | `/notes/edit?path=...` | User | Source editor with revision token. |
| `POST` | `/notes` | User | Create a note. |
| `POST` | `/notes/save` | User | Save an existing note with revision precondition. |
| `POST` | `/notes/rename` | User | Rename/move a note. |
| `POST` | `/notes/trash` | User | Move a note to the persisted Linux system trash. |
| `GET` | `/search?q=...` | Viewer | Search page or results fragment. |
| `GET` | `/sync` | Admin | Detailed sync/bootstrap diagnostics. |
| `POST` | `/sync/recheck` | Admin | Request a fixed status recheck; does not run arbitrary commands. |
| `GET` | `/admin/obsidian/bootstrap` | Admin + fresh login | Show first-time Headless bootstrap form only when setup is incomplete. |
| `POST` | `/admin/obsidian/bootstrap` | Admin + fresh login | Run the fixed one-time login/setup/sync-configuration workflow. |
| `GET` | `/auth/login` | Public | Begin OIDC Authorization Code flow with PKCE. |
| `GET` | `/auth/callback` | Public protocol endpoint | Validate the OIDC response, resolve a role, and establish a session. |
| `POST` | `/auth/logout` | Authenticated | Destroy the local session and optionally initiate provider logout when supported. |
| `GET` | `/auth/forbidden` | Public | Explain that authentication succeeded but no mapped role was granted. |
| `GET` | `/healthz` | Public | Process liveness. No expensive work or diagnostics. |
| `GET` | `/readyz` | Public | Vault readiness. No upstream dependency or diagnostics. |

Paths stay in the query string for v1. This avoids ambiguous wildcard routing for filenames containing slashes while retaining bookmarkable URLs.

### 9.4 Search

Search scans Markdown files on demand. It matches path, title derived during the scan, and body text; it does not precompute tags, backlinks, or a metadata graph. Exact path/title matches rank ahead of body matches, and a bounded top-N result heap avoids retaining every match. Results include an escaped, size-limited snippet.

Each response reports elapsed time, files/bytes examined, skipped/error counts, and whether configured limits truncated the scan. The implementation sits behind a query interface so a future index can replace it without changing routes, but v1 contains no SQLite, FTS database, persisted catalog, or in-memory vault-wide index.

The current vault contains approximately 2,000 files. The proposed 10,000-file limit therefore leaves roughly fivefold headroom; Phase 0 still measures Markdown bytes and representative search latency before freezing the defaults.

### 9.5 Error behavior

Errors have a stable internal category and a request ID. Browser responses give an actionable explanation without leaking absolute host paths, command arguments, environment values, or credentials. HTMX requests receive an error fragment that can be swapped into a designated alert region; full requests receive an error page.

### 9.6 Versioned JSON API

The JSON API is a v1 deliverable, not a later adapter over HTML handlers. It lives under `/api/v1` and calls the same note, vault-query, sync, identity, and authorization services as the web application. HTML handlers and API handlers may have different representations and status codes, but they cannot implement different business rules.

Initial resources and operations are:

| Method | Route | Minimum role | Purpose |
| --- | --- | --- | --- |
| `GET` | `/api/v1/status` | Viewer | Basic daemon, vault, and sync availability filtered for the caller's role. |
| `GET` | `/api/v1/notes?prefix=&cursor=&limit=` | Viewer | List note metadata with opaque cursor pagination. |
| `POST` | `/api/v1/notes` | User | Create a note from a logical path and Markdown content; create-if-absent. |
| `GET` | `/api/v1/note?path=...` | Viewer | Read note metadata and source content; return a strong `ETag`. |
| `PUT` | `/api/v1/note?path=...` | User | Replace note content; require `If-Match` with the previously returned revision. |
| `POST` | `/api/v1/notes/rename` | User | Atomically rename/move a note with a revision precondition. |
| `DELETE` | `/api/v1/note?path=...` | User | Move a note to the persisted Linux system trash; require `If-Match`. |
| `GET` | `/api/v1/search?q=&limit=` | Viewer | Run a bounded on-demand search and return escaped snippets plus scan/truncation metadata. |
| `GET` | `/api/v1/sync` | Admin | Detailed sync state and sanitized recent output. |
| `POST` | `/api/v1/sync/recheck` | Admin | Schedule a fixed sync/setup recheck and return `202 Accepted`. |
| `GET` | `/api/v1/backup` | Admin | Detailed Git backup state, last successful commit/push, and sanitized failure category. |
| `POST` | `/api/v1/backup` | Admin | Schedule the fixed “back up now” state machine and return `202 Accepted`; accepts no command or Git arguments. |
| `GET` | `/api/v1/openapi.json` | Viewer | OpenAPI description for the exact running API version. |

API conventions:

- JSON request and response media type is `application/json`; errors use `application/problem+json` with a stable machine-readable type, title, HTTP status, request ID, and safe detail.
- Mutation request bodies have strict schemas, reject unknown fields, and use the same size/path limits as browser forms.
- A successful create returns `201 Created`, `Location`, and `ETag`. A successful update returns the new `ETag`.
- Missing `If-Match` on an existing-resource mutation returns `428 Precondition Required`; a stale value returns `412 Precondition Failed`. The HTML conflict flow may continue to use `409` because it returns an interactive merge view.
- Directory-list collection responses use bounded `limit` values and opaque cursors with deterministic ordering. On-demand search returns a bounded top-N response rather than pretending it can efficiently resume a vault scan.
- The API never exposes host filesystem paths, raw `ob` commands, OIDC tokens/claims, or unsanitized child output.
- `/api/v1` compatibility is maintained within v1. Additive response fields are allowed; breaking representation or semantic changes require `/api/v2`.
- An OpenAPI conformance test ensures registered API routes, schemas, status codes, and minimum roles agree with the checked-in contract.

Same-origin browser API calls may use the normal OIDC session cookie and must send CSRF protection on unsafe methods. Programmatic clients may use an OIDC JWT access token when bearer support is enabled. Bearer validation requires the configured exact issuer, signature, expiry, and `OIDC_API_AUDIENCE`; role derivation uses the same `OIDC_GROUPS_CLAIM` and three configured role groups. ID tokens are never accepted as API bearer tokens. Opaque access tokens are not supported in v1 unless standards-based introspection is separately designed.

CORS is disabled by default. Enabling it requires an explicit list of exact HTTPS origins; wildcard origins and credentialed wildcard requests are prohibited. Rate and concurrency limits apply independently to browser and API traffic, with `429 Too Many Requests` and `Retry-After` where appropriate.

For the initial release, API calls are on behalf of signed-in people through the normal server-side browser session. Application-issued personal/API tokens are not included. Such tokens make CLI use straightforward and provider-independent and can have narrow scopes, expirations, and explicit revocation. The tradeoff is that `braind` would become a credential issuer and durable secret store: it would need token creation/display-once UX, hashed storage, rotation, revocation, audit, backup/restore semantics, and rules for reacting to OIDC group removal. Long-lived tokens also bypass Pocket ID's interactive authentication and session controls.

Pocket ID supports user-delegated, audience-bound API access tokens using an API resource and permissions. That is the preferred future path for non-browser clients because Pocket ID remains the issuer and tokens can be validated by issuer, signature, expiry, audience, and permission. Before enabling it, a spike must confirm how the authenticated person's `groups` membership is represented in the API access token; the documented `groups` scope guarantees the claim in ID tokens and UserInfo, not necessarily in a resource access token. Until that is proven, `OIDC_API_BEARER_ENABLED=false` and no application-issued token fallback is added.

## 10. HTTP security model

### 10.1 Network boundary

The supplied Kubernetes YAML keeps the application Service as ClusterIP and exposes it through a standard Kubernetes Ingress at `brain.lab.johnrowley.co`. The selected manifest explicitly uses `ingressClassName: traefik` for k3s's bundled controller; a future reusable overlay may parameterize that value. Direct access to the pod or Service remains restricted by NetworkPolicy.

OIDC is the application authentication boundary, not a substitute for TLS. The canonical external URL is `https://brain.lab.johnrowley.co`, making the callback `https://brain.lab.johnrowley.co/auth/callback`. The Ingress must terminate TLS with a valid certificate. Forwarded scheme/host headers are trusted only from configured proxy CIDRs. Host-header input is never used to invent a redirect URI.

### 10.2 OIDC authentication

`braind` is an OpenID Connect relying party. It uses the Authorization Code flow with PKCE and provider discovery from `OIDC_CONFIGURATION_URL`. The returned metadata's exact issuer becomes the expected issuer for token validation. The implementation should use a mature Go OIDC/OAuth library rather than implementing JWT or protocol validation directly.

The login flow is:

1. Generate high-entropy `state`, `nonce`, and PKCE verifier values, associate them with a short-lived, one-time login transaction, and redirect to the provider.
2. On callback, require an exact `state` match and exchange the authorization code using the configured client credentials and PKCE verifier.
3. Validate the ID token signature, issuer, audience, expiry, nonce, and other library-required claims against discovered provider metadata. Do not accept tokens from a merely similar issuer URL.
4. Obtain the claim named by `OIDC_GROUPS_CLAIM` from the validated ID token or, when explicitly configured for a provider, its UserInfo response. The claim name is treated as an opaque top-level JSON key, so namespaced claim names remain usable. The source must be deterministic; claims from two sources are not silently unioned.
5. Map groups to a `braind` role. A successfully authenticated identity with no mapping receives `403 Forbidden`, not an implicit viewer role.
6. Create an opaque, high-entropy session ID in a secure cookie. Tokens and group claims remain server-side and are never exposed to HTMX or browser JavaScript.

The required scopes start with `openid profile email`; provider-specific configuration may add a scope needed to release group membership. `groups` is a common private claim but is not one of OIDC Core's standard claims, so `OIDC_GROUPS_CLAIM` parameterizes its exact name. The default is `groups`.

The stable identity key is the tuple `(issuer, sub)`. Email and display name are presentation attributes and are never used as authorization identifiers. `email_verified` may be required by configuration if email is displayed, but email does not grant a role.

For Pocket ID, the selected defaults are:

- configuration URL: `https://oidc.lab.johnrowley.co/.well-known/openid-configuration`;
- confidential client with PKCE enabled;
- callback URL: `https://brain.lab.johnrowley.co/auth/callback`;
- scopes: `openid profile email groups`;
- groups claim: `groups`, read from the ID token;
- displayed user claim: `email`;
- provider label: `Pocket ID`;
- automatic redirect: disabled;
- Pocket ID Allowed User Groups restricted to `access-brain-admin`, `access-brain-user`, and `access-brain-viewer` as defense in depth.

Pocket ID documents that requesting the `groups` scope returns a string array in both the ID token and UserInfo. Using the ID token avoids an extra request while retaining `OIDC_GROUPS_SOURCE=userinfo` as a tested fallback. `braind` still performs its own group-to-role mapping; Pocket ID's Allowed User Groups gate only determines who may authenticate to the client.

OIDC is mandatory in production. A development-only authentication bypass may exist for local automated tests, but it must require an explicit development build/runtime switch, refuse to start when the canonical external URL is non-loopback, and never appear in the Kubernetes manifest.

### 10.3 Roles and authorization

Roles form an ordered hierarchy:

```text
admin > user > viewer
```

If an identity belongs to groups mapped to more than one role, the highest role wins. Matching is an exact, case-sensitive comparison after requiring every group value to be a string. Duplicate groups are ignored. An absent claim, a scalar where an array is expected, an unknown group, or malformed claim data grants no role and is logged without including the full claim value.

Roles are maximum permissions, not a way to override deployment policy. When `BRAIND_READ_ONLY=true` or the Obsidian mode is pull-only/mirror-remote, note mutations are disabled for users and administrators alike.

The authorization contract is:

| Capability | Viewer | User | Admin |
| --- | :---: | :---: | :---: |
| Sign in/out and view own identity/role | Yes | Yes | Yes |
| Browse, search, and render/read note source | Yes | Yes | Yes |
| Create, edit, rename, and trash notes | No | Yes | Yes |
| View basic sync availability | Yes | Yes | Yes |
| View detailed sync output, paths, versions, and operational diagnostics | No | No | Yes |
| Trigger a sync/setup recheck | No | No | Yes |
| View effective non-secret configuration and OIDC claim/group diagnostics | No | No | Yes |

`admin` does not provide a web shell, arbitrary `ob` arguments, permanent file deletion, OIDC configuration changes, or Kubernetes access. The bootstrap page executes only the fixed login/setup/configuration state machine and disappears after successful setup.

Every HTTP route declares its minimum role in one centralized route/policy table. The same policy functions protect full-page requests, HTMX fragments, and the v1 JSON API; hiding a button is not authorization. Authentication failures redirect browser page requests to login and return `401` for API requests. Authenticated-but-insufficient requests return `403`. Mutation handlers check authorization before reading request bodies or revealing whether a target note exists.

The health and readiness routes are intentionally unauthenticated but reveal only a status code and a fixed small body. Metrics are either cluster-network-only or separately authenticated; they do not inherit viewer access automatically.

### 10.4 Claim and group configuration

OIDC claim names and the group assigned to each role are explicit environment configuration. They are not editable through the web application. The v1 contract intentionally uses one exact group per role:

| Variable | Purpose |
| --- | --- |
| `OIDC_GROUPS_CLAIM` | Top-level ID-token/UserInfo claim containing group membership; default `groups`. |
| `OIDC_GROUPS_SOURCE` | Claim source: `id_token` (default) or `userinfo`. |
| `OIDC_ADMIN_GROUP` | Exact group value granting `admin`. |
| `OIDC_USER_GROUP` | Exact group value granting `user`. |
| `OIDC_VIEWER_GROUP` | Exact group value granting `viewer`. |
| `OIDC_USER_CLAIM` | Claim used only as the displayed username; default `email`. It never grants access and does not replace `(issuer, sub)` as identity. |

The claim named by `OIDC_GROUPS_CLAIM` must be a JSON array of strings. A missing claim, `null`, a scalar string, mixed element types, or another malformed value grants no role and produces a privacy-safe authentication failure. Requiring one unambiguous shape avoids provider-specific coercion. The configured claim name is an opaque top-level key rather than a dotted path; for example, `https://example.com/claims/groups` is a valid literal claim name.

All three role-group variables are required in production, must be non-empty, and must be pairwise distinct. Group matching is exact and case-sensitive. Configuration changes take effect through a pod restart; hot reload is not needed. Admin diagnostics show the configured claim name, provider display name, and role group names, but ordinary logs and viewer pages do not dump claims received for a person.

The intended Kubernetes environment shape is:

```yaml
env:
  - name: OIDC_CONFIGURATION_URL
    value: "https://oidc.lab.johnrowley.co/.well-known/openid-configuration"
  - name: OIDC_CLIENT_ID
    valueFrom:
      secretKeyRef:
        name: braind-oidc-secrets
        key: client-id
  - name: OIDC_CLIENT_SECRET
    valueFrom:
      secretKeyRef:
        name: braind-oidc-secrets
        key: client-secret
  - name: OIDC_ADMIN_GROUP
    value: "access-brain-admin"
  - name: OIDC_USER_GROUP
    value: "access-brain-user"
  - name: OIDC_VIEWER_GROUP
    value: "access-brain-viewer"
  - name: OIDC_GROUPS_CLAIM
    value: "groups"
  - name: OIDC_GROUPS_SOURCE
    value: "id_token"
  - name: OIDC_EXTRA_SCOPES
    value: "groups"
  - name: OIDC_USER_CLAIM
    value: "email"
  - name: OIDC_PROVIDER_NAME
    value: "Pocket ID"
  - name: OIDC_AUTO_REDIRECT
    value: "false"
```

An environment entry uses either `value` or `valueFrom`, never both. The client credentials come from `secretKeyRef`; the claim and group names are ordinary non-secret configuration.

For the example above:

- a member of `access-brain-user` becomes `user`;
- a member of `access-brain-viewer` becomes `viewer`;
- a member of both becomes `user` because the higher role wins;
- a member of `access-brain-admin` becomes `admin` regardless of the two lower mappings;
- an authenticated person with none of these groups gets `403` and no session with application access;
- changing `OIDC_GROUPS_CLAIM` to another top-level claim name moves group lookup to that claim without changing the authorization algorithm.

### 10.5 Sessions and logout

Sessions are server-side and in memory for v1, which fits the strict single-pod architecture. The browser cookie contains only a random session identifier and uses `Secure`, `HttpOnly`, `SameSite=Lax`, and a narrow path. Session fixation is prevented by issuing a new identifier after the callback. Restarting `braind` invalidates sessions and requires login again; this is acceptable and avoids placing OIDC tokens on the PVC.

Use both an idle timeout and an absolute lifetime, with conservative defaults such as one hour idle and eight hours absolute. If refresh tokens are used, they exist only in memory, are never logged, and are rotated when the provider supports rotation. Otherwise, expiry causes a new authorization redirect; the provider's own session usually makes this unobtrusive.

Role membership is refreshed on a new login or a configured periodic reauthentication. Until continuous provider-side revocation checking is designed, removal from a group can take up to the shorter of the session lifetime and reauthentication interval to take effect. This latency must be documented and configurable. Local logout immediately deletes the server session and expires the cookie. Provider end-session redirect is optional and must use discovered/safely configured endpoints and an allowlisted post-logout URI.

### 10.6 Browser protections

- Require CSRF tokens on every state-changing route, including HTMX requests.
- Use `SameSite=Lax`, `Secure`, and `HttpOnly` on login-transaction and session cookies.
- Validate `Origin`/`Host` for mutations as a secondary control.
- Send a restrictive Content Security Policy; no remote script CDN is required. Vendor HTMX into the image and pin it.
- Set `X-Content-Type-Options: nosniff`, an appropriate `Referrer-Policy`, and frame restrictions.
- Apply request body, header, concurrency, and timeout limits.
- Escape template data and sanitize rendered Markdown.

### 10.7 Secrets

No Obsidian, OIDC, or Git credential value is embedded in a Kubernetes manifest, ConfigMap, image layer, remote URL, or command log. The admin bootstrap form accepts Obsidian credentials transiently under the controls in section 7.3 and never persists or reflects them. Resulting CLI-managed authentication state lives in the dedicated PVC home. The OIDC client ID/secret are supplied by a pre-created Kubernetes Secret through `secretKeyRef`; the Git deploy key and pinned host keys are supplied as read-only Secret files. The base manifest references these Secrets but does not create or contain their values. Public OIDC client mode may be supported only when the chosen provider and threat model permit it.

If future non-interactive Obsidian bootstrap uses Kubernetes Secrets, secret values are never echoed or copied into diagnostics and are removed from child environments when not required.

PVC access is equivalent to vault and persisted Obsidian state access. Cluster storage encryption, optional snapshot access, private Git repository access, and the Git deploy key are all part of the security boundary.

## 11. Configuration contract

Static configuration is supplied with environment variables or equivalent flags. Environment names are preferred in Kubernetes; flags are useful locally. Proposed variables:

| Name | Default | Meaning |
| --- | --- | --- |
| `BRAIND_LISTEN_ADDR` | `0.0.0.0:8080` | HTTP listener. |
| `BRAIND_EXTERNAL_URL` | `https://brain.lab.johnrowley.co` in Kubernetes | Canonical HTTPS origin used for OIDC redirects; never inferred from an untrusted request. |
| `BRAIND_DATA_PATH` | `/data` | Persistent application root. |
| `BRAIND_VAULT_PATH` | `/data/vault` | Vault root. Must be beneath the data path in the supported deployment. |
| `BRAIND_READ_ONLY` | `false` | Disable all filesystem mutations. Required for pull-only/mirror-remote. |
| `BRAIND_SYNC_ENABLED` | `true` | Supervise Obsidian Headless. Useful for local tests or maintenance. |
| `BRAIND_OB_PATH` | `ob` | Executable path; fixed by the image in production. |
| `BRAIND_MAX_NOTE_BYTES` | `2MiB` | Maximum editable Markdown file size. |
| `BRAIND_SEARCH_MAX_FILES` | `10000` | Maximum Markdown files examined by one on-demand search. |
| `BRAIND_SEARCH_MAX_BYTES` | `256MiB` | Maximum aggregate bytes read by one search. |
| `BRAIND_SEARCH_TIMEOUT` | `10s` | Wall-clock limit for one search request. |
| `BRAIND_SEARCH_MAX_RESULTS` | `100` | Maximum matches returned by one search. |
| `BRAIND_GIT_BACKUP_ENABLED` | `true` | Enable the managed one-way Git backup workflow. |
| `BRAIND_GIT_DIR` | `/data/git/vault.git` | Git metadata directory, kept outside the synchronized vault worktree. |
| `BRAIND_GIT_REMOTE_URL` | required when backup enabled | Private backup repository URL. Never include credentials in this value. |
| `BRAIND_GIT_BRANCH` | `main` | Single remote backup branch. |
| `BRAIND_GIT_BACKUP_INTERVAL` | `6h` | Maximum interval between automatic backup attempts after initial synchronization. |
| `BRAIND_GIT_QUIET_PERIOD` | `30s` | Desired vault quiet period before briefly quiescing writers for a commit. |
| `BRAIND_GIT_SSH_KEY_PATH` | `/run/secrets/git/ssh-privatekey` | Read-only deploy-key path; not stored on the PVC or placed in an argument. |
| `BRAIND_GIT_KNOWN_HOSTS_PATH` | `/run/secrets/git/known_hosts` | Pinned SSH host-key file; strict host verification is mandatory. |
| `BRAIND_LOG_LEVEL` | `info` | Structured log level. |
| `OIDC_ENABLED` | `true` | Require OIDC authentication. May be disabled only under the guarded local-development policy. |
| `OIDC_CONFIGURATION_URL` | required | Exact HTTPS OpenID Provider configuration URL. Its returned issuer must match every accepted token's `iss`. |
| `OIDC_CLIENT_ID` | required | Registered relying-party client ID, normally sourced from a Kubernetes Secret. |
| `OIDC_CLIENT_SECRET` | required for confidential clients | Client secret sourced from a Kubernetes Secret. Never logged or shown in diagnostics. |
| `OIDC_GROUPS_CLAIM` | `groups` | Exact top-level claim name containing the array of group strings. |
| `OIDC_GROUPS_SOURCE` | `id_token` | Read group membership from `id_token` or `userinfo`; never merge both. |
| `OIDC_ADMIN_GROUP` | required | Exact group granting `admin`. |
| `OIDC_USER_GROUP` | required | Exact group granting `user`. |
| `OIDC_VIEWER_GROUP` | required | Exact group granting `viewer`. |
| `OIDC_USER_CLAIM` | `email` | Claim displayed as the username; not an authorization key. |
| `OIDC_PROVIDER_NAME` | derived from issuer host | Human-readable label used on login UI, such as `Pocket ID`. |
| `OIDC_AUTO_REDIRECT` | `false` | Redirect unauthenticated page navigation directly to OIDC instead of showing a login page. Does not change API `401` behavior. |
| `OIDC_EXTRA_SCOPES` | `groups` for Pocket ID | Provider-specific scopes needed to release the configured group claim. |
| `OIDC_API_BEARER_ENABLED` | `false` | Accept OIDC JWT access tokens for programmatic API clients in addition to browser sessions. |
| `OIDC_API_AUDIENCE` | required when bearer enabled | Exact audience required in API access tokens; ID-token audience is not reused implicitly. |
| `BRAIND_SESSION_IDLE_TIMEOUT` | `1h` | Maximum inactivity before local session expiry. |
| `BRAIND_SESSION_ABSOLUTE_LIFETIME` | `8h` | Maximum local session lifetime regardless of activity. |
| `OIDC_REAUTH_INTERVAL` | `1h` | Maximum interval before group membership is re-evaluated. Must not exceed absolute lifetime. |
| `BRAIND_TRUSTED_PROXY_CIDRS` | empty | Proxies allowed to supply forwarding headers used for request metadata; identity still comes from OIDC. |

Configuration is validated before filesystem mutation. Effective non-secret configuration appears in diagnostics. Runtime web editing of process configuration is deferred; immutable pod configuration is easier to reason about.

## 12. Observability and health

### 12.1 Structured logs

Logs go to stdout/stderr as structured records. Fields include timestamp, level, component, message, request ID, logical note path where safe, child exit code, and duration. Never log request bodies, note contents, cookies, authorization headers, or `ob` credential arguments.

Security-relevant events use a consistent audit shape: login success/failure category, logout, access denial, resolved role, note mutation, sync recheck, and administrative diagnostics access. Identity is recorded as a one-way keyed pseudonym derived from `(issuer, sub)` unless operators explicitly enable a less private identifier. Raw tokens, group arrays, authorization codes, and client secrets are never logged.

### 12.2 Status model

Expose a single snapshot containing at least:

- build version/commit and uptime;
- OIDC discovery state, issuer, client ID, provider label, configured groups-claim name/source, configured role group names, and active session count, with no secrets or raw user claims;
- vault state: unavailable, scanning, ready, read-only, or degraded;
- last on-demand search duration/files/bytes/truncation for aggregate operational diagnostics, without query text;
- sync state: disabled, checking, needs-authentication, needs-setup, starting, running, backoff, or stopped;
- child PID, start time, exit code, and restart count;
- last sanitized sync error and output tail;
- detected `ob` version;
- Git backup state, last successful commit ID/time, last push time, pending-local-commit count, next scheduled attempt, and a sanitized error without remote credentials or note paths.

### 12.3 Probes

`GET /healthz` returns `200` if the Go process's core event loop is responsive. It must not touch the network, scan the vault, or invoke `ob`.

`GET /readyz` returns `200` after the vault access check completes and while the application can serve its configured read/write mode. It returns `503` during startup, shutdown, or inaccessible storage. It remains `200` when Obsidian Sync or OIDC discovery is temporarily offline.

The Kubernetes design uses:

- a startup probe with enough time for volume attachment and the non-recursive vault access check;
- a liveness probe against `/healthz` with conservative thresholds;
- a readiness probe against `/readyz`;
- no shell-based exec probes.

### 12.4 Metrics

Prometheus metrics are desirable but can follow the basic status/logging implementation. Candidate counters and gauges include HTTP requests/duration, login outcomes, authorization denials by required role, active sessions, note mutation outcomes, on-demand search duration/files/bytes/truncation, sync process up, restarts, seconds since the child last started/exited, backup attempts/failures, and seconds since the last successful push. Metrics must not label by identity, group, note path, query, remote URL, or error text because of privacy and cardinality.

## 13. Kubernetes design

The selected cluster is k3s `v1.36.2+k3s1` with Longhorn `v1.12.1`, the bundled Traefik ingress controller, and cert-manager. Its default `longhorn` StorageClass allows expansion, uses ext4 and the v1 data engine, requests three replicas, and currently has `reclaimPolicy: Delete`; StatefulSet PVC retention therefore remains mandatory. Longhorn 1.12.1 explicitly supports `ReadWriteOncePod`, so the claim uses that access mode and an initial 10 GiB request. `ReadWriteOnce` is not used as a fallback. The repository ships the single-replica ValidatingAdmissionPolicy and binding as a separate optional cluster-scoped manifest; applying it is recommended wherever the deployer has permission.

### 13.1 Manifest deliverable

Implementation should add a plain, reviewable manifest at `deploy/k8s/braind.yaml`. It should not require Helm for the first deployment. It targets namespace `brain`, `ingressClassName: traefik`, hostname `brain.lab.johnrowley.co`, `storageClassName: longhorn`, TLS Secret `braind-tls`, and the existing cert-manager ClusterIssuer `letsencrypt-cloudflaredns-production`. Kustomize support can be added later without changing the base resource contract.

The manifest contains:

1. the `brain` Namespace;
2. a headless Service governing StatefulSet identity;
3. a ClusterIP HTTP Service;
4. a standard `networking.k8s.io/v1` Ingress for `brain.lab.johnrowley.co` with `ingressClassName: traefik`, `cert-manager.io/cluster-issuer: letsencrypt-cloudflaredns-production`, and `braind-tls` TLS Secret;
5. the one-replica StatefulSet and its Longhorn volume claim template, with `imagePullSecrets: [{name: regcred}]`, non-secret OIDC/backup settings, `secretKeyRef` entries for OIDC credentials, and a read-only Secret mount for Git SSH credentials;
6. a default-deny/allowlisted NetworkPolicy if the installed k3s CNI enforces it;
7. a separately documented ValidatingAdmissionPolicy and binding for Kubernetes 1.30+ clusters.

It does **not** create or embed credential values, the ClusterIssuer, `regcred`, the Git remote repository, or Git credentials. It creates the `brain` namespace, references the required OIDC/Git Secrets and `regcred`, and relies on cert-manager to create/renew `braind-tls` from `letsencrypt-cloudflaredns-production`. The selected ClusterIssuer was observed Ready in the target cluster during design validation.

### 13.2 StatefulSet shape

The required workload properties are:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: braind
spec:
  serviceName: braind-headless
  replicas: 1
  podManagementPolicy: OrderedReady
  updateStrategy:
    type: RollingUpdate
  persistentVolumeClaimRetentionPolicy:
    whenDeleted: Retain
    whenScaled: Retain
  selector:
    matchLabels:
      app.kubernetes.io/name: braind
  template:
    # labels, security context, one braind container, probes, resources
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: [ReadWriteOncePod]
        resources:
          requests:
            storage: 10Gi
```

This is design-shape YAML, not yet the deployable manifest. Image, labels, ports, mount, environment, resource requests, security contexts, and policies must be filled out and validated during implementation.

With one replica, StatefulSet rolling update semantics terminate the old ordinal before creating its replacement; there is no Deployment-style surge. `OrderedReady` is retained for explicitness. `persistentVolumeClaimRetentionPolicy: Retain` prevents workload deletion or an accidental scale operation from automatically deleting vault storage. Cluster support for that stable field must be included in the documented minimum Kubernetes version.

### 13.3 Enforcing “one pod maximum”

`replicas: 1` expresses desired state but does not prevent an authorized user or controller from changing it to 2. The complete safety model therefore has layers:

1. The checked-in StatefulSet specifies exactly one replica and no HPA.
2. The PVC requests `ReadWriteOncePod`, providing single-pod attachment for that particular claim when supported by the CSI driver.
3. `braind` takes an exclusive lock on its mounted data root.
4. Production clusters on Kubernetes 1.30+ should apply a narrowly scoped `ValidatingAdmissionPolicy` that denies create/update when the `braind` StatefulSet's `spec.replicas` is not exactly 1.
5. GitOps/CI checks reject a manifest with any other replica count.

The admission expression should scope by an immutable, project-specific label and namespace, then require `object.spec.replicas == 1`. Cluster-scoped admission policy is kept in a separate optional file because many application deployers cannot install cluster-scoped resources.

A subtle limit remains: `volumeClaimTemplates` creates a different claim for each ordinal. If someone bypasses admission and scales to 2, `braind-1` could receive a new empty PVC and independently connect to the same remote vault. `ReadWriteOncePod` alone does not prevent that. The admission policy is therefore the control that enforces the user's literal maximum; the filesystem lock protects only a shared claim.

### 13.4 Storage

- Required access mode: `ReadWriteOncePod`, supported by current Longhorn releases.
- StorageClass: `longhorn`; the observed Longhorn 1.12.1 StorageClass is default, has expansion enabled, uses ext4/v1 data engine, and requests three replicas.
- Production requires `ReadWriteOncePod`; fail manifest validation or deployment prerequisites rather than silently weakening it to `ReadWriteOnce`.
- Initial request: 10 GiB, configurable before deployment and expandable where the StorageClass supports expansion.
- Mount the claim once at `/data`; do not split auth and vault into ephemeral volumes.
- Retain the PVC when the StatefulSet is deleted or scaled.
- Git backup metadata and local commits live on the same claim under `/data/git`; the remote Git push is the durable off-cluster backup. Optional Longhorn snapshots are operational rollback aids, not the backup of record.

Running the container with a read-only root filesystem is desirable. Writable locations needed by Node/`ob` must be identified during image testing and placed under `/data` or small `emptyDir` mounts such as `/tmp`. `HOME` is explicitly `/data/home`.

### 13.5 Image build and Kubernetes rendering with `ko`

Using [`ko`](https://ko.build/) makes sense for `braind`: the application is a Go command, the deployment is Kubernetes-first, and `ko` can compile the binary, publish an OCI image, generate an SPDX SBOM, and replace a `ko://` image reference in Kubernetes YAML with an immutable digest.

There is one important qualification. A normal `ko` image is ideal for a self-contained Go binary, while `braind` also needs Node.js 22 and the `obsidian-headless` npm package. `ko` does not run arbitrary package-install steps while assembling the application layer. The preferred design is therefore:

1. Build a small, separately maintained runtime base image containing Node.js 22+, Git and SSH clients, CA certificates, the exact pinned `obsidian-headless` version, its license material, the fixed non-root user, and no `braind` binary.
2. Publish and vulnerability-scan the base image as `ghcr.io/johnrowl/braind-runtime`, then pin it by digest in `.ko.yaml` using `defaultBaseImage` or an import-path-specific `baseImageOverrides` entry.
3. Let `ko` compile `./cmd/braind` and append the Go binary plus `kodata` assets such as templates, vendored HTMX, CSS, and icons.
4. Use `ko resolve` in CI to emit a reviewable release manifest with the `ko://github.com/johnrowl/brain/cmd/braind` reference replaced by the built image digest. `ko apply` is convenient for disposable/local clusters; production GitOps should apply the resolved, archived manifest rather than building during reconciliation.

The intended configuration shape is:

```yaml
# .ko.yaml
defaultBaseImage: ghcr.io/johnrowl/braind-runtime@sha256:<pinned-digest>
defaultPlatforms:
  - linux/amd64
  - linux/arm64
builds:
  - id: braind
    main: ./cmd/braind
    ldflags:
      - -s
      - -w
      - -X main.version={{.Env.VERSION}}
      - -X main.commit={{.Git.FullCommit}}
```

The exact ldflag package path will be chosen with the implementation; the example expresses the build contract, not current code. CI pins the `ko` tool version, sets `KO_DOCKER_REPO=ghcr.io/johnrowl`, preserves default SPDX SBOM generation, sets OCI source/revision metadata, builds only from a clean checkout for releases, and records both the runtime-base digest and final application digest.

Both the runtime base and final `ghcr.io/johnrowl/braind` image support `linux/amd64` and `linux/arm64`. A release publishes one multi-platform OCI manifest to private GHCR, tagged by semantic version and source revision, while the Kubernetes release manifest references its immutable digest and uses an `imagePullSecret`. CI also retains per-platform OCI archives for amd64 and arm64 with checksums as private release artifacts; these are recovery/offline artifacts, not separate mutable deployment tags.

The current `obsidian-headless` npm package declares its license as `UNLICENSED`. The operator has explicitly confirmed that this private, operator-controlled GHCR packaging does not count as redistribution under the applicable Obsidian terms; the design therefore permits the private runtime image and private release archives. This is recorded as an operator-supplied deployment assumption rather than an independent legal conclusion. Recheck it when the package version, registry audience, or terms change, and do not make the image public without a separate review.

This hybrid still means the **application image is built by `ko`**. A small Dockerfile or equivalent build definition remains necessary for the Node/Obsidian runtime base unless a trustworthy upstream image with the exact required contents is available. Trying to hide Node and npm dependencies in `kodata` would make updates, licenses, native modules, and vulnerability scanning harder to reason about.

The feasibility spike in Phase 0 must prove that `ko` preserves the intended entrypoint, user, multi-platform compatibility, Node executable, and `ob` native-module behavior. If it fails for a documented technical reason, a conventional multi-stage image is the fallback; convenience alone is not sufficient reason to abandon `ko`.

### 13.6 Container and pod security

The final pod/container security context should include:

- `runAsNonRoot: true` with fixed UID/GID owned by the image;
- pod `fsGroup` compatible with dynamically provisioned storage;
- `allowPrivilegeEscalation: false`;
- all Linux capabilities dropped;
- `seccompProfile.type: RuntimeDefault`;
- read-only root filesystem if CLI behavior permits it;
- no service-account token mount (`automountServiceAccountToken: false`);
- no privileged mode, host namespaces, host paths, or Kubernetes RBAC permissions.

The image should be Debian/Ubuntu slim rather than Alpine unless Obsidian Headless is explicitly tested on musl. It contains CA certificates, timezone data if required, Node.js 22+, the pinned npm package, and the statically or dynamically linked Go binary. Build metadata and licenses should be retained.

The OIDC client ID/secret enter the container through explicit `secretKeyRef` environment entries. Claim and role-group names are ordinary environment values. None are copied to the PVC or image, and the process must not include its environment in diagnostics.

### 13.7 Services and network policy

The governing headless Service has `clusterIP: None` and selects the pod. The HTTP ClusterIP Service exposes port 80 to container port 8080. Neither is public.

The NetworkPolicy should, when enabled:

- allow inbound HTTP only from the ingress/proxy namespace and probe source as required by the cluster;
- allow DNS egress;
- allow HTTPS egress needed by Obsidian Sync and the configured OIDC issuer/JWKS/UserInfo endpoints;
- allow TCP/22 egress for the configured Git SSH remote, narrowed to provider addresses where the installed CNI can maintain that policy safely;
- deny other ingress and unnecessary egress.

Obsidian and Git-provider endpoint details should be verified before hard-coding address policy because Kubernetes NetworkPolicy natively filters IPs/ports, not domain names, and service addresses may change. If provider IP ranges cannot be maintained safely, allowing the required outbound port is an explicit limitation of standard NetworkPolicy rather than pretending hostname filtering exists.

### 13.8 Resources, scheduling, and disruption

Start with conservative resource requests and measured limits rather than guesses baked into this design. Node plus `ob` is a second runtime in the same container, and concurrent on-demand searches consume filesystem bandwidth and transient buffers. A reasonable implementation exercise is to benchmark small, medium, and large representative vaults before choosing defaults and search concurrency.

Do not set a PodDisruptionBudget with `minAvailable: 1`; it can block voluntary maintenance for a service that has no possible second replica. Short downtime during node drain and rollout is an accepted consequence of strict single-instance operation.

Use `revisionHistoryLimit`, termination grace period, and standard topology/scheduling configuration, but do not add anti-affinity or spread constraints for a one-pod workload.

### 13.9 Upgrade and rollback behavior

A rollout causes downtime:

1. old `braind-0` becomes unready and terminates its sync child;
2. the PVC detaches/reattaches as required;
3. new `braind-0` starts against the retained claim;
4. the vault access check completes and readiness returns;
5. continuous sync resumes from persisted CLI/vault state.

Rollback means restoring the previous image, not restoring the PVC. Any release that changes on-disk application metadata must use forward/backward-compatible formats or an explicit backed-up migration. V1 should avoid durable application metadata beyond CLI-managed state and vault files.

### 13.10 Backup and restore policy

Obsidian Sync protects synchronization/version history but is not the backup of record. The selected backup is a private remote Git repository containing the complete vault worktree and its history. Longhorn remote backup is not required by this design. A Longhorn snapshot may still be taken before upgrades as a convenient same-cluster rollback point, but it does not replace the Git push and is not expected to survive loss of the Longhorn cluster.

#### 13.10.1 Repository layout and scope

The vault at `/data/vault` is the Git worktree. Repository metadata is kept separately at `/data/git/vault.git` and every command supplies the fixed `--git-dir` and `--work-tree` arguments. No `.git` directory or pointer file is placed inside the vault. This prevents Git objects, refs, locks, and configuration from entering Obsidian Sync, search results, or note paths.

The backup stages all changes with the equivalent of `git add -A`: additions, modifications, renames as inferred by Git, and deletions. It intentionally backs up every file in the vault, including `.obsidian` configuration and attachment/binary files, with no generated `.gitignore`. It does not back up `/data/home`, Obsidian account credentials, OIDC secrets, Git credentials, or application runtime state. After total PVC loss, the vault can be restored from Git but Obsidian Headless must be authenticated again through the bootstrap flow.

Git stores large binary history inefficiently. The current approximately 2,000-file vault is a reasonable starting size, but Phase 0 must record total bytes, largest files, initial pack size, and commit/push duration. If large binary history becomes material, Git LFS or a second blob-backup mechanism requires a separate design; files must never be silently omitted from backup.

#### 13.10.2 Initialization and credentials

When backup is enabled, bootstrap requires a configured private remote and an empty or explicitly adopted branch. `braind` initializes the separate Git directory, configures the vault worktree, creates `main`, sets a fixed non-personal author such as `braind Backup <braind@localhost>`, creates the first commit only after the initial Obsidian synchronization reaches a quiescent state, and pushes with upstream tracking. It does not create the remote repository through a provider API.

The preferred authentication is a repository-scoped SSH deploy key mounted read-only from a Kubernetes Secret, plus a pinned `known_hosts` file and strict host-key checking. The remote URL never contains a password or token. Credentials, remote query strings, and command environment are excluded from logs and diagnostics. The deployment convention is a Secret named `braind-git-credentials` with `ssh-privatekey` and `known_hosts` keys; a different secret name may be wired at deployment time without changing application behavior.

The remote branch is append-only from `braind`'s perspective. It never runs pull, merge, rebase, reset, or force-push. A non-fast-forward push means another writer modified the backup branch; backup enters a blocked/degraded state and requires administrator reconciliation. This prevents an automated backup job from discarding or combining unexpected remote history.

#### 13.10.3 Backup transaction

The default maximum interval between attempts is six hours, with an administrator-only “back up now” action using the same state machine. Only one backup may run at a time. A successful backup is defined as a commit, if changes exist, followed by confirmation that every pending local backup commit is present on the configured remote branch. A no-change run still attempts to push previously unpushed commits.

To produce a coherent commit while Obsidian Sync and later browser mutations share the filesystem, the coordinator:

1. waits for the configured quiet period when possible;
2. acquires the application-wide vault write lock, causing new mutations to wait or return a retryable response;
3. gracefully stops the continuous `ob sync` child and waits for its exit;
4. stages the entire worktree and creates a commit with a fixed `backup: <UTC timestamp>` message when the index differs from `HEAD`;
5. releases the vault lock and immediately restarts continuous sync;
6. pushes the local branch separately, so a slow or unavailable Git host does not keep Sync stopped.

Git is executed directly with fixed argument vectors, never through a user-controlled shell. Commit and push timeouts are bounded. If staging or commit fails, continuous sync is restarted before retry/backoff. If push fails, the local commit remains on the PVC and later runs retry it before or along with newer commits. Backup failure is visible but does not fail HTTP readiness, stop local reads, or continuously restart the pod.

This transaction is application-consistent with respect to the two supported writers because both are quiesced during staging/commit. An abrupt container or node failure may interrupt the operation, but Git lock cleanup and repository integrity checks run at startup before another attempt. `braind` never automatically deletes a stale Git lock unless it can prove no Git child remains and the repository passes the documented recovery checks.

#### 13.10.4 Restore and validation

A restore never checks out historical data over the live vault first. The runbook creates a new PVC or isolated worktree, checks out the selected remote commit with Sync and application mutations disabled, and verifies repository integrity, expected file counts, representative note contents, Markdown search, and attachment checksums where relevant. Quarterly test restores establish that the remote is usable and measure recovery time.

After validation, the operator deliberately switches the StatefulSet to the restored data and re-runs Obsidian authentication/setup if the PVC was replaced. Bidirectional Sync remains disabled until the operator chooses how the historical Git state should interact with the newer Obsidian remote. Starting Sync blindly after restoring an old commit could upload old content or reintroduce deletions, so that choice is never automated.

The Git repository must be private and protected like the vault itself. Provider-side encryption and access controls are the minimum; client-side encryption is a future option if the chosen Git host is not trusted with plaintext vault contents. Remote retention is the Git commit graph: ordinary history is never pruned or force-rewritten by `braind`.

## 14. Failure scenarios

| Scenario | Expected behavior | Operator signal |
| --- | --- | --- |
| OIDC issuer/JWKS temporarily unavailable | Existing unexpired local sessions continue; new login/reauthentication fails closed and discovery retries with backoff. Vault and sync remain available to existing sessions. | Login error without provider internals, admin status, logs/metrics. |
| Authenticated identity has no mapped group | No authorized application session is created; return the forbidden page. | `403`, privacy-safe audit event. |
| Identity belongs to several mapped groups | Resolve the single highest role (`admin > user > viewer`). | Own-account display shows effective role; admin diagnostics show mapping metadata. |
| Group membership is removed | Access ends at logout, periodic reauthentication, or absolute session expiry, whichever occurs first. | Configured revocation-latency documentation and audit events. |
| Obsidian service/network unavailable | HTTP remains ready; local reads/writes continue; child retries with backoff. | Dashboard warning, logs, sync status/backoff. |
| Authentication expires | Child stops/retries slowly; UI shows `needs-authentication`; vault remains local. | Bootstrap instructions and last sanitized error. |
| E2EE/setup missing | No continuous child; UI shows `needs-setup`; a freshly reauthenticated administrator can use the one-time bootstrap workflow. | Sanitized bootstrap status and errors. |
| PVC cannot mount | Pod remains Pending; no second pod can use a `ReadWriteOncePod` claim. | Kubernetes events. |
| Vault becomes unreadable | Readiness fails; liveness stays healthy so the process can report/recover. | `/readyz`, dashboard, logs. |
| Git remote is unavailable | Local backup commit remains on the PVC; push retries with backoff while reads and Sync continue. | Backup status, pending commit count, sanitized error. |
| Git push is non-fast-forward | Never pull, merge, rebase, reset, or force-push automatically; enter blocked backup state. | Admin action required with local/remote commit IDs. |
| Git staging/commit fails | Restart Sync, preserve the previous valid repository state where possible, and retry only after integrity checks. | Backup degraded state and privacy-safe diagnostics. |
| File changes during browser edit | Save returns `409`; neither version is silently lost. | Conflict view with both versions. |
| On-demand search reaches a limit | Return bounded partial results marked `truncated`; do not continue consuming disk/CPU after disconnect or deadline. | Search response metadata and aggregate metrics. |
| `ob` crashes repeatedly | Backoff capped; HTTP stays up. | Restart count, exit code, output tail. |
| Container is killed ungracefully | Atomic note writes avoid partial destination files; next boot checks the vault and resumes sync without rebuilding derived state. | Previous pod status/logs and startup status. |
| StatefulSet is deleted | PVC remains because retention is `Retain`. | Orphaned PVC visible to operator. |
| StatefulSet is accidentally scaled | Admission policy rejects the update in protected clusters. Without it, this is unsafe and unsupported. | Admission error/GitOps drift alert. |
| Disk fills | Mutation fails without truncating existing note; sync may fail; readiness becomes degraded/unready based on inability to meet configured write mode. | UI alert, metrics/logs, PVC usage monitoring. |

## 15. Testing strategy

### 15.1 Unit tests

- OIDC callback validation, state/nonce/PKCE lifecycle, issuer/audience rejection, session rotation/expiry, and secret redaction using a fake provider;
- group-claim parsing and exact mappings, malformed claims, unknown groups, multi-group precedence, and deny-by-default behavior;
- authorization matrix coverage for every registered route, response mode, and HTTP method;
- API schema validation, problem responses, cursor bounds, strong ETags, and `If-Match` preconditions;
- path normalization, traversal, symlink, Unicode, and collision cases;
- configuration validation and secret redaction;
- optimistic concurrency and atomic writer behavior;
- Markdown sanitization and link rewriting;
- full-page/HTMX fragment negotiation and status codes;
- sync supervisor state transitions, backoff, cancellation, and output redaction using a fake executable;
- bootstrap state-machine authorization, fresh-login requirement, single-flight behavior, failure cleanup, and credential redaction;
- Git backup state transitions, fixed argument construction, quiescing/restart behavior, no-change and pending-push cases, non-fast-forward blocking, timeout/backoff, and credential/remote redaction using a fake Git executable;
- bounded on-demand directory/search traversal, cancellation, truncation, hidden-path skipping, and malformed-file handling.

### 15.2 Integration tests

- run against an ephemeral standards-compliant OIDC test provider and exercise login, logout, role changes, expired signing keys, key rotation, and provider outage;
- verify a viewer cannot mutate or fetch admin fragments/diagnostics, a user cannot invoke admin actions, and an admin receives no command-execution capability;
- run the OpenAPI conformance suite and exercise every `/api/v1` operation with session authentication; exercise bearer authentication separately while it remains disabled by default;
- run the server against a temporary vault and exercise create/read/edit/conflict/rename/trash;
- replace `ob` with a controllable fake child to test long-running behavior and signal propagation;
- restart `braind` against the same data directory and verify stateless vault-query behavior and sync-state recovery;
- initialize a temporary remote, commit every vault path including additions/deletions/binary files, retry an interrupted push, reject divergent remote history, and restore a selected commit into a distinct empty worktree;
- test read-only mode rejects every mutation path;
- scan a corpus with malformed Markdown/frontmatter, large notes, binary files, symlinks, and unusual filenames;
- run browser-level tests with normal navigation and HTMX enabled.

### 15.3 Image and Kubernetes tests

- build the application image with the pinned `ko` version and custom runtime base; verify the result is digest-addressed and has an SPDX SBOM;
- verify the GHCR manifest and retained OCI archives contain working `linux/amd64` and `linux/arm64` images;
- verify Node and pinned `ob` versions in the built image;
- run as the configured non-root UID with read-only root filesystem settings;
- deploy to a disposable cluster, bootstrap a test vault, restart the pod, and verify auth/setup persistence;
- verify the Traefik Ingress, cert-manager-created `braind-tls` Secret, private GHCR pull Secret, and Pocket ID callback path in a k3s test environment;
- supply OIDC credentials from a pre-created Secret and claim/group names through environment values; verify secret values do not appear in rendered manifests, logs, or the PVC;
- update and roll back the image while observing that old/new pods do not overlap;
- validate probes and graceful child termination;
- confirm PVC retention on StatefulSet deletion;
- confirm a `ReadWriteOncePod` claim and admission policy block the intended unsafe cases;
- restore the private Git backup into a distinct test PVC with sync disabled and validate repository integrity, file counts, representative notes, and binary checksums;
- lint and server-side dry-run the manifest against the documented minimum Kubernetes version.

Real Obsidian end-to-end tests should use a dedicated account/vault, never a personal production vault, and should be opt-in because they require a subscription, network, and credentials.

## 16. Delivery phases

The PR-sized execution backlog, dependencies, and acceptance criteria for these phases live in [`docs/braind-tasks.md`](braind-tasks.md). The phases below remain the product-level release gates.

### Phase 0: contracts and spikes

- Confirm the official CLI's exit behavior, signal handling, output, persistent paths, and setup detection in a disposable environment.
- Test interactive stdin/PTY bootstrap and the documented flag fallback, including MFA/E2EE cases and command-line/log exposure.
- Build a custom Node/Headless runtime base and use it from `ko`; test the pinned Headless version, `kodata`, non-root user, SBOM, and target platforms.
- Verify the Pocket ID `groups` scope and ID-token array against the deployed instance, plus fresh-login and logout behavior.
- Confirm the installed Longhorn version provisions `ReadWriteOncePod` claims through the `longhorn` StorageClass.
- Measure the approximately 2,000-file vault's total bytes, largest files, Git pack size, initial commit/push time, and bounded-search latency using a non-production copy.
- Exercise the quiesced Git commit/push transaction and a full isolated restore with credentials mounted from a Secret.
- Record the operator-confirmed `obsidian-headless` private-packaging assumption and ensure release automation cannot publish the images publicly.
- Prototype safe Markdown rendering and filesystem atomic replacement without building the product UI.

Exit criterion: unknowns that could invalidate the process/storage model are resolved and recorded in this document.

### Phase 1: local read-only daemon

- Configuration, OIDC login/logout, sessions, group mappings, route authorization, HTTP lifecycle, health/readiness, vault access check, and bounded on-demand search.
- Explorer, note view, search, dashboard.
- Read-only `/api/v1` status, note, search, and OpenAPI endpoints.
- Sanitized standard Markdown plus plain-source reading; no attachment endpoint.
- Sync supervisor with fake-process tests, then real `ob` status/continuous operation.
- Admin-only web bootstrap and verification of the bidirectional sync-everything policy.
- Separate-metadata Git repository initialization, scheduled/admin-triggered quiesced commits, private remote push, backup status, and isolated restore test.

Exit criterion: a local daemon can safely serve and continuously synchronize a test vault.

### Phase 2: safe mutation

- Edit/create with revision tokens and atomic writes.
- Rename and persisted Linux system-trash behavior.
- Conflict UI and strict read-only mode.
- JSON API mutations with strong ETags, required preconditions, problem responses, and session/bearer authorization.
- CSRF and complete browser security headers.

Exit criterion: externally synchronized edits cannot be silently overwritten in tested races.

### Phase 3: container and Kubernetes

- `ko`-built immutable application image using the pinned Node/Headless runtime base, with SBOM and digest provenance.
- Publish multi-platform runtime/application images to GHCR and retain checked OCI archives for amd64 and arm64.
- `deploy/k8s/braind.yaml`, Services, Ingress for `brain.lab.johnrowley.co`, StatefulSet, 10 GiB `ReadWriteOncePod` PVC, `regcred`, Git credential Secret mount, and NetworkPolicy.
- parameterized OIDC claim/group environment settings and existing-Secret references, with provider-specific setup instructions.
- Optional single-replica ValidatingAdmissionPolicy manifest.
- Bootstrap, backup, restore, upgrade, rollback, and troubleshooting runbooks.

Exit criterion: a fresh disposable cluster deployment can be bootstrapped, restarted, upgraded, and restored from backup.

### Phase 4: hardening and operations

- Metrics, performance benchmarks, scan limits, load tests.
- Dependency/SBOM and image scanning.
- Recovery exercises for full disk, corrupt files, expired auth, and truncated/cancelled searches.
- Accessibility and browser compatibility pass.

## 17. Decisions and open questions

### 17.1 Decisions made by this proposal

- One vault, one OIDC provider, multiple authenticated people, and three hierarchical roles per daemon.
- Native Authorization Code + PKCE OIDC, server-side sessions, a configurable groups-claim name, exact admin/user/viewer group settings, and no implicit access for an unmapped identity.
- Pocket ID requests `openid profile email groups`; `groups` is read from the ID token by default, with UserInfo as a fallback.
- A versioned `/api/v1` ships from the beginning and shares services, authorization, and concurrency rules with the HTMX application.
- V1 API access is on behalf of people through browser sessions; application-issued API tokens are deferred.
- One Go process supervises one `ob` child; no sync sidecar.
- Filesystem is canonical; v1 has no background, in-memory vault-wide, or persistent index.
- Server-rendered HTML plus HTMX, with ordinary HTTP fallback.
- The selected deployment uses bidirectional sync with every documented file/config category and no excluded folders; pull/mirror configurations remain read-only.
- Normal first-time Obsidian authentication/setup uses a fresh-admin-authenticated bootstrap page, not `kubectl exec`, and persists only CLI-managed state in the dedicated PVC home.
- The first milestone is read/search-only, uses bounded on-demand filesystem search with no index, renders sanitized standard Markdown without explicit Obsidian extensions, and has no attachment endpoint.
- Later deletion follows Obsidian's default system-trash behavior using the persisted Linux/XDG trash; it never falls back to permanent unlink.
- `ko` is the preferred Go application image builder and Kubernetes image resolver, using a separately built pinned Node/Obsidian runtime base.
- The runtime base and application image publish to GHCR as linux/amd64 and linux/arm64 multi-platform images, with per-platform OCI archives retained.
- The observed target is k3s `v1.36.2+k3s1` with Longhorn `v1.12.1`; its default expandable `longhorn` StorageClass uses ext4, the v1 data engine, and three replicas. The StatefulSet uses one replica, a retained 10 GiB `ReadWriteOncePod` claim, no HPA, and separately shipped admission enforcement.
- Namespace `brain`; k3s Traefik Ingress serves `https://brain.lab.johnrowley.co`, with cert-manager managing `braind-tls` through the Ready ClusterIssuer `letsencrypt-cloudflaredns-production`.
- The vault is a Git worktree with metadata outside the synchronized tree; every vault file is staged, committed, and pushed to a private append-only backup branch every six hours and on an admin request after quiescing supported writers.
- The private GHCR image uses `regcred`. Git SSH credentials are mounted separately from a narrowly scoped Secret and never embedded in the remote URL.
- The operator has confirmed private operator-controlled packaging of Obsidian Headless is not redistribution under the applicable terms; release automation must remain private and the assumption is revisited when circumstances change.
- Sync outage does not fail readiness; inaccessible vault storage does.
- No embedded credential Secret values in the base manifest; it references operator-created OIDC, image-pull, and Git credential Secrets.

### 17.2 Questions to resolve before implementation

1. **Git remote:** What private repository URL/provider should receive backups, and will its `main` branch be empty for first initialization or contain history that must be explicitly adopted?
2. **Git credentials:** Confirm SSH deploy-key authentication and the proposed `braind-git-credentials` Secret convention, or select another non-interactive credential mechanism.
3. **Backup confidentiality:** Is the private Git provider trusted to store plaintext vault content with provider-side encryption/access controls, or is client-side encryption required?
4. **Backup cadence:** Confirm the proposed six-hour automatic interval plus administrator-triggered backups. Git history is retained indefinitely unless an explicit pruning policy is designed later.
5. **Vault measurements:** The roughly 2,000-file count fits the proposed 10,000-file search ceiling, but total bytes, largest-file size, and initial Git pack/push measurements still need to be recorded from a non-production copy.

These are product/deployment choices, not reasons to weaken the lifecycle, path-safety, single-instance, or optimistic-concurrency requirements.

## 18. Reference material

- [Obsidian Headless](https://obsidian.md/help/headless) — installation, Node.js requirement, and authentication commands.
- [Obsidian Headless Sync](https://obsidian.md/help/sync/headless) — setup, continuous sync, status, and sync configuration.
- [Kubernetes StatefulSets](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) — identity, volume claim templates, and rolling-update ordering.
- [Kubernetes persistent volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) — access mode semantics and `ReadWriteOncePod`.
- [Kubernetes Validating Admission Policy](https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/) — optional enforcement of the one-replica invariant on Kubernetes 1.30+.
- [`ko` configuration](https://ko.build/configuration/) — custom base images, build settings, and multi-platform defaults.
- [`ko` Kubernetes integration](https://ko.build/features/k8s/) — `ko://` image references, `ko resolve`, and `ko apply`.
- [`ko` SBOMs](https://ko.build/features/sboms/) — default SPDX software-bill-of-material generation.
- [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html) — relying-party flows, token validation, and standard versus additional claims.
- [Pocket ID APIs and Permissions](https://pocket-id.org/docs/guides/apis) — user-delegated resource/audience access tokens and API permission validation.
- [Pocket ID Allowed User Groups](https://pocket-id.org/docs/configuration/allowed-groups) — restricting which groups may authenticate to an OIDC client.
- [Pocket ID Harbor example](https://pocket-id.org/docs/client-examples/harbor) — documented `groups` scope/claim and email username configuration.
- [Pocket ID maintainer answer on group claims](https://github.com/pocket-id/pocket-id/discussions/275) — requesting the `groups` scope returns group names in both the ID token and UserInfo response.
- [Longhorn volume access modes](https://longhorn.io/docs/1.12.1/nodes-and-volumes/volumes/create-volumes/) — explicit `ReadWriteOncePod` support.
- [Longhorn production best practices](https://longhorn.io/docs/1.12.1/best-practices/) — recurring remote backups and snapshots.
- [K3s networking services](https://docs.k3s.io/networking/networking-services) — bundled Traefik Ingress and the embedded network-policy controller.
- [cert-manager annotated Ingress](https://cert-manager.io/docs/usage/ingress/) — ClusterIssuer annotation and generated TLS Secret behavior.
- [Obsidian settings](https://obsidian.md/help/settings) — documented default system-trash behavior and alternative trash modes.
- [`obsidian-headless` package metadata](https://github.com/obsidianmd/obsidian-headless/blob/master/package.json) — current Node/platform requirements and the `UNLICENSED` declaration relevant to the recorded private-packaging assumption.
