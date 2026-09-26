# `braind` implementation backlog

Status: active execution plan

Last updated: 2026-09-25

This backlog decomposes the design in [`docs/braind.md`](braind.md) into small, independently reviewable deliverables. A task should normally fit in one pull request, preserve a passing main branch, include its own tests and documentation, and avoid depending on unfinished code hidden in another branch.

## 1. Working rules

- Task IDs are stable even if ordering changes.
- A task is complete only when its acceptance criteria are automated where practical.
- A spike produces a checked-in decision or fixture, not production code that silently becomes permanent.
- Security boundaries—paths, authentication, authorization, subprocess arguments, and secrets—receive tests in the same task that introduces them.
- Later tasks may extend an interface, but an earlier task must remain useful and testable by itself.
- The initial product milestone is read/search-only. Mutation tasks remain separate and may ship later.
- Deployment tasks use fake Obsidian, OIDC, and Git dependencies until their explicitly named end-to-end task.

## 2. Milestone gates

| Milestone | Outcome | Required tasks |
| --- | --- | --- |
| M0 — contracts proven | The external tools and packaging model are sufficiently understood to build against. | `SPK-001`–`SPK-004` |
| M1 — local secure reader | An OIDC-protected local daemon can browse, read, render, and search a vault through HTML and `/api/v1`. | `FND-001`–`FND-006`, `AUTH-001`–`AUTH-005`, `READ-001`–`READ-007` |
| M2 — synchronized and backed up | The daemon supervises Obsidian Sync, bootstraps without normal `kubectl exec`, and pushes coherent Git backups. | `SYNC-001`–`SYNC-006`, `GIT-001`–`GIT-006` |
| M3 — deployable | A private, multi-platform image and validated k3s manifests deploy the M2 service. | `IMG-001`–`IMG-003`, `K8S-001`–`K8S-005` |
| M4 — safe mutation | Authorized users can create, edit, rename, and trash notes without silently overwriting synchronized changes. | `MUT-001`–`MUT-004` |
| M5 — operational release | Metrics, recovery exercises, and hardening checks make the service supportable. | `OPS-001`–`OPS-004` |

## 3. Contract and feasibility spikes

### SPK-001 — Record the Obsidian Headless command contract

- **Deliverable:** A checked-in note and reproducible probe script/fixture documenting the pinned `ob` version, authentication/setup/status/config commands, exit codes, persistent paths, stdout/stderr behavior, continuous-sync signals, MFA/E2EE prompts, and graceful termination.
- **Depends on:** none.
- **Done when:** The probe runs against a disposable account/vault; captured output is sanitized; unknown or undocumented behavior required by the design is called out explicitly.

### SPK-002 — Verify Pocket ID claims against the deployed provider

- **Deliverable:** A sanitized verification note showing discovery issuer, Authorization Code + PKCE behavior, `groups` in the ID token and UserInfo, email display claim, logout behavior, and the three configured Brain groups.
- **Depends on:** none.
- **Done when:** A disposable client proves exact claim shapes without committing tokens, codes, client secrets, or personal group memberships.

### SPK-003 — Prove the `ko` runtime-base approach

- **Deliverable:** A disposable runtime-base experiment containing Node.js, pinned Obsidian Headless, Git, SSH, CA certificates, and a non-root user, then used successfully as a `ko` base on amd64 and arm64.
- **Depends on:** `SPK-001`.
- **Done when:** Both platform images run `ob`, `git`, and a minimal Go binary as the intended UID with the proposed writable paths; limitations are recorded.

### SPK-004 — Measure the representative vault and Git workflow

- **Deliverable:** Sanitized measurements from a non-production copy of the approximately 2,000-file vault: total bytes, Markdown bytes, largest files, bounded search time, initial Git pack size, commit time, and push time to a disposable private remote.
- **Depends on:** none.
- **Done when:** Results either validate the proposed search limits and ordinary Git storage or produce a narrowly scoped design amendment.

## 4. Daemon foundation

### FND-001 — Add the `braind` command and HTTP lifecycle

