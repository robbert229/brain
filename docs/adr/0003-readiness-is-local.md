# Readiness depends on local vault availability

**Status**: accepted

The service remains ready when the local vault is accessible, even if OIDC discovery, Obsidian Sync, or Git backup is degraded. Liveness reports whether the process is alive; readiness reports whether it can serve local vault work. During an OIDC outage, new sign-ins fail and an existing session may use its last verified role only until its configured reauthentication interval (one hour by default); afterward protected requests fail closed without extending the session. This keeps the local service available without implying that authentication remains available indefinitely.

See [Daemon Foundation and Status](../../specs/001-daemon-foundation/spec.md) and [Obsidian Sync and Bootstrap](../../specs/004-obsidian-sync/spec.md).
