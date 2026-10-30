# Architecture decision records

These records capture architectural choices and their rationale. User-visible
behavior and acceptance criteria live in the feature specs under `specs/`.

| ADR | Decision | Status |
| --- | --- | --- |
| [0001](0001-markdown-vault-is-canonical.md) | Markdown vault is canonical | Accepted |
| [0002](0002-single-writer-vault-ownership.md) | One service instance owns the vault | Accepted |
| [0003](0003-readiness-is-local.md) | Readiness depends on local vault availability | Accepted |
| [0004](0004-html-and-api-share-behavior.md) | Browser and JSON access share behavior | Accepted |
| [0005](0005-oidc-group-based-roles.md) | One OIDC provider maps groups to three roles | Accepted |
| [0006](0006-use-obsidian-headless-for-sync.md) | Obsidian Headless owns remote synchronization | Accepted |
| [0007](0007-git-backup-is-one-way.md) | Git backup is separate and one-way | Accepted |
| [0008](0008-private-single-replica-kubernetes.md) | Private single-replica Kubernetes deployment | Accepted |
| [0009](0009-private-obsidian-headless-artifacts.md) | Keep Obsidian Headless artifacts private | Accepted |
| [0010](0010-adopt-standard-oidc-libraries.md) | Use go-oidc and oauth2 for OIDC protocol mechanics | Accepted |
