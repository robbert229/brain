# HTTP & Invocation Contract: Daemon Foundation and Status

**Feature**: [spec.md](spec.md) | **Date**: 2026-10-26

Baseline: `braind` serves `GET /healthz` (FND-001) plus the landing page and
assets (spec 009). This contract governs the foundation surface and the two new
routes. It **supersedes** the single row "e.g., `/readyz` → 404" in
[009/contracts/http.md](../../specs/009-landing-page/contracts/http.md); all
other 009 semantics (root page, `/static/`, 405/404 elsewhere) are unchanged.

## Invocation

| Invocation | Result |
| --- | --- |
| `braind -version` | exit `0`; stdout `braind <version>`; server NOT started, config NOT loaded (FR-001) |
| `braind` with valid settings | serves from startup until termination |
| `braind` with invalid settings | exit `1`; stderr/log names the offending setting; no secret values printed (FR-002) |
| second `braind` on a locked data root | exit `1`, "data lock failed" (wraps `ErrAlreadyLocked`); no listener opened (FR-003) |
| unknown positional arguments / unknown flags | exit `2`; server NOT started |

## Routes

| Method | Path | Status | Content-Type | Body / Notes |
| --- | --- | --- | --- | --- |
| `GET` | `/healthz` | `200 OK` | `text/plain; charset=utf-8` | Liveness only. Body `ok\n`; `Cache-Control: no-store`. MUST succeed while the process serves, regardless of vault, OIDC, sync, or backup state. |
| `GET` | `/readyz` (delta) | `200 OK` / `503 Service Unavailable` | `text/plain; charset=utf-8` | Readiness. `200` body `ready\n` only when lifecycle `Ready` AND the vault directory is listable; else `503` body `not ready\n` (single line, no vault path, no reason strings beyond the two fixed bodies). Headers: `Cache-Control: no-store`. |
| `GET` | `/status` (delta) | `200 OK` | `application/json; charset=utf-8` | Current allowlisted `PublicSnapshot` (data-model.md). Field values sanitized of configured secrets. Headers: `Cache-Control: no-store`. Reflects `lifecycle.state: "shutting-down"` during drain (SC-003). |
| any | `/readyz`, `/status` (other methods) | `405 Method Not Allowed` | — | Register `GET` patterns so Go's mux returns 405. |
| `GET` | `/` , `/static/*` | per [009 contract](../../specs/009-landing-page/contracts/http.md) | — | Landing page and assets unchanged. |
| `GET` | any other path | `404 Not Found` | — | Preserved behavior. |

## Response guarantees

1. **Separation of signals** (FR-004/FR-005): `/healthz` never reflects vault or
   upstream state; `/readyz` never reads OIDC/sync/backup state; `/status` is the
   only aggregate view.
2. **Shutdown ordering** (FR-006/SC-003): `lifecycle.BeginShutdown` flips
   readiness to false before draining; during the drain window `/readyz` returns
   `503` and `/status` reports `shutting-down`; the lock is released by exit.
3. **Correlation** (FR-007, US2-AC3): every response, including probe responses,
   carries `X-Request-ID` (echoed or generated, 16 random bytes hex); the
   matching completion log uses the same `request_id`.
4. **Privacy** (FR-008, SC-004): probe bodies and status fields contain no
   credentials, tokens, note contents, vault paths, or remote URLs; configured
   secret values appearing in any status string are replaced with `[redacted]`.
5. **Concurrency** (FR-008): `/status` and `/readyz` are safe for unlimited
   concurrent readers; snapshots publish atomically (copy-on-write).

## Kubernetes surface

| Probe | Path | Behavior |
| --- | --- | --- |
| `livenessProbe` | `/healthz` | unchanged in `deploy/install.yaml` |
| `readinessProbe` | `/readyz` (delta) | replaces the current `/healthz` pointer in `deploy/install.yaml` |

The graceful drain is bounded by `BRAIND_SHUTDOWN_TIMEOUT` (default `5s`);
terminationGracePeriodSeconds should exceed it in deployment manifests.

## Verification mapping

| Contract point | Success criteria |
| --- | --- |
| Liveness answers despite vault/upstream loss | SC-001, US2-AC1 |
| Invalid config / duplicate owner refuse to serve with non-zero exit | SC-002 |
| Readiness fails before exit; drain within configured bound | SC-003 |
| No secrets or note contents in logs/status | SC-004 |