- **Status:** Complete (2026-09-25).
- **Deliverable:** A new `cmd/braind` entry point with an HTTP server, signal-aware graceful shutdown, `/healthz`, and a build/version value; the existing `brain` CLI continues to work.
- **Depends on:** none.
- **Done when:** Unit/integration tests start the server on an ephemeral port, exercise health, cancel it, and prove bounded shutdown; `go test ./...` passes.

### FND-002 — Implement typed configuration loading

- **Status:** Complete (2026-09-25).
- **Deliverable:** One immutable configuration type for the documented `BRAIND_*` and OIDC settings, including duration/size parsing, defaults, cross-field validation, and redacted diagnostics.
- **Depends on:** `FND-001`.
- **Done when:** Table-driven tests cover valid defaults, required production values, malformed inputs, incompatible modes, and non-disclosure of secret values.

### FND-003 — Establish structured logging and request IDs

- **Status:** Complete (2026-09-26).
- **Deliverable:** Structured stdout/stderr logging, inbound/generated request IDs, component fields, and a central redaction policy.
- **Depends on:** `FND-001`, `FND-002`.
- **Done when:** Tests prove tokens, cookies, authorization headers, form bodies, note bodies, and configured secrets do not appear in representative logs.

### FND-004 — Add the process-wide data lock and lifecycle state

- **Deliverable:** The `/data/braind.lock` exclusive lock plus startup, running, degraded, and shutting-down state transitions shared by probes and status handlers.
- **Depends on:** `FND-002`, `FND-003`.
- **Done when:** A second process/fixture cannot acquire the same data root, shutdown makes readiness fail before exit, and stale ordinary process termination releases the lock.

### FND-005 — Implement the status snapshot model

- **Deliverable:** A concurrency-safe immutable status snapshot with build, vault, OIDC, Sync, and Git-backup substate, initially populated by fakes/placeholders.
- **Depends on:** `FND-004`.
- **Done when:** Race-enabled tests cover concurrent readers/writers and public status serialization cannot expose secret fields.

### FND-006 — Add readiness and startup probe semantics

- **Deliverable:** `/readyz` and startup-state behavior based on a non-recursive vault accessibility check, distinct from liveness and upstream health.
- **Depends on:** `FND-004`, `FND-005`.
- **Done when:** Tests prove inaccessible storage returns `503`, while simulated OIDC, Sync, and Git outages do not make an otherwise readable vault unready.

## 5. Authentication and authorization

### AUTH-001 — Implement OIDC discovery and login transactions

- **Deliverable:** Provider discovery plus Authorization Code + PKCE login initiation using one-time state, nonce, and verifier records.
- **Depends on:** `FND-002`, `FND-003`.
- **Done when:** A fake provider test covers discovery retry, exact issuer handling, secure redirect construction, transaction expiry, and replay rejection.

### AUTH-002 — Validate callbacks and create server-side sessions

- **Deliverable:** Callback exchange/ID-token validation and opaque in-memory sessions with secure cookie settings, rotation, idle expiry, and absolute expiry.
- **Depends on:** `AUTH-001`.
- **Done when:** Tests reject bad state, nonce, issuer, audience, signature, and expiry; restart invalidates sessions; browser cookies contain no provider token or claims.

### AUTH-003 — Extract configurable groups and resolve roles

- **Deliverable:** Deterministic `id_token` or `userinfo` group extraction with configurable claim name and exact admin/user/viewer mappings.
- **Depends on:** `AUTH-002`.
- **Done when:** Tests cover malformed/missing claims, exact string matching, unmapped denial, and `admin > user > viewer` precedence.

### AUTH-004 — Centralize route authorization

- **Deliverable:** A route-policy registry and authorization middleware with browser-versus-API unauthenticated behavior.
- **Depends on:** `AUTH-003`.
- **Done when:** Every registered route has a declared minimum role; matrix tests prove viewer/user/admin boundaries for full pages, HTMX fragments, and JSON requests.

### AUTH-005 — Add browser request protections

