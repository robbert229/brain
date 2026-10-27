# Git backup is separate and one-way

**Status**: accepted

Git is a separate recovery mechanism, not a replacement for Obsidian Sync. The service creates coherent local snapshots and pushes them to a configured private remote; it never pulls, merges, rebases, resets, or force-pushes remote history. This makes backup divergence visible rather than silently treating an external branch as authoritative. The remote, confidentiality model, credentials, and cadence remain operator decisions.

See [Git Backup and Isolated Restore](../../specs/005-git-backup/spec.md).
