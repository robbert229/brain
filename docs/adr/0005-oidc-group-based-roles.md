# One OIDC provider maps groups to three roles

**Status**: accepted

The service uses one configured OpenID Connect provider and maps verified group claims to administrator, user, or viewer roles; identities with no exact configured match receive no access. When multiple groups match, precedence is administrator, then user, then viewer. This keeps account management with the identity provider and centralizes authorization, at the cost of not supporting local accounts, multiple providers, or per-note permissions in the initial product. Pocket ID is the current target provider; its observed claim behavior is recorded in the [Pocket ID verification spike](../spikes/spk-002-pocket-id.md).

See [OIDC Sign-In and Role Access](../../specs/002-oidc-access/spec.md).