- **Deliverable:** CSRF tokens, Origin/Host validation, security headers, cookie policy enforcement, request body/header limits, and safe forwarding-header handling.
- **Depends on:** `AUTH-002`, `AUTH-004`, `FND-002`.
- **Done when:** Negative tests cover missing/invalid CSRF, cross-origin mutation, hostile Host/forwarding headers, oversized requests, framing, MIME sniffing, and unsafe content-policy regressions.

## 6. Read-only vault, web application, and API

### READ-001 — Implement logical vault path validation

- **Deliverable:** A typed logical-path package that rejects absolute paths, traversal, NULs, unsafe symlinks, and out-of-root resolution without exposing host paths.
- **Depends on:** `FND-002`.
- **Done when:** Table/fuzz tests cover Unicode, separators, dot segments, symlink escapes, case collisions, and unusual valid filenames.

### READ-002 — Add bounded directory listing and note reads

- **Deliverable:** Read-only vault services for one-directory-at-a-time listings and size-limited Markdown reads with metadata and strong content revisions.
- **Depends on:** `READ-001`.
- **Done when:** Tests cover deterministic ordering, pagination bounds, hidden/config/trash filtering, binary files, oversized notes, disappearing files, and cancellation.

### READ-003 — Add bounded on-demand search

- **Deliverable:** Cancellable Markdown path/title/body search with concurrency, time, file, byte, result, and snippet limits plus truncation metadata.
- **Depends on:** `READ-001`, `SPK-004`.
- **Done when:** Corpus tests cover ranking, malformed content, limit enforcement, client cancellation, skipped files, and the documented response statistics without building an index.

### READ-004 — Add sanitized Markdown rendering

- **Deliverable:** Server-side CommonMark/GFM rendering and plain-source output with sanitization and safe external-link behavior.
- **Depends on:** `READ-002`.
- **Done when:** Security tests reject scripts, event handlers, unsafe URLs, raw dangerous HTML, and injection through frontmatter or filenames.

### READ-005 — Build the server-rendered HTMX shell

- **Deliverable:** Vendored HTMX/assets plus accessible layouts and full-page/fragment templates for login, forbidden, dashboard, and errors.
- **Depends on:** `AUTH-005`, `FND-005`.
- **Done when:** Browser tests exercise login/dashboard navigation with JavaScript disabled and HTMX swaps enabled; fragments never bypass the same policy as full pages.

### READ-006 — Add explorer, note, and search pages

- **Deliverable:** Server-rendered explorer, note/source, and search pages with matching HTMX fragments and pagination/truncation feedback.
- **Depends on:** `READ-002`, `READ-003`, `READ-004`, `READ-005`.
- **Done when:** Browser tests cover ordinary and HTMX navigation, unusual filenames, empty/error/truncated states, sanitized rendering, and absence of mutation controls.

### READ-007 — Publish the read-only `/api/v1` contract

- **Deliverable:** Problem responses, status/list/read/search endpoints, checked-in OpenAPI, cursor rules, ETags, and conformance tests.
- **Depends on:** `AUTH-005`, `READ-002`, `READ-003`, `FND-005`.
- **Done when:** OpenAPI and registered routes agree; viewer access works through the server session; unmapped/unauthenticated/insufficient callers receive the designed status without information leakage.

## 7. Obsidian Sync integration

### SYNC-001 — Build the fixed-command child-process adapter

- **Deliverable:** A narrow `ob` runner with fixed executable/argument construction, controlled environment, process-group cancellation, timeouts, and sanitized bounded output capture.
- **Depends on:** `SPK-001`, `FND-003`.
- **Done when:** A fake executable proves no shell interpretation, correct signals, deadlines, output truncation, and credential redaction.

### SYNC-002 — Implement the continuous-sync supervisor

- **Deliverable:** The zero-or-one child state machine with exponential backoff/jitter, stable-run reset, blocked authentication/setup states, and shutdown integration.
- **Depends on:** `SYNC-001`, `FND-005`.
- **Done when:** Deterministic clock/process tests cover crashes, repeated failures, recheck, cancellation, and readiness independence.

