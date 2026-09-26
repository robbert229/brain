# SPK-002 — Pocket ID claim verification

Status: complete; live behavior verified and the temporary verification client removed

Observed: 2026-09-26

This spike verifies the OIDC behavior of the deployed Pocket ID instance without committing tokens, authorization codes, client secrets, subject identifiers, email addresses, or unrelated group names.

## Deployment observed

- Pocket ID image: `ghcr.io/pocket-id/pocket-id:v2.16.0`
- Issuer: `https://oidc.lab.johnrowley.co`
- Discovery URL: `https://oidc.lab.johnrowley.co/.well-known/openid-configuration`
- The version is corroborated by the authenticated Pocket ID settings footer.

The cluster inspection read only workload metadata and Secret reference names. It did not retrieve Secret values.

## Discovery evidence

The live discovery document was fetched successfully on 2026-09-26. It advertises:

| Requirement | Observed value | Result |
| --- | --- | --- |
| Exact issuer | `https://oidc.lab.johnrowley.co` | Pass |
| Authorization endpoint | `/authorize` on the issuer origin | Pass |
| Authorization Code flow | `response_types_supported` contains `code` | Pass |
| PKCE | `code_challenge_methods_supported` contains `S256` and `plain` | Pass; production should use only `S256` |
| Public spike client | token endpoint auth methods contain `none` | Pass |
| Confidential production client | token endpoint auth methods contain `client_secret_basic` and `client_secret_post` | Pass; prefer `client_secret_basic` |
| Required scopes | `openid`, `profile`, `email`, and `groups` are advertised | Pass |
| Required claims | `sub`, `email`, `email_verified`, and `groups` are advertised | Pass |
| ID-token signing | `RS256` | Pass |
| UserInfo | `/api/oidc/userinfo` advertised | Pass |
| JWKS | `/.well-known/jwks.json` advertised | Pass |
| RP-initiated logout | `/api/oidc/end-session` advertised | Pass; behavior exercised below |
| Fresh authentication | prompt values include `login` | Pass |

Discovery proves capability, not the exact runtime shape of claims for a signed-in person. The authorization transaction below remains required.

## Current Pocket ID configuration finding

At initial inspection time, no OIDC client for Brain and none of the three designed Brain groups existed. The spike created:

- persistent groups `access-brain-admin`, `access-brain-user`, and `access-brain-viewer`;
- the verification identity assigned only to `access-brain-admin` among those three groups;
- temporary public client `braind SPK-002 temporary verifier` with PKCE enforced and re-authentication required;
- callback URL `http://127.0.0.1:18765/callback`;
- logout callback URL `http://127.0.0.1:18765/logout-complete`;
- allowed groups restricted to the three Brain groups.

The client was restricted to the three Brain groups. The groups are intended production configuration and remain in Pocket ID. The disposable public client was removed after verification; production remains a confidential client with PKCE.

## Reproducible verifier

The repository includes `tools/oidcverify`, a standard-library-only Go verifier. It:

1. fetches discovery and requires the exact issuer;
2. generates high-entropy state, nonce, and an S256 PKCE verifier/challenge;
3. listens only on loopback for the registered callback;
4. exchanges the code as a public client;
5. validates the ID-token RS256 signature from the live JWKS plus issuer, audience, expiry, nonce, and subject;
6. fetches UserInfo with the access token;
7. reports only claim types/counts and membership booleans for the three expected Brain groups;
8. compares ID-token and UserInfo group sets without printing their values;
9. optionally sends the browser through the discovered end-session endpoint and records the logout callback;
10. never writes raw tokens, authorization codes, email addresses, subject values, or unexpected group names.

Run it after creating the disposable client:

```sh
go run ./tools/oidcverify \
  -client-id '<disposable-public-client-id>' \
  -verify-logout \
  -report /tmp/spk-002-pocket-id-report.json
```

Open the one-time authorization URL printed by the tool. The report path is deliberately outside the repository even though its content is sanitized. Copy only reviewed, non-sensitive conclusions into this note.

## Live authorization evidence

The verifier completed at `2026-09-26T15:56:12Z`. Its sanitized result established:

| Check | Result |
| --- | --- |
| Authorization Code received | Pass |
| S256 PKCE used | Pass |
| Public-client token exchange | Pass |
| ID-token RS256 signature validated from discovered JWKS | Pass |
| ID-token issuer, audience, expiry, nonce, and subject validated | Pass |
| UserInfo fetched with the access token | Pass |
| ID-token `email` present as a string | Pass |
| UserInfo `email` present as a string | Pass |
| ID-token `email_verified` present | Pass |
| UserInfo `email_verified` present | Pass |
| ID-token `groups` is an array of strings | Pass |
| UserInfo `groups` is an array of strings | Pass |
| ID-token and UserInfo group sets exactly equal | Pass |
| `access-brain-admin` membership | Present in both sources |
| `access-brain-user` membership | Absent in both sources, as configured |
| `access-brain-viewer` membership | Absent in both sources, as configured |
| End-session request accepted | Pass |
| Registered logout callback observed | Pass |

Both claim sources contained 12 group strings. The report intentionally did not retain or print the other group names. It also omitted the subject, email value, tokens, authorization code, PKCE material, and raw claims. The sanitized report was reviewed from `/tmp` and was not copied into the repository.

After the evidence above was recorded, Pocket ID confirmed successful deletion of the temporary `braind SPK-002 temporary verifier` client. The three persistent Brain groups were retained for production configuration.

## Design impact so far

The live discovery and authorization evidence supports the current design choices:

- request `openid profile email groups`;
- use Authorization Code with S256 PKCE;
- validate RS256 through the discovered JWKS;
- default `OIDC_GROUPS_CLAIM=groups`;
- keep `OIDC_GROUPS_SOURCE=id_token`; the live ID-token and UserInfo group sets were equal, and the ID token avoids an additional request;
- retain UserInfo as a tested alternative source;
- use the discovered end-session endpoint rather than constructing one from request input.

The spike used a public client only to avoid creating or handling a disposable client secret. Production remains a confidential web client with S256 PKCE. Discovery advertises both `client_secret_basic` and `client_secret_post`; the production implementation should prefer `client_secret_basic` and keep the secret server-side.
