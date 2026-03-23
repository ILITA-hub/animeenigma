import { defineConfig, devices } from '@playwright/test'

/**
 * Playwright config for daily player health checks.
 * Runs against production (animeenigma.ru) across real browser engines.
 *
 * Usage:
 *   bunx playwright test --config=playwright.health.config.ts
 *   bunx playwright test --config=playwright.health.config.ts --project=chrome
 */
export default defineConfig({
  testDir: './e2e',
  testMatch: 'player-health.spec.ts',
  fullyParallel: false, // Sequential — avoid hammering external APIs
  retries: 2, // External streams can be flaky
  workers: 1,
  reporter: [
    ['html', { open: 'never', outputFolder: 'health-report' }],
    ['json', { outputFile: 'health-results.json' }],
    ['list'],
  ],
  timeout: 90000, // 90s — streams can be slow to start
  expect: {
    timeout: 15000,
  },
  use: {
    baseURL: process.env.BASE_URL || 'https://animeenigma.ru',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    actionTimeout: 20000,
    navigationTimeout: 30000,
  },
  projects: [
    {
      name: 'chrome',
      use: {
        ...devices['Desktop Chrome'],
        channel: 'chromium', // Use bundled Chromium in CI (chrome channel needs Chrome installed)
      },
    },
    {
      name: 'firefox',
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'webkit',
      use: { ...devices['Desktop Safari'] },
    },
  ],
})
