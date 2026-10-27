# brain

A minimal Go project scaffold with a tiny CLI and tests.

The repository also contains the early `braind` HTTP daemon described in
[`docs/braind.md`](docs/braind.md). Its first foundation increment exposes only
process liveness and graceful shutdown.

## Prerequisites

- Go 1.22+
- [`ko`](https://ko.build/) v0.19.1 and Docker when building or smoke-testing
  the bootstrap container

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
BRAIND_DATA_PATH=/tmp/braind-data \
BRAIND_VAULT_PATH=/tmp/braind-data/vault \
BRAIND_GIT_DIR=/tmp/braind-data/git/vault.git \
OIDC_ENABLED=false BRAIND_GIT_BACKUP_ENABLED=false go run ./cmd/braind
curl http://127.0.0.1:8080/healthz
```

Use `go run ./cmd/braind --version` to print its build version. The default
listen address is `0.0.0.0:8080` and can be changed with
`BRAIND_LISTEN_ADDR`. OIDC and Git backup are enabled by default, so the smoke
command disables those not-yet-implemented subsystems. Production startup
requires the corresponding settings documented in
[`docs/braind.md`](docs/braind.md#configuration-contract).

Runtime logs are newline-delimited JSON. HTTP responses echo a valid inbound
`X-Request-ID` or contain a generated one; the same value appears in the
request-completion log record.

Only one daemon may own a data root at a time. `braind` holds an advisory lock
at `<BRAIND_DATA_PATH>/braind.lock` until graceful shutdown or process exit.

## Test

```bash
go test ./...
```

## Bootstrap container

DEP-001 provides a temporary, pure-Go image so the current daemon can be
deployed before its Obsidian runtime is ready. Build and exercise the image on
the local Docker daemon with:

```bash
make image-smoke
```

To publish the `linux/amd64` and `linux/arm64` OCI index manually:

```bash
KO_DOCKER_REPO=ghcr.io/robbert229/braind \
IMAGE_TAG="sha-$(git rev-parse HEAD)" \
make image-publish
```

The GitHub workflow publishes the same private GHCR package after relevant
changes land on `main`, records the immutable digest in its job summary, and
also updates the temporary `latest` tag. Repository/package visibility must
remain private. The image is non-root, has a read-only-root smoke test, and
contains only `braind` plus CA data. It deliberately does **not** contain Node,
Obsidian Headless, Git, or SSH; `IMG-001` and `IMG-002` replace this bootstrap
base before Sync or backup is enabled.
