# Browser E2E tests

These tests launch a real `braind` process in a temporary data directory, put it
behind local HTTPS, and drive it with Chromium through the contributor OIDC login
flow. The local fixture uses the same simulated OIDC provider as the Go auth tests;
it does not connect to a live identity provider or use production vault data.

## Run locally

```sh
npm ci
npx playwright install chromium
npm run test:e2e
```

The fixture listens on `127.0.0.1:4173` for HTTPS, `127.0.0.1:4174` for the
internal HTTP app, and `127.0.0.1:4175` for a loopback-only test control endpoint
that selects the simulated OIDC role. Override those with `BRAIND_E2E_PROXY_ADDR`,
`BRAIND_E2E_APP_ADDR`, and `BRAIND_E2E_CONTROL_ADDR` if the ports are already in use.
When overriding the control port, set `E2E_CONTROL_URL` to the matching HTTP URL for
Playwright. The temporary data directory is removed when the fixture shuts down.

Playwright writes screenshots, videos, and traces for every run under `test-results/`;
the CI job uploads those with the HTML report under `playwright-report/`.
