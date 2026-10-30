# Quickstart: Validate the Landing Page

**Feature**: [spec.md](./spec.md) | **Date**: 2026-10-26

This is a runnable validation guide for the landing page deliverable. It proves the
feature works end-to-end; it is not implementation code. Contract details:
[contracts/http.md](./contracts/http.md). Data model: [data-model.md](./data-model.md).

## Prerequisites

- Go 1.22+ on the build machine (`go version`).
- No network access is required at build or runtime for this feature (assets are
  embedded/vendored).

## 1. Unit tests (CI-fast gate)

```bash
go test ./internal/braind/ -v -run 'Handler|Landing'
```

**Expected outcomes**:

- `GET /` → `200`, `text/html`, body contains `brain`, the exact tagline
  `a second brain, powered by markdown`, and the under-construction notice.
- `GET /static/htmx.min.js` → `200`, `text/javascript; charset=utf-8`.
- `GET /static/style.css` → `200`, `text/css; charset=utf-8`.
- `POST /` → `405`; `GET /readyz` → `404`.
- `GET /healthz` → `200`, body `ok\n`, `Cache-Control: no-store` (unchanged).
- A sensitive-string probe (secret-like, vault-path-like values) matches nothing in any
  rendered body.

Full suite: `go test ./...` and `go vet ./...` must both pass.

## 2. Run the daemon locally

```bash
export BRAIND_LISTEN_ADDR=127.0.0.1:8787
export BRAIND_DATA_PATH="$(mktemp -d)/data"
export BRAIND_VAULT_PATH="$BRAIND_DATA_PATH/vault"
export BRAIND_GIT_DIR="$BRAIND_DATA_PATH/git/vault.git"
export OIDC_CONFIGURATION_URL=https://oidc.example.test/.well-known/openid-configuration
export OIDC_CLIENT_ID=braind OIDC_CLIENT_SECRET=placeholder-secret
export OIDC_ADMIN_GROUP=access-brain-admin OIDC_USER_GROUP=access-brain-user OIDC_VIEWER_GROUP=access-brain-viewer
export BRAIND_GIT_BACKUP_ENABLED=false
go run ./cmd/braind
```

(Check the exact environment variable names in
[docs/braind-configuration.md](../../docs/braind-configuration.md)).

## 3. Manual endpoint checks

With the daemon on `127.0.0.1:8787`:

```bash
curl -i http://127.0.0.1:8787/                 # 302 to /login?next=%2F (session required)
curl -i http://127.0.0.1:8787/static/htmx.min.js   # 200, text/javascript
curl -i http://127.0.0.1:8787/static/style.css     # 200, text/css
curl -i http://127.0.0.1:8787/healthz              # 200 "ok" (regression)
curl -i -X POST http://127.0.0.1:8787/             # 405
curl -i http://127.0.0.1:8787/readyz               # 404
```

**Expected**: status codes and content types per [contracts/http.md](./contracts/http.md);
the tagline appears exactly as `a second brain, powered by markdown`. The index page is
session-gated (spec 002): the checks below need a signed-in session — with a real
provider, complete sign-in in the browser and use *View Source*, or replay the request
with the `braind_session` cookie copied from DevTools:
`curl -s -b 'braind_session=<value>' http://127.0.0.1:8787/`.

## 4. No-script check

```bash
curl -s -b 'braind_session=<value>' http://127.0.0.1:8787/ | grep -c "second brain, powered by markdown"
```

The content is in the initial response, so disabling JavaScript in a browser (or using
a text browser such as `lynx`) must show the same full page. **Expected**: the page is
complete with scripting disabled (SC-003).

## 5. Responsive/design check

Sign in at `http://127.0.0.1:8787/` (with a real provider configured), then use DevTools
device emulation at **320 px, 768 px, 1024 px, and 1920 px** widths.

**Expected**: no horizontal scrolling, no overlapping or clipped content; product name,
tagline, and under-construction notice visible at every width (SC-002).

## 6. Sensitive-content scan

```bash
curl -s -b 'braind_session=<value>' http://127.0.0.1:8787/ | grep -Ei 'secret|token|vault/|/notes/|client_secret|BEGIN ' || true
```

**Expected**: no matches (SC-005). The page is static marketing copy only.

## 7. Build smoke

```bash
go build ./cmd/braind   # produces self-contained binary containing the page + assets
```

**Expected**: build succeeds; the binary runs offline and serves the page without any
external fetch (private-deployment safe).

## Outcome mapping

| Step | Proves |
| --- | --- |
| 1 | FR-001…FR-007, SC-001, SC-003, SC-005, SC-006 |
| 3 | Contract statuses/content types, FR-001, FR-002, FR-007, SC-006 |
| 4 | FR-003, SC-003 |
| 5 | FR-004, SC-002 |
| 6 | FR-005, SC-005 |
| 7 | Asset embedding, offline capability |
