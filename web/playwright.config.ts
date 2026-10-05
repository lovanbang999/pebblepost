import { defineConfig, devices } from '@playwright/test'
import process from 'node:process'

export default defineConfig({

  testDir: './e2e',
  timeout: 30000,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:8999',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: '../bin/pebblepost serve --host 127.0.0.1 --port 8999 --token smoke-token-test-123 --data-dir ../tmp-smoke-data',
    url: 'http://127.0.0.1:8999/api/health',
    reuseExistingServer: false,
    timeout: 15000,
  },
})
