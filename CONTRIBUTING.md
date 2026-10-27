# Contributing

This document records the conventions this project follows.

## Commit messages

This project uses Conventional Commits
(<https://www.conventionalcommits.org/>). Commit messages MUST follow the
format:

    <type>(<optional scope>): <subject>

Common `type` values: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`,
`build`, `ci`, `perf`.

Examples from this repository's history:

    feat(status): add immutable status snapshots
    feat(config): add typed daemon configuration
    docs(oidc): record Pocket ID claim verification

A `!` after the type or scope (or a `BREAKING CHANGE:` trailer) marks a
breaking change:

    feat(api)!: rename note endpoints

CI checks every pull request commit and its title. Squash-merge pull request
titles should follow the same format because the title becomes the commit on
`main` and Release Please uses Conventional Commits to prepare releases.

## Releases

Release Please maintains a draft release pull request from changes merged to
`main`. Merge that pull request to create a `vX.Y.Z` tag and draft GitHub
release. The same workflow run builds the matching private, multi-platform
`braind` image in GHCR. See [the release guide](docs/release.md) for the
publication and retry flow.

## Before opening a pull request

- Run `go test ./...`.
- Follow the principles in `.specify/memory/constitution.md`.
