# Obsidian Headless owns remote synchronization

**Status**: accepted

The service delegates remote vault synchronization to the official Obsidian Headless CLI rather than implementing Obsidian Sync or its wire protocol. It supervises one continuous Sync process and exposes sanitized state to operators; the exact supported command contract must be verified before production integration. This preserves compatibility with Obsidian Sync while adding a pinned external runtime and its lifecycle to the service.

See [Obsidian Sync and Bootstrap](../../specs/004-obsidian-sync/spec.md).
