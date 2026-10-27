# Browser and JSON access share behavior

**Status**: accepted

The browser interface and versioned JSON interface use the same domain behavior, validation, authorization, and concurrency rules. In the initial release, both authenticate people through the same server-side sessions; application-issued API tokens and bearer-token API access are deferred. Browser flows remain usable through ordinary HTTP navigation and forms, with JavaScript enhancements as optional progressive behavior. This prevents API-only policy drift and keeps core reading and editing accessible without JavaScript.

See [OIDC Sign-In and Role Access](../../specs/002-oidc-access/spec.md), [Read-Only Vault Browser and Search](../../specs/003-vault-reading/spec.md), and [Safe Note Mutations](../../specs/006-safe-mutations/spec.md).
