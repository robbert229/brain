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
go run ./cmd/braind
curl http://127.0.0.1:8080/healthz
```

Use `go run ./cmd/braind --version` to print its build version. The default
listen address is `0.0.0.0:8080`; `--listen` is a temporary foundation flag
until the typed configuration task is implemented.

## Test

```bash
go test ./...
```
