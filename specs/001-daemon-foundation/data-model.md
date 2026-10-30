# Phase 1 Data Model: Daemon Foundation and Status

**Feature**: [spec.md](spec.md) | **Date**: 2026-10-26

Models below are the domain view of `internal/braind/`; field names match the
code where the entity already exists. "Delta" marks fields/rules this feature adds.

## Entities

### Service lifecycle (`lifecycle.Lifecycle` → `lifecycle.Snapshot`)

Point-in-time process state. Snapshot is immutable; transitions are atomic under
a mutex (`sync.RWMutex`, reads lock-free).

| Field | Type | Notes |
| --- | --- | --- |
| `State` | enum: `starting` \| `running` \| `degraded` \| `shutting-down` | process-wide |
| `Ready` | bool | whether the local service can serve configured read/write mode |
| `ChangedAt` | time (UTC) | set on every transition |

**State transitions** (all legal transitions; re-entering the same state is a no-op):

```text
starting ──MarkRunning──▶ running
running ──MarkDegraded(ready)──▶ degraded   (local impairment only — never for
degraded ──MarkDegraded(ready)──▶ degraded    upstream OIDC/Sync/backup outage, D3)
degraded ──MarkRunning──▶ running
*  ──BeginShutdown──▶ shutting-down          (idempotent; Ready flips false first)
*  ──(transition after shutting-down)──▶ ErrShuttingDown (rejected)
```

**Rules**

- `BeginShutdown` MUST make `Ready=false` before any shutdown work begins
  (SC-003).
- `MarkDegraded(ready)`'s `ready` argument is decided by local capability;
  no code path sets Degraded because of an upstream outage (FR-005).

### Health state (delta — readiness decision)

Not a new stored object: the composition `lifecycle.Ready AND vault-accessible`
evaluated per request (research D2). It answers:

| Probe | Reads | Fails when |
| --- | --- | --- |
| Liveness | process aliveness only | never (while serving) |
| Readiness | `lifecycle.Snapshot.Ready` + vault path probe | shutting down, not yet running, or vault unreadable |

**Invariant**: no upstream subsystem state participates in either probe (FR-004,
FR-005, ADR-0003).

**Vault accessibility probe (delta)**: succeeds iff `BRAIND_VAULT_PATH` resolves
to a directory that can be listed. Becomes false while the vault is unavailable
during operation; recovery automatically restores readiness.

### Operational snapshot (`status.Snapshot` → published `status.PublicSnapshot`)

Internal snapshot deliberately carries no secrets/raw-content fields; the
published view is an allowlist (`PublicSnapshot`, `public.go`). Store publishes
atomically (`atomic.Pointer`), writers serialize (`sync.Mutex`), and every write
clones and sanitizes against configured secret values.

| Entity | Key fields | States / notes |
| --- | --- | --- |
| `Build` | `Version`, `Commit` (delta: actually populated, D6), `StartedAt` | plus computed `uptime_seconds` in the public view |
| `Lifecycle` | mirrors lifecycle Snapshot | exposed as `lifecycle` in JSON |
| `Vault` | `State`, `ReadOnly` | `scanning` (initial) → `ready` \| `read-only` \| `degraded` \| `unavailable` |
| `OIDC` | discovery + group-config names, `ActiveSessions` | `pending`/`ready`/`degraded`; secret fields structurally absent |
| `Search` | last run stats | optional; zero until first run (edge case "not yet populated") |
| `Sync` | state machine `disabled`/`checking`/`needs-authentication`/`needs-setup`/`starting`/`running`/`backoff`/`stopped`, `LastError`/`OutputTail` (sanitized) | optional subsystem |
| `Backup` | `disabled`/`checking`/`ready`/`running`/`backoff`/`blocked`/`degraded`, commit/push times, `LastError` (sanitized) | optional subsystem |

**Validation/publication rules**

- Every published string is scrubbed of configured secret values (client secret,
  Git remote URL) before storage (FR-008).
- Concurrent readers never see torn snapshots: publish is copy-on-write.
- Raw note content and credentials have no representable field.

### Request identifier (`logging` middleware)

| Field | Rule |
| --- | --- |
| inbound `X-Request-ID` | valid iff 1–128 chars of `[A-Za-z0-9-_.:]`; used verbatim |
| generated | 16 random bytes hex; generation failure panics (never a predictable ID) |
| echo | same value in the response header and the `request_id` field of the single completion log per request |

### Configuration (`config.Config`) — validation surface

Immutable after load; accessors return defensive copies. Secrets are never
printable: `String()`/`Format()` redact, diagnostics use `[configured]`/
`[redacted]`. Relevant to this feature:

| Setting | Default | Rule (this feature) |
| --- | --- | --- |
| `BRAIND_DATA_PATH` | `/data` | absolute; the lock root; ownership = single flock holder |
| `BRAIND_VAULT_PATH` | `/data/vault` | absolute, beneath data path; probe target for readiness |
| `BRAIND_READ_ONLY` | `false` | reflected in `status.Vault.ReadOnly` |
| `BRAIND_SHUTDOWN_TIMEOUT` (delta) | `5s` | positive duration, parsed as Go duration; invalid → startup fails naming the setting |
| `BRAIND_LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |

### Data-root lock (`datalock.Lock`)

- Created `braind.lock` inside the data root, mode `0600`, containing the owning
  PID (`recordOwner`), held via advisory flock for the process lifetime.
- Relationships: `run` acquires after config validation and before listening;
  a second contender fails with `ErrAlreadyLocked` and exit code 1 without
  serving (FR-003); `Release` runs on every exit path.

## Relationships (delta interactions to verify)

- `/readyz` result = `f(lifecycle.Snapshot.Ready, vault probe)` — both inputs,
  neither bypassed (US1-AC4, US2-AC2).
- `/status` = `status.Store.Load()` of the allowlisted view — same store the
  lifecycle wiring updates via `SetLifecycle`, so shutdown shows as
  `shutting-down` before the process exits (SC-003).
- Config `BRAIND_SHUTDOWN_TIMEOUT` is the single source for
  `serve(..., shutdownTimeout)` — the deadline source for SC-003's bound.