### SYNC-003 — Detect authentication and vault setup state

- **Deliverable:** Supported-command discovery of authenticated, unauthenticated, configured, and needs-setup states without parsing undocumented credential files.
- **Depends on:** `SYNC-001`, `SPK-001`.
- **Done when:** Sanitized fixtures cover every observed state and unknown CLI output fails safe with actionable diagnostics.

### SYNC-004 — Implement the fixed bootstrap state machine

- **Deliverable:** A single-flight application service for Obsidian login, MFA/E2EE, remote-vault selection, and setup, using PTY/stdin first and documented flags only as the reviewed fallback.
- **Depends on:** `SYNC-003`, `SPK-001`.
- **Done when:** Fake-command tests prove fixed state transitions, transient secret disposal/redaction, retry cleanup, timeouts, and absence of arbitrary command input.

### SYNC-005 — Add the admin bootstrap web flow

- **Deliverable:** Fresh-login-protected pages/forms that drive `SYNC-004`, with no-store responses, CSRF, safe progress/error presentation, and recheck behavior.
- **Depends on:** `AUTH-005`, `SYNC-004`, `READ-005`.
- **Done when:** Browser tests prove admin-only access, recent-auth enforcement, CSRF, single-flight conflicts, refresh/retry behavior, and no credential reflection or persistence.

### SYNC-006 — Enforce and expose the sync-everything policy

- **Deliverable:** Apply/verify bidirectional mode, all documented file/config categories, and no excluded folders; expose sanitized Sync status/recheck in HTML and `/api/v1`.
- **Depends on:** `SYNC-002`, `SYNC-003`, `SYNC-005`, `READ-007`.
- **Done when:** Drift is reported rather than silently overwritten, mutations would be disabled on drift, and a disposable vault completes an initial and continuous sync.

## 8. Git backup and restore

### GIT-001 — Build the fixed-command Git adapter

- **Deliverable:** A Git runner using fixed `--git-dir` and `--work-tree`, controlled environment, timeouts, sanitized output, and injected executor for tests.
- **Depends on:** `FND-003`, `SPK-004`.
- **Done when:** Tests prove the vault cannot alter command structure, remote credentials/URLs are redacted, and no shell is used for Git arguments.

### GIT-002 — Initialize and validate repository state

- **Deliverable:** Separate metadata initialization, fixed author/branch/remote configuration, integrity checks, empty-remote initialization, and explicit refusal of unapproved existing history.
- **Depends on:** `GIT-001`.
- **Done when:** Temporary-repository tests cover first initialization, restart idempotence, wrong remote/branch, damaged repository, stale lock handling, and non-empty remote refusal.

### GIT-003 — Create coherent local backup commits

- **Deliverable:** The write lock/Sync-quiesce/stage-all/commit/restart transaction, including no-change behavior and recovery paths.
- **Depends on:** `GIT-002`, `SYNC-002`.
- **Done when:** Integration tests modify/add/delete binary and text files during controlled Sync activity and prove each successful commit is coherent while Sync always restarts after failure.

### GIT-004 — Push securely and handle divergence

- **Deliverable:** Read-only-mounted SSH Secret integration, strict pinned host keys, upstream push, pending-commit retry/backoff, remote confirmation, and blocked non-fast-forward state.
- **Depends on:** `GIT-003`.
- **Done when:** Tests cover unavailable remote, interrupted push, credential failure, host-key failure, later retry, and divergence without pull/merge/rebase/reset/force-push.

### GIT-005 — Schedule and expose backups

- **Deliverable:** Six-hour scheduling, admin-only backup-now HTML/API action, and status/metrics integration.
- **Depends on:** `GIT-004`, `READ-006`, `READ-007`.
- **Done when:** Only one run executes at a time; manual and scheduled triggers share the same state machine; no-change runs push pending commits; role, CSRF, API, and shutdown behavior are tested.

### GIT-006 — Add and test isolated restore

