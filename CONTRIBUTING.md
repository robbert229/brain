# Contributing

This document records the conventions this project follows. It is a stub
and will grow as conventions are established.

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

This convention is not currently enforced by tooling or CI; it is a
project-wide convention and applies to all commits.

## Before opening a pull request

- Run `go test ./...`.
- Follow the principles in `.specify/memory/constitution.md`.
