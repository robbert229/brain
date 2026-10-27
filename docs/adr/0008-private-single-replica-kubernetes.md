# Deploy as a private single-replica Kubernetes service

**Status**: accepted

The target deployment is a private Kubernetes StatefulSet with one desired replica, persistent vault storage, HTTPS ingress, externally managed credentials, and least-privilege workload settings. A single replica and persistent identity fit the single-writer vault model; disposable-cluster validation remains a release gate for this accepted target. Cluster-specific hostnames, storage classes, registry settings, and TLS issuers belong in deployment configuration rather than product requirements.

See [Private Service Deployment](../../specs/007-private-deployment/spec.md).