- **Deliverable:** A documented restore procedure and test harness that checks out a selected remote commit into a new empty worktree/PVC with Sync and mutations disabled.
- **Depends on:** `GIT-004`, `READ-002`, `READ-003`.
- **Done when:** The restored repository passes integrity, file-count, representative note/search, and binary checksum checks, and the procedure never overwrites the live vault.

## 9. Images and Kubernetes deployment

### IMG-001 — Define the pinned runtime base image

- **Deliverable:** A reviewable multi-platform base-image build for Node.js, Obsidian Headless, Git/SSH, certificates, fixed UID/GID, license material, and required writable paths.
- **Depends on:** `SPK-003`.
- **Done when:** CI builds amd64/arm64, scans the result, and runs version/non-root/read-only-root smoke tests.

### IMG-002 — Configure `ko` for the application image

- **Deliverable:** Pinned `.ko.yaml`, runtime-base digest, embedded templates/assets, version ldflags, SBOM, and multi-platform application build.
- **Depends on:** `IMG-001`, `READ-005`.
- **Done when:** A clean checkout produces one amd64/arm64 OCI index and the resulting container passes health and tool-version smoke tests.

### IMG-003 — Publish private GHCR artifacts

- **Deliverable:** CI publication of immutable private runtime/application images plus per-platform OCI archives and checksums; public publication is structurally disabled.
- **Depends on:** `IMG-002`.
- **Done when:** A release candidate can be pulled with `regcred`, manifests reference digests, archives load successfully, and unauthenticated pulls fail.

### K8S-001 — Add Namespace, Services, StatefulSet, and PVC

- **Deliverable:** Base manifest for namespace `brain`, headless/ClusterIP Services, one-replica StatefulSet, retained 10 GiB Longhorn `ReadWriteOncePod` claim, probes, resources, and hardened pod/container contexts.
- **Depends on:** `IMG-002`, `FND-006`.
- **Done when:** Server-side dry-run succeeds against the target cluster; policy tests assert one replica, no service-account token, no privilege, correct volume retention, and no embedded secrets.

### K8S-002 — Add Traefik ingress and cert-manager TLS

- **Deliverable:** Ingress for `brain.lab.johnrowley.co` using class `traefik`, TLS Secret `braind-tls`, and ClusterIssuer `letsencrypt-cloudflaredns-production`.
- **Depends on:** `K8S-001`.
- **Done when:** A cluster test obtains a valid certificate, serves HTTPS, preserves the canonical callback URL, and does not expose the headless or ClusterIP Service externally.

### K8S-003 — Wire application and credential configuration

- **Deliverable:** Non-secret environment configuration, OIDC Secret references, `imagePullSecrets: regcred`, and read-only `braind-git-credentials` SSH/known-host mounts.
- **Depends on:** `K8S-001`, `GIT-004`, `SYNC-005`.
- **Done when:** Rendered manifests contain no secret values, the pod reads the expected files/variables, and logs/PVC inspection reveal no copied Git or OIDC secret.

### K8S-004 — Add NetworkPolicy and single-replica admission policy

- **Deliverable:** Default-deny policy with required ingress/DNS/HTTPS/Git-SSH paths plus separately installable ValidatingAdmissionPolicy/binding scoped to the Brain StatefulSet.
- **Depends on:** `K8S-001`, `K8S-002`, `K8S-003`.
- **Done when:** Cluster tests prove allowed OIDC/Obsidian/Git traffic works, unrelated ingress/egress is denied where enforceable, and attempts to scale the StatefulSet above one are rejected.

### K8S-005 — Run the disposable-cluster acceptance test

- **Deliverable:** An opt-in end-to-end procedure covering deploy, OIDC login, web bootstrap, initial Sync, browse/search, Git backup, restart, image upgrade/rollback, certificate renewal observation, and isolated Git restore.
- **Depends on:** `IMG-003`, `K8S-002`, `K8S-003`, `K8S-004`, `GIT-006`, `SYNC-006`.
- **Done when:** The procedure passes on the target k3s/Longhorn environment without normal `kubectl exec`, and sanitized evidence is retained.

## 10. Safe mutation milestone

