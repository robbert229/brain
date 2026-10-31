import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.E2E_BASE_URL ?? 'https://127.0.0.1:4173';

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  workers: process.env.CI ? 1 : undefined,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  timeout: 30_000,
  expect: { timeout: 5_000 },
  use: {
    ...devices['Desktop Chrome'],
    baseURL,
    ignoreHTTPSErrors: true,
    trace: 'on',
    screenshot: 'on',
    video: 'on',
  },
  webServer: {
    command: 'go run ./internal/e2e/fixture',
    url: process.env.E2E_HEALTH_URL ?? 'http://127.0.0.1:4174/healthz',
    reuseExistingServer: false,
    timeout: 60_000,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 5_000 },
    env: { TZ: 'UTC' },
  },
});
