# Release process

Brain uses Conventional Commits and Release Please to prepare versioned
releases. Releases are drafts until a maintainer reviews and publishes them.
The first version is `v0.1.0`.

## What a release publishes

- A GitHub draft release with generated notes and an updated `CHANGELOG.md`.
- A checked-in [installation bundle](../deploy/install.yaml) whose `braind`
  image tag is updated to `vX.Y.Z`, also attached to the GitHub release as
  `install.yaml`.
- A private multi-platform image at
  `ghcr.io/robbert229/braind:vX.Y.Z` for Linux amd64 and arm64.
- The `braind --version` output embedded as `vX.Y.Z` in that image.

The image remains private, as required by
[ADR 0009](adr/0009-private-obsidian-headless-artifacts.md). GHCR package
visibility and access must remain private. The bundle is a bootstrap install
for the current daemon; it does not provide HTTPS ingress or the planned Sync,
backup, and OIDC features.

## Normal flow

1. Merge changes to `main` using Conventional Commit pull request titles. CI
   checks commits and titles; `feat` produces a minor release, `fix` and
   `perf` produce a patch release, and a breaking change produces a major
   release.
2. Release Please opens or updates one draft release pull request containing
   the version and changelog update. With the default `GITHUB_TOKEN`, GitHub
   may require a maintainer to approve workflow runs on that pull request.
3. Review and merge the release pull request. Release Please creates a `vX.Y.Z`
   tag and draft release, updating the image reference in `deploy/install.yaml`.
   In the same workflow run, the release image is built from the tag's exact
   commit, pushed, and checked against the build digest and both architectures.
4. The workflow attaches the tagged `deploy/install.yaml` as `install.yaml` to
   the draft release. Review the notes and attachment, confirm the package
   remains private, then publish the GitHub release manually.

The bootstrap workflow continues to publish `sha-<commit>` and `latest` tags
from `main` for development deployment. Release publication uses only the
version tag and does not update `latest`.

## Retry

If image publication fails after Release Please creates its tag, run the
**Retry release publication** workflow from `main`, supplying that existing
`vX.Y.Z` tag. Optionally supply its expected commit SHA. The workflow validates
the tag format, resolves its commit, and the publisher verifies the tag points
to that same commit before rebuilding. It reuses the same image tag and
replaces the release attachment with the byte-for-byte bundle from the tag; it
does not create another GitHub release.

## Install the bootstrap bundle

The bundle creates the `brain` namespace, a single-replica StatefulSet, a
10 GiB `ReadWriteOnce` PVC, and an internal `ClusterIP` Service. It expects a
private GHCR pull secret named `ghcr-pull` in the `brain` namespace. Create that
secret using an operator-managed credential with read access to the GHCR
package before applying the bundle. The cluster also needs a default StorageClass
that supports `ReadWriteOnce`, or the operator must edit the PVC's storage size
and `storageClassName` for its environment.

For a checked-out release tag, apply the bundle with:

```sh
kubectl apply -f deploy/install.yaml
kubectl -n brain rollout status statefulset/braind
```

The Service has no external ingress or TLS configuration. It is only reachable
inside the cluster until an operator configures a private HTTPS edge. The
current daemon exposes `/healthz` as both startup/liveness and readiness
probe; readiness will need updating when the application readiness endpoint is
implemented. This bootstrap bundle is not evidence that the full deployment
requirements in [spec 007](../specs/007-private-deployment/spec.md) are met.

## Maintainer setup

The workflow uses the repository's `GITHUB_TOKEN` with `contents: write` and
`pull-requests: write` for Release Please, and `packages: write` only for image
publication. Ensure Actions can publish to the private GHCR package
`ghcr.io/robbert229/braind`. The retry workflow is manually dispatched by a
maintainer and does not require a separate registry credential.
