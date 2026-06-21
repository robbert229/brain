# Contributing

Thanks for contributing to `brain`.

This project uses **Conventional Commits** so history stays readable and changes can be categorized consistently.

## Commit Message Format

Use this format:

```text
type(scope)!: short summary

optional body

optional footer(s)
```

Rules:
- Use lowercase for `type` and `scope`.
- Keep the summary short, imperative, and specific.
- Use `!` only when the change is breaking.
- Add details in the body when context is useful.
- Reference issues in footers (example: `Refs: #12`).

## Allowed Types

- `feat`: new functionality
- `fix`: bug fix
- `docs`: documentation-only change
- `refactor`: code change that is not a fix or feature
- `test`: add or update tests
- `chore`: maintenance work (tooling, housekeeping)
- `ci`: CI/CD workflow changes

## Scope Guidance

Use a short scope tied to the part of the repo you changed.

Examples:
- `cli` for `internal/cli`
- `cmd` for `cmd/brain`
- `docs` for `README.md` or `CONTRIBUTING.md`
- `ci` for `.github/workflows`

## Examples

```text
feat(cli): add --name flag for greeting
fix(cli): return exit code 2 on invalid flags
docs(readme): add quick start section
test(cli): cover version output behavior
ci(actions): run tests on pull requests
chore(makefile): add test target
```

Breaking change examples:

```text
feat(cli)!: rename --name to --user

BREAKING CHANGE: --name flag was removed; use --user instead.
```

## Pull Request Expectations

Before opening a PR:
- Keep commits focused and logically grouped.
- Use conventional commit messages for each commit.
- Update docs/tests when behavior changes.
- Ensure checks pass in CI.

PR descriptions should include:
- What changed
- Why it changed
- Any breaking behavior or migration notes

