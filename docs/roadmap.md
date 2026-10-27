# `braind` delivery roadmap

This is a sequencing and status index, not a second requirements document.
Feature behavior is specified in [`../specs/`](../specs/); durable architectural
choices are recorded in [`adr/`](adr/).

## Sequence

| Stage | Feature specs | Status / dependency |
| --- | --- | --- |
| Foundation | [001 Daemon foundation](../specs/001-daemon-foundation/spec.md) | FND-001–FND-005 are implemented; readiness work remains. |
| Foundation (web) | [009 Landing page](../specs/009-landing-page/spec.md) | Implemented (2026-10-26); serves the landing page and HTMX assets at the daemon root. |
| First usable milestone | [002 OIDC access](../specs/002-oidc-access/spec.md), [003 Vault reading](../specs/003-vault-reading/spec.md) | Planned. |
| Sync and recovery | [004 Obsidian Sync](../specs/004-obsidian-sync/spec.md), [005 Git backup](../specs/005-git-backup/spec.md) | Planned; validate Obsidian command behavior and resolve backup decisions first. |
| Private deployment | [007 Private deployment](../specs/007-private-deployment/spec.md) | Planned; depends on successful image/runtime and cluster validation. |
| Safe mutations | [006 Safe mutations](../specs/006-safe-mutations/spec.md) | Later milestone; requires concurrency safeguards and Sync coordination. |
| Operations | [008 Operational recovery](../specs/008-operational-recovery/spec.md) | Follows the service capabilities it monitors and recovers. |

## Milestone gates

Each gate is an integration/release decision. Detailed behavior and acceptance
scenarios remain in the linked feature specs.

| Gate | Exit criteria | Current state |
| --- | --- | --- |
| **M0 — Feasibility proven** | Obsidian Headless command, authentication, setup, output, and shutdown behavior are documented from a disposable environment; the private runtime-image approach is demonstrated on supported architectures; representative vault/search/backup measurements confirm the initial search defaults; no unresolved feasibility issue invalidates the single-writer lifecycle. | Pocket ID verification is complete. Obsidian CLI, image, and vault/Git measurements remain. |
| **M1 — Local secure reader** | An authenticated person can browse, read, render, and search the local vault through the browser and versioned JSON interface; role decisions match across both; readiness reflects local vault access; path and rendering security checks pass. | Foundation FND-001–FND-005 is implemented; readiness, authentication, and reader features remain. |
| **M2 — Synchronized and backed up** | A disposable vault can be bootstrapped and continuously synchronized; operators can diagnose and recover from Sync failure; coherent private Git backups can be pushed; a selected backup can be validated in an isolated destination without touching the live vault. | Planned; depends on M0 command-contract evidence and resolution of backup decisions. |
| **M3 — Deployable** | A clean disposable-cluster deployment runs one hardened instance with persistent storage, private HTTPS access, managed secrets, and working health behavior; restart, upgrade, rollback, and restore procedures pass. | Planned; depends on runtime/image feasibility and M2. |
| **M4 — Safe mutation** | Authorized users can create, edit, rename, and trash notes through browser and JSON surfaces; stale or concurrent changes never silently overwrite newer content; deletion remains recoverable. | Planned; depends on M1 and tested Sync coordination. |
| **M5 — Operational release** | Privacy-safe metrics, threat-boundary checks, recovery runbooks, and release evidence cover the supported failure scenarios; remaining limitations and external validation needs are recorded. | Planned; depends on the preceding service and deployment capabilities. |

## Feasibility work

- `SPK-002` Pocket ID claim verification is complete; evidence remains in
  [`spikes/spk-002-pocket-id.md`](spikes/spk-002-pocket-id.md).
- `SPK-001` Obsidian Headless command contract remains to be verified.
- `SPK-003` private runtime image feasibility remains to be verified.
- `SPK-004` representative vault and Git measurements remain to be recorded.

## Decisions to resolve

- Select the private Git remote and decide whether its existing history may be
  adopted.
- Confirm SSH deploy-key credentials and known-host management.
- Decide whether the private Git provider may store plaintext vault content or
  whether client-side encryption is required.
- Confirm automatic backup cadence and retention expectations.
- Measure representative vault size, largest files, and backup/search timings.

Resolve these decisions in the affected feature spec before finalizing its plan.

## Recommended next step

Plan the M1 slice next: finish readiness in the foundation spec, then plan the
OIDC access and read-only vault specs. Keep the remaining M0 spikes in front of
Sync, image, and backup implementation; they do not need to block planning the
local reader.
