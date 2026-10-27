# Implementation Plan: Daemon Foundation and Status

**Branch**: `001-daemon-foundation` | **Date**: 2026-10-26 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-daemon-foundation/spec.md`

## Summary

Complete the `braind` daemon foundation so its process lifecycle, health, status,
and privacy-safe diagnostics match the specification. FND-001–FND-005 already exist
in `internal/braind/` (version flag, validated configuration, single-owner data-root
lock, liveness, graceful shutdown, redacted structured logs). The remaining work is
FND-006: expose local-vault-backed **readiness** (`GET /readyz`, per ADR-0003),
make the graceful-drain bound configurable (`BRAIND_SHUTDOWN_TIMEOUT`), publish the
already-sanitized operational snapshot (`GET /status`), and point the Kubernetes
readiness probe at `/readyz`.

## Technical Context

<!-- Resolved 2026-10-26 from repository inspection; research rationale in research.md. -->

**Language/Version**: Go 1.22 (`go.mod`), standard library only for `braind`
(`net/http`, `log/slog`, `os`, `syscall`); no Node.js tier (Principle IV).

**Primary Dependencies**: None in-process. Landing-page assets (spec 009) are
embedded via `go:embed`; advisory locking uses `syscall.Flock` behind build tags.

**Storage**: Filesystem. The data root (`BRAIND_DATA_PATH`) holds `braind.lock`;
the vault (`BRAIND_VAULT_PATH`) is canonical Markdown — operational data (locks,
status, logs) is outside Principle V per constitution v2.0.0.

**Testing**: `go test ./...`; table-driven unit tests, in-process HTTP tests against
`Handler()`/`Serve()` with `net.Pipe`/loopback listeners and `t.TempDir()` data
roots. CI-runnable with no external services (Principle I).

**Target Platform**: Linux, single-replica private Kubernetes (ADR-0008); locking
build tags fall back to `ErrUnsupported` on other platforms.

**Project Type**: Single-module web-service daemon (`cmd/braind`) plus the unrelated
greeting CLI (`cmd/brain`, unchanged scope per spec assumptions).

**Performance Goals**: Operational probes only; liveness/readiness answers within
one synchronous filesystem check. No throughput targets (single-operator private
deployment).

**Constraints**: Shutdown drains within the configured bound (current default 5s,
not the final deployment value); zero configured secrets or note contents in logs
and published status (SC-004); at most one owner per data root (FR-003).

**Scale/Scope**: One daemon per data root; ~5 packages under `internal/braind/`;
planned delta is 4 surfaces: `/readyz`, `/status`, `BRAIND_SHUTDOWN_TIMEOUT`,
probe/contract/doc updates.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate result | Evidence |
| --- | --- | --- |
| I. Independently testable | PASS | Every surface is exercisable through `braind.Run`/`Handler` with temp dirs and a fake `LookupEnv`; existing `run_test.go`, `lifecycle_test.go`, `datalock_test.go` run in `go test ./...` without external services. New `/readyz`, `/status`, shutdown-timeout tests follow the same pattern. |
| II. Explicit business logic | PASS | Readiness becomes one named, typed decision point (research D2) directly testable against vault/lifecycle states; config validation errors name the setting without secret values. |
| III. Simplicity over abstraction | PASS | Stdlib only; readiness is a synchronous check at request time, not a scanner/subsystem (research D2 rejects periodic polling); no interfaces added where a function suffices. |
| IV. Go backend, HTMX frontend | PASS | Pure Go daemon; this feature adds no frontend. |
| V. Markdown as single source of truth | PASS | Only vault note content is canonical; `braind.lock`, status snapshots, and logs are operational data explicitly outside the principle. |

**Gate decision**: PASS — no violations, no complexity-tracking rows required.

## Project Structure

### Documentation (this feature)

```text
specs/001-daemon-foundation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── http.md          # Phase 1 output (healthz/readyz/status surface)
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created here)
```

### Source Code (repository root)

```text
cmd/braind/
└── main.go                      # signal.NotifyContext(SIGINT/SIGTERM) → braind.Run

internal/braind/
├── run.go                       # orchestration: config → lock → listener → serve → drain
├── run_test.go                  # lifecycle, duplicate-owner, shutdown acceptance tests
├── config/
│   ├── config.go                # env load + validation + redacted diagnostics
│   └── dotenv.go                # .env overlay with process-env precedence
├── datalock/
│   ├── datalock.go              # single-owner data-root lock, owner PID record
│   └── lock_unix.go             # FLOCK syscall; lock_unsupported.go otherwise
├── lifecycle/
│   └── lifecycle.go             # starting/running/degraded/shutting-down + Ready flag
├── logging/
│   ├── logging.go               # slog JSON, secret-value + sensitive-key redaction
│   └── request.go               # X-Request-ID middleware, completion logging
└── status/
    ├── status.go                # atomic snapshot store + sanitizer
    └── public.go                # allowlisted PublicSnapshot JSON

deploy/
└── install.yaml                 # livenessProbe /healthz; readinessProbe → /readyz (delta)

docs/
├── braind-configuration.md      # gains BRAIND_SHUTDOWN_TIMEOUT (delta)
├── adr/0002, 0003               # single-writer ownership; local readiness
└── roadmap.md                   # FND status note update after completion
```

**Structure Decision**: Keep the existing single-module layout; every delta lands in
its existing concern package (`run.go` for endpoints wiring, `config` for the new
setting, `lifecycle`/`status` semantics unchanged in shape). No new packages — the
plan adds behavior, not structure (Principle III).

### Planned delta (detail lands in tasks.md)

1. `GET /readyz`: 200 when the lifecycle reports ready **and** the local vault is
   accessible; 503 otherwise. During shutdown this is automatic (ready=false).
2. `BRAIND_SHUTDOWN_TIMEOUT`: positive-duration env setting, default 5s, replaces
   the hard-coded `defaultShutdownTimeout` in `run.go`, validated in `config`.
3. `GET /status`: JSON `status.Store` snapshot (existing allowlist, `no-store`).
4. `deploy/install.yaml`: `readinessProbe.httpGet.path: /readyz`.
5. Tests for all of the above plus SC-001–SC-004 mappings; doc updates.

## Complexity Tracking

No constitution violations to justify.
