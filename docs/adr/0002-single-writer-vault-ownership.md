# One service instance owns the vault

**Status**: accepted

Only one `braind` instance may own a vault at a time, and that instance supervises at most one Obsidian Headless Sync process against it. The service enforces local process ownership, while the intended Kubernetes deployment adds a single-replica policy and single-pod volume access where supported. This avoids concurrent writers and split lifecycle ownership; it rules out horizontal scaling and a separate Sync sidecar for this vault.

See [Daemon Foundation and Status](../../specs/001-daemon-foundation/spec.md), [Obsidian Sync and Bootstrap](../../specs/004-obsidian-sync/spec.md), and [Private Service Deployment](../../specs/007-private-deployment/spec.md).
