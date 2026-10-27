# Quickstart: Daemon Foundation and Status

**Feature**: [spec.md](spec.md) | Contracts: [http.md](contracts/http.md) | Model: [data-model.md](data-model.md)

Validate `braind`'s foundation end-to-end on one machine, with no external
services (Principle I). Replace `$BIN` with the built binary or
`go run ./cmd/braind`.

## Prerequisites

- Go 1.22+ (`go version`)
- Linux (data-root locking uses `flock`)
- An empty working directory for `BRAIND_DATA_PATH` (temp dir is fine)

```bash
go build ./cmd/braind            # build
go test ./...                    # full unit suite (covers SC-001–SC-004 assertions)
```

All scenarios below use these exports; adjust the port to avoid clashes:

```bash
export ROOT="$(mktemp -d)/data" VAULT="$ROOT/vault" PORT=18080
mkdir -p "$VAULT"
export BRAIND_DATA_PATH="$ROOT" BRAIND_VAULT_PATH="$VAULT" \
       BRAIND_LISTEN_ADDR="127.0.0.1:$PORT" BRAIND_GIT_DIR="$ROOT/git/vault.git" \
       BRAIND_LOG_LEVEL=debug
unset OIDC_ENABLED BRAIND_SYNC_ENABLED BRAIND_GIT_BACKUP_ENABLED   # run without optional subsystems
```

> Note: `BRAIND_GIT_DIR` must be beneath `BRAIND_DATA_PATH` even when backup is
> disabled (path validation is unconditional), so it is set alongside the temp root.

> Note: with optional subsystems disabled (`OIDC_ENABLED=false`,
> `BRAIND_SYNC_ENABLED=false`, `BRAIND_GIT_BACKUP_ENABLED=false`) the daemon
> starts with no external dependencies; otherwise config validation requires
> their settings.

## Scenario 1 — Valid startup and probes (SC-001, US1-AC1)

```bash
"$BIN" & echo $! > braind.pid
curl -fsS "http://127.0.0.1:$PORT/healthz"        # → ok
curl -fsS "http://127.0.0.1:$PORT/readyz"         # → ready
curl -fsS "http://127.0.0.1:$PORT/status" | head  # → JSON with build.version, lifecycle.state "running"
curl -fsSI "http://127.0.0.1:$PORT/readyz" | grep -i cache-control   # no-store
grep '"message":"server listening"' daemon.log    # includes version
```

**Expected:** liveness, readiness, and status all healthy; startup log reports the
version. Also `"$BIN" -version` prints `braind <version>`, exits `0`, and starts
nothing.

## Scenario 2 — Invalid settings refuse to start (SC-002, US1-AC2)

```bash
BRAIND_VAULT_PATH="/tmp/outside-data" BRAIND_LISTEN_ADDR="bad-no-port" \
  "$BIN"; echo "exit=$?"
```

**Expected:** non-zero exit (`1`), stderr/log errors name `BRAIND_LISTEN_ADDR`
and `BRAIND_VAULT_PATH`; nothing listens. Now start once with a valid
`OIDC_CLIENT_SECRET=s3cr3t-value ...` and a bad log level and confirm the error
names `BRAIND_LOG_LEVEL` while `s3cr3t-value` appears nowhere in output (FR-002,
SC-004).

## Scenario 3 — One owner per data root (FR-003, US1-AC3)

```bash
"$BIN" & PID2=$!
sleep 0.5
"$BIN"; echo "exit=$?"      # second process on $ROOT
kill -TERM $PID2
```

**Expected:** second process exits `1` with "data root is already locked"; its
port check shows it never listened.

## Scenario 4 — Graceful shutdown within the configured bound (SC-003, US1-AC4)

```bash
export BRAIND_SHUTDOWN_TIMEOUT=1s      # configured bound
"$BIN" & PID4=$!
sleep 0.5
# (optional) keep one request in flight with a slow handler test — unit-level in run_test.go
date +%s%N > t0; curl -fsS "http://127.0.0.1:$PORT/status" >/dev/null
kill -TERM $PID4
curl -sS -o /dev/null -w '%{http_code}\n' "http://127.0.0.1:$PORT/readyz"   # 503 (races shutdown, fine)
wait $PID4; date +%s%N > t1
```

**Expected:** process exits `0`; an invalid `BRAIND_SHUTDOWN_TIMEOUT` (e.g. `0s`,
`-1s`, `banana`) is rejected at startup by name. Readiness flips to `not ready`
before exit; elapsed `(t1-t0)/1e9` ≤ bound plus connection latency. Lock file
remains on disk but is released — a new owner can start immediately.

## Scenario 5 — Vault availability drives readiness only (US2-AC1, AC2)

```bash
curl -fsS "http://127.0.0.1:$PORT/healthz"       # ok — liveness
chmod 000 "$VAULT"                                # simulate vault loss
sleep 0.2   # next probe observes it
curl -sS -o /dev/null -w '%{http_code}\n' "http://127.0.0.1:$PORT/readyz"   # 503
curl -fsS "http://127.0.0.1:$PORT/healthz"       # ok — liveness unaffected
curl -fsS "http://127.0.0.1:$PORT/status" | grep -o '"state":"[^"]*"'       # vault state changes; lifecycle stays running
chmod 700 "$VAULT"; sleep 0.2
curl -fsS "http://127.0.0.1:$PORT/readyz"         # ready again
```

**Expected:** only readiness changes; OIDC/sync/backup states in `/status` stay
`disabled` and must never affect either probe.

## Scenario 6 — Correlation and privacy-safe output (FR-007, US2-AC3, SC-004)

```bash
curl -fsS -D - -H 'X-Request-ID: trace-me-123' "http://127.0.0.1:$PORT/readyz" -o /dev/null | grep -i x-request-id
curl -fsS "http://127.0.0.1:$PORT/readyz" -o /dev/null -D - | grep -i x-request-id   # generated 32-hex
```

**Expected:** echoed `trace-me-123` on the first, a generated identifier on the
second; each appears in exactly one `request completed` log line. Search daemon
output for the configured OIDC client secret / git remote values: zero hits.
`/status` contains no note content or raw credentials.

## Regression guard

`go test ./internal/braind/...` must keep all pre-existing foundation tests green
(009 landing-page behavior, 405/404 semantics, middleware redaction); this
feature adds surfaces without changing them.
