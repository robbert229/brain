# Release process

Brain uses Conventional Commits and Release Please to prepare versioned
releases. Releases are drafts until a maintainer reviews and publishes them.
The first version is `v0.1.0`.

## What a release publishes

- A GitHub draft release with generated notes and an updated `CHANGELOG.md`.
- A private multi-platform image at
  `ghcr.io/robbert229/braind:vX.Y.Z` for Linux amd64 and arm64.
- The `braind --version` output embedded as `vX.Y.Z` in that image.

The image remains private, as required by
[ADR 0009](adr/0009-private-obsidian-headless-artifacts.md). GHCR package
visibility and access must remain private. Releases do not publish an install
bundle; deployment is not yet a supported release artifact.

## Normal flow

1. Merge changes to `main` using Conventional Commit pull request titles. CI
   checks commits and titles; `feat` produces a minor release, `fix` and
   `perf` produce a patch release, and a breaking change produces a major
   release.
2. Release Please opens or updates one draft release pull request containing
   the version and changelog update. With the default `GITHUB_TOKEN`, GitHub
   may require a maintainer to approve workflow runs on that pull request.
3. Review and merge the release pull request. Release Please creates a `vX.Y.Z`
   tag and draft release. In the same workflow run, the release image is built
   from the tag's exact commit, pushed, and checked against the build digest.
4. Review the draft notes and the publication job. Confirm the package remains
   private, then publish the GitHub release manually.

The bootstrap workflow continues to publish `sha-<commit>` and `latest` tags
from `main` for development deployment. Release publication uses only the
version tag and does not update `latest`.

## Retry

If image publication fails after Release Please creates its tag, run the
**Retry release publication** workflow from `main`, supplying that existing
`vX.Y.Z` tag. Optionally supply its expected commit SHA. The workflow validates
the tag format, resolves its commit, and the publisher verifies the tag points
to that same commit before rebuilding. It reuses the same image tag and does
not create another GitHub release.

## Maintainer setup

The workflow uses the repository's `GITHUB_TOKEN` with `contents: write` and
`pull-requests: write` for Release Please, and `packages: write` only for image
publication. Ensure Actions can publish to the private GHCR package
`ghcr.io/robbert229/braind`. The retry workflow is manually dispatched by a
maintainer and does not require a separate registry credential.
