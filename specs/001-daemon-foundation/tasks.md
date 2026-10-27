# Tasks: Daemon Foundation and Status

**Input**: Design documents from `specs/001-daemon-foundation/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/http.md, quickstart.md

**Tests**: Included — the repository constitution (Principle I: "Every feature
MUST land together with tests"; Development Workflow gate) makes tests mandatory,
and spec success criteria SC-001–SC-004 are defined as testable outcomes.

**Organization**: Tasks are grouped by user story. FND-001–FND-005 already exist in
`internal/braind/`; only FND-006 deltas appear below.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 = start/operate daemon (P1); US2 = diagnose service state (P2)

## Path Conventions

Single Go module: `cmd/braind/`, `internal/braind/...`, `deploy/`, `docs/`.
Tests live next to their package (`_test.go` files in the same package).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Prepare the tree; no new code (the project already exists).

- [X] T001 Create/verify feature branch `001-daemon-foundation` from current `main` (git checkout -b 001-daemon-foundation)
- [X] T002 Run baseline `go test ./...` and confirm all existing foundation and landing-page tests pass before any change (record result)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The readiness decision and `/readyz` surface — both user stories
depend on them (contract http.md; research D1/D2/D3).

**⚠️ CRITICAL**: No user story work begins until this phase is complete.

- [X] T003 Implement the readiness decision in `internal/braind/run.go`: one named function composing `lifecycle.Snapshot().Ready` AND a vault accessibility probe — probe succeeds iff `BRAIND_VAULT_PATH` resolves to a directory that can be listed (`os.Stat` + open + `File.ReadDir`, any error fails closed). No caching, no background polling (research D2); it MUST NOT read OIDC/Sync/Backup state (FR-005)
- [X] T004 [P] Unit-test the decision in `internal/braind/readiness_test.go`: ready true only when lifecycle ready + vault listable; false for missing path, non-directory, permission-denied, and during ShuttingDown; liveness composition never consults vault (US2-AC1/AC2 matrix)
- [X] T005 Register `GET /readyz` on the HTTP surface in `internal/braind/run.go` (`Handler`): `200` body `ready\n` / `503` body `not ready\n`, `Cache-Control: no-store`, `GET /{$}`-style registration so other methods get 405 (contracts/http.md). Thread the vault path through the existing `run` → `Handler` wiring without introducing new interfaces or packages (Principle III)
- [X] T006 [P] HTTP tests in `internal/braind/readyz_test.go`: 200/503 transitions, `no-store` header, 405 on `POST /readyz`, and that during graceful shutdown `/readyz` answers 503 before the process exits (SC-003 ordering)

**Checkpoint**: `/readyz` behaves per contract; both stories can proceed.

---

## Phase 3: User Story 1 - Start and operate the daemon (Priority: P1) 🎯 MVP

**Goal**: Operator controls lifecycle with a *configured* shutdown bound; readiness
fails before exit; duplicate owners and bad config refuse to serve.

**Independent Test**: Start/stop one `braind` with valid and invalid settings and
two processes on one data root; run quickstart.md Scenarios 1–4.

### Implementation & Tests for User Story 1

- [X] T007 [US1] Add `BRAIND_SHUTDOWN_TIMEOUT` to `internal/braind/config/config.go` using the existing `readPositiveDuration` helper: constraint — "positive duration, parsed as Go duration; invalid → startup fails naming the setting"; default `5s`; add validation + default tests in `internal/braind/config/config_test.go` including `0s`, `-1s`, and `banana` rejection cases
- [X] T008 [US1] Replace the hard-coded `defaultShutdownTimeout` in `internal/braind/run.go` with the validated config value (FR-006, "configured shutdown bound")
- [X] T009 [US1] Test shutdown semantics in `internal/braind/run_test.go`: in-flight request completes within the configured bound; an expired bound forces closure and returns/logs that the bound expired; readiness is `not ready` before exit (SC-003); extend the existing terminate/drain cases rather than duplicating them
- [X] T010 [US1] Update `deploy/install.yaml`: `readinessProbe.httpGet.path: /readyz`; leave `livenessProbe` at `/healthz`; confirm `terminationGracePeriodSeconds` exceeds the configured `BRAIND_SHUTDOWN_TIMEOUT` (contracts/http.md K8s section)
- [X] T011 [US1] Document `BRAIND_SHUTDOWN_TIMEOUT` in `docs/braind-configuration.md`: type (Go duration), default `5s`, positive-only, effect (graceful drain bound)

**Checkpoint**: US1 independently verifiable (quickstart Scenarios 1–4, SC-001–SC-003).

---

## Phase 4: User Story 2 - Diagnose service state safely (Priority: P2)

**Goal**: `/status` + probes let operators distinguish liveness, local vault
availability, and optional subsystem health — without leaking secrets or notes.

**Independent Test**: Exercise status transitions with simulated vault states and
inspect logs/`/status` for sensitive values (quickstart Scenarios 5–6).

### Implementation & Tests for User Story 2

- [X] T012 [US2] Add `GET /status` in `internal/braind/run.go` (`Handler`): marshals the current `status.Store` snapshot (existing `PublicSnapshot` allowlist — no new model types), `application/json; charset=utf-8`, `Cache-Control: no-store`, 405 on other methods (contracts/http.md)
- [X] T013 [US2] Test the status surface in `internal/braind/status_endpoint_test.go`: JSON contains `build`, `lifecycle`, `vault`, `oidc`, `search`, `sync`, `backup` sections with `disabled`/initial states when subsystems are off (edge case "not yet populated"); concurrent readers never tear (FR-008); configured secret values never appear (replaced with `[redacted]`); during drain `lifecycle.state` is `"shutting-down"` (SC-003/SC-004)
- [X] T014 [P] [US2] Test correlation across the new routes in `internal/braind/readyz_test.go` (extend): valid `X-Request-ID: trace-me-123` echoed on `/readyz` and `/status` responses AND the matching `request completed` log line; absent header generates a 32-hex identifier used identically (FR-007, US2-AC3)
- [X] T015 [P] [US2] End-to-end privacy test in `internal/braind/privacy_test.go`: representative run configured with `OIDC_CLIENT_SECRET` and `BRAIND_GIT_REMOTE_URL` values, with requests logged; assert zero occurrences of those values in stdout/stderr logs and in `/status` output (SC-004)
- [X] T016 [US2] Populate build metadata (research D6): add `var commit = ""` alongside `var version` in `cmd/braind/main.go`, thread it through `braind.Run` into `status.Initial(version, commit, ...)` in `internal/braind/run.go`; empty commit renders as `omitempty` in `/status`; update `internal/braind/run_test.go` call sites

**Checkpoint**: US1 and US2 both independently functional.

---

## Phase 5: Polish & Cross-Cutting Concerns

- [X] T017 [P] Update `docs/roadmap.md` Foundation row (FND-001–FND-006 complete) and add a supersession note to the `/readyz → 404` example row in `specs/009-landing-page/contracts/http.md` pointing at `specs/001-daemon-foundation/contracts/http.md`
- [X] T018 [P] Run `gofmt -l .` (empty output) and `go vet ./...` (clean) across touched packages
- [X] T019 Run `specs/001-daemon-foundation/quickstart.md` scenarios 1–6 manually against the built binary and confirm every SC-001–SC-004 mapping line holds; fix regressions found
- [X] T020 [P] Record user-facing changes per `CONTRIBUTING.md` conventions (Conventional Commits, changelog per release-please config)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: after Setup — BLOCKS both user stories (`/readyz`
  serves US1-AC4 shutdown ordering and US2-AC2 vault readiness simultaneously)
- **US1 (Phase 3)** and **US2 (Phase 4)**: after Phase 2; independent of each other
  (US2's T016 commit plumbing does not affect US1 behavior)
- **Polish (Phase 5)**: after desired stories complete

### Within Each User Story

- Implementation → its tests may land in one commit (constitution requires tests
  "together with" the feature, not strict red-green ordering)
- Same-file conflicts avoided: run.go is touched sequentially (T003/T005/T008/T016);
  [P] tasks always touch distinct files

### Parallel Opportunities

- T004 ∥ T005 (after T003); T006 after T005
- Phase 3 ∥ Phase 4 once Phase 2 completes (two implementers)
- T014 ∥ T015 (distinct test files); T017–T020 [P] in polish

---

## Parallel Example: Phase 2

```bash
# After T003 lands (readiness decision implemented):
Task: "Unit-test readiness decision in internal/braind/readiness_test.go"          # T004
Task: "Register GET /readyz with Handler wiring in internal/braind/run.go"         # T005
# Then T006 endpoint tests once T005 is in place
```

---

## Implementation Strategy

### MVP First (User Story 1)

1. Phase 1 + Phase 2 (readiness surface is foundational by design)
2. Phase 3: configured shutdown bound + deploy probes + docs
3. **STOP and VALIDATE**: quickstart Scenarios 1–4 pass → deployable delta

### Incremental Delivery

1. Phase 2 complete → `/readyz` live, probes correct (readiness gap closed)
2. + US1 → SC-001/SC-002/SC-003 verified end-to-end
3. + US2 → `/status`, correlation, SC-004 privacy review green
4. Polish → docs/roadmap/quickstart validation

### Parallel Team Strategy

Developer A: Phase 2 → US1; Developer B: (waiting on Phase 2) → US2. They converge
only on `run.go`/`Handler` wiring — merge US1 first (smaller surface).

---

## Notes

- FND-001–FND-005 are existing implementation evidence (plan.md Summary); no tasks
  duplicate them — T002/T019 prove they still hold.
- Constraints are quoted from data-model.md/contracts/http.md where present so
  implementation inherits no discretion.
- Every task is executable with only the named files and the linked design docs.
