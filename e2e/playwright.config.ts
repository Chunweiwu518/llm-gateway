import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  // The mock upstream's failure switch is global, so tests run one at a time.
  workers: 1,
  fullyParallel: false,
  failOnFlakyTests: true,
  forbidOnly: true,
  retries: 0,
  globalSetup: './global-setup.ts',
  globalTeardown: './global-teardown.ts',
  reporter: [['list'], ['json', { outputFile: 'test-results/results.json' }], ['html', { open: 'never' }]],
  use: {
    baseURL: process.env.BASE_URL ?? 'http://localhost:5173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['Desktop Chrome'], viewport: { width: 390, height: 844 } } },
  ],
})
