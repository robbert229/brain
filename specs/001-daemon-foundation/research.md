# Phase 0 Research: Daemon Foundation and Status

**Feature**: [spec.md](spec.md) | **Date**: 2026-10-26 | **Plan**: [plan.md](plan.md)

Scope: resolve the unknowns and choices in the plan's Technical Context. FND-001–
FND-005 implementation evidence (`internal/braind/`) constrains all decisions; the
remaining surface is FND-006 readiness plus the "configured shutdown bound" wording
in User Story 1.

## D1 — Readiness surface: dedicated `GET /readyz`

- **Decision**: Add `GET /readyz` answering 200 (`ready`) only when the process
  lifecycle reports ready AND the local vault passes its accessibility check;
  otherwise 503 (`not ready`) with `Cache-Control: no-store`. `/healthz` stays a
  pure liveness answer and never reflects dependencies.
- **Rationale**: The kubelet needs distinct liveness/readiness probes, and
  [ADR-0003](../../docs/adr/0003-readiness-is-local.md) defines readiness as
  local-vault availability independent of OIDC/Sync/backup. Today
  `deploy/install.yaml` points `readinessProbe` at `/healthz`, so vault loss or
  shutdown drain never removes the pod from rotation — an actual gap against
  FR-004/FR-005 and SC-003.
- **Alternatives considered**:
  - *Same endpoint, different semantics* (`/healthz` returns readiness): rejected;
    conflates the two signals, breaks the liveness contract of 009, and would let
    an unready-but-alive process fail its liveness probe and get killed.
  - *Header-negotiated variants of `/healthz`*: needless indirection (Principle III).
- **Consequence**: `specs/009-landing-page/contracts/http.md` documents `/readyz`
  as a 404; this feature supersedes that single line (405/404 semantics for all
  other paths are unchanged).

## D2 — How vault readiness is evaluated: synchronous check at request time

- **Decision**: One named decision function (e.g. `readiness` in package
  `braind`) composes `lifecycle.Snapshot().Ready` with a vault accessibility
  probe: `VaultPath` exists, is a directory, and is openable/readable for
  reading entries. Run per `/readyz` request; no background scanner, no cache.
- **Rationale**: The check is a cheap `os.Stat` + `File.ReadDir` on a hot-loop-
  friendly path; readiness must observe "the vault becomes unavailable during
  operation" (edge case) immediately, so a cached or polled design adds staleness
  for no benefit at probe frequencies (Principle III: simplest solution;
  Principle II: one explicit decision point, directly testable).
- **Alternatives considered**:
  - *Background vault watcher setting `MarkDegraded`*: rejected — a subsystem
    with timers and failure modes, unjustified for a probe answered in one
    syscall; would also duplicate state the request path can compute truthfully.
  - *Trusting only the lifecycle `Ready` flag*: cannot see vault loss after
    `MarkRunning`, so SC-003/US2-AC2 could not be verified.
- **Consequence**: During shutdown, `BeginShutdown` flips `Ready=false` before
  draining (existing behavior), so `/readyz` fails before exit without extra
  wiring — satisfies SC-003 with no new shutdown-path code.

## D3 — Degradation vs. vault availability: FR-005 separation

- **Decision**: `/readyz` ignores OIDC, Sync, and backup state entirely.
  Upstream degradation is observable only in `/status` (D4) and logs. The
  lifecycle `Degraded` state keeps its existing meaning (partial local
  impairment with an explicit `ready` argument); it is never set for upstream
  outages.
- **Rationale**: FR-005 and US2-AC1 require liveness/readiness unaffected by
  upstream outages, and [ADR-0003](../../docs/adr/0003-readiness-is-local.md)
  is explicit. The composition in D2 enforces this structurally: nothing in the
  `/readyz` path reads subsystem state.
- **Alternatives considered**: *Weighted readiness aggregation* — rejected, would
  couple local readiness to the very outages FR-005 excludes.

## D4 — Published status: read-only `GET /status` over the existing store

- **Decision**: Expose the current `status.Store` snapshot as JSON at
  `GET /status` (allowlisted `PublicSnapshot`, `Cache-Control: no-store`,
  `X-Request-ID` via the existing middleware). No new model, no polling loop.
- **Rationale**: US2 requires operators to inspect "public status" distinguishing
  subsystems, and FR-008's safety and concurrency requirements are already
  satisfied by the atomic-pointer store, writer mutex, sanitizer, and allowlist
  in `internal/braind/status/`. A thin handler is the whole delta (Principle III).
- **Alternatives considered**:
  - *Keep status internal-only*: rejected — leaves US2 and the SC-004 status half
    of the security review untestable as a published surface.
  - *Status push/SSE*: unnecessary; polling an operational endpoint is enough for
    a single-operator deployment.
- **Consequence**: The page/asset behavior from spec 009 stays untouched;
  `/status` answers runtime data while `/` stays static (its cacheable
  `public, max-age=3600` must not leak to `/status`).

## D5 — Configurable shutdown bound: `BRAIND_SHUTDOWN_TIMEOUT`

- **Decision**: New positive-duration setting, default `5s` (current hard-coded
  value), validated in `config.Load`, consumed by `run` instead of
  `defaultShutdownTimeout`. Documented in `docs/braind-configuration.md`.
- **Rationale**: US1-AC4 says "drains requests within the configured shutdown
  bound," and the spec assumption warns 5s "is not the final deployment timeout,"
  so the bound must be deployment-tunable (FR-006). Validation reuses the
  existing `readPositiveDuration` helper — zero new mechanisms.
- **Alternatives considered**: *CLI flag*: env-file convention already governs all
  deployment settings (`BRAIND_*`); two configuration channels for one value would
  violate consistency. *Hard-coded constant*: cannot satisfy SC-003's "declared
  bound" across environments.

## D6 — Version and commit reporting

- **Decision**: Keep the existing `-version` flag and startup-log version (FR-001
  met). Wire a `commit` value into `status.Build.Commit` via the same `ldflags`
  pattern `cmd/braind/main.go` already uses for `version`, with the empty default
  tolerated.
- **Rationale**: FR-001 requires build version and version-only exit — both
  present. The status `Commit` field exists but is always `""`; passing it through
  `Run`'s version parameter is the existing injection point, so release tooling
  gains commit visibility without new plumbing.
- **Alternatives considered**: *Build-time generated file*: rejected (Principle III
  — ldflags suffice). *Defer entirely*: acceptable but the plumbing is trivial and
  improves SC-001-era diagnostics; if release tooling does not populate it, empty
  string renders as `omitempty` in `/status`.

## D7 — Request correlation and log redaction (verification-only)

- **Decision**: No code change; verify by test that `X-Request-ID` propagation,
  response header echo, and completion-log matching already satisfy FR-007 and
  US2-AC3, including the redaction tests for credentials, remote URLs, and note
  bodies (SC-004).
- **Rationale**: `logging.HTTPMiddleware` and the key/value redactors implement
  this fully; rewriting to "improve" it would be speculative churn.

## Resolved unknowns summary

| Technical Context item | Resolution |
| --- | --- |
| Readiness semantics (FND-006) | D1, D2, D3 |
| Shutdown bound configuration | D5 |
| Published status surface | D4 |
| Build/version reporting | D6 |
| Diagnostics adequacy | D7 (verification only) |

All Technical Context items are resolved; no NEEDS CLARIFICATION markers remain.
