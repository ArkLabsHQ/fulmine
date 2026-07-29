import { defineConfig } from '@playwright/test';

// The web UI is reachable here once global-setup brings up fulmine-web (see
// regtest-web.compose.yml). Override with FULMINE_WEB_URL to point at an
// already-running instance.
const BASE_URL = process.env.FULMINE_WEB_URL ?? 'http://localhost:7019';

export default defineConfig({
  testDir: './tests',
  // The specs drive ONE shared wallet, so they run serially in filename order:
  // 01-init creates + unlocks the wallet before 02-receive / 03-send use it.
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? [['github'], ['list']] : 'list',
  globalSetup: './global-setup.ts',
  globalTeardown: './global-teardown.ts',
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
});
