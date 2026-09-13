// The browser test stand: a real browser against the assembled stack. The two-client checks of T07
// need two independent browser contexts, which no HTTP check can stand in for.
import { defineConfig, devices } from '@playwright/test';
import { SERVICE_ORIGIN } from '../../scripts/service.mjs';

export default defineConfig({
  testDir: '.',
  // The suites share one demonstration and one database, so they run one at a time and in order.
  fullyParallel: false,
  workers: 1,
  forbidOnly: true,
  reporter: [['list']],
  // A check waits for a real change to be delivered, so its own timeout is several times the bound
  // the change must arrive within; a failure says which frame or state never appeared.
  timeout: 90_000,
  expect: { timeout: 20_000 },
  use: {
    baseURL: SERVICE_ORIGIN,
    trace: 'retain-on-failure',
    viewport: { width: 1280, height: 800 },
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
