# brain

A minimal Go project scaffold with a tiny CLI and tests.

The repository also contains the early `braind` HTTP daemon described in
[`docs/braind.md`](docs/braind.md). Its first foundation increment exposes only
process liveness and graceful shutdown.

## Prerequisites

- Go 1.22+

## Quick start

```bash
go run ./cmd/brain
```

Try a custom name:

```bash
go run ./cmd/brain --name john
```

Print version:

```bash
go run ./cmd/brain --version
```

Run the daemon:

```bash
OIDC_ENABLED=false BRAIND_GIT_BACKUP_ENABLED=false go run ./cmd/braind
curl http://127.0.0.1:8080/healthz
```

Use `go run ./cmd/braind --version` to print its build version. The default
listen address is `0.0.0.0:8080` and can be changed with
`BRAIND_LISTEN_ADDR`. OIDC and Git backup are enabled by default, so the smoke
command disables those not-yet-implemented subsystems. Production startup
requires the corresponding settings documented in
[`docs/braind.md`](docs/braind.md#configuration-contract).

## Test

```bash
go test ./...
```