### MUT-001 — Implement atomic create/update primitives

- **Deliverable:** Create-if-absent and atomic replace with strong revision tokens, size/path validation, fsync policy, write locking, and `If-Match` semantics.
- **Depends on:** `READ-001`, `READ-002`, `GIT-003`.
- **Done when:** Race/integration tests prove synchronized external edits cannot be silently overwritten and failed writes leave the prior file intact.

### MUT-002 — Add web create/edit and conflict handling

- **Deliverable:** User/admin create and source-edit forms, revision tokens, validation, Post/Redirect/Get fallback, HTMX fragments, and an explicit conflict view.
- **Depends on:** `MUT-001`, `READ-006`, `AUTH-005`.
- **Done when:** Viewer denial, CSRF, validation, successful save, stale revision, JavaScript-free flow, and HTMX flow all have browser coverage.

### MUT-003 — Add rename and system-trash operations

- **Deliverable:** Same-volume rename/move and persisted Linux/XDG system-trash moves with original-location metadata and no permanent-unlink fallback.
- **Depends on:** `MUT-001`.
- **Done when:** Tests cover collisions, case-only renames where supported, symlinks, cross-directory moves, trash failures, and restore-oriented metadata.

### MUT-004 — Publish mutation API operations

- **Deliverable:** `/api/v1` create/update/rename/delete operations with strict schemas, ETags, preconditions, problem responses, authorization, and OpenAPI updates.
- **Depends on:** `MUT-001`, `MUT-003`, `READ-007`.
- **Done when:** Conformance tests cover `201`, `409`, `412`, `428`, validation, role denial, read-only/sync-drift denial, and parity with web business rules.

## 11. Operational hardening

### OPS-001 — Add privacy-safe metrics

- **Deliverable:** HTTP, authentication, authorization, search, Sync, and Git-backup counters/gauges/histograms without identity, path, query, remote, or error-text labels.
- **Depends on:** `SYNC-006`, `GIT-005`, `READ-007`.
- **Done when:** Tests inspect metric descriptors/labels, and a documented sample dashboard can identify stale Sync and stale backup conditions.

### OPS-002 — Complete threat-focused testing

- **Deliverable:** Fuzz/property tests and a threat checklist for path traversal, symlink races, OIDC replay, session fixation, CSRF, HTML sanitization, subprocess injection, secret leakage, and authorization gaps.
- **Depends on:** M3; repeat after `MUT-004` for mutation surfaces.
- **Done when:** Each documented trust boundary has an automated negative test or a recorded reason it requires external validation.

### OPS-003 — Exercise failure and recovery runbooks

- **Deliverable:** Tested runbooks for expired Obsidian authentication, unavailable OIDC, unavailable/diverged Git remote, corrupt Git metadata, full PVC, failed rollout, lost PVC, and historical Git restore.
- **Depends on:** `K8S-005`, `GIT-006`.
- **Done when:** A quarterly-style exercise restores a chosen commit into a new PVC, validates it read-only, and records measured recovery time without overwriting the live claim.

### OPS-004 — Release-readiness review

- **Deliverable:** Dependency/image scan review, accessibility/browser pass, resource/search benchmark review, documented limitations, and an operator checklist tied back to the design.
- **Depends on:** `OPS-001`, `OPS-002`, `OPS-003`; include mutation tasks only if M4 is in the release.
- **Done when:** Every open design question is resolved or explicitly deferred, all milestone acceptance tests pass, and the release artifact/manifests are immutable and private.

## 12. Recommended starting sequence

The first four deliverables can begin independently except where noted:

1. `SPK-001` — Obsidian Headless command contract.
2. `SPK-002` — Pocket ID claim verification.
3. `SPK-004` — representative vault/Git measurements.
4. `FND-001` — `braind` command and HTTP lifecycle.

After `SPK-001`, run `SPK-003`. After `FND-001`, proceed through the foundation lane while the spikes finish. No production Obsidian supervisor, bootstrap workflow, Git transaction, or runtime image should be built before its corresponding spike is accepted.
