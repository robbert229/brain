# brain

`brain` is the client for `braind`, an HTTP daemon that provides access to a
Markdown vault. The client connects to a running daemon. The project is under
active development; see the [roadmap](docs/roadmap.md) for current status and
the [feature specs](specs/) for planned capabilities.

## Run locally

Requires Go 1.26 or newer.

```bash
BRAIND_DATA_PATH=/tmp/braind-data \
BRAIND_VAULT_PATH=/tmp/braind-data/vault \
BRAIND_GIT_DIR=/tmp/braind-data/git/vault.git \
OIDC_CONFIGURATION_URL=https://oidc.example.test/.well-known/openid-configuration \
OIDC_CLIENT_ID=braind OIDC_CLIENT_SECRET=dev-secret \
OIDC_ADMIN_GROUP=access-brain-admin OIDC_USER_GROUP=access-brain-user \
OIDC_VIEWER_GROUP=access-brain-viewer \
BRAIND_GIT_BACKUP_ENABLED=false go run ./cmd/braind
```

In another terminal, check the daemon:

```bash
curl http://127.0.0.1:8080/healthz
```

The default listen address is `0.0.0.0:8080`. OIDC is mandatory: point the
`OIDC_*` settings at your identity provider to make sign-in work. The example
above uses a placeholder issuer, which lets the daemon start (probes and status
stay reachable; the landing page redirects to sign-in, which reports
`authentication_unavailable` until a real provider is configured). Git backup
is disabled because it is not implemented yet. `braind` can
also load settings from a `.env` file in the current directory. See the
[`braind` configuration reference](docs/braind-configuration.md) for details.

## Install on Kubernetes

The [`deploy/install.yaml`](deploy/install.yaml) bundle installs the current
bootstrap deployment: one `braind` replica, a 10 GiB persistent volume claim,
and an internal `ClusterIP` service. The cluster needs a default storage class
that supports `ReadWriteOnce`; edit the PVC settings in the manifest if needed.

From a checkout of the release version you want to install, apply the bundle
and wait for the daemon to start:

```bash
kubectl apply -f deploy/install.yaml
kubectl -n brain rollout status statefulset/braind
```

The bundle provides an in-cluster Service. To make `braind` externally
reachable, provide an ingress resource for your cluster and configure it for
your desired host and HTTPS setup. Sign-in is mandatory, so fill in the OIDC
block in the manifest (and create the `braind-oidc` secret) before applying.
Sync and backup stay disabled in this bootstrap install; see the
[release guide](docs/release.md) for release artifacts and deployment details.
