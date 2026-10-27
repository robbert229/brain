# Markdown vault is canonical

**Status**: accepted

The configured Obsidian vault is the only canonical store of note content. The service does not keep an independent database copy; indexes and search results are derived and rebuildable. This preserves ordinary files for Obsidian and other tools, at the cost of computing queries from the filesystem and coordinating writes with Sync.

See [Read-Only Vault Browser and Search](../../specs/003-vault-reading/spec.md) and [Safe Note Mutations](../../specs/006-safe-mutations/spec.md).
